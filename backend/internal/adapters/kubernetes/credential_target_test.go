package kubernetes

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"orchestrator/internal/application/connection"
	ct "orchestrator/internal/application/connection/connectiontest"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/ports/execution"
)

// stubKubectl records each invocation, the permissions of a --kubeconfig
// file and whether it holds the expected credential. It never contacts a
// cluster.
const stubKubectl = `#!/bin/sh
kc=""; prev=""
for a in "$@"; do
  if [ "$prev" = "--kubeconfig" ]; then kc="$a"; fi
  prev="$a"
done
echo "ARGS $*" >> "$STUB_LOG"
if [ -n "$kc" ]; then
  echo "FILE $kc $(stat -c %a "$kc") $(stat -c %a "$(dirname "$kc")")" >> "$STUB_LOG"
  if grep -q "$STUB_EXPECT" "$kc"; then echo "CRED ok" >> "$STUB_LOG"; fi
fi
if [ -n "$STUB_LEAK" ]; then
  case "$*" in
    *"$STUB_LEAK"*)
      # A hostile API server or proxy echoing everything it was sent.
      echo "$STUB_LEAK_MSG" >&2
      echo "kubeconfig $kc in $(dirname "$kc")" >&2
      [ -n "$kc" ] && cat "$kc" >&2
      exit 1 ;;
  esac
fi
case "$*" in
  *"version -o json"*) echo '{"serverVersion":{"gitVersion":"v1.34.0"}}' ;;
  *"auth can-i"*) echo "${STUB_CAN_I:-yes}"; [ "${STUB_CAN_I:-yes}" = yes ] || exit 1 ;;
  *" get "*) if [ -n "$STUB_GET" ]; then echo "$STUB_GET"; else echo "Error from server (NotFound): not found" >&2; exit 1; fi ;;
  *" delete "*) if [ -n "$STUB_FAIL_DELETE" ]; then echo "error: loading $kc failed" >&2; exit 1; fi ;;
esac
`

type stubResolver struct {
	document []byte
	err      error
	targets  []execution.Target
}

func (r *stubResolver) ResolveKubeconfig(_ context.Context, target execution.Target) ([]byte, error) {
	r.targets = append(r.targets, target)
	return r.document, r.err
}

func installStub(t *testing.T) (kubectl, logPath string) {
	t.Helper()
	dir := t.TempDir()
	kubectl = filepath.Join(dir, "kubectl")
	if err := os.WriteFile(kubectl, []byte(stubKubectl), 0o700); err != nil {
		t.Fatal(err)
	}
	logPath = filepath.Join(dir, "log")
	t.Setenv("STUB_LOG", logPath)
	t.Setenv("STUB_EXPECT", "stub-token-value")
	return kubectl, logPath
}

func readLog(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return string(b)
}

// assertPrivateAndRemoved checks every logged credential file was 0600 in a
// 0700 directory, held the credential and no longer exists.
func assertPrivateAndRemoved(t *testing.T, log string) {
	t.Helper()
	files := 0
	for _, line := range strings.Split(log, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 4 || fields[0] != "FILE" {
			continue
		}
		files++
		if fields[2] != "600" || fields[3] != "700" {
			t.Fatalf("credential file not private: %s", line)
		}
		if _, err := os.Stat(filepath.Dir(fields[1])); !os.IsNotExist(err) {
			t.Fatalf("credential directory not removed: %s", fields[1])
		}
	}
	if files == 0 || !strings.Contains(log, "CRED ok") {
		t.Fatalf("no credential file used: %s", log)
	}
}

func credentialTarget() execution.Target {
	return execution.Target{Kind: "kubernetes", Context: "lab", ClusterName: "lab-cluster", Namespace: "team-a", Organization: "acme", Connection: "lab"}
}

func normalized(t *testing.T) []byte {
	t.Helper()
	selected, err := connection.NormalizeKubeconfig(ct.Single("lab", "https://lab.example", "stub-token-value"), "lab")
	if err != nil {
		t.Fatal(err)
	}
	return selected.Document
}

