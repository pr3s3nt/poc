package v03run

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	idA = "11111111-1111-4111-8111-111111111111"
	idB = "22222222-2222-4222-8222-222222222222"
)

// fakeVault serves the KV2 metadata/data endpoints of one organization.
type fakeVault struct {
	objects  map[string]map[string]bool // key -> ids
	listCode int                        // forced LIST status (0 = normal)
	listBody string
	deleteOK bool
	token    string
	outputs  map[string]bool
}

func (f *fakeVault) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Vault-Token") != f.token {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/secret/"), "/")
		if len(parts) == 5 && parts[0] == "metadata" && parts[1] == "orchestrator" && parts[2] == "outputs" {
			path := strings.Join(parts[1:], "/")
			switch r.Method {
			case http.MethodDelete:
				if f.deleteOK {
					delete(f.outputs, path)
				}
				w.WriteHeader(http.StatusNoContent)
			case http.MethodGet:
				if f.outputs[path] {
					_, _ = w.Write([]byte(`{"data":{}}`))
					return
				}
				w.WriteHeader(http.StatusNotFound)
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
			return
		}
		// metadata|data / orchestrator / connections / org / key / credentials [/ id]
		if len(parts) < 6 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		kind, key := parts[0], parts[4]
		switch {
		case r.Method == "LIST" && kind == "metadata" && len(parts) == 6:
			if f.listCode != 0 {
				w.WriteHeader(f.listCode)
				_, _ = w.Write([]byte(f.listBody))
				return
			}
			ids := f.objects[key]
			if len(ids) == 0 {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			var list []string
			for id := range ids {
				list = append(list, `"`+id+`"`)
			}
			_, _ = w.Write([]byte(`{"data":{"keys":[` + strings.Join(list, ",") + `]}}`))
		case r.Method == http.MethodDelete && kind == "metadata" && len(parts) == 7:
			if f.deleteOK {
				delete(f.objects[key], parts[6])
			}
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && kind == "data" && len(parts) == 7:
			if f.objects[key][parts[6]] {
				_, _ = w.Write([]byte(`{"data":{}}`))
				return
			}
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

func TestVaultCleanOutputRefsUsesOnlyExactRunRecordedPaths(t *testing.T) {
	refA := "secret://vault/secret/orchestrator/outputs/0123456789abcdef/" + idA
	refB := "secret://vault/secret/orchestrator/outputs/fedcba9876543210/" + idB
	pathA := "orchestrator/outputs/0123456789abcdef/" + idA
	pathB := "orchestrator/outputs/fedcba9876543210/" + idB
	f := &fakeVault{token: "t", deleteOK: true, outputs: map[string]bool{pathA: true, pathB: true, "orchestrator/outputs/foreign/" + idA: true}}
	result := newVault(t, f).CleanOutputRefs(context.Background(), []string{refA, refB, refA})
	if len(result.Failed) != 0 || len(result.Deleted) != 2 || f.outputs[pathA] || f.outputs[pathB] || !f.outputs["orchestrator/outputs/foreign/"+idA] {
		t.Fatalf("result %+v, remaining %+v", result, f.outputs)
	}
}

func TestVaultCleanOutputRefsFailsClosedForMalformedForeignAndUnprovenObjects(t *testing.T) {
	valid := "secret://vault/secret/orchestrator/outputs/0123456789abcdef/" + idA
	for name, f := range map[string]*fakeVault{
		"delete leaves object": {token: "t", deleteOK: false, outputs: map[string]bool{"orchestrator/outputs/0123456789abcdef/" + idA: true}},
		"forbidden token":      {token: "other", deleteOK: true, outputs: map[string]bool{"orchestrator/outputs/0123456789abcdef/" + idA: true}},
	} {
		v := newVault(t, f)
		if name == "forbidden token" {
			v.Token = "t"
		}
		if got := v.CleanOutputRefs(context.Background(), []string{valid}); len(got.Failed) == 0 {
			t.Fatalf("%s: %+v", name, got)
		}
	}
	for _, ref := range []string{"secret://vault/other/orchestrator/outputs/0123456789abcdef/" + idA, "secret://vault/secret/orchestrator/outputs/../" + idA, "secret://vault/secret/orchestrator/connections/acme/a"} {
		if got := newVault(t, &fakeVault{token: "t"}).CleanOutputRefs(context.Background(), []string{ref}); len(got.Failed) == 0 {
			t.Fatalf("unsafe ref accepted: %q", ref)
		}
	}
}

func newVault(t *testing.T, f *fakeVault) Vault {
	server := httptest.NewServer(f.handler(t))
	t.Cleanup(server.Close)
	return Vault{Address: server.URL, Mount: "secret", Token: f.token, Client: server.Client()}
}

func TestVaultCleanDeletesExactKeysAndProvesAbsence(t *testing.T) {
	f := &fakeVault{token: "t", deleteOK: true, objects: map[string]map[string]bool{"lab-1": {idA: true, idB: true}, "other": {idA: true}}}
	result := newVault(t, f).CleanAccounts(context.Background(), "acme", []string{"lab-1"})
	if len(result.Failed) != 0 || len(result.Deleted) != 2 {
		t.Fatalf("result = %+v", result)
	}
	if !f.objects["other"][idA] {
		t.Fatal("an object of another account key was removed")
	}
}

func TestVaultCleanNeverTreatsAnAPIFailureAsAbsence(t *testing.T) {
	for name, f := range map[string]*fakeVault{
		"server error":   {token: "t", listCode: 500},
		"forbidden":      {token: "t", listCode: 403},
		"malformed body": {token: "t", listCode: 200, listBody: `not json`},
		"no keys field":  {token: "t", listCode: 200, listBody: `{"data":{}}`},
		"bad entry":      {token: "t", listCode: 200, listBody: `{"data":{"keys":["../x"]}}`},
		"wrong token":    {token: "other"},
	} {
		v := newVault(t, f)
		if name == "wrong token" {
			v.Token = "t"
		}
		result := v.CleanAccounts(context.Background(), "acme", []string{"lab-1"})
		if len(result.Failed) == 0 {
			t.Fatalf("%s: a failed LIST must be a cleanup failure, got %+v", name, result)
		}
	}
	// A delete that the server acknowledges but that leaves the object behind is not proof.
	f := &fakeVault{token: "t", deleteOK: false, objects: map[string]map[string]bool{"lab-1": {idA: true}}}
	if result := newVault(t, f).CleanAccounts(context.Background(), "acme", []string{"lab-1"}); len(result.Failed) == 0 {
		t.Fatalf("an object that is still readable must fail: %+v", result)
	}
	// An unreachable server is a failure too.
	dead := Vault{Address: "http://127.0.0.1:1", Mount: "secret", Token: "t"}
	if result := dead.CleanAccounts(context.Background(), "acme", []string{"lab-1"}); len(result.Failed) == 0 {
		t.Fatalf("an unreachable Vault must fail: %+v", result)
	}
	// Unsafe path segments are refused before any request.
	if _, err := newVault(t, &fakeVault{token: "t"}).ListObjects(context.Background(), "acme", "../x"); err == nil {
		t.Fatal("a path traversal key must be refused")
	}
}

func TestVaultKeysAreTheProductReceiptsPlusTheScenarioRecord(t *testing.T) {
	refs := []ConnectionRef{
		{Organization: "acme", Key: "lab-1", SecretRef: "kv2://secret/orchestrator/connections/acme/lab-1/credentials/" + idA},
		{Organization: "acme", Key: "odd", SecretRef: "memory://x"},
		{Organization: "other", Key: "lab-9", SecretRef: "kv2://secret/orchestrator/connections/other/lab-9/credentials/" + idB},
	}
	keys := VaultKeys("acme", refs, []string{"lab-2", "Bad Key", "lab-1"})
	if strings.Join(keys, ",") != "lab-1,lab-2" {
		t.Fatalf("keys = %v", keys)
	}
}
