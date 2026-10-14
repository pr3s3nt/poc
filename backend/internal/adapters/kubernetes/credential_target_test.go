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
deleted_file="${STUB_DELETED_FILE:-$STUB_LOG.deleted}"
was_deleted() {
  grep -Fqx "$1" "$deleted_file" 2>/dev/null || return 1
  if [ "${STUB_DELAY_DELETE_POLLS:-0}" -gt 0 ]; then
    count_file="$deleted_file.count"
    count=0
    [ -f "$count_file" ] && count=$(cat "$count_file")
    count=$((count + 1))
    echo "$count" > "$count_file"
    [ "$count" -le "$STUB_DELAY_DELETE_POLLS" ] && return 1
  fi
  return 0
}
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
  *" get deployment api "*)
    path="/apis/apps/v1/namespaces/team-a/deployments/api"
    if [ -n "$STUB_FAIL_WAIT_GET" ] && grep -Fqx "$path" "$deleted_file" 2>/dev/null; then echo "Error from server (Forbidden): wait denied" >&2; exit 1; fi
    if was_deleted "$path"; then echo "Error from server (NotFound): not found" >&2; exit 1; fi
    if [ -n "$STUB_DEPLOYMENT" ]; then doc="$STUB_DEPLOYMENT"; [ -n "$STUB_REPLACEMENT_AFTER_DELETE" ] && grep -Fqx "$path" "$deleted_file" 2>/dev/null && doc=$(printf '%s' "$doc" | sed "s/11111111-1111-4111-8111-111111111111/$STUB_REPLACEMENT_AFTER_DELETE/"); printf '%s\n' "$doc"; else echo "Error from server (NotFound): not found" >&2; exit 1; fi ;;
  *" get service api "*)
    path="/api/v1/namespaces/team-a/services/api"
    if [ -n "$STUB_FAIL_WAIT_GET" ] && grep -Fqx "$path" "$deleted_file" 2>/dev/null; then echo "Error from server (Forbidden): wait denied" >&2; exit 1; fi
    if was_deleted "$path"; then echo "Error from server (NotFound): not found" >&2; exit 1; fi
    if [ -n "$STUB_SERVICE" ]; then printf '%s\n' "$STUB_SERVICE"; else echo "Error from server (NotFound): not found" >&2; exit 1; fi ;;
  *" get secret api-env "*)
    path="/api/v1/namespaces/team-a/secrets/api-env"
    if [ -n "$STUB_FAIL_WAIT_GET" ] && grep -Fqx "$path" "$deleted_file" 2>/dev/null; then echo "Error from server (Forbidden): wait denied" >&2; exit 1; fi
    if was_deleted "$path"; then echo "Error from server (NotFound): not found" >&2; exit 1; fi
    if [ -n "$STUB_SECRET" ]; then printf '%s\n' "$STUB_SECRET"; else echo "Error from server (NotFound): not found" >&2; exit 1; fi ;;
  *" get "*) if [ -n "$STUB_GET" ]; then echo "$STUB_GET"; else echo "Error from server (NotFound): not found" >&2; exit 1; fi ;;
  *" delete --raw "*)
    raw_path=""; previous=""
    for arg in "$@"; do [ "$previous" = "--raw" ] && raw_path="$arg"; previous="$arg"; done
    if [ -n "$STUB_DELETE_BODY" ]; then cat > "$STUB_DELETE_BODY"; else cat >/dev/null; fi
    if [ -n "$STUB_REPLACEMENT_UID" ] && ! grep -q "$STUB_REPLACEMENT_UID" "$STUB_DELETE_BODY"; then echo "Error from server (Conflict): UID precondition failed" >&2; exit 1; fi
    if [ -n "$STUB_FAIL_DELETE" ]; then echo "error: loading $kc failed" >&2; exit 1; fi
    echo "$raw_path" >> "$deleted_file" ;;
  *" delete "*)
    if [ -n "$STUB_FAIL_DELETE" ]; then echo "error: loading $kc failed" >&2; exit 1; fi ;;
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
	t.Setenv("STUB_DELETED_FILE", filepath.Join(dir, "deleted-objects"))
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
	return execution.Target{Kind: "kubernetes", Context: "lab", ClusterName: "lab-cluster", Namespace: "team-a", Organization: "acme", Connection: "lab", Extra: map[string]string{"application": "shop", "environment": "staging", "deployment": "deploy-123"}}
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
	owned := `{"metadata":{"uid":"11111111-1111-4111-8111-111111111111","labels":{"orchestrator.io/application":"shop","orchestrator.io/environment":"staging","orchestrator.io/deployment-id":"deploy-123","app.kubernetes.io/managed-by":"orchestrator","app.kubernetes.io/name":"api"}}}`
	t.Setenv("STUB_DEPLOYMENT", owned)
	t.Setenv("STUB_SERVICE", owned)
	t.Setenv("STUB_SECRET", `{"metadata":{"uid":"33333333-3333-4333-8333-333333333333","labels":{"orchestrator.io/application":"shop","orchestrator.io/environment":"staging","orchestrator.io/deployment-id":"deploy-123","app.kubernetes.io/managed-by":"orchestrator","app.kubernetes.io/name":"api"}}}`)
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
	t.Setenv("STUB_DELETE_BODY", filepath.Join(t.TempDir(), "delete-options.json"))
	if err := os.WriteFile(os.Getenv("STUB_DELETED_FILE"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	err := deployer.Remove(ctx, target, "api")
	if err == nil || strings.Contains(err.Error(), "orch-kube-") || strings.Contains(err.Error(), "loading") || !errors.Is(err, ErrKubectlCommand) {
		t.Fatalf("error must not carry kubectl output: %v", err)
	}
}

func TestDeployerRemoveDeletesOnlyRendererSecretWithExactOwnership(t *testing.T) {
	kubectl, logPath := installStub(t)
	labels := `"labels":{"orchestrator.io/application":"shop","orchestrator.io/environment":"staging","orchestrator.io/deployment-id":"deploy-123","app.kubernetes.io/managed-by":"orchestrator","app.kubernetes.io/name":"api"}`
	t.Setenv("STUB_DEPLOYMENT", `{"metadata":{"name":"api","uid":"11111111-1111-4111-8111-111111111111",`+labels+`}}`)
	t.Setenv("STUB_SERVICE", `{"metadata":{"name":"api","uid":"22222222-2222-4222-8222-222222222222",`+labels+`}}`)
	t.Setenv("STUB_SECRET", `{"metadata":{"name":"api-env","uid":"33333333-3333-4333-8333-333333333333",`+labels+`}}`)
	deleteBody := filepath.Join(t.TempDir(), "delete-options.json")
	t.Setenv("STUB_DELETE_BODY", deleteBody)
	deployer := &Deployer{KubectlPath: kubectl, Credentials: &stubResolver{document: normalized(t)}}
	if err := deployer.Remove(context.Background(), credentialTarget(), "api"); err != nil {
		t.Fatal(err)
	}
	log := readLog(t, logPath)
	for _, want := range []string{"delete --raw /apis/apps/v1/namespaces/team-a/deployments/api -f -", "delete --raw /api/v1/namespaces/team-a/services/api -f -", "delete --raw /api/v1/namespaces/team-a/secrets/api-env -f -"} {
		if !strings.Contains(log, want) {
			t.Fatalf("owned object %q was not removed: %s", want, log)
		}
	}
	body, err := os.ReadFile(deleteBody)
	if err != nil || !strings.Contains(string(body), `"kind":"DeleteOptions"`) || !strings.Contains(string(body), `"uid":"33333333-3333-4333-8333-333333333333"`) {
		t.Fatalf("raw delete did not carry the inspected UID precondition: %v", err)
	}
	if !strings.Contains(string(body), `"propagationPolicy":"Foreground"`) {
		t.Fatalf("delete must wait for controller garbage collection: %s", body)
	}

	t.Setenv("STUB_LOG", filepath.Join(t.TempDir(), "foreign-log"))
	if err := os.WriteFile(os.Getenv("STUB_DELETED_FILE"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("STUB_SECRET", `{"metadata":{"name":"api-env","uid":"44444444-4444-4444-8444-444444444444","labels":{"orchestrator.io/application":"other","orchestrator.io/environment":"staging","orchestrator.io/deployment-id":"deploy-123","app.kubernetes.io/managed-by":"orchestrator","app.kubernetes.io/name":"api"}}}`)
	err = deployer.Remove(context.Background(), credentialTarget(), "api")
	if err == nil || !strings.Contains(err.Error(), "unverified application ownership") {
		t.Fatalf("foreign same-name Secret was not refused: %v", err)
	}
	if log := readLog(t, os.Getenv("STUB_LOG")); strings.Contains(log, " delete ") {
		t.Fatalf("ownership mismatch must be detected before every delete: %s", log)
	}
}

func TestDeployerRemoveWaitsForForegroundAndFailsClosedDuringAbsenceCheck(t *testing.T) {
	for _, tc := range []struct {
		name            string
		delayPolls      string
		replacementUID  string
		waitError       string
		wantErrContains string
	}{
		{name: "waits for controller garbage collection", delayPolls: "1"},
		{name: "replacement appears", delayPolls: "1", replacementUID: "99999999-9999-4999-8999-999999999999", wantErrContains: "replacement object"},
		{name: "absence query is forbidden", waitError: "1", wantErrContains: "not permitted"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			kubectl, logPath := installStub(t)
			labels := `"labels":{"orchestrator.io/application":"shop","orchestrator.io/environment":"staging","orchestrator.io/deployment-id":"deploy-123","app.kubernetes.io/managed-by":"orchestrator","app.kubernetes.io/name":"api"}`
			t.Setenv("STUB_DEPLOYMENT", `{"metadata":{"uid":"11111111-1111-4111-8111-111111111111",`+labels+`}}`)
			t.Setenv("STUB_SERVICE", `{"metadata":{"uid":"22222222-2222-4222-8222-222222222222",`+labels+`}}`)
			t.Setenv("STUB_SECRET", `{"metadata":{"uid":"33333333-3333-4333-8333-333333333333",`+labels+`}}`)
			t.Setenv("STUB_DELAY_DELETE_POLLS", tc.delayPolls)
			t.Setenv("STUB_REPLACEMENT_AFTER_DELETE", tc.replacementUID)
			t.Setenv("STUB_FAIL_WAIT_GET", tc.waitError)
			deployer := &Deployer{KubectlPath: kubectl, Timeout: time.Second, Credentials: &stubResolver{document: normalized(t)}}
			err := deployer.Remove(context.Background(), credentialTarget(), "api")
			if tc.wantErrContains == "" {
				if err != nil {
					t.Fatal(err)
				}
				log := readLog(t, logPath)
				if strings.Count(log, " get deployment api ") < 3 {
					t.Fatalf("removal did not wait for the original controller to disappear: %s", log)
				}
				return
			}
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), tc.wantErrContains) {
				t.Fatalf("absence check error=%v, want %q", err, tc.wantErrContains)
			}
		})
	}
}

