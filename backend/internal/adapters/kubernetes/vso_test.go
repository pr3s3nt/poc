package kubernetes

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

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
