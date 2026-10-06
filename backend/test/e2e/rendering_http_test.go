package e2e

import (
	"context"
	"net/http"
	"net/http/httptest"
	"orchestrator/internal/bootstrap"
	"orchestrator/internal/seed"
	"os/exec"
	"testing"
)

func TestScoreK8sPublicDefinitionPreviewDeploy(t *testing.T) {
	path, err := exec.LookPath("score-k8s")
	if err != nil {
		t.Skip("local score-k8s unavailable")
	}
	app, err := bootstrap.Build(context.Background(), bootstrap.Options{Seed: seed.Defaults(), ScoreK8sPath: path})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(app.Server)
	defer server.Close()
	pe := authenticatedClientAs(t, server.URL, "platform-engineer")
	dev := authenticatedClient(t, server.URL)
	body := `{"key":"score-workloads","resourceType":"workload","executionProfile":"internal-k8s","driverType":"score-k8s","driverInputs":{"values":{"variables":{"render_bundle":"score-k8s-internal-v1"}}},"criteria":[{}]}`
	if resp, out := call(t, dev, http.MethodPost, server.URL+"/api/v1/resource-definitions", body); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("developer registered Definition: %d %s", resp.StatusCode, out)
	}
	if resp, out := call(t, pe, http.MethodPost, server.URL+"/api/v1/resource-definitions", body); resp.StatusCode != http.StatusCreated {
		t.Fatalf("registration: %d %s", resp.StatusCode, out)
	}
	status, created := requestJSON(t, dev, http.MethodPost, server.URL+"/api/v1/applications", map[string]any{"name": "Rendered App", "subdomain": "rendered-app"})
	if status != http.StatusCreated {
		t.Fatal(status, created)
	}
	appKey := created["application"].(map[string]any)["key"].(string)
	base := server.URL + "/api/v1/applications/" + appKey + "/environments/staging"
	score := previewScore("api")
	delete(score["containers"].(map[string]any)["main"].(map[string]any), "variables")
	status, p := requestJSON(t, dev, http.MethodPost, base+"/score-preview", map[string]any{"workloadId": "api", "action": "deploy", "runId": "render", "scoreAfter": score})
	if status != http.StatusOK {
		t.Fatal(status, p)
	}
	if p["rendering"].(map[string]any)["api"].(map[string]any)["definitionKey"] != "score-workloads" {
		t.Fatal("missing public provenance")
	}
	if len(app.FakeExec.Calls) != 0 || len(app.FakeDeploy.Applied) != 0 {
		t.Fatal("preview caused side effects")
	}
	status, out := requestJSON(t, dev, http.MethodPut, base+"/workloads/api", map[string]any{"version": 0, "score": score})
	if status != http.StatusOK {
		t.Fatal(status, out)
	}
	status, p = requestJSON(t, dev, http.MethodPost, base+"/preview", map[string]any{})
	if status != http.StatusOK {
		t.Fatal(status, p)
	}
	status, out = requestJSON(t, dev, http.MethodPost, base+"/deploy", map[string]any{"token": p["token"]})
	if status != http.StatusOK || out["status"] != "SUCCEEDED" {
		t.Fatal(status, out)
	}
	status, p = requestJSON(t, dev, http.MethodPost, base+"/preview", map[string]any{})
	if status != http.StatusOK || len(p["changes"].([]any)) != 0 {
		t.Fatal("deployment did not record renderer", status, p)
	}
}
