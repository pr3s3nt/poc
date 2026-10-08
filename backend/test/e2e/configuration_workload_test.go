package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"orchestrator/internal/bootstrap"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/seed"
)

type frontendFailDeployer struct{ fail bool }

func (d *frontendFailDeployer) Apply(_ context.Context, _ execution.Target, manifests []execution.Manifest) error {
	for _, manifest := range manifests {
		if d.fail && manifest.Kind == "Deployment" && manifest.Name == "frontend" {
			return fmt.Errorf("simulated frontend apply failure")
		}
	}
	return nil
}
func (*frontendFailDeployer) WaitReady(context.Context, execution.Target, []execution.WorkloadRef) error {
	return nil
}
func (*frontendFailDeployer) Remove(context.Context, execution.Target, string) error { return nil }

func requestJSON(t *testing.T, client *http.Client, method, url string, body any) (int, map[string]any) {
	t.Helper()
	var data []byte
	if body != nil {
		var err error
		data, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	req, err := http.NewRequest(method, url, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out := map[string]any{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, out
}

func TestUC12UC16HTTPDraftFlow(t *testing.T) {
	server, _ := newServer(t, "")
	client := authenticatedClient(t, server.URL)
	created := createConfiguredApplication(t, client, server.URL, "Draft Test", "draft-test", "internal-cluster")
	app := created["application"].(map[string]any)["key"].(string)
	base := server.URL + "/api/v1/applications/" + app + "/environments/staging"
	secret := "never-show-this-secret"
	// A Secret needs an explicitly selected store; Variables do not.
	status, rejected := requestJSON(t, client, http.MethodPut, base+"/configuration/keys/API_TOKEN", map[string]any{"kind": "SECRET", "value": secret, "version": 0})
	if status != http.StatusUnprocessableEntity || rejected["field"] != "secretStoreKey" || strings.Contains(toJSON(t, rejected), secret) {
		t.Fatalf("secret without a store: %d %v", status, rejected)
	}
	selectSecretStore(t, client, server.URL, app, "staging", registerSecretStore(t, server.URL, "Draft Vault"))
	status, config := requestJSON(t, client, http.MethodPut, base+"/configuration/keys/API_TOKEN", map[string]any{"kind": "SECRET", "value": secret, "version": 0})
	if status != http.StatusOK || strings.Contains(toJSON(t, config), secret) {
		t.Fatalf("secret value leaked or save failed: %d %v", status, config)
	}
	status, _ = requestJSON(t, client, http.MethodGet, base+"/configuration", nil)
	if status != http.StatusOK {
		t.Fatalf("configuration read: %d", status)
	}
	other := strings.Replace(base, "/staging", "/production", 1)
	status, production := requestJSON(t, client, http.MethodGet, other+"/configuration", nil)
	if status != http.StatusOK || len(production["keys"].([]any)) != 0 {
		t.Fatalf("production must be isolated: %d %v", status, production)
	}
	score := map[string]any{"apiVersion": "score.dev/v1b1", "metadata": map[string]any{"name": "frontend"}, "containers": map[string]any{"main": map[string]any{"image": "example.invalid/frontend:test", "variables": map[string]any{"TOKEN": "${resources.env.API_TOKEN}"}}}, "resources": map[string]any{"env": map[string]any{"type": "environment"}}}
	status, parsed := requestJSON(t, client, http.MethodPost, base+"/workloads/parse", map[string]any{"content": toJSON(t, score)})
	if status != http.StatusOK || parsed["score"] == nil {
		t.Fatalf("Score import: %d %v", status, parsed)
	}
	status, draft := requestJSON(t, client, http.MethodPut, base+"/workloads/frontend", map[string]any{"score": score, "version": 0})
	if status != http.StatusOK || draft["draftVersion"] != float64(1) || strings.Contains(toJSON(t, draft), secret) {
		t.Fatalf("draft save: %d %v", status, draft)
	}
	status, _ = requestJSON(t, client, http.MethodPut, base+"/workloads/frontend", map[string]any{"score": score, "version": 0})
	if status != http.StatusConflict {
		t.Fatalf("stale draft version must be 409, got %d", status)
	}
	status, preview := requestJSON(t, client, http.MethodPost, base+"/preview", map[string]any{})
	if status != http.StatusOK || preview["token"] == "" || len(preview["changes"].([]any)) != 1 {
		t.Fatalf("pending preview: %d %v", status, preview)
	}
	status, _ = requestJSON(t, client, http.MethodPut, base+"/configuration/keys/API_TOKEN", map[string]any{"kind": "SECRET", "value": "new-private-value", "version": 1})
	if status != http.StatusOK {
		t.Fatalf("update pending secret: %d", status)
	}
	status, stale := requestJSON(t, client, http.MethodPost, base+"/deploy", map[string]any{"token": preview["token"]})
	if status != http.StatusConflict {
		t.Fatalf("changed revision must stale preview: %d %v", status, stale)
	}
	status, preview = requestJSON(t, client, http.MethodPost, base+"/preview", map[string]any{})
	if status != http.StatusOK {
		t.Fatalf("preview current revision: %d %v", status, preview)
	}
	status, deployed := requestJSON(t, client, http.MethodPost, base+"/deploy", map[string]any{"token": preview["token"]})
	if status != http.StatusOK || deployed["status"] != "SUCCEEDED" || strings.Contains(toJSON(t, deployed), secret) {
		t.Fatalf("pending deploy: %d %v", status, deployed)
	}
	status, retry := requestJSON(t, client, http.MethodPost, base+"/deploy", map[string]any{"token": preview["token"]})
	if status != http.StatusConflict {
		t.Fatalf("stale preview must be rejected after deploy: %d %v", status, retry)
	}
	status, _ = requestJSON(t, client, http.MethodPut, base+"/configuration/keys/API_TOKEN", map[string]any{"kind": "SECRET", "value": "rotated-private-value", "version": 2})
	if status != http.StatusOK {
		t.Fatalf("rotate secret: %d", status)
	}
	status, rotationPreview := requestJSON(t, client, http.MethodPost, base+"/preview", map[string]any{})
	if status != http.StatusOK || len(rotationPreview["changes"].([]any)) != 1 || rotationPreview["changes"].([]any)[0].(map[string]any)["workloadId"] != "frontend" {
		t.Fatalf("rotation preview: %d %v", status, rotationPreview)
	}
	status, rotation := requestJSON(t, client, http.MethodPost, base+"/deploy", map[string]any{"token": rotationPreview["token"]})
	if status != http.StatusOK || rotation["status"] != "SUCCEEDED" {
		t.Fatalf("rotation deploy: %d %v", status, rotation)
	}
	status, noChange := requestJSON(t, client, http.MethodPost, base+"/preview", map[string]any{})
	if status != http.StatusOK || len(noChange["changes"].([]any)) != 0 {
		t.Fatalf("rotation should be applied: %d %v", status, noChange)
	}
	status, _ = requestJSON(t, http.DefaultClient, http.MethodGet, base+"/workloads", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated draft read must be 401, got %d", status)
	}
}

func TestPendingDeployPartialFailureCanRetryOnlyRemainingWorkload(t *testing.T) {
	failer := &frontendFailDeployer{fail: true}
	app, err := bootstrap.Build(context.Background(), bootstrap.Options{Seed: seed.Defaults(), Adapters: bootstrap.AdapterFake, DeployerOverride: failer})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(app.Server)
	defer server.Close()
	client := authenticatedClient(t, server.URL)
	created := createConfiguredApplication(t, client, server.URL, "Partial Test", "partial-test", "internal-cluster")
	appKey := created["application"].(map[string]any)["key"].(string)
	base := server.URL + "/api/v1/applications/" + appKey + "/environments/staging"
	backend := map[string]any{"apiVersion": "score.dev/v1b1", "metadata": map[string]any{"name": "backend"}, "containers": map[string]any{"main": map[string]any{"image": "example.invalid/backend:test"}}, "service": map[string]any{"ports": map[string]any{"http": map[string]any{"port": 8080}}}}
	frontend := map[string]any{"apiVersion": "score.dev/v1b1", "metadata": map[string]any{"name": "frontend"}, "containers": map[string]any{"main": map[string]any{"image": "example.invalid/frontend:test", "variables": map[string]any{"BACKEND_URL": "${resources.backend.url}"}}}, "resources": map[string]any{"backend": map[string]any{"type": "service", "params": map[string]any{"workload": "backend", "port": "http"}}}}
	for i, item := range []struct {
		name  string
		score map[string]any
	}{{"backend", backend}, {"frontend", frontend}} {
		status, body := requestJSON(t, client, http.MethodPut, base+"/workloads/"+item.name, map[string]any{"score": item.score, "version": i})
		if status != http.StatusOK {
			t.Fatalf("save %s: %d %v", item.name, status, body)
		}
	}
	status, preview := requestJSON(t, client, http.MethodPost, base+"/preview", map[string]any{})
	if status != http.StatusOK || len(preview["changes"].([]any)) != 2 {
		t.Fatalf("preview: %d %v", status, preview)
	}
	status, report := requestJSON(t, client, http.MethodPost, base+"/deploy", map[string]any{"token": preview["token"]})
	if status != http.StatusOK || report["status"] != "PARTIAL" {
		t.Fatalf("partial deploy: %d %v", status, report)
	}
	results := report["results"].([]any)
	if results[0].(map[string]any)["workloadId"] != "backend" || results[0].(map[string]any)["status"] != "SUCCEEDED" || results[1].(map[string]any)["status"] != "FAILED" {
		t.Fatalf("wrong per-workload results: %v", results)
	}
	failer.fail = false
	status, retryPreview := requestJSON(t, client, http.MethodPost, base+"/preview", map[string]any{})
	if status != http.StatusOK || len(retryPreview["changes"].([]any)) != 1 || retryPreview["changes"].([]any)[0].(map[string]any)["workloadId"] != "frontend" {
		t.Fatalf("retry preview: %d %v", status, retryPreview)
	}
	status, retried := requestJSON(t, client, http.MethodPost, base+"/deploy", map[string]any{"token": retryPreview["token"]})
	if status != http.StatusOK || retried["status"] != "SUCCEEDED" {
		t.Fatalf("retry deploy: %d %v", status, retried)
	}
}

func toJSON(t *testing.T, value any) string {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
