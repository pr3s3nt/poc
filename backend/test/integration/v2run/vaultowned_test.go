package v2run_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"orchestrator/test/integration/v2run"
)

// fakeVault is a minimal KV v2 + ACL policy + Kubernetes auth role API.
type fakeVault struct {
	mu       sync.Mutex
	values   map[string]bool
	policies map[string]string
	roles    map[string]map[string]any
	deleted  []string
}

func (f *fakeVault) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("X-Vault-Token") != "scoped-token-value" {
		http.Error(w, "denied", http.StatusForbidden)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/v1/")
	reply := func(v any) { _ = json.NewEncoder(w).Encode(v) }
	keysOf := func(prefix string, names []string) []string {
		var out []string
		seen := map[string]bool{}
		for _, n := range names {
			if rest, ok := strings.CutPrefix(n, prefix); ok {
				k := rest
				if i := strings.Index(rest, "/"); i >= 0 {
					k = rest[:i+1]
				}
				if !seen[k] {
					seen[k] = true
					out = append(out, k)
				}
			}
		}
		return out
	}
	if r.Method == http.MethodDelete {
		f.deleted = append(f.deleted, path)
		delete(f.values, strings.TrimPrefix(path, "kv/metadata/"))
		delete(f.policies, strings.TrimPrefix(path, "sys/policies/acl/"))
		delete(f.roles, strings.TrimPrefix(path, "auth/kubernetes/role/"))
		w.WriteHeader(http.StatusNoContent)
		return
	}
	switch {
	case path == "sys/policies/acl":
		var names []string
		for n := range f.policies {
			names = append(names, n)
		}
		reply(map[string]any{"data": map[string]any{"keys": names}})
	case strings.HasPrefix(path, "sys/policies/acl/"):
		p, ok := f.policies[strings.TrimPrefix(path, "sys/policies/acl/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		reply(map[string]any{"data": map[string]any{"policy": p}})
	case path == "auth/kubernetes/role":
		var names []string
		for n := range f.roles {
			names = append(names, n)
		}
		reply(map[string]any{"data": map[string]any{"keys": names}})
	case strings.HasPrefix(path, "auth/kubernetes/role/"):
		role, ok := f.roles[strings.TrimPrefix(path, "auth/kubernetes/role/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		reply(map[string]any{"data": role})
	case strings.HasPrefix(path, "kv/metadata/"):
		var names []string
		for n := range f.values {
			names = append(names, n)
		}
		keys := keysOf(strings.TrimPrefix(path, "kv/metadata/"), names)
		if len(keys) == 0 {
			http.NotFound(w, r)
			return
		}
		reply(map[string]any{"data": map[string]any{"keys": keys}})
	default:
		http.NotFound(w, r)
	}
}

func TestVaultOwnedObjectsAreProvenByContentAndForeignOnesAreNeverDeleted(t *testing.T) {
	f := &fakeVault{
		values:   map[string]bool{"orchestrator/apps/acceptance/envs/staging/values/old": true},
		policies: map[string]string{"orch-before": `path "kv/data/orchestrator/apps/acc-r1/x" { capabilities = ["read"] }`},
		roles:    map[string]map[string]any{"orch-before": {"bound_service_account_namespaces": []string{"orch-v2-test-r1"}, "token_policies": []string{"orch-before"}}},
	}
	srv := httptest.NewServer(f)
	defer srv.Close()
	v := v2run.VaultAccess{Address: srv.URL, Token: "scoped-token-value", Mount: "kv", AuthMount: "kubernetes", Client: srv.Client()}
	ctx := context.Background()
	snap, err := v2run.TakeVaultSnapshot(ctx, v)
	if err != nil {
		t.Fatal(err)
	}
	// The run creates its objects; another actor creates foreign ones meanwhile.
	f.values["orchestrator/apps/acc-r1/envs/staging/values/d1"] = true
	f.policies["orch-own"] = `path "kv/data/orchestrator/apps/acc-r1/envs/staging/values/d1" { capabilities = ["read"] }`
	f.roles["orch-own"] = map[string]any{"bound_service_account_namespaces": []string{"orch-v2-test-r1"}, "token_policies": []string{"orch-own"}}
	f.policies["orch-mixed"] = `path "kv/data/orchestrator/apps/acc-r1/a" { capabilities = ["read"] }` + "\n" + `path "kv/data/orchestrator/apps/other/b" { capabilities = ["read"] }`
	f.policies["orch-other"] = `path "kv/data/orchestrator/apps/other/b" { capabilities = ["read"] }`
	f.roles["orch-otherns"] = map[string]any{"bound_service_account_namespaces": []string{"someone-else"}, "token_policies": []string{"orch-own"}}
	owned, err := v2run.FindVaultOwned(ctx, v, snap, "acc-r1", "orch-v2-test-r1")
	if err != nil {
		t.Fatal(err)
	}
	if len(owned.Values) != 1 || len(owned.Policies) != 1 || owned.Policies[0] != "orch-own" || len(owned.Roles) != 1 || owned.Roles[0] != "orch-own" || len(owned.Foreign) != 3 {
		t.Fatalf("ownership by content: %+v", owned)
	}
	if _, err := v2run.CleanVaultOwned(ctx, v, owned); err != nil {
		t.Fatal(err)
	}
	for _, d := range f.deleted {
		if strings.Contains(d, "before") || strings.Contains(d, "other") || strings.Contains(d, "mixed") || strings.Contains(d, "apps/acceptance") {
			t.Fatalf("a pre-existing or foreign object was deleted: %s", d)
		}
	}
	after, err := v2run.FindVaultOwned(ctx, v, snap, "acc-r1", "orch-v2-test-r1")
	if err != nil || !after.Empty() {
		t.Fatalf("owned objects must be gone: %+v %v", after, err)
	}
	if !f.values["orchestrator/apps/acceptance/envs/staging/values/old"] {
		t.Fatal("the existing acceptance prefix must be untouched")
	}
}

func TestVaultFailureNeverEchoesTheToken(t *testing.T) {
	v := v2run.VaultAccess{Address: "http://127.0.0.1:1", Token: "scoped-token-value", Mount: "kv", AuthMount: "kubernetes", Client: &http.Client{}}
	_, err := v2run.TakeVaultSnapshot(context.Background(), v)
	if err == nil || strings.Contains(err.Error(), "scoped-token-value") || strings.Contains(err.Error(), "127.0.0.1") {
		t.Fatalf("transport errors must be fixed text: %v", err)
	}
}
