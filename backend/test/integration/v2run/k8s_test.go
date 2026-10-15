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
	namespaces  map[string]string // name -> JSON
	objects     map[string][]string
	objectsJSON string
	pvJSON      string
	calls       []string
	denyGet     bool
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
		if args[len(args)-1] == "json" {
			return []byte(f.objectsJSON), "", 0
		}
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
	return fmt.Sprintf(`{"metadata":{"name":%q,"labels":{"run-id":%q,"managed-by":%q,"orchestrator-managed":"true","orchestrator-instance":"0123456789abcdef"}}}`, name, runID, managedBy)
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

func TestGuardBeforeEnvironmentBindingRequiresPositiveNoCreateProof(t *testing.T) {
	c := goodConfig(t)
	m := v2run.NewManifest(c, time.Now())
	m.Armed = true
	ctx := context.Background()
	clean := &fakeCluster{namespaces: map[string]string{}, pvJSON: `{"items":[]}`}
	if got := v2run.GuardNamespaceForRun(ctx, clean, m, c); got != v2run.ExitNotOwned {
		t.Fatalf("a proven no-create run should be skipped safely, got %d; calls=%v", got, clean.calls)
	}
	if len(clean.calls) != 2 || clean.calls[0] != "get namespace "+c.Kube.Namespace+" -o name" || clean.calls[1] != "get pv -o json" {
		t.Fatalf("no-create proof must check the exact intended namespace and PV claims: %v", clean.calls)
	}

	created := &fakeCluster{namespaces: map[string]string{c.Kube.Namespace: nsJSON(c.Kube.Namespace, c.RunID, v2run.ProductManagedByTag)}, pvJSON: `{"items":[]}`}
	if got := v2run.GuardNamespaceForRun(ctx, created, m, c); got != 1 {
		t.Fatalf("an unrecorded namespace must remain unknown, got %d", got)
	}

	withRun := m
	_ = withRun.AddTerraformRun("pending-0123456789abcdef")
	untouched := &fakeCluster{namespaces: map[string]string{}, pvJSON: `{"items":[]}`}
	if got := v2run.GuardNamespaceForRun(ctx, untouched, withRun, c); got != 1 || len(untouched.calls) != 0 {
		t.Fatalf("an attempted Terraform run without namespace acknowledgement is unknown, got %d calls=%v", got, untouched.calls)
	}

	withPV := m
	if err := withPV.AddAsset(v2run.Asset{Kind: "pv", ID: "run-pv", Layer: v2run.LayerAppResources, Owner: v2run.OwnerCreatedByRun}); err != nil {
		t.Fatal(err)
	}
	if got := v2run.GuardNamespaceForRun(ctx, clean, withPV, c); got != 1 {
		t.Fatalf("a manifest-owned PV makes no-create proof invalid, got %d", got)
	}
	claimed := &fakeCluster{namespaces: map[string]string{}, pvJSON: `{"items":[{"metadata":{"name":"run-pv"},"spec":{"claimRef":{"namespace":"orch-v2-test-a01-0001"}}}]}`}
	if got := v2run.GuardNamespaceForRun(ctx, claimed, m, c); got != 1 {
		t.Fatalf("a remaining PV claim makes no-create proof invalid, got %d", got)
	}
}