func TestCredentialTarget_DeployerUsesScopedPrivateKubeconfig(t *testing.T) {
	kubectl, logPath := installStub(t)
	resolver := &stubResolver{document: normalized(t)}
	deployer := &Deployer{KubectlPath: kubectl, Timeout: time.Second, Credentials: resolver}
	ctx := context.Background()
	target := credentialTarget()
	if err := deployer.Apply(ctx, target, []execution.Manifest{{Object: map[string]any{"kind": "ConfigMap"}}}); err != nil {
		t.Fatal(err)
	}
	if err := deployer.WaitReady(ctx, target, []execution.WorkloadRef{{Kind: "Deployment", Name: "api"}}); err != nil {
		t.Fatal(err)
	}
	if err := deployer.Remove(ctx, target, "api"); err != nil {
		t.Fatal(err)
	}
	log := readLog(t, logPath)
	assertPrivateAndRemoved(t, log)
	for _, line := range strings.Split(log, "\n") {
		if strings.HasPrefix(line, "ARGS ") && (!strings.Contains(line, "--kubeconfig") || !strings.Contains(line, "--context lab")) {
			t.Fatalf("kubectl call without the scoped credential: %s", line)
		}
	}
	if len(resolver.targets) != 3 || resolver.targets[0].Organization != "acme" || resolver.targets[0].Connection != "lab" {
		t.Fatalf("resolver targets: %#v", resolver.targets)
	}

	t.Setenv("STUB_FAIL_DELETE", "1")
	err := deployer.Remove(ctx, target, "api")
	if err == nil || strings.Contains(err.Error(), "orch-kube-") || strings.Contains(err.Error(), "loading") || !errors.Is(err, ErrKubectlCommand) {
		t.Fatalf("error must not carry kubectl output: %v", err)
	}
}

func TestCredentialTarget_FailsClosedWithoutHostFallback(t *testing.T) {
	kubectl, logPath := installStub(t)
	ctx := context.Background()
	target := credentialTarget()
	for name, source := range map[string]execution.KubeconfigSource{
		"no resolver":    nil,
		"resolver error": &stubResolver{err: connection.ErrCredentialUnavailable},
	} {
		deployer := &Deployer{KubectlPath: kubectl, Credentials: source}
		if err := deployer.Apply(ctx, target, nil); !errors.Is(err, ErrCredentialTarget) {
			t.Fatalf("%s apply: %v", name, err)
		}
		if err := deployer.Remove(ctx, target, "api"); !errors.Is(err, ErrCredentialTarget) {
			t.Fatalf("%s remove: %v", name, err)
		}
		routes := &PublicRoutes{KubectlPath: kubectl, Credentials: source}
		if err := routes.Reconcile(ctx, target, execution.PublicRoute{ApplicationID: "a", EnvironmentID: "staging"}); !errors.Is(err, ErrCredentialTarget) {
			t.Fatalf("%s routes: %v", name, err)
		}
		vso := &VSOSynchronizer{KubectlPath: kubectl, Credentials: source}
		bundle := execution.ConfigBundle{StoreKey: "store-a", AuthMount: "kubernetes", Address: "http://vault", Mount: "kv", Path: "p", Role: "r", ServiceAccount: "sa", SecretName: "orch-x", Keys: map[string]map[string]string{"main": {"A": "main_A"}}}
		if err := vso.Sync(ctx, target, bundle); !errors.Is(err, ErrCredentialTarget) {
			t.Fatalf("%s vso: %v", name, err)
		}
		executor := &Executor{KubectlPath: kubectl, Credentials: source}
		if _, err := executor.Provision(ctx, execution.ProvisionRequest{ResourceType: "k8s-namespace", Inputs: map[string]any{"name": "team-a"}, Target: target}); !errors.Is(err, ErrCredentialTarget) {
			t.Fatalf("%s executor: %v", name, err)
		}
	}
	if log := readLog(t, logPath); log != "" {
		t.Fatalf("kubectl ran without a resolved credential: %s", log)
	}
}

