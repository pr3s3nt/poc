package v2run_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"orchestrator/test/integration/v2run"
)

// fakeCluster is a pure-logic stand-in for kubectl used only to unit test the
// guard and verification decisions. It is NOT acceptance evidence.
type fakeCluster struct {
	namespaces map[string]string // name -> JSON
	objects    map[string][]string
	pvJSON     string
	calls      []string
	denyGet    bool
}

func (f *fakeCluster) Run(_ context.Context, args ...string) ([]byte, string, int) {
	line := strings.Join(args, " ")
	f.calls = append(f.calls, line)
	switch {
	case f.denyGet:
		return nil, "Error from server (Forbidden): forbidden", 1
	case strings.HasPrefix(line, "get namespace "):
		name := args[2]
		doc, ok := f.namespaces[name]
		if !ok {
			return nil, fmt.Sprintf(`Error from server (NotFound): namespaces %q not found`, name), 1
		}
		if args[len(args)-1] == "name" {
			return []byte("namespace/" + name + "\n"), "", 0
		}
		return []byte(doc), "", 0
	case args[0] == "-n" && args[2] == "get":
		return []byte(strings.Join(f.objects[args[3]], "\n")), "", 0
	case line == "get pv -o json":
		return []byte(f.pvJSON), "", 0
	case args[0] == "-n" && args[2] == "delete":
		f.objects[args[3]] = nil
		return nil, "", 0
	case args[0] == "delete" && args[1] == "namespace":
		delete(f.namespaces, args[2])
		return nil, "", 0
	}
	return nil, "unexpected " + line, 99
}

func nsJSON(name, runID, managedBy string) string {
	return fmt.Sprintf(`{"metadata":{"name":%q,"labels":{"run-id":%q,"managed-by":%q}}}`, name, runID, managedBy)
}

