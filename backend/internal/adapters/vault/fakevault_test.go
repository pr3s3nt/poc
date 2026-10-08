package vault

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeVault is a minimal in-memory Vault KV v2 server for adapter tests.
type fakeVault struct {
	t      *testing.T
	server *httptest.Server
	token  string
	mount  string
	auth   string

	mu           sync.Mutex
	values       map[string]string // data path -> JSON of data
	policies     map[string]string
	roles        map[string]map[string]any
	capabilities map[string][]string // override per capability path suffix; nil = all
	noDelete     bool
	failWrite    bool
	notKV2       bool
	authMissing  bool
	writes       int
	requests     []string
}

func newFakeVault(t *testing.T, token string) *fakeVault {
	f := &fakeVault{t: t, token: token, mount: "kv", auth: "kubernetes", values: map[string]string{}, policies: map[string]string{}, roles: map[string]map[string]any{}}
	f.server = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeVault) handle(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r.Method+" "+r.URL.Path)
	if r.Header.Get("X-Vault-Token") != f.token {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/v1/")
	switch {
	case path == "auth/token/lookup-self":
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{}}`))
	case strings.HasPrefix(path, "sys/internal/ui/mounts/"):
		mount := strings.TrimPrefix(path, "sys/internal/ui/mounts/")
		if mount != f.mount {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		version := "2"
		if f.notKV2 {
			version = "1"
		}
		_, _ = w.Write([]byte(`{"data":{"type":"kv","options":{"version":"` + version + `"}}}`))
	case path == "sys/capabilities-self":
		var body struct {
			Paths []string `json:"paths"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		out := map[string]any{}
		for _, p := range body.Paths {
			granted := []string{"create", "read", "update", "delete", "list"}
			for suffix, caps := range f.capabilities {
				if strings.Contains(p, suffix) {
					granted = caps
				}
			}
			out[p] = granted
		}
		_ = json.NewEncoder(w).Encode(out)
	case strings.HasPrefix(path, f.mount+"/data/"):
		key := strings.TrimPrefix(path, f.mount+"/data/")
		switch r.Method {
		case http.MethodPost:
			if f.failWrite {
				// The write is stored but the response reports failure.
				f.values[key] = `{"value":"x"}`
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			var body struct {
				Options map[string]any    `json:"options"`
				Data    map[string]string `json:"data"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if _, exists := f.values[key]; exists && body.Options["cas"] == float64(0) {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			raw, _ := json.Marshal(body.Data)
			f.values[key] = string(raw)
			f.writes++
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":{"version":1}}`))
		case http.MethodGet:
			raw, ok := f.values[key]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write([]byte(`{"data":{"data":` + raw + `}}`))
		}
	case strings.HasPrefix(path, f.mount+"/metadata/") && r.Method == http.MethodDelete:
		if f.noDelete {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		delete(f.values, strings.TrimPrefix(path, f.mount+"/metadata/"))
		w.WriteHeader(http.StatusNoContent)
	case strings.HasPrefix(path, "sys/policies/acl/") && r.Method == http.MethodPost:
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.policies[strings.TrimPrefix(path, "sys/policies/acl/")] = body["policy"]
		w.WriteHeader(http.StatusNoContent)
	case strings.HasPrefix(path, "auth/"+f.auth+"/role/") && r.Method == http.MethodPost:
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.roles[strings.TrimPrefix(path, "auth/"+f.auth+"/role/")] = body
		w.WriteHeader(http.StatusNoContent)
	case path == "auth/"+f.auth+"/config" && r.Method == http.MethodGet:
		if f.authMissing {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"data":{"kubernetes_host":"https://k8s"}}`))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (f *fakeVault) valueCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.values)
}
