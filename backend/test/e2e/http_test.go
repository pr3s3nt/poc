// Package e2e drives the orchestrator over HTTP with fake executors: the same
// path a Web Console user takes, without touching a cluster or a cloud account.
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"orchestrator/internal/bootstrap"
	"orchestrator/internal/seed"
)

func newServer(t *testing.T, uiDir string) (*httptest.Server, seed.Options) {
	t.Helper()
	seedOptions := seed.Defaults()
	seedOptions.Region = "us-east-1"
	seedOptions.AccountID = "000000000000"
	seedOptions.RunID = "run-e2e"
	app, err := bootstrap.Build(context.Background(), bootstrap.Options{
		Seed:     seedOptions,
		Adapters: bootstrap.AdapterFake,
		UIDir:    uiDir,
	})
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	server := httptest.NewServer(app.Server)
	t.Cleanup(server.Close)
	return server, seedOptions
}

func postJSON(t *testing.T, url string, body any) (int, map[string]any) {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("post %s: %v", url, err)
	}
	defer resp.Body.Close()
	out := map[string]any{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func getJSON(t *testing.T, url string) (int, map[string]any) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("get %s: %v", url, err)
	}
	defer resp.Body.Close()
	out := map[string]any{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func authenticatedClient(t *testing.T, baseURL string) *http.Client {
	return authenticatedClientAs(t, baseURL, "developer")
}

func authenticatedClientAs(t *testing.T, baseURL, username string) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	client := &http.Client{Jar: jar}
	payload, err := json.Marshal(map[string]string{"username": username, "password": "test-password"})
	if err != nil {
		t.Fatalf("sign-in marshal: %v", err)
	}
	resp, err := client.Post(baseURL+"/api/v1/auth/sign-in", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("sign-in: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("sign-in status %d", resp.StatusCode)
	}
	return client
}

func TestResourceTypeRegistrationRequiresPlatformEngineer(t *testing.T) {
	server, _ := newServer(t, "")
	payload := []byte(`{"key":"redis","inputs":[{"name":"size","type":"number"}],"outputs":[{"name":"host","type":"string","required":true}]}`)
	post := func(client *http.Client) int {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/resource-types", bytes.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}
	if got := post(http.DefaultClient); got != http.StatusUnauthorized {
		t.Fatalf("anonymous registration returned %d", got)
	}
	if got := post(authenticatedClient(t, server.URL)); got != http.StatusForbidden {
		t.Fatalf("developer registration returned %d", got)
	}
	platform := authenticatedClientAs(t, server.URL, "platform-engineer")
	if got := post(platform); got != http.StatusCreated {
		t.Fatalf("platform registration returned %d", got)
	}
	if got := post(platform); got != http.StatusConflict {
		t.Fatalf("duplicate registration returned %d", got)
	}
	status, catalog := getJSONClient(t, platform, server.URL+"/api/v1/resource-types")
	if status != http.StatusOK {
		t.Fatalf("catalog status %d", status)
	}
	found := false
	for _, item := range catalog["resourceTypes"].([]any) {
		if item.(map[string]any)["key"] == "redis" {
			found = true
		}
	}
	if !found {
		t.Fatal("registered type was not published to the catalog")
	}
}

func getJSONClient(t *testing.T, client *http.Client, url string) (int, map[string]any) {
	t.Helper()
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("get %s: %v", url, err)
	}
	defer resp.Body.Close()
	out := map[string]any{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func TestHTTPDeploymentEndToEnd(t *testing.T) {
	server, seedOptions := newServer(t, "")
	scores := seed.AcceptanceScores(seedOptions)
	client := authenticatedClient(t, server.URL)

	status, apps := getJSONClient(t, client, server.URL+"/api/v1/applications")
	if status != http.StatusOK {
		t.Fatalf("applications returned %d", status)
	}
	// Both Execution Profiles are registered, so the console can offer either.
	if len(apps["applications"].([]any)) != 2 {
		t.Fatalf("expected the two seeded applications: %v", apps)
	}

	var lastID string
	for _, workload := range seed.AcceptanceOrder() {
		status, created := postJSON(t, server.URL+"/api/v1/deployments", map[string]any{
			"applicationKey": seedOptions.ApplicationKey,
			"environmentKey": seedOptions.EnvironmentKey,
			"workloadId":     workload,
			"actor":          "e2e-test",
			"score":          scores[workload],
		})
		if status != http.StatusCreated {
			t.Fatalf("deploy %s returned %d: %v", workload, status, created)
		}
		if created["status"] != "SUCCEEDED" {
			t.Fatalf("deploy %s ended in %v", workload, created["status"])
		}
		lastID, _ = created["deploymentId"].(string)
	}

	status, view := getJSON(t, server.URL+"/api/v1/deployments/"+lastID)
	if status != http.StatusOK {
		t.Fatalf("deployment view returned %d", status)
	}
	if view["graph"] == nil || view["batches"] == nil || view["matches"] == nil {
		t.Fatalf("the deployment view is missing plan artifacts: %v", view)
	}
	workloads, _ := view["workloads"].([]any)
	if len(workloads) != 3 {
		t.Fatalf("expected three workloads in the view, got %v", workloads)
	}
	resources, _ := view["resources"].([]any)
	if len(resources) == 0 {
		t.Fatal("expected resource rows in the view")
	}
	for _, item := range resources {
		row := item.(map[string]any)
		if row["resourceType"] != "postgres" {
			continue
		}
		outputs := row["outputs"].(map[string]any)
		if outputs["password"] != "***redacted***" {
			t.Fatalf("the API must redact secret outputs: %v", outputs)
		}
	}
}

func TestHTTPRejectsInvalidScore(t *testing.T) {
	server, seedOptions := newServer(t, "")
	status, body := postJSON(t, server.URL+"/api/v1/deployments", map[string]any{
		"applicationKey": seedOptions.ApplicationKey,
		"environmentKey": seedOptions.EnvironmentKey,
		"workloadId":     "backend",
		"score":          map[string]any{"apiVersion": "score.dev/v1b1", "metadata": map[string]any{"name": "backend"}},
	})
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 for a Score without containers, got %d: %v", status, body)
	}
}

func TestHTTPDeploymentNotFound(t *testing.T) {
	server, _ := newServer(t, "")
	status, _ := getJSON(t, server.URL+"/api/v1/deployments/does-not-exist")
	if status != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", status)
	}
}

func TestUIServesBundleAndBrowserFallback(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!doctype html><title>console</title>"), 0o600); err != nil {
		t.Fatalf("write index: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatalf("mkdir assets: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "assets", "app.js"), []byte("export const ok = true;"), 0o600); err != nil {
		t.Fatalf("write asset: %v", err)
	}
	server, _ := newServer(t, dir)

	resp, err := http.Get(server.URL + "/ui/assets/app.js")
	if err != nil {
		t.Fatalf("get asset: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("asset returned %d", resp.StatusCode)
	}

	fallback, err := http.Get(server.URL + "/ui/deployments/some-id")
	if err != nil {
		t.Fatalf("get fallback: %v", err)
	}
	defer fallback.Body.Close()
	if fallback.StatusCode != http.StatusOK {
		t.Fatalf("browser fallback returned %d", fallback.StatusCode)
	}
	body := make([]byte, 64)
	n, _ := fallback.Body.Read(body)
	if !bytes.Contains(body[:n], []byte("<!doctype html>")) {
		t.Fatalf("browser fallback did not serve index.html: %q", body[:n])
	}
}

// TestHTTPUsesTheConfiguredRunID proves a request that carries no run id — the
// body the Web Console sends — still produces run-scoped cloud resource names.
func TestHTTPUsesTheConfiguredRunID(t *testing.T) {
	server, seedOptions := newServer(t, "")
	scores := seed.AcceptanceScores(seedOptions)

	status, created := postJSON(t, server.URL+"/api/v1/deployments", map[string]any{
		"applicationKey": seedOptions.CloudApplicationKey,
		"environmentKey": seedOptions.EnvironmentKey,
		"workloadId":     "backend",
		"actor":          "web-console",
		"score":          scores["backend"],
	})
	if status != http.StatusCreated {
		t.Fatalf("deploy returned %d: %v", status, created)
	}
	deploymentID, _ := created["deploymentId"].(string)

	_, view := getJSON(t, server.URL+"/api/v1/deployments/"+deploymentID)
	resources, _ := view["resources"].([]any)
	found := false
	for _, item := range resources {
		row := item.(map[string]any)
		if row["resourceType"] != "k8s-cluster" {
			continue
		}
		inputs, _ := row["resolvedInputs"].(map[string]any)
		name, _ := inputs["name"].(string)
		if !strings.HasSuffix(name, seedOptions.RunID) {
			t.Fatalf("cloud resource name %q does not carry the configured run id %q", name, seedOptions.RunID)
		}
		found = true
	}
	if !found {
		t.Fatalf("the cloud deployment has no k8s-cluster resource: %v", resources)
	}
}
