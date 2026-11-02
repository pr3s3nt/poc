package e2e

import (
	"context"
	"net/http"
	"net/http/httptest"
	"orchestrator/internal/adapters/postgres/postgrestest"
	"orchestrator/internal/seed/seedtest"
	"strings"
	"sync"
	"testing"

	"orchestrator/internal/adapters/credentialmemory"
	"orchestrator/internal/bootstrap"
)

// TestUC02UC04RegistrationIsUsableWithoutRestart registers a Connection, a
// runtime-supported PostgreSQL Definition for class "fast" and a new Resource
// Type over HTTP, then uses them from a Developer session on the same process:
// Score Preview matches the new Definition, and the new Type validates Score
// params although no runtime driver can provision it.

// The new Type's input schema validates Score params in planning at once
// (score stage), but runtime drivers stay fixed: valid params then fail
// only because no Definition can provision the new Type (graph stage).

// Connection bodies carry a kubeconfig of up to 1 MiB, so their JSON
// envelope is bounded at 8 MiB to allow escaping.

// Developers read Resource Types (UC-16 editor); Definitions and
// Connections stay Platform Engineer/Admin.

func TestUC02UC04ConcurrentRegistrationOnPostgres(t *testing.T) {
	databaseURL := postgrestest.Fresh(t).Console
	opts := seedtest.Defaults()
	app, err := bootstrap.Build(context.Background(), bootstrap.Options{Seed: opts, Adapters: bootstrap.AdapterFake, DatabaseURL: databaseURL, KubeconfigVerifierOverride: approvedCluster{}, ConnectionCredentialsOverride: credentialmemory.New()})
	if err != nil {
		t.Fatal(err)
	}
	if closer, ok := app.Store.(interface{ Close() }); ok {
		t.Cleanup(closer.Close)
	}
	server := httptest.NewServer(app.Server)
	t.Cleanup(server.Close)
	concurrentRegistration(t, server.URL, authenticatedClientAs(t, server.URL, "platform-engineer"))
}

// concurrentRegistration posts two different documents with the same key at
// once for each registration endpoint: one 201, one 409, winner unchanged.
// Concurrent uploads of one display name both succeed with distinct keys.
func concurrentRegistration(t *testing.T, baseURL string, client *http.Client) {
	t.Helper()
	for _, tc := range []struct{ path, list, a, b string }{
		{"/api/v1/resource-types", "/api/v1/resource-types", `{"key":"race-type","outputs":[{"name":"host","type":"string"}]}`, `{"key":"race-type","inputs":[{"name":"size","type":"number"}],"outputs":[{"name":"host","type":"string"}]}`},
		{"/api/v1/resource-definitions", "/api/v1/resource-definitions", postgresBody("race-def", []any{map[string]any{"class": "fast"}}), postgresBody("race-def", []any{map[string]any{"class": "slow"}, map[string]any{}})},
	} {
		statuses := make([]int, 2)
		var wg sync.WaitGroup
		start := make(chan struct{})
		for i, body := range []string{tc.a, tc.b} {
			wg.Add(1)
			go func(i int, body string) {
				defer wg.Done()
				<-start
				req, _ := http.NewRequest(http.MethodPost, baseURL+tc.path, strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				resp, err := client.Do(req)
				if err != nil {
					t.Error(err)
					return
				}
				resp.Body.Close()
				statuses[i] = resp.StatusCode
			}(i, body)
		}
		close(start)
		wg.Wait()
		winner := -1
		for i, s := range statuses {
			if s == http.StatusCreated {
				winner = i
			}
		}
		if winner < 0 || statuses[1-winner] != http.StatusConflict {
			t.Fatalf("%s: statuses %v, want one 201 and one 409", tc.path, statuses)
		}
		_, body := call(t, client, http.MethodGet, baseURL+tc.list, "")
		for _, key := range []string{`"key":"race-type"`, `"key":"race-def"`, `"key":"race-cluster"`} {
			if strings.Contains(tc.a, key) && strings.Count(body, key) != 1 {
				t.Fatalf("%s: winner record %s missing or repeated: %s", tc.path, key, body)
			}
		}
		switch tc.path {
		case "/api/v1/resource-types":
			if strings.Contains(body, `"name":"size"`) != (winner == 1) {
				t.Fatalf("type winner %d not stored: %s", winner, body)
			}
		case "/api/v1/resource-definitions":
			if strings.Contains(body, `"class":"slow"`) != (winner == 1) {
				t.Fatalf("definition winner %d not stored: %s", winner, body)
			}
		}
	}
	concurrentUploads(t, baseURL, client)
}

func concurrentUploads(t *testing.T, baseURL string, client *http.Client) {
	t.Helper()
	statuses := make([]int, 2)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range statuses {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			req, _ := http.NewRequest(http.MethodPost, baseURL+"/api/v1/connections/kubernetes", strings.NewReader(uploadBody("Race cluster")))
			req.Header.Set("Content-Type", "application/json")
			resp, err := client.Do(req)
			if err != nil {
				t.Error(err)
				return
			}
			resp.Body.Close()
			statuses[i] = resp.StatusCode
		}(i)
	}
	close(start)
	wg.Wait()
	if statuses[0] != http.StatusCreated || statuses[1] != http.StatusCreated {
		t.Fatalf("concurrent uploads: statuses %v, want both 201", statuses)
	}
	_, body := call(t, client, http.MethodGet, baseURL+"/api/v1/connections", "")
	if strings.Count(body, `"name":"Race cluster"`) != 2 {
		t.Fatalf("concurrent uploads must keep two distinct records: %s", body)
	}
}

// ADR-013/UC-03 BR-08 (T17): public POST rejects driver existing-cluster with
// 400 for any key (including the reserved system key) and inserts nothing.
