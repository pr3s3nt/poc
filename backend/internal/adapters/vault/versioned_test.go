package vault

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// versionedKV is an in-memory KV v2 path store with versions and CAS.
type versionedKV struct {
	mu       sync.Mutex
	versions map[string][]map[string]string
	deleted  map[string]map[int]bool
	writes   int
	policies map[string]string
	roles    map[string]map[string]any
}

func newVersionedKV(t *testing.T) (*versionedKV, *Provider) {
	kv := &versionedKV{versions: map[string][]map[string]string{}, deleted: map[string]map[int]bool{}, policies: map[string]string{}, roles: map[string]map[string]any{}}
	server := httptest.NewServer(http.HandlerFunc(kv.handle))
	t.Cleanup(server.Close)
	p, err := New(server.URL, "token", "kv", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.SetAgentAddress("http://vault.vault:8200"); err != nil {
		t.Fatal(err)
	}
	return kv, p
}

func (kv *versionedKV) handle(w http.ResponseWriter, r *http.Request) {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	path := strings.TrimPrefix(r.URL.Path, "/v1/")
	switch {
	case strings.HasPrefix(path, "kv/data/"):
		key := strings.TrimPrefix(path, "kv/data/")
		list := kv.versions[key]
		if r.Method == http.MethodPost {
			var body struct {
				Options struct {
					CAS *int `json:"cas"`
				} `json:"options"`
				Data map[string]string `json:"data"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.Options.CAS != nil && *body.Options.CAS != len(list) {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			kv.versions[key] = append(list, body.Data)
			kv.writes++
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"version": len(list) + 1}})
			return
		}
		version := len(list)
		if raw := r.URL.Query().Get("version"); raw != "" {
			version, _ = strconv.Atoi(raw)
		}
		if version < 1 || version > len(list) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		meta := map[string]any{"version": version, "deletion_time": "", "destroyed": false}
		if kv.deleted[key][version] {
			meta["deletion_time"] = "2026-11-10T00:00:00Z"
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"data": nil, "metadata": meta}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"data": list[version-1], "metadata": meta}})
	case strings.HasPrefix(path, "kv/metadata/") && r.Method == http.MethodDelete:
		delete(kv.versions, strings.TrimPrefix(path, "kv/metadata/"))
		w.WriteHeader(http.StatusNoContent)
	case strings.HasPrefix(path, "sys/policies/acl/"):
		name := strings.TrimPrefix(path, "sys/policies/acl/")
		if r.Method == http.MethodDelete {
			delete(kv.policies, name)
		} else {
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			kv.policies[name] = body["policy"]
		}
		w.WriteHeader(http.StatusNoContent)
	case strings.HasPrefix(path, "auth/kubernetes/role/"):
		name := strings.TrimPrefix(path, "auth/kubernetes/role/")
		if r.Method == http.MethodDelete {
			delete(kv.roles, name)
		} else {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			kv.roles[name] = body
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func TestVersionedBundleReusesEqualVersionsAndWritesNewOnChange(t *testing.T) {
	kv, p := newVersionedKV(t)
	ctx := context.Background()
	write := func(value string) int {
		t.Helper()
		bundle, err := p.writeVersionedBundle(ctx, "shop", "staging", "backend", "app-shop-staging", map[string]map[string]string{"main": {"API_KEY": value}})
		if err != nil {
			t.Fatal(err)
		}
		if bundle.SecretName != "backend-config-v"+strconv.Itoa(bundle.Version) || bundle.Path != "orchestrator/apps/shop/envs/staging/values/workload-backend" {
			t.Fatalf("bundle identity = %+v", bundle)
		}
		if bundle.Keys["main"]["API_KEY"] != "main_API_KEY" || bundle.ServiceAccount == "" || bundle.Role == "" {
			t.Fatalf("bundle access = %+v", bundle)
		}
		return bundle.Version
	}
	if v := write("one"); v != 1 {
		t.Fatalf("first version = %d", v)
	}
	if v := write("one"); v != 1 || kv.writes != 1 {
		t.Fatalf("unchanged value wrote version %d (writes %d)", v, kv.writes)
	}
	if v := write("two"); v != 2 || kv.writes != 2 {
		t.Fatalf("changed value version %d (writes %d)", v, kv.writes)
	}
	// Rolling back to an earlier value reuses that version without a write.
	if v := write("one"); v != 1 || kv.writes != 2 {
		t.Fatalf("rollback value version %d (writes %d)", v, kv.writes)
	}
	// A deleted earlier version is not reused.
	kv.deleted["orchestrator/apps/shop/envs/staging/values/workload-backend"] = map[int]bool{1: true}
	if v := write("one"); v != 3 || kv.writes != 3 {
		t.Fatalf("deleted version reused: %d (writes %d)", v, kv.writes)
	}
	role := WorkloadRole("shop", "staging", "backend")
	if !strings.Contains(kv.policies[role], `"kv/data/orchestrator/apps/shop/envs/staging/values/workload-backend"`) || strings.Count(kv.policies[role], "path ") != 1 {
		t.Fatalf("policy = %q", kv.policies[role])
	}
	if kv.roles[role]["bound_service_account_namespaces"] != "app-shop-staging" {
		t.Fatalf("role = %+v", kv.roles[role])
	}
	if err := p.DeleteWorkload(ctx, "shop", "staging", "backend"); err != nil {
		t.Fatal(err)
	}
	if len(kv.versions) != 0 || len(kv.policies) != 0 || len(kv.roles) != 0 {
		t.Fatalf("delete left %v %v %v", kv.versions, kv.policies, kv.roles)
	}
}

func TestVersionedBundleRejectsUnsafeScope(t *testing.T) {
	_, p := newVersionedKV(t)
	if _, err := p.writeVersionedBundle(context.Background(), "shop", "dev", "backend", "ns", map[string]map[string]string{"main": {"A": "b"}}); err == nil {
		t.Fatal("unknown Environment accepted")
	}
	if _, err := p.writeVersionedBundle(context.Background(), "shop", "staging", "back/end", "ns", map[string]map[string]string{"main": {"A": "b"}}); err == nil {
		t.Fatal("unsafe workload accepted")
	}
}