func TestCredentialTarget_RoutesVSOAndExecutorUseScopedCredential(t *testing.T) {
	kubectl, logPath := installStub(t)
	ctx := context.Background()
	resolver := &stubResolver{document: normalized(t)}
	target := credentialTarget()
	if err := (&PublicRoutes{KubectlPath: kubectl, Credentials: resolver}).Reconcile(ctx, target, execution.PublicRoute{ApplicationID: "a", EnvironmentID: "staging"}); err != nil {
		t.Fatal(err)
	}
	if _, err := (&Executor{KubectlPath: kubectl, Credentials: resolver}).Provision(ctx, execution.ProvisionRequest{ResourceType: "k8s-namespace", Descriptor: "ns", Inputs: map[string]any{"name": "team-a"}, Target: target}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("STUB_GET", `{"data":{"main_A":"b3BhcXVl"}}`)
	bundle := execution.ConfigBundle{StoreKey: "store-a", AuthMount: "kubernetes", Address: "http://vault", Mount: "kv", Path: "p", Role: "r", ServiceAccount: "sa", SecretName: "orch-x", Keys: map[string]map[string]string{"main": {"A": "main_A"}}}
	if err := (&VSOSynchronizer{KubectlPath: kubectl, Credentials: resolver, Timeout: time.Second}).Sync(ctx, target, bundle); err != nil {
		t.Fatal(err)
	}
	assertPrivateAndRemoved(t, readLog(t, logPath))
	if len(resolver.targets) != 3 {
		t.Fatalf("resolver calls: %d", len(resolver.targets))
	}
}

func TestExistingCluster_CredentialBackedConnectionIsAuthoritative(t *testing.T) {
	kubectl, logPath := installStub(t)
	resolver := &stubResolver{document: normalized(t)}
	adapter := &ExistingClusterAdapter{KubectlPath: kubectl, Credentials: resolver}
	conn := application.Connection{Key: "lab", OrganizationKey: "acme", Kind: application.ConnectionKubernetes, AuthenticationType: application.AuthKubeconfig,
		Status: application.ConnectionReady, SecretRef: "kv2://kv/orchestrator/connections/acme/lab/credentials/x",
		Config: map[string]any{"cluster": "lab-cluster", "kubeContext": "lab", "endpoint": "https://lab.example"}}
	req := execution.ProvisionRequest{OrganizationKey: "acme", Connection: conn,
		Inputs: map[string]any{"kubeContext": "kind-idp-internal", "name": "elsewhere"}}
	result, err := adapter.Provision(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if result.Target == nil || !reflect.DeepEqual(*result.Target, execution.Target{Kind: "kubernetes", Context: "lab", ClusterName: "lab-cluster", Organization: "acme", Connection: "lab"}) {
		t.Fatalf("target: %#v", result.Target)
	}
	if result.Outputs["kubeContext"] != "lab" || result.Outputs["name"] != "lab-cluster" || result.Outputs["kubeconfig"] != nil {
		t.Fatalf("outputs: %#v", result.Outputs)
	}
	for _, values := range []map[string]any{result.Outputs, result.State} {
		for _, value := range values {
			if s, _ := value.(string); strings.Contains(s, "stub-token-value") || strings.Contains(s, "orch-kube-") || strings.Contains(s, "kv2://") {
				t.Fatalf("credential material or path persisted: %#v", values)
			}
		}
	}
	log := readLog(t, logPath)
	if strings.Contains(log, "kind-idp-internal") || !strings.Contains(log, "--context lab") {
		t.Fatalf("driver input redirected the context: %s", log)
	}
	assertPrivateAndRemoved(t, log)

	for name, mutate := range map[string]func(*execution.ProvisionRequest){
		"not READY":          func(r *execution.ProvisionRequest) { r.Connection.Status = application.ConnectionVerifying },
		"other organization": func(r *execution.ProvisionRequest) { r.OrganizationKey = "other" },
		"unknown auth":       func(r *execution.ProvisionRequest) { r.Connection.AuthenticationType = "TOKEN" },
	} {
		bad := req
		bad.Connection.Config = conn.Config
		mutate(&bad)
		if _, err := adapter.Provision(context.Background(), bad); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
}

func TestVerifyKubeconfig_ReadOnlyChecksWithSelectedContextOnly(t *testing.T) {
	kubectl, logPath := installStub(t)
	selected, err := connection.NormalizeKubeconfig(ct.Single("lab", "https://lab.example", "stub-token-value"), "lab")
	if err != nil {
		t.Fatal(err)
	}
	verifier := ConnectionVerifier{KubectlPath: kubectl}
	got, err := verifier.VerifyKubeconfig(context.Background(), selected)
	if err != nil || got.Endpoint != "https://lab.example" || got.Version != "v1.34.0" {
		t.Fatalf("verify: %#v %v", got, err)
	}
	log := readLog(t, logPath)
	assertPrivateAndRemoved(t, log)
	for _, line := range strings.Split(log, "\n") {
		if rest, ok := strings.CutPrefix(line, "ARGS "); ok {
			command := strings.Fields(strings.SplitN(rest, "--context lab ", 2)[1])[0]
			if command != "version" && command != "auth" {
				t.Fatalf("verification ran a non read-only command: %s", line)
			}
		}
	}
	if strings.Count(log, "auth can-i create") != 5 {
		t.Fatalf("permission checks: %s", log)
	}
	t.Setenv("STUB_CAN_I", "no")
	if _, err := verifier.VerifyKubeconfig(context.Background(), selected); !errors.Is(err, connection.ErrPermissionDenied) {
		t.Fatalf("missing permission: %v", err)
	}
}

func TestLegacyTargetKeepsHostContext(t *testing.T) {
	kubectl, logPath := installStub(t)
	deployer := &Deployer{KubectlPath: kubectl}
	if err := deployer.Remove(context.Background(), execution.Target{Context: "kind-idp-internal", Namespace: "ns"}, "api"); err != nil {
		t.Fatal(err)
	}
	log := readLog(t, logPath)
	if strings.Contains(log, "--kubeconfig") || !strings.Contains(log, "--context kind-idp-internal") {
		t.Fatalf("legacy target changed: %s", log)
	}
}