// Verification is read-only and judged by cluster contents, never by an exit
// code; the package has no delete path at all (Terraform-owned objects belong
// to the original engine, delivered Workloads to the product's Remove).
func TestVerifyLayersAreReadOnlyAndJudgedByContents(t *testing.T) {
	m := k8sManifest(t)
	ctx := context.Background()
	c := &fakeCluster{
		namespaces:  map[string]string{m.Namespace: nsJSON(m.Namespace, "a01-0001", v2run.ProductManagedByTag)},
		objects:     map[string][]string{"statefulsets,persistentvolumeclaims": {"statefulset.apps/redis"}},
		objectsJSON: `{"items":[{"kind":"Deployment","metadata":{"name":"web","labels":{}}}]}`,
		pvJSON:      `{"items":[{"metadata":{"name":"pv-1"},"spec":{"claimRef":{"namespace":"` + m.Namespace + `"}}}]}`,
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
	c.objectsJSON = `{"items":[]}`
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

func TestDeliveryVerificationRoutesOnlyKnownTerraformObjects(t *testing.T) {
	m := k8sManifest(t)
	if err := m.AddTerraformRun("pending-0123456789abcdef"); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	baseLabels := `"run-id":"a01-0001","managed-by":"orchestrator-verification","orchestrator-managed":"true","orchestrator-created-by-run":"pending-0123456789abcdef","orchestrator-instance":"0123456789abcdef","app.kubernetes.io/managed-by":"orchestrator","app.kubernetes.io/name":"postgres","app.kubernetes.io/instance":"postgres-0123456789abcdef"`
	tests := []struct {
		name, item string
		wantErr    bool
	}{
		{"owned auth secret", `{"kind":"Secret","metadata":{"name":"postgres-0123456789abcdef-auth","labels":{` + baseLabels + `}}}`, false},
		{"owned terraform service", `{"kind":"Service","metadata":{"name":"postgres-0123456789abcdef","labels":{` + baseLabels + `}}}`, false},
		{"foreign same name", `{"kind":"Secret","metadata":{"name":"postgres-0123456789abcdef-auth","labels":{}}}`, true},
		{"mismatched Terraform run tag", `{"kind":"Service","metadata":{"name":"postgres-0123456789abcdef","labels":{` + strings.Replace(baseLabels, `"orchestrator-created-by-run":"pending-0123456789abcdef"`, `"orchestrator-created-by-run":"pending-fedcba9876543210"`, 1) + `}}}`, true},
		{"outer scenario run is not Terraform run", `{"kind":"Service","metadata":{"name":"postgres-0123456789abcdef","labels":{` + strings.Replace(baseLabels, `"orchestrator-created-by-run":"pending-0123456789abcdef"`, `"orchestrator-created-by-run":"a01-0001"`, 1) + `}}}`, true},
		{"missing outer run tag", `{"kind":"Service","metadata":{"name":"postgres-0123456789abcdef","labels":{` + strings.Replace(baseLabels, `"run-id":"a01-0001",`, "", 1) + `}}}`, true},
		{"foreign outer run tag", `{"kind":"Service","metadata":{"name":"postgres-0123456789abcdef","labels":{` + strings.Replace(baseLabels, `"run-id":"a01-0001"`, `"run-id":"other-run"`, 1) + `}}}`, true},
		{"wrong verifier managed-by tag", `{"kind":"Service","metadata":{"name":"postgres-0123456789abcdef","labels":{` + strings.Replace(baseLabels, `"managed-by":"orchestrator-verification"`, `"managed-by":"other"`, 1) + `}}}`, true},
		{"missing physical instance", `{"kind":"Secret","metadata":{"name":"custom-secret","labels":{"orchestrator-managed":"true","orchestrator-created-by-run":"pending-0123456789abcdef","app.kubernetes.io/managed-by":"orchestrator"}}}`, true},
		{"custom secret with spoofed labels", `{"kind":"Secret","metadata":{"name":"custom-secret","labels":{` + baseLabels + `}}}`, true},
		{"wrong app identity", `{"kind":"Secret","metadata":{"name":"redis-0123456789abcdef-auth","labels":{` + baseLabels + `}}}`, true},
		{"wrong module object kind", `{"kind":"Deployment","metadata":{"name":"postgres-0123456789abcdef","labels":{` + baseLabels + `}}}`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &fakeCluster{namespaces: map[string]string{m.Namespace: nsJSON(m.Namespace, m.RunID, v2run.ProductManagedByTag)}, objectsJSON: `{"items":[` + tt.item + `]}`}
			err := v2run.VerifyLayer(ctx, c, m, v2run.LayerDelivery)
			if (err != nil) != tt.wantErr {
				t.Fatalf("VerifyLayer error=%v, wantErr=%v", err, tt.wantErr)
			}
		})
	}
}

func TestDeliveryVerificationRoutesOnlyPristineNamespaceDefaults(t *testing.T) {
	m := k8sManifest(t)
	ctx := context.Background()
	tests := []struct {
		name, item string
		wantErr    bool
	}{
		{"default service account", `{"kind":"ServiceAccount","metadata":{"name":"default","uid":"uid-sa"}}`, false},
		{"root CA config map", `{"kind":"ConfigMap","data":{"ca.crt":"opaque-cert"},"metadata":{"name":"kube-root-ca.crt","uid":"uid-ca","annotations":{"kubernetes.io/description":"cluster CA certificate"},"managedFields":[{"manager":"kube-controller-manager"}]}}`, false},
		{"default account custom annotation", `{"kind":"ServiceAccount","metadata":{"name":"default","uid":"uid-sa","annotations":{"team":"custom"}}}`, true},
		{"default account image pull secret", `{"kind":"ServiceAccount","imagePullSecrets":[{"name":"registry"}],"metadata":{"name":"default","uid":"uid-sa"}}`, true},
		{"root CA custom label", `{"kind":"ConfigMap","data":{"ca.crt":"opaque-cert"},"metadata":{"name":"kube-root-ca.crt","uid":"uid-ca","labels":{"team":"custom"}}}`, true},
		{"lookalike config map", `{"kind":"ConfigMap","data":{"ca.crt":"opaque-cert","extra":"x"},"metadata":{"name":"kube-root-ca.crt","uid":"uid-ca"}}`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &fakeCluster{namespaces: map[string]string{m.Namespace: nsJSON(m.Namespace, m.RunID, v2run.ProductManagedByTag)}, objectsJSON: `{"items":[` + tt.item + `]}`}
			err := v2run.VerifyLayer(ctx, c, m, v2run.LayerDelivery)
			if (err != nil) != tt.wantErr {
				t.Fatalf("VerifyLayer error=%v wantErr=%v", err, tt.wantErr)
			}
		})
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
		if !strings.Contains(strings.Join(step.Guard, " "), "-config /priv/config.json") {
			t.Fatalf("step %s guard lacks the config needed to verify a pre-deployment namespace: %v", step.Name, step.Guard)
		}
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
