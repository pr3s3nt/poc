package fleet

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	publication "orchestrator/internal/adapters/gitpublication"
	k8s "orchestrator/internal/adapters/kubernetes"
	domain "orchestrator/internal/domain/delivery"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/execution"
	gitport "orchestrator/internal/ports/gitpublication"
)

type fixtureCredentials struct{ document []byte }

func (c fixtureCredentials) ResolveKubeconfig(_ context.Context, target execution.Target) ([]byte, error) {
	if target.Connection != "task-cluster" || target.Organization != "org" {
		return nil, ErrOwnership
	}
	return c.document, nil
}

type fixtureTargets struct{}

func (fixtureTargets) ResolveManagementTarget(context.Context, port.BindingVersion) (execution.Target, error) {
	return execution.Target{Organization: "org", Connection: "task-cluster"}, nil
}
func (fixtureTargets) ResolveWorkloadTarget(context.Context, port.BindingVersion, string) (execution.Target, error) {
	return execution.Target{Organization: "org", Connection: "task-cluster"}, nil
}

type fixturePublisher struct{ writer *publication.Writer }

func (f fixturePublisher) ResolvePublisher(context.Context, port.BindingVersion) (gitport.Publisher, error) {
	return f.writer, nil
}

// Explicit opt-in only: this test creates objects on the verified disposable
// cluster, never a default/host kube context or preexisting shared target.
func TestFleetOwnedClusterHTTPSPublicationAndWithdrawal(t *testing.T) {
	run := os.Getenv("ORCH_RECON_FLEET_LIVE_DIR")
	if run == "" {
		t.Skip("explicit owned Fleet conformance environment required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	nameRaw, err := os.ReadFile(filepath.Join(run, "cluster-name"))
	if err != nil {
		t.Fatal("cluster proof unavailable")
	}
	name := strings.TrimSpace(string(nameRaw))
	if !strings.HasPrefix(name, "orch-reconciliation-") {
		t.Fatal("not task cluster")
	}
	expected, err := os.ReadFile(filepath.Join(run, "container-id"))
	if err != nil {
		t.Fatal("container proof unavailable")
	}
	actual, err := exec.Command("docker", "inspect", "--format", "{{.Id}}", name+"-control-plane").Output()
	if err != nil || string(actual) != string(expected) {
		t.Fatal("cluster incarnation changed")
	}
	document, err := os.ReadFile(filepath.Join(run, "kubeconfig"))
	if err != nil {
		t.Fatal("private kubeconfig unavailable")
	}
	creds := fixtureCredentials{document}
	target := execution.Target{Organization: "org", Connection: "task-cluster", Namespace: "orch-fleet-conformance", Extra: map[string]string{"application": "fixture", "environment": "staging", "deployment": "conformance"}}
	cli, close, err := k8s.OpenCLI(ctx, "", creds, target)
	if err != nil {
		t.Fatal(err)
	}
	defer close()
	ns, exists, err := cli.Get(ctx, "", "namespace", "kube-system")
	if err != nil || !exists {
		t.Fatal("cluster identity unavailable")
	}
	md, _ := ns["metadata"].(map[string]any)
	uid, err := os.ReadFile(filepath.Join(run, "kube-system-uid"))
	if err != nil || md["uid"] != string(uid) {
		t.Fatal("cluster UID changed")
	}
	cluster, exists, err := cli.Get(ctx, "fleet-local", "clusters.fleet.cattle.io", "local")
	if err != nil || !exists {
		t.Fatal("Fleet registration unavailable")
	}
	clusterMD, _ := cluster["metadata"].(map[string]any)
	ca, err := os.ReadFile(filepath.Join(run, "git-tls", "ca.pem"))
	if err != nil {
		t.Fatal("Git CA unavailable")
	}
	// Release storage belongs to Fleet, not the Terraform application namespace.
	if _, found, err := cli.Get(ctx, "", "namespace", "fleet-releases"); err != nil {
		t.Fatal(err)
	} else if !found {
		if err := cli.Apply(ctx, []map[string]any{{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]any{"name": "fleet-releases"}}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := cli.Apply(ctx, []map[string]any{
		{"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "Role", "metadata": map[string]any{"name": "release-storage", "namespace": "fleet-releases"}, "rules": []any{map[string]any{"apiGroups": []any{""}, "resources": []any{"secrets"}, "verbs": []any{"get", "list", "watch", "create", "update", "patch", "delete"}}}},
		{"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "RoleBinding", "metadata": map[string]any{"name": "release-storage", "namespace": "fleet-releases"}, "roleRef": map[string]any{"apiGroup": "rbac.authorization.k8s.io", "kind": "Role", "name": "release-storage"}, "subjects": []any{map[string]any{"kind": "ServiceAccount", "name": "workload-delivery", "namespace": "cattle-fleet-system"}}},
		{"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "ClusterRole", "metadata": map[string]any{"name": "orch-conformance-release-namespace"}, "rules": []any{map[string]any{"apiGroups": []any{""}, "resources": []any{"namespaces"}, "resourceNames": []any{"fleet-releases"}, "verbs": []any{"get", "patch", "update"}}}},
		{"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "ClusterRoleBinding", "metadata": map[string]any{"name": "orch-conformance-release-namespace"}, "roleRef": map[string]any{"apiGroup": "rbac.authorization.k8s.io", "kind": "ClusterRole", "name": "orch-conformance-release-namespace"}, "subjects": []any{map[string]any{"kind": "ServiceAccount", "name": "workload-delivery", "namespace": "cattle-fleet-system"}}},
	}); err != nil {
		t.Fatal(err)
	}
	// This SA is fixture-owned in the task namespace. Keep it until cluster cleanup.
	if err := cli.Apply(ctx, []map[string]any{{"apiVersion": "v1", "kind": "ServiceAccount", "metadata": map[string]any{"name": "workload-delivery", "namespace": "cattle-fleet-system"}}}); err != nil {
		t.Fatal(err)
	}
	// A tightly scoped namespace role avoids granting Fleet broad cluster ownership.
	if err := cli.Apply(ctx, []map[string]any{{"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "Role", "metadata": map[string]any{"name": "workload-delivery", "namespace": target.Namespace}, "rules": []any{map[string]any{"apiGroups": []any{""}, "resources": []any{"configmaps", "secrets"}, "verbs": []any{"get", "list", "watch", "create", "update", "patch", "delete"}}}}, {"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "RoleBinding", "metadata": map[string]any{"name": "workload-delivery", "namespace": target.Namespace}, "roleRef": map[string]any{"apiGroup": "rbac.authorization.k8s.io", "kind": "Role", "name": "workload-delivery"}, "subjects": []any{map[string]any{"kind": "ServiceAccount", "name": "workload-delivery", "namespace": "cattle-fleet-system"}}}}); err != nil {
		t.Fatal(err)
	}
	i := packageIntent()
	i.Target = target
	i.Binding.Config["releaseNamespace"] = "fleet-releases"
	i.Binding.Config["repositoryURL"] = "https://172.18.0.1:19443/repo.git"
	i.Binding.Config["caBundle"] = string(ca)
	i.Binding.Config["readSecret"] = ""
	i.Binding.Config["targetClusterUID"] = clusterMD["uid"].(string)
	i.Binding.Config["targetAccountKey"] = "task-cluster"
	i.Revision.Scope.EnvironmentID = "fixture-https-environment"
	i.Revision.Scope.UnitKey = "fixture/configmap"
	i.Revision.ID = "fixture-https-revision"
	i.ID = "fixture-https-intent"
	m := configMap()
	m.Namespace = target.Namespace
	metadata := m.Object["metadata"].(map[string]any)
	metadata["namespace"] = target.Namespace
	m.Name = "adapter-config"
	metadata["name"] = m.Name
	labels := metadata["labels"].(map[string]any)
	labels["orchestrator.io/application"] = "fixture"
	labels["orchestrator.io/environment"] = "staging"
	labels["orchestrator.io/workload"] = "fixture"
	pkg, err := PackageUnit(i, []execution.Manifest{m})
	if err != nil {
		t.Fatal(err)
	}
	writer, err := publication.New(filepath.Join(run, "lifecycle", "writer"), pkg.Publication.DestinationKey, []string{"GIT_SSL_CAINFO=" + filepath.Join(run, "git-tls", "ca.pem")})
	if err != nil {
		t.Fatal(err)
	}
	provider := &Provider{Manager: &Manager{Verifier: &Verifier{Credentials: creds, Targets: fixtureTargets{}}, Guard: func(context.Context, port.Intent) error { return nil }}, Publishers: fixturePublisher{writer}}
	prepared, err := provider.Prepare(ctx, i, []execution.Manifest{m})
	if err != nil {
		t.Fatal(err)
	}
	i.Revision = prepared.Revision
	receipt, err := provider.Submit(ctx, i, prepared)
	if err != nil {
		t.Fatal("submit", err)
	}
	var observed domain.Observation
	for {
		observed, err = provider.Observe(ctx, i, receipt)
		if err != nil {
			t.Fatal("observe", err)
		}
		if observed.Ready(i.Revision, time.Now(), 30*time.Second) {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("Fleet did not reach exact revision readiness", observed)
		case <-time.After(2 * time.Second):
		}
	}
	obj, exists, err := cli.Get(ctx, target.Namespace, "configmap", m.Name)
	if err != nil || !exists {
		t.Fatal("live object absent")
	}
	objectMD, _ := obj["metadata"].(map[string]any)
	originalUID := objectMD["uid"]
	proof, err := provider.Withdraw(ctx, i, receipt)
	if err != nil || !proof.ManagementStopped {
		t.Fatal("withdrawal not proven", err)
	}
	obj, exists, err = cli.Get(ctx, target.Namespace, "configmap", m.Name)
	if err != nil || !exists {
		t.Fatal("retained object absent")
	}
	objectMD, _ = obj["metadata"].(map[string]any)
	if objectMD["uid"] != originalUID {
		t.Fatal("object recreated during withdrawal")
	}
	evidence, _ := json.MarshalIndent(map[string]any{"receipt": receipt, "observation": observed, "retainedUID": originalUID, "managementStopped": proof.ManagementStopped}, "", "  ")
	if err := os.WriteFile(filepath.Join(run, "adapter-https-conformance.json"), evidence, 0o600); err != nil {
		t.Fatal(err)
	}
}
