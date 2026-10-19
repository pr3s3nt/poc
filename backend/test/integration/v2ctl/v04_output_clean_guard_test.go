package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestV04OutputCleanGuardsEveryDeletionIncludingEmptyInventory(t *testing.T) {
	const group = "0123456789abcdef"
	const protected = "11111111-1111-4111-8111-111111111111"
	const unknown = "22222222-2222-4222-8222-222222222222"
	ref := func(id string) string { return "secret://vault/secret/orchestrator/outputs/" + group + "/" + id }
	for _, tc := range []struct {
		name                string
		before, live, owned []string
		wantFail            bool
	}{
		{"protected overlap", []string{protected}, []string{protected}, []string{protected}, true},
		{"unknown new object", []string{protected}, []string{protected, unknown}, []string{}, true},
		{"lost protected object", []string{protected}, []string{unknown}, []string{unknown}, true},
		{"unaccounted with empty baseline and inventory", []string{}, []string{unknown}, []string{}, true},
		{"verified empty owned inventory preserves baseline", []string{protected}, []string{protected}, []string{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var deletes atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "DELETE" {
					deletes.Add(1)
					w.WriteHeader(204)
					return
				}
				if r.Method != "LIST" || r.Header.Get("X-Vault-Token") != "scoped-operator" {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					w.WriteHeader(403)
					return
				}
				var keys []string
				if strings.HasSuffix(r.URL.Path, "outputs/") {
					if len(tc.live) > 0 {
						keys = []string{group + "/"}
					} else {
						w.WriteHeader(404)
						return
					}
				} else if strings.HasSuffix(r.URL.Path, group+"/") {
					keys = tc.live
				} else {
					t.Errorf("unexpected metadata path %s", r.URL.Path)
					w.WriteHeader(400)
					return
				}
				json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"keys": keys}})
			}))
			defer server.Close()
			dir := t.TempDir()
			token, baseline, owned := filepath.Join(dir, "token"), filepath.Join(dir, "baseline.json"), filepath.Join(dir, "owned.json")
			if err := os.WriteFile(token, []byte("scoped-operator"), 0600); err != nil {
				t.Fatal(err)
			}
			refs := func(ids []string) []string {
				out := []string{}
				for _, id := range ids {
					out = append(out, ref(id))
				}
				return out
			}
			if err := writeJSON(baseline, v04OutputBaseline{Version: 1, RunID: "guard-run", Address: server.URL, Mount: "secret", Refs: refs(tc.before)}); err != nil {
				t.Fatal(err)
			}
			if err := writeJSON(owned, refs(tc.owned)); err != nil {
				t.Fatal(err)
			}
			code := v04OutputClean([]string{"-vault-addr", server.URL, "-mount", "secret", "-token-file", token, "-refs-file", owned, "-baseline-file", baseline, "-run-id", "guard-run"})
			if (code != 0) != tc.wantFail {
				t.Fatalf("code=%d wantFail=%v", code, tc.wantFail)
			}
			if deletes.Load() != 0 {
				t.Fatal("protected/unknown metadata guard allowed DELETE")
			}
		})
	}
}
