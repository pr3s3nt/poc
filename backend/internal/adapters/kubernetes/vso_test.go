package kubernetes

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	configport "orchestrator/internal/ports/configuration"
	"orchestrator/internal/ports/execution"
)

func TestHasBundleKeys(t *testing.T) {
	keys := map[string]map[string]string{"main": {"API_TOKEN": "main_API_TOKEN", "DB_HOST": "main_DB_HOST"}}
	for _, tc := range []struct {
		name   string
		secret map[string]any
		want   bool
	}{
		{"complete", map[string]any{"data": map[string]any{"main_API_TOKEN": "opaque", "main_DB_HOST": "opaque"}}, true},
		{"missing", map[string]any{"data": map[string]any{"main_API_TOKEN": "opaque"}}, false},
		{"no-data", map[string]any{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasBundleKeys(tc.secret, keys); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// A kubectl stand-in with two VaultStaticSecrets in one namespace: one below
// the Application's own value prefix, one of another Application. Only the
// first (and the objects it references) may be deleted.
const vsoStub = `#!/bin/sh
echo "$*" >> "$STUB_LOG"
case "$*" in
  *"get vaultstaticsecrets"*) cat <<'JSON'
{"items":[
 {"metadata":{"name":"orch-own"},"spec":{"path":"orchestrator/apps/acc-r1/envs/staging/values/d1","vaultAuthRef":"orch-own-auth","destination":{"name":"orch-own"}}},
 {"metadata":{"name":"orch-other"},"spec":{"path":"orchestrator/apps/other/envs/staging/values/d9","vaultAuthRef":"orch-other-auth","destination":{"name":"orch-other"}}}]}
JSON
  ;;
  *"get vaultauths"*) cat <<'JSON'
{"items":[
 {"metadata":{"name":"orch-own-auth"},"spec":{"vaultConnectionRef":"orch-vault-own","kubernetes":{"serviceAccount":"orch-sa-own"}}},
 {"metadata":{"name":"orch-other-auth"},"spec":{"vaultConnectionRef":"orch-vault-shared","kubernetes":{"serviceAccount":"orch-sa-other"}}}]}
JSON
  ;;
  *"get vaultconnections"*) echo '{"metadata":{"name":"orch-vault-own"},"spec":{"caCertSecretRef":"orch-vault-own-ca"}}' ;;
esac
`

func TestRemoveSynchronizedDeletesOnlyTheApplicationsOwnObjectsByExactName(t *testing.T) {
	dir := t.TempDir()
	kubectl := filepath.Join(dir, "kubectl")
	if err := os.WriteFile(kubectl, []byte(vsoStub), 0o700); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "log")
	t.Setenv("STUB_LOG", logPath)
	kubeconfig := filepath.Join(dir, "kubeconfig")
	if err := os.WriteFile(kubeconfig, []byte("synthetic"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &VSOSynchronizer{KubectlPath: kubectl}
	removed, err := s.RemoveSynchronized(context.Background(), execution.Target{Kind: "kubernetes", Namespace: "orch-v2-test-r1", Kubeconfig: kubeconfig, Context: "ctx"}, "acc-r1", "staging")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"secret/orch-own", "secret/orch-vault-own-ca", "serviceaccount/orch-sa-own", "vaultauths.secrets.hashicorp.com/orch-own-auth",
		"vaultconnections.secrets.hashicorp.com/orch-vault-own", "vaultstaticsecrets.secrets.hashicorp.com/orch-own"}
	if !reflect.DeepEqual(removed, want) {
		t.Fatalf("removed %v want %v", removed, want)
	}
	raw, _ := os.ReadFile(logPath)
	for _, forbidden := range []string{"orch-other", "orch-sa-other", "orch-vault-shared", "--all", "namespace "} {
		for _, line := range strings.Split(string(raw), "\n") {
			if strings.Contains(line, "delete") && strings.Contains(line, forbidden) {
				t.Fatalf("a foreign or bulk deletion was issued: %s", line)
			}
		}
	}
}

// UC-07 VSO-01: removing one workload deletes only the VSO objects bound to
// that workload's delivery ServiceAccount; another workload of the same
// Application keeps its objects and the VaultConnection they share.
func TestRemoveWorkloadSynchronizedDeletesOnlyTheWorkloadsObjects(t *testing.T) {
	own := configport.WorkloadServiceAccount("acc-r1", "staging", "backend")
	other := configport.WorkloadServiceAccount("acc-r1", "staging", "worker")
	stub := `#!/bin/sh
echo "$*" >> "$STUB_LOG"
case "$*" in
  *"get vaultstaticsecrets"*) cat <<'JSON'
{"items":[
 {"metadata":{"name":"orch-b1"},"spec":{"path":"orchestrator/apps/acc-r1/envs/staging/values/d1","vaultAuthRef":"orch-b1-auth","destination":{"name":"orch-b1"}}},
 {"metadata":{"name":"orch-b2"},"spec":{"path":"orchestrator/apps/acc-r1/envs/staging/values/d2","vaultAuthRef":"orch-b2-auth","destination":{"name":"orch-b2"}}},
 {"metadata":{"name":"orch-w1"},"spec":{"path":"orchestrator/apps/acc-r1/envs/staging/values/d3","vaultAuthRef":"orch-w1-auth","destination":{"name":"orch-w1"}}},
 {"metadata":{"name":"orch-x1"},"spec":{"path":"orchestrator/apps/other/envs/staging/values/d4","vaultAuthRef":"orch-x1-auth","destination":{"name":"orch-x1"}}}]}
JSON
  ;;
  *"get vaultauths"*) cat <<JSON
{"items":[
 {"metadata":{"name":"orch-b1-auth"},"spec":{"vaultConnectionRef":"orch-vault-shared","kubernetes":{"serviceAccount":"OWN"}}},
 {"metadata":{"name":"orch-b2-auth"},"spec":{"vaultConnectionRef":"orch-vault-shared","kubernetes":{"serviceAccount":"OWN"}}},
 {"metadata":{"name":"orch-w1-auth"},"spec":{"vaultConnectionRef":"orch-vault-shared","kubernetes":{"serviceAccount":"OTHER"}}},
 {"metadata":{"name":"orch-x1-auth"},"spec":{"vaultConnectionRef":"orch-vault-x","kubernetes":{"serviceAccount":"OWN"}}}]}
JSON
  ;;
esac
`
	stub = strings.ReplaceAll(strings.ReplaceAll(stub, "OWN", own), "OTHER", other)
	dir := t.TempDir()
	kubectl := filepath.Join(dir, "kubectl")
	if err := os.WriteFile(kubectl, []byte(stub), 0o700); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "log")
	t.Setenv("STUB_LOG", logPath)
	kubeconfig := filepath.Join(dir, "kubeconfig")
	if err := os.WriteFile(kubeconfig, []byte("synthetic"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &VSOSynchronizer{KubectlPath: kubectl}
	removed, err := s.RemoveWorkloadSynchronized(context.Background(), execution.Target{Kind: "kubernetes", Namespace: "app-ns", Kubeconfig: kubeconfig, Context: "ctx"}, "acc-r1", "staging", "backend")
	if err != nil {
		t.Fatal(err)
	}
	// The ServiceAccount is still bound by the other Application's VaultAuth
	// (orch-x1-auth names the same account) and is kept; the shared
	// VaultConnection is still used by the worker.
	want := []string{"secret/orch-b1", "secret/orch-b2", "vaultauths.secrets.hashicorp.com/orch-b1-auth", "vaultauths.secrets.hashicorp.com/orch-b2-auth",
		"vaultstaticsecrets.secrets.hashicorp.com/orch-b1", "vaultstaticsecrets.secrets.hashicorp.com/orch-b2"}
	if !reflect.DeepEqual(removed, want) {
		t.Fatalf("removed %v want %v", removed, want)
	}
	raw, _ := os.ReadFile(logPath)
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.Contains(line, "delete") && (strings.Contains(line, "orch-w1") || strings.Contains(line, "orch-x1") || strings.Contains(line, "orch-vault-shared") || strings.Contains(line, other)) {
			t.Fatalf("another workload's object was deleted: %s", line)
		}
	}
	if _, err := s.RemoveWorkloadSynchronized(context.Background(), execution.Target{Kind: "kubernetes", Namespace: "app-ns", Kubeconfig: kubeconfig, Context: "ctx"}, "acc-r1", "staging", ""); err == nil {
		t.Fatal("an empty workload identity must be refused")
	}
}

func TestWorkloadCleanupHandlesMissingVSOTypesWithRedactedCredentials(t *testing.T) {
	for _, tc := range []struct {
		name, message string
		want          error
	}{
		{"operator absent", `error: the server doesn't have a resource type "vaultstaticsecrets"`, nil},
		{"forbidden", "Error from server (Forbidden): credential-sentinel", ErrForbidden},
		{"unauthorized", "Error from server (Unauthorized): credential-sentinel", ErrUnauthorized},
		{"unreachable", "Unable to connect to the server: connection refused credential-sentinel", ErrUnreachable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "kubectl")
			calls := filepath.Join(dir, "calls")
			script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> '" + calls + "'\ncat >&2 <<'MESSAGE'\n" + tc.message + "\nMESSAGE\nexit 1\n"
			if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
				t.Fatal(err)
			}
			sync := &VSOSynchronizer{KubectlPath: path, Credentials: telemetryCredentials{}}
			removed, err := sync.RemoveWorkloadSynchronized(context.Background(), execution.Target{Organization: "org", Connection: "account", Namespace: "app-staging", Context: "context"}, "app", "staging", "main")
			if tc.want == nil {
				if err != nil || len(removed) != 0 {
					t.Fatalf("missing operator is not an empty cleanup: %v", err)
				}
			} else if !errors.Is(err, tc.want) {
				t.Fatalf("failed read treated as absence: %v", err)
			}
			if err != nil && strings.Contains(err.Error(), "credential-sentinel") {
				t.Fatal("remote credential diagnostic escaped")
			}
			raw, _ := os.ReadFile(calls)
			if strings.Contains(string(raw), " delete ") {
				t.Fatal("cleanup deleted without an inventory")
			}
		})
	}
}
