package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// T25: the removed service.publicPort is rejected over HTTP with a stable,
// safe INVALID_SCORE and never changes drafts, versions or the current set.
func TestHTTPRejectsRemovedPublicPortWithoutStateChange(t *testing.T) {
	server, _, app := newServerApp(t, "")
	client := authenticatedClient(t, server.URL)
	created := createConfiguredApplication(t, client, server.URL, "Port Removed", "port-removed", "internal-cluster")
	key := created["application"].(map[string]any)["key"].(string)
	base := server.URL + "/api/v1/applications/" + key + "/environments/staging/workloads"
	const canary = "T25-CANARY-public-port"
	score := func(publicPort any, withRoutes bool) map[string]any {
		service := map[string]any{"ports": map[string]any{"http": map[string]any{"port": 8080}}, "publicPort": publicPort}
		if withRoutes {
			service["publicRoutes"] = []any{map[string]any{"path": "/api", "port": "http"}}
		}
		return map[string]any{"apiVersion": "score.dev/v1b1", "metadata": map[string]any{"name": "api"}, "containers": map[string]any{"main": map[string]any{"image": "example.invalid/api:t25"}}, "service": service}
	}
	envBefore, err := app.Store.GetEnvironment(context.Background(), key, "staging")
	if err != nil {
		t.Fatal(err)
	}
	status, before := requestJSON(t, client, http.MethodGet, base, nil)
	if status != http.StatusOK {
		t.Fatalf("list: %d %v", status, before)
	}
	for name, publicPort := range map[string]any{"value": canary, "empty": "", "null": nil} {
		for _, withRoutes := range []bool{false, true} {
			doc := score(publicPort, withRoutes)
			raw, _ := json.Marshal(doc)
			for label, call := range map[string]func() (int, map[string]any){
				"save": func() (int, map[string]any) {
					return requestJSON(t, client, http.MethodPut, base+"/api", map[string]any{"score": doc, "version": 0})
				},
				"parse": func() (int, map[string]any) {
					return requestJSON(t, client, http.MethodPost, base+"/parse", map[string]any{"content": string(raw)})
				},
			} {
				code, body := call()
				if code != http.StatusBadRequest || body["code"] != "INVALID_SCORE" || body["field"] != "service.publicPort" || strings.Contains(toJSON(t, body), canary) || strings.Contains(toJSON(t, body), "environment:") {
					t.Fatalf("%s %s routes=%v: want safe 400 INVALID_SCORE service.publicPort, got %d %v", label, name, withRoutes, code, body)
				}
			}
		}
	}
	status, after := requestJSON(t, client, http.MethodGet, base, nil)
	if status != http.StatusOK || toJSON(t, after) != toJSON(t, before) {
		t.Fatalf("draft state changed: %d %v want %v", status, after, before)
	}
	envAfter, err := app.Store.GetEnvironment(context.Background(), key, "staging")
	if err != nil || envAfter.DraftVersion != envBefore.DraftVersion || envAfter.CurrentDeploymentSetID != envBefore.CurrentDeploymentSetID {
		t.Fatalf("environment changed: %+v -> %+v (%v)", envBefore, envAfter, err)
	}
}
