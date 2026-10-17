package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"

	"orchestrator/internal/bootstrap"
	"orchestrator/internal/bootstrap/bootstraptest"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/seed/seedtest"
)

// The product renderer policy over public HTTP (UC-05 BR-09, UC-06 BR-17):
// no score-k8s Definition means Preview is a 400 PLANNING_REJECTED with the
// renderer reason, before any resource; a wrong class has no match either;
// with the Definition a pure Preview shows the safe resource projection.
func TestRequiredRendererOverHTTPGuardsMissingTemplateAndWrongClass(t *testing.T) {
	path, err := exec.LookPath("score-k8s")
	if err != nil {
		t.Skip("local score-k8s 0.15.0 unavailable")
	}
	app, err := bootstraptest.Build(context.Background(), bootstrap.Options{Seed: seedtest.Defaults(), ScoreK8sPath: path, RequireScoreRenderer: true})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(app.Server)
	defer server.Close()
	pe := authenticatedClientAs(t, server.URL, "platform-engineer")
	dev := authenticatedClient(t, server.URL)
	// V02 has no profile guard: this new Application needs its own matching PostgreSQL Definition.
	if err := app.Store.SaveResourceDefinition(context.Background(), seedtest.Defaults().OrganizationKey, resource.Definition{Key: "postgres-required-app", Name: "PostgreSQL cho ứng dụng thử", Version: 1, ResourceTypeKey: "postgres", DriverType: resource.DriverKubernetes,
		DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{"image": "postgres:17-alpine", "storage": "1Gi", "namespace": namespaceOutput}}}, Criteria: []resource.Criterion{{}}}); err != nil {
		t.Fatal(err)
	}
	created := createConfiguredApplication(t, dev, server.URL, "Required App", "required-app", "internal-cluster")
	appKey := created["application"].(map[string]any)["key"].(string)
	base := server.URL + "/api/v1/applications/" + appKey + "/environments/staging"
	score := previewScore("api")
	delete(score["containers"].(map[string]any)["main"].(map[string]any), "variables")
	score["resources"] = map[string]any{"db": map[string]any{"type": "postgres", "class": "default", "params": map[string]any{"database": "appdb", "username": "appuser"}}}
	if status, out := requestJSON(t, dev, http.MethodPut, base+"/workloads/api", map[string]any{"version": 0, "score": score}); status != http.StatusOK {
		t.Fatal(status, out)
	}
	assertRejected := func(label, reason string) {
		t.Helper()
		status, out := requestJSON(t, dev, http.MethodPost, base+"/preview", map[string]any{})
		params, _ := out["params"].(map[string]any)
		if status != http.StatusBadRequest || out["code"] != "PLANNING_REJECTED" || (reason != "" && params["reason"] != reason) {
			t.Fatalf("%s: %d %v", label, status, out)
		}
		raw, _ := json.Marshal(out)
		for _, leak := range []string{"appuser", "score-k8s", "/tmp", "definition \""} {
			if strings.Contains(string(raw), leak) {
				t.Fatalf("%s: error leaks %q: %s", label, leak, raw)
			}
		}
		if len(app.FakeExec.Calls) != 0 || len(app.FakeDeploy.Applied) != 0 {
			t.Fatalf("%s: side effects before the guard", label)
		}
	}
	assertRejected("no rendering Definition", "renderer-no-match")

	body := `{"key":"score-workloads","name":"Mẫu dựng","resourceType":"workload","driverType":"score-k8s","driverInputs":{"values":{"variables":{"render_bundle":"score-k8s-internal-v1"}}},"criteria":[{}]}`
	if resp, out := call(t, pe, http.MethodPost, server.URL+"/api/v1/resource-definitions", body); resp.StatusCode != http.StatusCreated {
		t.Fatalf("registration: %d %s", resp.StatusCode, out)
	}
	// A wrong class matches no Definition: the draft is rejected, never switched.
	wrong := previewScore("api")
	delete(wrong["containers"].(map[string]any)["main"].(map[string]any), "variables")
	// The seeded PostgreSQL Definition is a wildcard (it matches any class), so the
	// guard is shown on a Type no Definition serves: matching stays server-authoritative.
	if resp, out := call(t, pe, http.MethodPost, server.URL+"/api/v1/resource-types", `{"key":"cache","inputs":[],"outputs":[{"name":"host","type":"string","required":true}]}`); resp.StatusCode != http.StatusCreated {
		t.Fatalf("type registration: %d %s", resp.StatusCode, out)
	}
	wrong["resources"] = map[string]any{"cache": map[string]any{"type": "cache", "class": "cloud-managed"}}
	if status, out := requestJSON(t, dev, http.MethodPut, base+"/workloads/api", map[string]any{"version": 1, "score": wrong}); status != http.StatusOK {
		t.Fatal(status, out)
	}
	status, out := requestJSON(t, dev, http.MethodPost, base+"/preview", map[string]any{})
	if params, _ := out["params"].(map[string]any); status != http.StatusBadRequest || out["code"] != "PLANNING_REJECTED" || params["stage"] != "graph" {
		t.Fatalf("wrong class must have no match (graph stage): %d %v", status, out)
	}
	if len(app.FakeExec.Calls) != 0 || len(app.FakeDeploy.Applied) != 0 {
		t.Fatal("wrong class had side effects")
	}
	if status, out := requestJSON(t, dev, http.MethodPut, base+"/workloads/api", map[string]any{"version": 2, "score": score}); status != http.StatusOK {
		t.Fatal(status, out)
	}
	tie := strings.Replace(body, "score-workloads", "score-workloads-2", 1)
	if resp, out := call(t, pe, http.MethodPost, server.URL+"/api/v1/resource-definitions", tie); resp.StatusCode != http.StatusCreated {
		t.Fatalf("second registration: %d %s", resp.StatusCode, out)
	}
	assertRejected("tied rendering Definitions", "renderer-ambiguous")

}
