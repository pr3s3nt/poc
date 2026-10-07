package deployment_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"orchestrator/internal/adapters/credentialmemory"
	"orchestrator/internal/adapters/fake"
	"orchestrator/internal/adapters/kubernetes"
	"orchestrator/internal/application/connection"
	ct "orchestrator/internal/application/connection/connectiontest"
	appsvc "orchestrator/internal/application/deployment"
	"orchestrator/internal/bootstrap"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/seed"
)

const restartToken = "restart-token-0123456789"

// restartKubectl is a stub kubectl. It records each call, whether the call
// carried a --kubeconfig file holding the uploaded credential, and answers
// NotFound for reads. With STUB_LEAK set, apply fails and echoes the
// credential file like a hostile API server. It never contacts a cluster.
const restartKubectl = `#!/bin/sh
kc=""; prev=""
for a in "$@"; do
  if [ "$prev" = "--kubeconfig" ]; then kc="$a"; fi
  prev="$a"
done
cred="none"
if [ -n "$kc" ] && grep -q "$STUB_EXPECT" "$kc"; then cred="ok"; fi
echo "CALL cred=$cred $*" >> "$STUB_LOG"
case "$*" in
  *" apply "*) if [ -n "$STUB_LEAK" ]; then echo "Error from server (BadRequest): $kc" >&2; cat "$kc" >&2; exit 1; fi ;;
  *" get "*) echo 'Error from server (NotFound): ingresses.networking.k8s.io "x" not found' >&2; exit 1 ;;
esac
`

type uploadVerifier struct{}

func (uploadVerifier) VerifyKubeconfig(_ context.Context, selected connection.SelectedKubeconfig) (connection.KubernetesVerification, error) {
	return connection.KubernetesVerification{Endpoint: selected.Context.Endpoint, Version: "v1.34.0"}, nil
}

// restartInstance is one orchestrator process: a fresh store loaded from the
// snapshot file, a fresh resolver and fresh kubectl adapters.
type restartInstance struct {
	app      *bootstrap.App
	executor *credentialExecutor
}

func startInstance(t *testing.T, statePath, kubectl string, creds *credentialmemory.Store) restartInstance {
	t.Helper()
	executor := &credentialExecutor{fake: fake.NewResourceExecutor()}
	deployer := &kubernetes.Deployer{KubectlPath: kubectl, Timeout: time.Second}
	app, err := bootstrap.Build(context.Background(), bootstrap.Options{
		Seed: seed.Defaults(), Adapters: bootstrap.AdapterFake, StatePath: statePath,
		RegistryOverride: executor, DeployerOverride: deployer,
		// The credential store stands in for durable Vault: it outlives the
		// process, the snapshot holds only the opaque reference.
		ConnectionCredentialsOverride: creds,
	})
	if err != nil {
		t.Fatal(err)
	}
	resolver := connection.NewCredentialResolver(app.Store, creds)
	deployer.Credentials = resolver
	app.Deployments.SetPublicRouteManager(&kubernetes.PublicRoutes{KubectlPath: kubectl, Credentials: resolver}, "example.com")
	return restartInstance{app: app, executor: executor}
}

