package vault

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"orchestrator/internal/ports/credentials"
)

// fakeKV is a minimal KV v2 server: create-only CAS writes, reads and
// metadata deletes, recording every request it receives.
type fakeKV struct {
	mu       sync.Mutex
	objects  map[string]map[string]string
	requests []string
	failPut  bool
}

func (f *fakeKV) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r.Method+" "+r.URL.Path)
	if r.Header.Get("X-Vault-Token") != "scoped-token" {
		http.Error(w, "permission denied: token scoped-token invalid", http.StatusForbidden)
		return
	}
	switch {
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v1/kv/data/"):
		var body struct {
			Options map[string]any    `json:"options"`
			Data    map[string]string `json:"data"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		path := strings.TrimPrefix(r.URL.Path, "/v1/kv/data/")
		if f.failPut {
			f.objects[path] = body.Data // applied, but the response fails
			http.Error(w, "internal error with body secrets", http.StatusInternalServerError)
			return
		}
		if body.Options["cas"] != float64(0) {
			http.Error(w, "cas required", http.StatusBadRequest)
			return
		}
		if _, exists := f.objects[path]; exists {
			http.Error(w, "check-and-set parameter did not match", http.StatusBadRequest)
			return
		}
		f.objects[path] = body.Data
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"version":1}}`))
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/kv/data/"):
		data, ok := f.objects[strings.TrimPrefix(r.URL.Path, "/v1/kv/data/")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"data": data}})
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/v1/kv/metadata/"):
		delete(f.objects, strings.TrimPrefix(r.URL.Path, "/v1/kv/metadata/"))
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func newFakeKV(t *testing.T) (*fakeKV, *ConnectionCredentials) {
	t.Helper()
	kv := &fakeKV{objects: map[string]map[string]string{}}
	server := httptest.NewServer(kv)
	t.Cleanup(server.Close)
	store, err := NewConnectionCredentials(server.URL, "scoped-token\n", "kv", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	return kv, store
}

func TestConnectionCredentials_ScopedCreateOnlyLifecycle(t *testing.T) {
	ctx := context.Background()
	kv, store := newFakeKV(t)
	ref, err := store.Put(ctx, "acme", "lab", []byte("kubeconfig-bytes"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(ref, "kv2://kv/orchestrator/connections/acme/lab/credentials/") {
		t.Fatalf("reference: %s", ref)
	}
	path := strings.TrimPrefix(ref, "kv2://kv/")
	if kv.objects[path]["value"] == "" || strings.Contains(kv.objects[path]["value"], "kubeconfig-bytes") {
		t.Fatalf("stored object is not encoded: %#v", kv.objects[path])
	}
	value, err := store.Get(ctx, "acme", "lab", ref)
	if err != nil || string(value) != "kubeconfig-bytes" {
		t.Fatalf("get: %v %q", err, value)
	}
	if err := store.Delete(ctx, "acme", "lab", ref); err != nil {
		t.Fatal(err)
	}
	if _, ok := kv.objects[path]; ok {
		t.Fatal("metadata delete left the object")
	}
	if _, err := store.Get(ctx, "acme", "lab", ref); !errors.Is(err, credentials.ErrNotFound) {
		t.Fatalf("deleted object: %v", err)
	}
	for _, req := range kv.requests {
		if strings.Contains(req, "/sys/") || strings.Contains(req, "/auth/") || strings.Contains(req, "orchestrator/apps") {
			t.Fatalf("credential store touched policy, auth or workload paths: %s", req)
		}
	}
}

func TestConnectionCredentials_RejectsOutOfScopeReferencesWithoutRequests(t *testing.T) {
	ctx := context.Background()
	kv, store := newFakeKV(t)
	ref, err := store.Put(ctx, "acme", "lab", []byte("x"))
	if err != nil {
		t.Fatal(err)
	}
	before := len(kv.requests)
	id := ref[strings.LastIndex(ref, "/")+1:]
	for _, bad := range []struct{ org, conn, ref string }{
		{"other", "lab", ref},
		{"acme", "prod", ref},
		{"acme", "lab", "kv2://kv/orchestrator/connections/acme/lab/credentials/../../prod/credentials/" + id},
		{"acme", "lab", "kv2://other/orchestrator/connections/acme/lab/credentials/" + id},
		{"acme", "lab", "https://evil.example/v1/kv/data/x"},
		{"acme", "lab", "kv2://kv/orchestrator/apps/acme/envs/staging/values/" + id},
		{"acme", "lab", "kv2://kv/orchestrator/connections/acme/lab/credentials/not-a-uuid"},
		{"ACME/..", "lab", ref},
	} {
		if _, err := store.Get(ctx, bad.org, bad.conn, bad.ref); !errors.Is(err, credentials.ErrInvalidReference) {
			t.Errorf("get %v: %v", bad, err)
		}
		if err := store.Delete(ctx, bad.org, bad.conn, bad.ref); !errors.Is(err, credentials.ErrInvalidReference) {
			t.Errorf("delete %v: %v", bad, err)
		}
	}
	if len(kv.requests) != before {
		t.Fatalf("out-of-scope references reached Vault: %v", kv.requests[before:])
	}
	if _, err := store.Put(ctx, "acme/../x", "lab", []byte("x")); !errors.Is(err, credentials.ErrInvalidReference) {
		t.Fatalf("unsafe scope accepted: %v", err)
	}
}

func TestConnectionCredentials_FailuresAreSafe(t *testing.T) {
	ctx := context.Background()
	kv, store := newFakeKV(t)
	kv.failPut = true
	ref, err := store.Put(ctx, "acme", "lab", []byte("secret-material"))
	if !errors.Is(err, credentials.ErrUnavailable) || ref == "" {
		t.Fatalf("failed put must return the attempt reference: %q %v", ref, err)
	}
	if strings.Contains(err.Error(), "body secrets") || strings.Contains(err.Error(), "scoped-token") {
		t.Fatalf("error leaks response or token: %v", err)
	}
	// Rollback of a write whose response was lost leaves nothing behind.
	if err := store.Delete(ctx, "acme", "lab", ref); err != nil || len(kv.objects) != 0 {
		t.Fatalf("rollback: %v %d", err, len(kv.objects))
	}

	unauthorized, err := NewConnectionCredentials(store.baseURL, "wrong", "kv", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := unauthorized.Put(ctx, "acme", "lab", []byte("x")); !errors.Is(err, credentials.ErrUnavailable) || strings.Contains(err.Error(), "wrong") {
		t.Fatalf("unauthorized: %v", err)
	}

	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://elsewhere.invalid/steal", http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	redirected, _ := NewConnectionCredentials(redirect.URL, "scoped-token", "kv", nil)
	if _, err := redirected.Put(ctx, "acme", "lab", []byte("x")); !errors.Is(err, credentials.ErrUnavailable) {
		t.Fatalf("redirect followed or accepted: %v", err)
	}

	for _, bad := range []struct{ addr, token, mount string }{{"ftp://vault", "t", "kv"}, {"http://user:pw@vault", "t", "kv"}, {"http://vault", " ", "kv"}, {"http://vault", "t", "kv/../sys"}} {
		if _, err := NewConnectionCredentials(bad.addr, bad.token, bad.mount, nil); err == nil {
			t.Errorf("accepted %v", bad)
		}
	}
	if !store.Durable() {
		t.Fatal("Vault store must report durable")
	}
}
