package e2e

import (
	"net/http"
	"strings"
	"testing"
)

const policySentinel = "sentinel-submitted-value-4d1e"

// TestUC02UC03RegistrationPolicyHTTP covers UC-02 BR-05/BR-06 and UC-03
// BR-10–BR-14 over HTTP: invalid documents are 400 with a safe message and
// leave the catalog unchanged.
func TestUC02UC03RegistrationPolicyHTTP(t *testing.T) {
	server, _ := newServer(t, "", approvedCluster{})
	pe := authenticatedClientAs(t, server.URL, "platform-engineer")
	count := func(path, field string) int {
		status, out := requestJSON(t, pe, http.MethodGet, server.URL+path, nil)
		if status != http.StatusOK {
			t.Fatalf("list %s: %d %v", path, status, out)
		}
		return len(out[field].([]any))
	}
	typesBefore := count("/api/v1/resource-types", "resourceTypes")
	defsBefore := count("/api/v1/resource-definitions", "resourceDefinitions")
	outputs := []any{map[string]any{"name": "url", "type": "string"}}
	for _, key := range []string{"Queue", "queue_v1", " queue", "queue.v1", "", "environment", "service"} {
		status, out := requestJSON(t, pe, http.MethodPost, server.URL+"/api/v1/resource-types", map[string]any{"key": key, "inputs": []any{}, "outputs": outputs})
		if status != http.StatusBadRequest {
			t.Errorf("type key %q: %d %v", key, status, out)
		}
	}
	if status, out := requestJSON(t, pe, http.MethodPost, server.URL+"/api/v1/resource-types", map[string]any{"key": "queue-1", "inputs": []any{}, "outputs": outputs}); status != http.StatusCreated {
		t.Fatalf("valid type key: %d %v", status, out)
	}

	namespace := "${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}"
	definition := func(key string, variables map[string]any) map[string]any {
		return map[string]any{
			"key": key, "resourceType": "postgres", "executionProfile": "internal-k8s", "driverType": "kubernetes",
			"driverInputs": map[string]any{"values": map[string]any{"variables": variables}},
			"criteria":     []any{map[string]any{"class": "policy"}},
		}
	}
	invalid := map[string]map[string]any{
		"uppercase key":        definition("Postgres-Policy", map[string]any{"namespace": namespace}),
		"unsupported variable": definition("pg-policy", map[string]any{"password": policySentinel}),
		"wrong type":           definition("pg-policy", map[string]any{"storage": 20}),
		"null variable":        definition("pg-policy", map[string]any{"image": nil}),
		"malformed":            definition("pg-policy", map[string]any{"image": "${context." + policySentinel}),
		"nested placeholder":   definition("pg-policy", map[string]any{"image": "${context.${" + policySentinel + "}}"}),
		"nested escaped token": definition("pg-policy", map[string]any{"image": "${context.$${" + policySentinel + "}}"}),
		"secret refs": {
			"key": "pg-policy", "resourceType": "postgres", "executionProfile": "internal-k8s", "driverType": "kubernetes",
			"driverInputs": map[string]any{"values": map[string]any{"variables": map[string]any{}}, "secret_refs": map[string]any{"password": policySentinel}},
			"criteria":     []any{map[string]any{"class": "policy"}},
		},
	}
	for name, body := range invalid {
		status, out := requestJSON(t, pe, http.MethodPost, server.URL+"/api/v1/resource-definitions", body)
		if status != http.StatusBadRequest || strings.Contains(toJSON(t, out), policySentinel) {
			t.Errorf("%s: %d %v", name, status, out)
		}
	}
	if status, out := requestJSON(t, pe, http.MethodPost, server.URL+"/api/v1/resource-definitions", definition("pg-policy", map[string]any{"namespace": namespace, "storage": "2Gi"})); status != http.StatusCreated {
		t.Fatalf("valid definition: %d %v", status, out)
	}
	if got := count("/api/v1/resource-types", "resourceTypes"); got != typesBefore+1 {
		t.Fatalf("types %d -> %d, want one new", typesBefore, got)
	}
	if got := count("/api/v1/resource-definitions", "resourceDefinitions"); got != defsBefore+1 {
		t.Fatalf("definitions %d -> %d, want one new", defsBefore, got)
	}
}

// TestUC16ResourceParamsRejectedOnSaveAndImportHTTP covers UC-16 BR-14 over
// HTTP: an unbound resource with invalid params fails both import and save
// with the same safe path, and the draft version does not advance.
func TestUC16ResourceParamsRejectedOnSaveAndImportHTTP(t *testing.T) {
	server, _ := newServer(t, "")
	client := authenticatedClient(t, server.URL)
	status, created := requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/applications", map[string]any{"name": "Params Test", "subdomain": "params-test"})
	if status != http.StatusCreated {
		t.Fatalf("create Application: %d %v", status, created)
	}
	base := server.URL + "/api/v1/applications/" + created["application"].(map[string]any)["key"].(string) + "/environments/staging"
	scoreWith := func(params map[string]any) map[string]any {
		return map[string]any{
			"apiVersion": "score.dev/v1b1", "metadata": map[string]any{"name": "api"},
			"containers": map[string]any{"main": map[string]any{"image": "example.invalid/api:v1"}},
			"resources":  map[string]any{"db": map[string]any{"type": "postgres", "params": params}},
		}
	}
	for name, tc := range map[string]struct {
		params map[string]any
		path   string
	}{
		"missing required": {map[string]any{"database": "shop"}, "resources.db.params.username"},
		"wrong type":       {map[string]any{"database": "shop", "username": 7}, "resources.db.params.username"},
		"undeclared":       {map[string]any{"database": "shop", "username": "app", "storage": policySentinel}, "resources.db.params.storage"},
		"null":             {map[string]any{"database": nil, "username": "app"}, "resources.db.params.database"},
	} {
		score := scoreWith(tc.params)
		importStatus, imported := requestJSON(t, client, http.MethodPost, base+"/workloads/parse", map[string]any{"content": toJSON(t, score)})
		saveStatus, saved := requestJSON(t, client, http.MethodPut, base+"/workloads/api", map[string]any{"score": score, "version": 0})
		for label, out := range map[string]map[string]any{"import": imported, "save": saved} {
			msg := toJSON(t, out)
			if !strings.Contains(msg, tc.path) || strings.Contains(msg, policySentinel) {
				t.Errorf("%s %s: want safe path %s, got %v", name, label, tc.path, out)
			}
		}
		if importStatus != http.StatusBadRequest || saveStatus != http.StatusBadRequest || imported["error"] != saved["error"] {
			t.Errorf("%s: import %d %v, save %d %v", name, importStatus, imported, saveStatus, saved)
		}
	}
	status, view := requestJSON(t, client, http.MethodGet, base+"/workloads", nil)
	if status != http.StatusOK || view["draftVersion"] != float64(0) || len(view["workloads"].([]any)) != 0 {
		t.Fatalf("rejected saves mutated drafts: %d %v", status, view)
	}
	status, saved := requestJSON(t, client, http.MethodPut, base+"/workloads/api", map[string]any{"score": scoreWith(map[string]any{"database": "shop", "username": "app"}), "version": 0})
	if status != http.StatusOK || saved["draftVersion"] != float64(1) {
		t.Fatalf("valid save: %d %v", status, saved)
	}
}