func TestDeleteUIDAllowsRendererSecretSuffixBeyondDNSLabelLength(t *testing.T) {
	kubectl, logPath := installStub(t)
	name := strings.Repeat("a", 63) + "-env"
	cli := NewCLI(kubectl, "", filepath.Join(t.TempDir(), "kubeconfig"))
	if err := os.WriteFile(cli.Kubeconfig, []byte("explicit test kubeconfig"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := cli.DeleteUID(context.Background(), "team-a", "secret", name, "33333333-3333-4333-8333-333333333333", time.Second); err != nil {
		t.Fatalf("valid rendered Secret name of %d characters was refused: %v", len(name), err)
	}
	if !strings.Contains(readLog(t, logPath), "/api/v1/namespaces/team-a/secrets/"+name) {
		t.Fatal("UID-preconditioned Secret delete did not address the exact long renderer name")
	}
	before := readLog(t, logPath)
	if err := cli.DeleteUID(context.Background(), "team-a", "secret", "valid/../../foreign", "33333333-3333-4333-8333-333333333333", time.Second); err == nil {
		t.Fatal("resource path injection was accepted")
	}
	if after := readLog(t, logPath); after != before {
		t.Fatal("invalid resource name reached kubectl")
	}
}

func TestDeployerRemoveUIDPreconditionRetainsSameNameReplacement(t *testing.T) {
	kubectl, logPath := installStub(t)
	labels := `"labels":{"orchestrator.io/application":"shop","orchestrator.io/environment":"staging","orchestrator.io/deployment-id":"deploy-123","app.kubernetes.io/managed-by":"orchestrator","app.kubernetes.io/name":"api"}`
	t.Setenv("STUB_DEPLOYMENT", `{"metadata":{"uid":"11111111-1111-4111-8111-111111111111",`+labels+`}}`)
	t.Setenv("STUB_SERVICE", `{"metadata":{"uid":"22222222-2222-4222-8222-222222222222",`+labels+`}}`)
	t.Setenv("STUB_SECRET", `{"metadata":{"uid":"33333333-3333-4333-8333-333333333333",`+labels+`}}`)
	// The API fixture now has a replacement UID, so its atomic delete precondition fails.
	t.Setenv("STUB_REPLACEMENT_UID", "99999999-9999-4999-8999-999999999999")
	deleteBody := filepath.Join(t.TempDir(), "delete-options.json")
	t.Setenv("STUB_DELETE_BODY", deleteBody)
	deployer := &Deployer{KubectlPath: kubectl, Credentials: &stubResolver{document: normalized(t)}}
	err := deployer.Remove(context.Background(), credentialTarget(), "api")
	if err == nil || !errors.Is(err, ErrRequestRejected) {
		t.Fatalf("UID replacement should conflict: %v", err)
	}
	log := readLog(t, logPath)
	body, readErr := os.ReadFile(deleteBody)
	if strings.Contains(log, "stub-token-value") || !strings.Contains(log, "delete --raw /apis/apps/v1/namespaces/team-a/deployments/api -f -") || readErr != nil || !strings.Contains(string(body), `"uid":"11111111-1111-4111-8111-111111111111"`) {
		t.Fatalf("delete request did not carry an opaque UID precondition safely: %s err=%v", log, readErr)
	}
}

func TestDeployerRemoveRefusesOwnedLabelShapeWithoutUIDBeforeAnyDelete(t *testing.T) {
	kubectl, logPath := installStub(t)
	labels := `"labels":{"orchestrator.io/application":"shop","orchestrator.io/environment":"staging","orchestrator.io/deployment-id":"deploy-123","app.kubernetes.io/managed-by":"orchestrator","app.kubernetes.io/name":"api"}`
	t.Setenv("STUB_DEPLOYMENT", `{"metadata":{"uid":"11111111-1111-4111-8111-111111111111",`+labels+`}}`)
	t.Setenv("STUB_SERVICE", `{"metadata":{"uid":"22222222-2222-4222-8222-222222222222",`+labels+`}}`)
	t.Setenv("STUB_SECRET", `{"metadata":{`+labels+`}}`)
	deployer := &Deployer{KubectlPath: kubectl, Credentials: &stubResolver{document: normalized(t)}}
	if err := deployer.Remove(context.Background(), credentialTarget(), "api"); err == nil || !strings.Contains(err.Error(), "without a verified UID") {
		t.Fatalf("missing UID did not fail closed: %v", err)
	}
	if log := readLog(t, logPath); strings.Contains(log, " delete ") {
		t.Fatalf("prevalidation failed after a deletion: %s", log)
	}
}

func TestDeployerRemoveRefusesForeignDeploymentOrServiceBeforeDeleting(t *testing.T) {
	for _, object := range []string{"deployment", "service"} {
		t.Run(object, func(t *testing.T) {
			kubectl, logPath := installStub(t)
			labels := `"labels":{"orchestrator.io/application":"other","orchestrator.io/environment":"staging","orchestrator.io/deployment-id":"deploy-123"}`
			doc := `{"metadata":{"name":"api",` + labels + `}}`
			if object == "deployment" {
				t.Setenv("STUB_DEPLOYMENT", doc)
			} else {
				t.Setenv("STUB_DEPLOYMENT", "")
				t.Setenv("STUB_SERVICE", doc)
			}
			deployer := &Deployer{KubectlPath: kubectl, Credentials: &stubResolver{document: normalized(t)}}
			if err := deployer.Remove(context.Background(), credentialTarget(), "api"); err == nil {
				t.Fatal("foreign same-name workload object was accepted")
			}
			if log := readLog(t, logPath); strings.Contains(log, " delete ") {
				t.Fatalf("ownership mismatch must be found before every delete: %s", log)
			}
		})
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
	conn := application.Connection{Name: "lab", Key: "lab", OrganizationKey: "acme", Kind: application.ConnectionKubernetes, AuthenticationType: application.AuthKubeconfig,
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

// Context-only and empty targets never reach kubectl: host kube configuration
// is not a default (T22).
func TestContextOnlyTargetFailsClosedWithoutRunningKubectl(t *testing.T) {
	kubectl, logPath := installStub(t)
	deployer := &Deployer{KubectlPath: kubectl}
	for name, target := range map[string]execution.Target{
		"context only": {Context: "kind-idp-internal", Namespace: "ns"},
		"empty":        {Namespace: "ns"},
	} {
		if err := deployer.Remove(context.Background(), target, "api"); err == nil {
			t.Fatalf("%s target was accepted", name)
		}
		if _, _, err := OpenCLI(context.Background(), kubectl, nil, target); !errors.Is(err, ErrCredentialTarget) {
			t.Fatalf("%s OpenCLI: %v", name, err)
		}
	}
	if _, err := NewCLI(kubectl, "kind-idp-internal", "").Run(context.Background(), nil, "get", "pods"); !errors.Is(err, ErrHostKubeconfig) {
		t.Fatalf("low-level Run without kubeconfig: %v", err)
	}
	// A nonexistent or directory kubeconfig path never reaches kubectl either.
	for name, path := range map[string]string{"missing file": filepath.Join(t.TempDir(), "absent"), "directory": t.TempDir()} {
		if _, err := NewCLI(kubectl, "eks", path).Run(context.Background(), nil, "get", "pods"); !errors.Is(err, ErrHostKubeconfig) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if log, err := os.ReadFile(logPath); err == nil && strings.TrimSpace(string(log)) != "" {
		t.Fatalf("kubectl ran for a host-default target: %s", log)
	}
}

// A Terraform-generated kubeconfig path is an explicit target and keeps working.
func TestExplicitKubeconfigPathTargetStillRuns(t *testing.T) {
	kubectl, logPath := installStub(t)
	path := filepath.Join(t.TempDir(), "eks.kubeconfig")
	if err := os.WriteFile(path, []byte("apiVersion: v1\nkind: Config\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	deployer := &Deployer{KubectlPath: kubectl}
	if err := deployer.Remove(context.Background(), execution.Target{Context: "eks-cluster", Kubeconfig: path, Namespace: "ns", Extra: map[string]string{"application": "shop", "environment": "staging", "deployment": "deploy-123"}}, "api"); err != nil {
		t.Fatal(err)
	}
	log := readLog(t, logPath)
	if !strings.Contains(log, "--kubeconfig "+path) || !strings.Contains(log, "--context eks-cluster") {
		t.Fatalf("explicit kubeconfig target changed: %s", log)
	}
}

// An existing-cluster Connection that is not KUBECONFIG never reaches kubectl.
func TestExistingClusterRejectsNonKubeconfigConnections(t *testing.T) {
	kubectl, logPath := installStub(t)
	adapter := NewExistingClusterAdapter(kubectl)
	for name, auth := range map[string]application.AuthenticationType{"host context": "HOST_CONTEXT", "missing": ""} {
		_, err := adapter.Provision(context.Background(), execution.ProvisionRequest{OrganizationKey: "acme", Connection: application.Connection{Name: "lab",
			Key: "lab", OrganizationKey: "acme", Kind: application.ConnectionKubernetes, Status: application.ConnectionReady, AuthenticationType: auth,
			Config: map[string]any{"kubeContext": "kind-idp-internal", "cluster": "c"}}})
		if err == nil {
			t.Fatalf("%s connection was provisioned", name)
		}
	}
	if log, err := os.ReadFile(logPath); err == nil && strings.TrimSpace(string(log)) != "" {
		t.Fatalf("kubectl ran for a host connection: %s", log)
	}
}
