package postgres_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"
	"orchestrator/internal/adapters/postgres"
	"orchestrator/internal/bootstrap"
	"orchestrator/internal/bootstrap/bootstraptest"
	"orchestrator/internal/platform/password"
	"orchestrator/internal/seed/seedtest"
)

// Real PostgreSQL + product HTTP handlers; workload execution is explicitly fake.
func TestUpgradeHTTPLoginPreviewDeploy(t *testing.T) {
	owner, console, roles := postgres.LegacyUpgradeFixtureForTest(t)
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	hash, err := password.Hash("retained-test-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = conn.Exec(ctx, `UPDATE user_accounts SET password_hash=$1 WHERE username='retained-user'`, hash); err != nil {
		t.Fatal(err)
	}
	if _, err = postgres.Setup(ctx, owner, roles); err != nil {
		t.Fatal(err)
	}
	opts := seedtest.Defaults()
	opts.EnvironmentKey = "staging"
	opts.EnvironmentName = "Staging"
	opts.EnvironmentType = "staging"
	app, err := bootstraptest.Build(ctx, bootstrap.Options{Seed: opts, Adapters: bootstrap.AdapterFake, DatabaseURL: console})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	server := httptest.NewServer(app.Server)
	defer server.Close()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	request := func(method, path string, payload any, want int) map[string]any {
		t.Helper()
		var body bytes.Buffer
		if payload != nil {
			if err := json.NewEncoder(&body).Encode(payload); err != nil {
				t.Fatal(err)
			}
		}
		req, err := http.NewRequest(method, server.URL+path, &body)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var result map[string]any
		if err = json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != want {
			t.Fatalf("%s %s: status %d, want %d (private body omitted)", method, path, resp.StatusCode, want)
		}
		return result
	}
	request("POST", "/api/v1/auth/sign-in", map[string]any{"username": "retained-user", "password": "retained-test-password"}, http.StatusOK)
	applications := request("GET", "/api/v1/applications", nil, http.StatusOK)
	found := false
	for _, raw := range applications["applications"].([]any) {
		if raw.(map[string]any)["key"] == "retained-app" {
			found = true
		}
	}
	if !found {
		t.Fatal("retained account cannot read its application")
	}
	// Use the normal seeded fake-execution catalog to exercise Preview and deploy
	// on the migrated schema, including new writes, without touching a cluster.
	request("POST", "/api/v1/auth/sign-in", map[string]any{"username": "developer", "password": "test-password"}, http.StatusOK)
	base := "/api/v1/applications/" + opts.ApplicationKey + "/environments/" + opts.EnvironmentKey
	score := map[string]any{"apiVersion": "score.dev/v1b1", "metadata": map[string]any{"name": "migration-api"}, "containers": map[string]any{"main": map[string]any{"image": "example.invalid/migration-api:v1"}}}
	request("PUT", base+"/workloads/migration-api", map[string]any{"version": 0, "score": score}, http.StatusOK)
	preview := request("POST", base+"/preview", map[string]any{}, http.StatusOK)
	token, ok := preview["token"].(string)
	if !ok || token == "" {
		t.Fatal("Preview did not produce a token")
	}
	report := request("POST", base+"/deploy", map[string]any{"token": token}, http.StatusOK)
	if report["status"] != "SUCCEEDED" {
		t.Fatal("deployment on upgraded schema did not succeed")
	}
	results, ok := report["results"].([]any)
	if !ok || len(results) != 1 || results[0].(map[string]any)["status"] != "SUCCEEDED" {
		t.Fatal("expected one successful workload")
	}
	request("GET", base+"/configuration", nil, http.StatusOK)
	server.Close()
	app.Close()
	restarted, err := bootstraptest.Build(ctx, bootstrap.Options{Seed: opts, Adapters: bootstrap.AdapterFake, DatabaseURL: console})
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	server = httptest.NewServer(restarted.Server)
	defer server.Close()
	workloads := request("GET", base+"/workloads", nil, http.StatusOK)
	encoded, err := json.Marshal(workloads)
	if err != nil || !bytes.Contains(encoded, []byte("migration-api")) {
		t.Fatal("deployed workload missing after restart")
	}
}