// TestCredentialBackedTarget_RemovalAfterRestart deploys to an uploaded
// Connection, discards the process, starts a new one from the persisted
// snapshot and removes the workload. The new process holds no in-memory
// target: the Organization/Connection identity comes from persisted state and
// the credential is resolved again from the credential store.
func TestCredentialBackedTarget_RemovalAfterRestart(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	kubectl := filepath.Join(dir, "kubectl")
	if err := os.WriteFile(kubectl, []byte(restartKubectl), 0o700); err != nil {
		t.Fatal(err)
	}
	stubLog := filepath.Join(dir, "kubectl.log")
	t.Setenv("STUB_LOG", stubLog)
	t.Setenv("STUB_EXPECT", restartToken)
	statePath := filepath.Join(dir, "state.json")
	creds := credentialmemory.New()
	opts := seed.Defaults()
	scores := seed.AcceptanceScores(opts)

	first := startInstance(t, statePath, kubectl, creds)
	registration := connection.NewService(first.app.Store, nil)
	registration.SetKubeconfigRegistration(uploadVerifier{}, creds)
	created, err := registration.RegisterKubeconfig(ctx, opts.OrganizationKey, connection.RegisterKubeconfigCommand{
		Name: "Lab", Context: "lab",
		Kubeconfig: ct.Document(ct.TokenContext("lab", "https://lab.example", restartToken), ct.TokenContext("other", "https://other.example", "other-token-value")),
	})
	if err != nil {
		t.Fatal(err)
	}
	opts.ApplicationKey, opts.EnvironmentKey = newBoundApplication(t, first.app.Store, opts.OrganizationKey, "Lab App", "lab-app", created.Key, ""), "staging"
	if err := first.app.Store.SaveResourceDefinition(ctx, opts.OrganizationKey, resource.Definition{
		Key: "cluster-lab", ResourceTypeKey: "k8s-cluster", DriverType: resource.DriverExistingCluster,
		ExecutionProfile: "internal-k8s", ConnectionKey: created.Key,
		DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{"name": "${context.connection.cluster}", "kubeContext": "${context.connection.context}"}}},
		Criteria:     []resource.Criterion{{Class: "internal", ApplicationID: opts.ApplicationKey}},
	}); err != nil {
		t.Fatal(err)
	}
	deploy := appsvc.DeployCommand{OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey, WorkloadID: "backend", ScoreAfter: scores["backend"], Actor: "test"}
	if _, err := first.app.Deployments.DeployWorkload(ctx, deploy); err != nil {
		t.Fatal(err)
	}
	beforeRestart := readStubLog(t, stubLog)
	if !strings.Contains(beforeRestart, "cred=ok") || !strings.Contains(beforeRestart, " apply ") {
		t.Fatalf("deploy did not use the uploaded credential: %s", beforeRestart)
	}
	first = restartInstance{} // the first process is gone

	snapshot, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{restartToken, "other-token-value", "orch-kube-"} {
		if strings.Contains(string(snapshot), leak) {
			t.Fatalf("snapshot holds %q", leak)
		}
	}
	if err := os.Truncate(stubLog, 0); err != nil {
		t.Fatal(err)
	}

	second := startInstance(t, statePath, kubectl, creds)
	stored, err := second.app.Store.GetConnection(ctx, opts.OrganizationKey, created.Key)
	if err != nil || stored.AuthenticationType != application.AuthKubeconfig || stored.ConfigString("kubeContext") != "lab" || stored.SecretRef != created.SecretRef {
		t.Fatalf("connection after restart: %#v %v", stored, err)
	}
	instances, err := second.app.Store.ListWorkloadInstances(ctx, opts.ApplicationKey+"/"+opts.EnvironmentKey)
	if err != nil || len(instances) != 1 || instances[0].TargetRef["connection"] != created.Key || instances[0].TargetRef["organization"] != opts.OrganizationKey {
		t.Fatalf("persisted target after restart: %v %#v", err, instances)
	}
	remove := deploy
	remove.ScoreBefore, remove.ScoreAfter = scores["backend"], nil
	if _, err := second.app.Deployments.DeployWorkload(ctx, remove); err != nil {
		t.Fatal(err)
	}
	afterRestart := readStubLog(t, stubLog)
	calls := strings.Split(strings.TrimSpace(afterRestart), "\n")
	deletes, reads := 0, 0
	for _, call := range calls {
		if !strings.HasPrefix(call, "CALL cred=ok ") || !strings.Contains(call, "--context lab ") {
			t.Fatalf("call after restart without the stored credential: %s", call)
		}
		if strings.Contains(call, " delete deployment backend ") || strings.Contains(call, " delete service backend ") {
			deletes++
		}
		if strings.Contains(call, " get ingress ") {
			reads++
		}
	}
	if deletes != 2 || reads == 0 {
		t.Fatalf("removal or route reconciliation after restart: %s", afterRestart)
	}
	if instances, _ := second.app.Store.ListWorkloadInstances(ctx, opts.ApplicationKey+"/"+opts.EnvironmentKey); len(instances) != 1 || instances[0].Status != "REMOVED" {
		t.Fatalf("workload instance not marked removed: %#v", instances)
	}
	// The removal plan runs the executor again for the Environment's base
	// resources; dependants must target the persisted Connection identity.
	for _, req := range second.executor.requests {
		if req.ResourceType == "k8s-namespace" && (req.Target.Connection != created.Key || req.Target.Organization != opts.OrganizationKey || req.Target.Context != "lab") {
			t.Fatalf("namespace target after restart: %#v", req.Target)
		}
	}

	// A failing apply that echoes the credential leaves no trace of it in
	// the returned error or the persisted state.
	t.Setenv("STUB_LEAK", "1")
	_, err = second.app.Deployments.DeployWorkload(ctx, deploy)
	if err == nil {
		t.Fatal("echoing apply failure reported success")
	}
	snapshot, _ = os.ReadFile(statePath)
	for _, leak := range []string{restartToken, "orch-kube-", `"clusters"`} {
		if strings.Contains(err.Error(), leak) || strings.Contains(string(snapshot), leak) {
			t.Fatalf("apply failure leaked %q: %v", leak, err)
		}
	}
}

func readStubLog(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
