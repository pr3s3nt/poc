// Package e2e drives the orchestrator over HTTP with fake executors: the same
// path a Web Console user takes, without touching a cluster or a cloud account.
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"orchestrator/internal/adapters/postgres/postgrestest"
	"orchestrator/internal/seed/seedtest"
	"os"
	"path/filepath"
	"testing"

	"orchestrator/internal/adapters/credentialmemory"
	connectionapp "orchestrator/internal/application/connection"
	ct "orchestrator/internal/application/connection/connectiontest"
	appsvc "orchestrator/internal/application/deployment"
	"orchestrator/internal/bootstrap"
	"orchestrator/internal/bootstrap/bootstraptest"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/planning"
	"orchestrator/internal/seed"
)

func newServer(t *testing.T, uiDir string, verifier ...connectionapp.KubeconfigVerifier) (*httptest.Server, seed.Options) {
	t.Helper()
	server, seedOptions, _ := newServerApp(t, uiDir, verifier...)
	return server, seedOptions
}

func newServerApp(t *testing.T, uiDir string, verifier ...connectionapp.KubeconfigVerifier) (*httptest.Server, seed.Options, *bootstrap.App) {
	t.Helper()
	return newServerAppWith(t, uiDir, nil, verifier...)
}

// newServerAppWith lets a test adjust the bootstrap options (state path,
// registry spy) before the app is built.
func newServerAppWith(t *testing.T, uiDir string, mutate func(*bootstrap.Options), verifier ...connectionapp.KubeconfigVerifier) (*httptest.Server, seed.Options, *bootstrap.App) {
	t.Helper()
	seedOptions := seedtest.Defaults()
	seedOptions.Region = "us-east-1"
	seedOptions.AccountID = "000000000000"
	seedOptions.RunID = "run-e2e"
	credentials := credentialmemory.New()
	options := bootstrap.Options{
		Seed:     seedOptions,
		Adapters: bootstrap.AdapterFake,
		UIDir:    uiDir,
		// Fake mode keeps platform credentials in memory so Secret Stores can register.
		ConnectionCredentialsOverride: credentials,
	}
	if len(verifier) > 0 {
		options.KubeconfigVerifierOverride = verifier[0]
	}
	if mutate != nil {
		mutate(&options)
	}
	app, err := bootstrap.Build(context.Background(), options)
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	prepareSeedCluster(t, app, credentials, seedOptions)
	server := httptest.NewServer(app.Server)
	t.Cleanup(server.Close)
	return server, seedOptions, app
}

// eksEnvironment is the Environment of the Definition-driven EKS catalog (V05): no execution profile, Connection or binding.
const eksEnvironment = "staging"

// newEKSServerApp serves the fake-adapter application whose only catalog is the Definition-driven EKS chain.
func newEKSServerApp(t *testing.T) (*httptest.Server, seed.Options, *bootstrap.App) {
	t.Helper()
	var opts seed.Options
	server, _, app := newServerAppWith(t, "", func(o *bootstrap.Options) {
		o.Seed.Catalog = seed.CatalogNone
		opts = o.Seed
	})
	bootstraptest.EKSCatalog(t, app, opts)
	return server, opts, app
}

// eksScores are the acceptance Scores whose PostgreSQL runs in the cluster the plan creates.
func eksScores(opts seed.Options) map[string]map[string]any {
	docs := seed.AcceptanceScores(opts)
	for _, doc := range docs {
		resources, _ := doc["resources"].(map[string]any)
		if db, ok := resources["db"].(map[string]any); ok {
			db["class"] = "in-cluster"
		}
	}
	return docs
}

// prepareSeedCluster gives the seeded cluster record explicit synthetic
// credentials. The product seed is VERIFYING without credentials; fake e2e
// workflows opt in to a READY scoped fake credential here and nowhere else.
func prepareSeedCluster(t *testing.T, app *bootstrap.App, creds *credentialmemory.Store, opts seed.Options) {
	t.Helper()
	if err := ct.PrepareReady(context.Background(), app.Store, creds, opts.OrganizationKey, opts.ConnectionKey, opts.KubeContext, opts.ClusterName); err != nil {
		t.Fatalf("prepare seeded cluster: %v", err)
	}
}