func k8sManifest(t *testing.T) v2run.Manifest {
	m := v2run.NewManifest(goodConfig(t), time.Now())
	if err := m.RecordNamespace("orch-v2-test-a01-0001", 0, "orch-v2-test-a01-0001"); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestGuardNamespaceDistinguishesOwnedForeignAndUnknown(t *testing.T) {
	m := k8sManifest(t)
	ctx := context.Background()
	owned := &fakeCluster{namespaces: map[string]string{m.Namespace: nsJSON(m.Namespace, "a01-0001", v2run.ProductManagedByTag)}}
	if got := v2run.GuardNamespace(ctx, owned, m); got != 0 {
		t.Fatalf("owned = %d", got)
	}
	if got := v2run.GuardNamespace(ctx, &fakeCluster{namespaces: map[string]string{}}, m); got != 0 {
		t.Fatalf("a never-created namespace is clean, got %d", got)
	}
	foreign := &fakeCluster{namespaces: map[string]string{m.Namespace: nsJSON(m.Namespace, "someone-else", v2run.ProductManagedByTag)}}
	if got := v2run.GuardNamespace(ctx, foreign, m); got != v2run.ExitNotOwned {
		t.Fatalf("foreign run id = %d", got)
	}
	untagged := &fakeCluster{namespaces: map[string]string{m.Namespace: `{"metadata":{"name":"` + m.Namespace + `"}}`}}
	if got := v2run.GuardNamespace(ctx, untagged, m); got != v2run.ExitNotOwned {
		t.Fatalf("untagged = %d", got)
	}
	if got := v2run.GuardNamespace(ctx, &fakeCluster{denyGet: true}, m); got != 1 {
		t.Fatalf("API uncertainty must be unknown, got %d", got)
	}
	guessed := m
	guessed.Namespace = "default"
	if got := v2run.GuardNamespace(ctx, owned, guessed); got != 1 {
		t.Fatalf("a namespace outside the run prefix must never be guarded as owned, got %d", got)
	}
}

// Verification is read-only and judged by cluster contents, never by an exit
// code; the package has no delete path at all (Terraform-owned objects belong
// to the original engine, delivered Workloads to the product's Remove).
func TestVerifyLayersAreReadOnlyAndJudgedByContents(t *testing.T) {
	m := k8sManifest(t)
	ctx := context.Background()
	c := &fakeCluster{
		namespaces: map[string]string{m.Namespace: nsJSON(m.Namespace, "a01-0001", v2run.ProductManagedByTag)},
		objects:    map[string][]string{"deployments,daemonsets,replicasets,jobs,cronjobs,services,ingresses,configmaps,secrets,serviceaccounts,roles,rolebindings": {"deployment.apps/web", "configmap/kube-root-ca.crt"}, "statefulsets,persistentvolumeclaims": {"statefulset.apps/redis"}},
		pvJSON:     `{"items":[{"metadata":{"name":"pv-1"},"spec":{"claimRef":{"namespace":"` + m.Namespace + `"}}}]}`,
	}
	for _, layer := range []string{v2run.LayerDelivery, v2run.LayerAppResources, v2run.LayerNamespace} {
		if err := v2run.VerifyLayer(ctx, c, m, layer); err == nil {
			t.Fatalf("%s verified absent while objects remain", layer)
		}
	}
	// The engine removed the objects: delivery objects other than the cluster defaults and the app resources are gone.
	for key := range c.objects {
		c.objects[key] = nil
	}
	if err := v2run.VerifyLayer(ctx, c, m, v2run.LayerDelivery); err != nil {
		t.Fatal(err)
	}
	if err := v2run.VerifyLayer(ctx, c, m, v2run.LayerAppResources); err == nil {
		t.Fatal("a leftover PV claimed from the run namespace was ignored")
	}
	c.pvJSON = `{"items":[{"metadata":{"name":"unrelated"},"spec":{"claimRef":{"namespace":"prod"}}}]}`
	if err := v2run.VerifyLayer(ctx, c, m, v2run.LayerAppResources); err != nil {
		t.Fatalf("an unrelated PV must not block: %v", err)
	}
	if err := v2run.VerifyLayer(ctx, c, m, v2run.LayerNamespace); err == nil {
		t.Fatal("an existing namespace verified as absent")
	}
	delete(c.namespaces, m.Namespace)
	if err := v2run.VerifyLayer(ctx, c, m, v2run.LayerNamespace); err != nil {
		t.Fatal(err)
	}
	for _, call := range c.calls {
		if strings.Contains(call, "delete") {
			t.Fatalf("verification issued a delete: %s", call)
		}
	}
}

func TestSentinelUIDMustBeUnchanged(t *testing.T) {
	cfg := goodConfig(t)
	k := &sentinelKubectl{uid: "uid-1"}
	if err := v2run.VerifySentinel(context.Background(), k, *cfg.Kube); err != nil {
		t.Fatal(err)
	}
	k.uid = "uid-other"
	if err := v2run.VerifySentinel(context.Background(), k, *cfg.Kube); err == nil {
		t.Fatal("a replaced sentinel passed")
	}
	k.missing = true
	if err := v2run.VerifySentinel(context.Background(), k, *cfg.Kube); err == nil {
		t.Fatal("a missing sentinel passed")
	}
}

type sentinelKubectl struct {
	uid     string
	missing bool
}

func (s *sentinelKubectl) Run(context.Context, ...string) ([]byte, string, int) {
	if s.missing {
		return nil, "NotFound", 1
	}
	return []byte(s.uid), "", 0
}

func TestK8sCleanupStepsUseTheOriginalEngineAndNeverKubectlDelete(t *testing.T) {
	cfg := goodConfig(t)
	steps := v2run.K8sCleanupSteps("/abs/v2ctl", "/abs/v2cleanup", "/priv/manifest.json", "/priv/cleanup-handles.json", "/priv/config.json", "/priv/vault-snapshot.json", cfg)
	for _, step := range steps {
		joined := strings.Join(append(append([]string{}, step.Argv...), step.Guard...), " ")
		if strings.Contains(joined, "kubectl") || strings.Contains(joined, " delete") || strings.Contains(joined, "--all") {
			t.Fatalf("step %s deletes directly: %s", step.Name, joined)
		}
		if step.Name == "vault-config" {
			// Run-scoped objects of the configuration Vault, proven by content against the pre-run snapshot.
			if step.Layer != v2run.LayerAux || step.Argv[1] != "vault-clean" || step.Verify[1] != "vault-clean" {
				t.Fatalf("vault step %v", step.Argv)
			}
			continue
		}
		if step.Argv[0] != "/abs/v2cleanup" {
			t.Fatalf("step %s does not run the independent safety net: %v", step.Name, step.Argv)
		}
	}
	want := map[string]string{"delivery": "delivery", "app-resources": "terraform", "namespace": "status"}
	for _, step := range steps {
		if step.Name == "vault-config" {
			continue
		}
		if step.Argv[1] != want[step.Name] || step.Argv[len(step.Argv)-2] != "-manifest" || step.Argv[len(step.Argv)-1] != "/priv/manifest.json" {
			t.Fatalf("step %s runs %s", step.Name, step.Argv[1])
		}
	}
	spec := v2run.Spec{RunID: cfg.RunID, StatusPath: "/p/s", FinishPath: "/p/f", LogDir: "/p/l", ExpiresAt: time.Now().Add(time.Hour),
		CleanupBudget: v2run.Duration{Duration: time.Hour}, Watch: []v2run.Watch{{Name: "runner", PID: 1}}, Steps: steps}
	if err := spec.Validate(time.Now()); err != nil {
		t.Fatal(err)
	}
	if steps[0].Layer != v2run.LayerDelivery || steps[2].Layer != v2run.LayerNamespace {
		t.Fatalf("order %v", steps)
	}
	if steps[0].Env["V2_KUBECONFIG"] != "/priv/kubeconfig" {
		t.Fatalf("handle env %v", steps[0].Env)
	}
}