// uploadBody is the strict upload registration body of a synthetic kubeconfig.
func uploadBody(name string) string {
	body, err := json.Marshal(map[string]string{"name": name, "kubeconfig": ct.Single("ctx", "https://cluster.example", "synthetic-token-0123456789"), "context": "ctx"})
	if err != nil {
		panic(err)
	}
	return string(body)
}

type approvedCluster struct{}

func (approvedCluster) VerifyKubeconfig(context.Context, connectionapp.SelectedKubeconfig) (connectionapp.KubernetesVerification, error) {
	return connectionapp.KubernetesVerification{Endpoint: "https://cluster.example", Version: "v1.34"}, nil
}

func TestRegisterKubernetesConnectionRequiresPlatformEngineer(t *testing.T) {
	server, _ := newServer(t, "", approvedCluster{})
	payload := []byte(uploadBody("Second cluster"))
	post := func(client *http.Client) int {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/connections/kubernetes", bytes.NewReader(payload))
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
		t.Fatalf("anonymous: %d", got)
	}
	developer := authenticatedClient(t, server.URL)
	if got := post(developer); got != http.StatusForbidden {
		t.Fatalf("developer: %d", got)
	}
	platform := authenticatedClientAs(t, server.URL, "platform-engineer")
	if got := post(platform); got != http.StatusCreated {
		t.Fatalf("platform: %d", got)
	}
	// Upload registration is insert-only: the same name gets a new key.
	if got := post(platform); got != http.StatusCreated {
		t.Fatalf("same name: %d", got)
	}
	status, body := getJSONClient(t, platform, server.URL+"/api/v1/connections")
	if status != http.StatusOK {
		t.Fatalf("list: %d", status)
	}
	found := false
	for _, item := range body["connections"].([]any) {
		if item.(map[string]any)["key"] == "second-cluster" {
			found = true
			// The public DTO never exposes the secret reference.
			if _, exposed := item.(map[string]any)["secretRef"]; exposed {
				t.Fatal("public connection DTO exposes secretRef")
			}
			if item.(map[string]any)["authenticationType"] != "KUBECONFIG" || item.(map[string]any)["name"] != "Second cluster" {
				t.Fatalf("registration is not a named KUBECONFIG connection: %v", item)
			}
		}
	}
	if !found {
		t.Fatal("registered connection not listed")
	}
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

func TestResourceDefinitionRegistrationRequiresPlatformEngineer(t *testing.T) {
	server, _ := newServer(t, "")
	payload := []byte(mustJSON(k8sModuleDefinition("namespace-custom", "k8s-namespace", "k8s-namespace", map[string]any{"name": "${context.env.namespace}"}, nil, []string{"name"}, []any{map[string]any{}})))
	post := func(client *http.Client) int {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/resource-definitions", bytes.NewReader(payload))
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
		t.Fatalf("anonymous: %d", got)
	}
	if got := post(authenticatedClient(t, server.URL)); got != http.StatusForbidden {
		t.Fatalf("developer: %d", got)
	}
	if status, _ := getJSONClient(t, authenticatedClient(t, server.URL), server.URL+"/api/v1/resource-definitions"); status != http.StatusForbidden {
		t.Fatalf("developer list: %d", status)
	}
	platform := authenticatedClientAs(t, server.URL, "platform-engineer")
	if got := post(platform); got != http.StatusCreated {
		t.Fatalf("platform: %d", got)
	}
	if got := post(platform); got != http.StatusConflict {
		t.Fatalf("duplicate: %d", got)
	}
	status, body := getJSONClient(t, platform, server.URL+"/api/v1/resource-definitions")
	if status != http.StatusOK {
		t.Fatalf("list: %d", status)
	}
	found := false
	for _, item := range body["resourceDefinitions"].([]any) {
		if item.(map[string]any)["key"] == "namespace-custom" {
			found = true
		}
	}
	if !found {
		t.Fatal("registered definition missing from catalog")
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
	// ADR-017: no profile-bound cloud Application is seeded.
	if len(apps["applications"].([]any)) != 1 {
		t.Fatalf("expected the one seeded application: %v", apps)
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

	status, view := getJSONClient(t, client, server.URL+"/api/v1/applications/"+seedOptions.ApplicationKey+"/environments/"+seedOptions.EnvironmentKey+"/deployments/"+lastID)
	if status != http.StatusOK {
		t.Fatalf("deployment view returned %d", status)
	}
	if view["graph"] == nil || view["batches"] == nil || view["matches"] == nil {
		t.Fatalf("the deployment view is missing plan artifacts: %v", view)
	}
	workloads, _ := view["workloads"].([]any)
	if len(workloads) != 1 || workloads[0].(map[string]any)["lastDeploymentId"] != lastID {
		t.Fatalf("expected this Deployment's workload snapshot, got %v", workloads)
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
		if _, leaked := row["resolvedInputs"]; leaked {
			t.Fatalf("the API must omit resolved resource inputs: %v", row)
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
	server, seedOptions := newServer(t, "")
	client := authenticatedClient(t, server.URL)
	status, _ := getJSONClient(t, client, server.URL+"/api/v1/applications/"+seedOptions.ApplicationKey+"/environments/"+seedOptions.EnvironmentKey+"/deployments/does-not-exist")
	if status != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", status)
	}
}

func TestHTTPDeploymentHistoryScopeFilterPlanningFailureAndSafeJSON(t *testing.T) {
	server, seedOptions := newServer(t, "")
	base := server.URL + "/api/v1/applications/" + seedOptions.ApplicationKey + "/environments/" + seedOptions.EnvironmentKey + "/deployments"
	if status, _ := getJSON(t, base); status != http.StatusUnauthorized {
		t.Fatalf("anonymous history returned %d", status)
	}
	client := authenticatedClient(t, server.URL)
	scores := seed.AcceptanceScores(seedOptions)
	status, created := postJSON(t, server.URL+"/api/v1/deployments", map[string]any{
		"applicationKey": seedOptions.ApplicationKey, "environmentKey": seedOptions.EnvironmentKey,
		"workloadId": "backend", "actor": "history-test", "score": scores["backend"],
	})
	if status != http.StatusCreated {
		t.Fatalf("successful deployment = %d %v", status, created)
	}
	successID := created["deploymentId"].(string)
	invalid := seed.AcceptanceScores(seedOptions)["backend"]
	invalid["resources"].(map[string]any)["db"].(map[string]any)["type"] = "unsupported-resource"
	status, _ = postJSON(t, server.URL+"/api/v1/deployments", map[string]any{
		"applicationKey": seedOptions.ApplicationKey, "environmentKey": seedOptions.EnvironmentKey,
		"workloadId": "broken", "actor": "history-test", "score": invalid,
	})
	if status != http.StatusBadRequest {
		t.Fatalf("planning failure request = %d", status)
	}

	status, history := getJSONClient(t, client, base)
	rows := history["deployments"].([]any)
	if status != http.StatusOK || len(rows) != 2 || rows[0].(map[string]any)["status"] != "FAILED" {
		t.Fatalf("newest-first history = %d %v", status, history)
	}
	status, failed := getJSONClient(t, client, base+"?status=FAILED")
	if status != http.StatusOK || len(failed["deployments"].([]any)) != 1 {
		t.Fatalf("server-side failed filter = %d %v", status, failed)
	}
	if status, _ := getJSONClient(t, client, base+"?status=BOGUS"); status != http.StatusBadRequest {
		t.Fatalf("invalid status filter returned %d", status)
	}
	wrongScope := server.URL + "/api/v1/applications/" + seedOptions.ApplicationKey + "/environments/not-this-environment/deployments/" + successID
	if status, _ := getJSONClient(t, client, wrongScope); status != http.StatusNotFound {
		t.Fatalf("cross-environment detail returned %d", status)
	}

	response, err := client.Get(base + "/" + successID)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("detail = %d %s %v", response.StatusCode, encoded, err)
	}
	if bytes.Contains(encoded, []byte(`"resolvedInputs"`)) {
		t.Fatalf("encoded response leaked resolved inputs: %s", encoded)
	}
	if !bytes.Contains(encoded, []byte(appsvc.RedactedValue)) {
		t.Fatalf("encoded response lacks redacted secret marker: %s", encoded)
	}

	failedID := rows[0].(map[string]any)["id"].(string)
	status, detail := getJSONClient(t, client, base+"/"+failedID)
	if status != http.StatusOK || detail["graph"] != nil || detail["delta"] != nil || len(detail["workloads"].([]any)) != 0 {
		t.Fatalf("planning failure detail = %d %v", status, detail)
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
// Resolved inputs are not part of the UC-09 read contract, so the persisted
// Deployment Resource rows are inspected through the store port instead.
func TestHTTPUsesTheConfiguredRunID(t *testing.T) {
	server, seedOptions, app := newEKSServerApp(t)
	scores := eksScores(seedOptions)

	status, created := postJSON(t, server.URL+"/api/v1/deployments", map[string]any{
		"applicationKey": seedOptions.ApplicationKey,
		"environmentKey": eksEnvironment,
		"workloadId":     "backend",
		"actor":          "web-console",
		"score":          scores["backend"],
	})
	if status != http.StatusCreated {
		t.Fatalf("deploy returned %d: %v", status, created)
	}
	deploymentID, _ := created["deploymentId"].(string)

	rows, err := app.Store.ListDeploymentResources(context.Background(), deploymentID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range rows {
		if row.ResourceTypeKey != "k8s-cluster" {
			continue
		}
		name, _ := row.ResolvedInputs["name"].(string)
		// The provider-safe name is stable per Environment; the run id is
		// provenance only and never part of it (ADR-014).
		named := func(run string) string {
			return planning.Context{
				App:   application.Application{Key: seedOptions.ApplicationKey},
				Env:   environment.Environment{ApplicationKey: seedOptions.ApplicationKey, Key: eksEnvironment},
				RunID: run,
			}.ResourceName()
		}
		if name != named(seedOptions.RunID) || name != named(seedOptions.RunID+"-other") {
			t.Fatalf("cloud resource name %q must be stable and independent of the run id %q", name, seedOptions.RunID)
		}
		found = true
	}
	if !found {
		t.Fatalf("the cloud deployment has no k8s-cluster resource: %v", rows)
	}
}

func TestHTTPDeploymentReadsRequireScopedSessionAndHideResolvedInputs(t *testing.T) {
	server, seedOptions, _ := newEKSServerApp(t)
	scores := eksScores(seedOptions)

	status, created := postJSON(t, server.URL+"/api/v1/deployments", map[string]any{
		"applicationKey": seedOptions.ApplicationKey,
		"environmentKey": eksEnvironment,
		"workloadId":     "backend",
		"actor":          "web-console",
		"score":          scores["backend"],
	})
	if status != http.StatusCreated {
		t.Fatalf("deploy returned %d: %v", status, created)
	}
	deploymentID, _ := created["deploymentId"].(string)

	path := server.URL + "/api/v1/applications/" + seedOptions.ApplicationKey + "/environments/" + eksEnvironment + "/deployments/" + deploymentID
	if status, _ := getJSON(t, path); status != http.StatusUnauthorized {
		t.Fatalf("anonymous detail returned %d", status)
	}
	client := authenticatedClient(t, server.URL)
	status, view := getJSONClient(t, client, path)
	if status != http.StatusOK {
		t.Fatalf("scoped detail returned %d: %v", status, view)
	}
	resources, _ := view["resources"].([]any)
	found := false
	for _, item := range resources {
		row := item.(map[string]any)
		if row["resourceType"] != "k8s-cluster" {
			continue
		}
		if _, leaked := row["resolvedInputs"]; leaked {
			t.Fatalf("resolved inputs leaked: %v", row)
		}
		found = true
	}
	if !found {
		t.Fatalf("the cloud deployment has no k8s-cluster resource: %v", resources)
	}
}

// TestHTTPDeploymentReadsOnPostgres runs the scoped read contract against a
// fresh local PostgreSQL database, where a non-UUID path segment must still be
// a plain 404 and storage errors must never reach the client.
func TestHTTPDeploymentReadsOnPostgres(t *testing.T) {
	databaseURL := postgrestest.Fresh(t).Console
	seedOptions := seedtest.Defaults()
	seedOptions.RunID = "run-e2e"
	// The deploy below needs the seeded cluster READY (fake-only setup, ID kept).
	app, err := bootstraptest.Build(context.Background(), bootstrap.Options{Seed: seedOptions, Adapters: bootstrap.AdapterFake, DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	// Cleanups run last-in first-out: close the pool before the database drop.
	if closer, ok := app.Store.(interface{ Close() }); ok {
		t.Cleanup(closer.Close)
	}
	server := httptest.NewServer(app.Server)
	t.Cleanup(server.Close)
	client := authenticatedClient(t, server.URL)
	base := server.URL + "/api/v1/applications/" + seedOptions.ApplicationKey + "/environments/" + seedOptions.EnvironmentKey + "/deployments"

	for _, id := range []string{"does-not-exist", "00000000-0000-4000-8000-000000000000"} {
		response, err := client.Get(base + "/" + id)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != http.StatusNotFound || bytes.Contains(body, []byte("uuid")) || bytes.Contains(body, []byte("SQLSTATE")) {
			t.Fatalf("detail %q = %d %s", id, response.StatusCode, body)
		}
	}

	status, created := postJSON(t, server.URL+"/api/v1/deployments", map[string]any{
		"applicationKey": seedOptions.ApplicationKey, "environmentKey": seedOptions.EnvironmentKey,
		"workloadId": "backend", "actor": "postgres-test", "score": seed.AcceptanceScores(seedOptions)["backend"],
	})
	if status != http.StatusCreated {
		t.Fatalf("deploy = %d %v", status, created)
	}
	id := created["deploymentId"].(string)
	status, view := getJSONClient(t, client, base+"/"+id)
	workloads, _ := view["workloads"].([]any)
	if status != http.StatusOK || len(workloads) != 1 || workloads[0].(map[string]any)["lastDeploymentId"] != id {
		t.Fatalf("detail = %d %v", status, view)
	}
	if status, _ := getJSONClient(t, client, server.URL+"/api/v1/applications/"+seedOptions.ApplicationKey+"/environments/not-this-environment/deployments/"+id); status != http.StatusNotFound {
		t.Fatalf("cross-environment detail = %d", status)
	}
}

// createConfiguredApplication creates an Application over HTTP (name and
// subdomain only) and sets the same Connection on staging and production
// through the set-once endpoint, as a Developer would in Environment Settings.
func createConfiguredApplication(t *testing.T, client *http.Client, baseURL, name, subdomain, connectionKey string) map[string]any {
	t.Helper()
	status, created := requestJSON(t, client, http.MethodPost, baseURL+"/api/v1/applications", map[string]any{"name": name, "subdomain": subdomain})
	if status != http.StatusCreated {
		t.Fatalf("create Application: %d %v", status, created)
	}
	app := created["application"].(map[string]any)
	if connectionKey != "internal-cluster" {
		for _, env := range app["environments"].([]any) {
			view := env.(map[string]any)
			if status, out := requestJSON(t, client, http.MethodPut, baseURL+"/api/v1/applications/"+app["key"].(string)+"/environments/"+view["key"].(string)+"/connection", map[string]any{"connectionKey": connectionKey, "expectedVersion": view["version"]}); status != http.StatusOK {
				t.Fatalf("set %s connection: %d %v", view["key"], status, out)
			}
		}
		return created
	}
	placeOnSeededCluster(t, baseURL, app["key"].(string), true)
	return created
}

// createUnrenderedApplication places the Application on the seeded cluster and namespace but not on
// the seeded workload rendering Definition, so a test registers its own rendering template.
func createUnrenderedApplication(t *testing.T, client *http.Client, baseURL, name, subdomain string) map[string]any {
	t.Helper()
	status, created := requestJSON(t, client, http.MethodPost, baseURL+"/api/v1/applications", map[string]any{"name": name, "subdomain": subdomain})
	if status != http.StatusCreated {
		t.Fatalf("create Application: %d %v", status, created)
	}
	placeOnSeededCluster(t, baseURL, created["application"].(map[string]any)["key"].(string), false)
	return created
}

// placedWorkloadVariables are the typed references an internal workload rendering Definition carries.
const placedWorkloadVariables = `"render_bundle":"score-k8s-internal-v1","namespace":"${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}","cluster":"${resources['k8s-cluster.internal#environments.@app.@env'].outputs.cluster}"`

// placeOnSeededCluster is the platform engineer adding the Application to the explicit app_id
// criteria of the seeded internal placement Definitions (ADR-016: Definitions place workloads;
// an Environment carries no deployment Connection).
func placeOnSeededCluster(t *testing.T, baseURL, appKey string, workload bool) {
	t.Helper()
	pe := authenticatedClientAs(t, baseURL, "platform-engineer")
	api := baseURL + "/api/v1/resource-definitions"
	_, list := requestJSON(t, pe, http.MethodGet, api, nil)
	for _, item := range list["resourceDefinitions"].([]any) {
		def := item.(map[string]any)
		criteria, _ := def["criteria"].([]any)
		seeded := false
		for _, raw := range criteria {
			if c, _ := raw.(map[string]any); c["app_id"] == "acceptance" {
				seeded = true
			}
		}
		if !seeded || def["resourceType"] == "postgres" || def["resourceType"] == "redis" || (!workload && def["resourceType"] == "workload") {
			continue
		}
		body := map[string]any{"key": def["key"], "name": def["name"], "resourceType": def["resourceType"], "driverType": def["driverType"], "version": def["version"],
			"criteria": append(append([]any{}, criteria...), map[string]any{"app_id": appKey})}
		for _, field := range []string{"driverAccount", "driverInputs", "provision"} {
			if v, ok := def[field]; ok {
				body[field] = v
			}
		}
		if status, out := requestJSON(t, pe, http.MethodPut, api+"/"+def["key"].(string), body); status != http.StatusOK {
			t.Fatalf("place %s on %s: %d %v", appKey, def["key"], status, out)
		}
	}
}

// registerSecretStore registers a store as the platform engineer and returns its key.
func registerSecretStore(t *testing.T, baseURL, name string) string {
	t.Helper()
	pe := authenticatedClientAs(t, baseURL, "platform-engineer")
	status, out := requestJSON(t, pe, http.MethodPost, baseURL+"/api/v1/secret-stores", map[string]any{
		"name": name, "backendAddress": "http://vault.example:8200", "workloadAddress": "http://vault.vault.svc:8200", "token": "e2e-token-" + name,
	})
	if status != http.StatusCreated {
		t.Fatalf("register secret store %s: %d %v", name, status, out)
	}
	return out["key"].(string)
}

// selectSecretStore selects a store for an Environment as the Developer.
func selectSecretStore(t *testing.T, client *http.Client, baseURL, app, env, storeKey string) {
	t.Helper()
	base := baseURL + "/api/v1/applications/" + app + "/environments/" + env
	_, application := requestJSON(t, client, http.MethodGet, baseURL+"/api/v1/applications/"+app, nil)
	var version float64
	for _, raw := range application["application"].(map[string]any)["environments"].([]any) {
		if view := raw.(map[string]any); view["key"] == env {
			version = view["version"].(float64)
		}
	}
	_, config := requestJSON(t, client, http.MethodGet, base+"/configuration", nil)
	status, out := requestJSON(t, client, http.MethodPut, base+"/secret-store", map[string]any{"secretStoreKey": storeKey, "expectedVersion": version, "expectedConfigVersion": config["version"]})
	if status != http.StatusOK {
		t.Fatalf("select secret store: %d %v", status, out)
	}
}
