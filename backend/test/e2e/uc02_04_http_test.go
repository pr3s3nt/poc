package e2e

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"orchestrator/internal/bootstrap"
	"orchestrator/internal/ports/persistence/persistencetest"
	"orchestrator/internal/seed"
)

const fastPostgres = `{"key":"postgres-fast","resourceType":"postgres","executionProfile":"internal-k8s","driverType":"kubernetes","connectionKey":"fast-cluster",` +
	`"driverInputs":{"values":{"variables":{"image":"postgres:17-alpine","storage":"2Gi","namespace":"${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}"}}},` +
	`"criteria":[{"class":"fast"}]}`

// TestUC02UC04RegistrationIsUsableWithoutRestart registers a Connection, a
// runtime-supported PostgreSQL Definition for class "fast" and a new Resource
// Type over HTTP, then uses them from a Developer session on the same process:
// Score Preview matches the new Definition, and the new Type validates Score
// params although no runtime driver can provision it.
func TestUC02UC04RegistrationIsUsableWithoutRestart(t *testing.T) {
	server, _ := newServer(t, "", approvedCluster{})
	pe := authenticatedClientAs(t, server.URL, "platform-engineer")
	for _, tc := range []struct{ path, body string }{
		{"/api/v1/connections/kubernetes", `{"key":"fast-cluster","clusterId":"fast","kubeContext":"kind-fast"}`},
		{"/api/v1/resource-definitions", fastPostgres},
		{"/api/v1/resource-types", `{"key":"cache","inputs":[{"name":"size","type":"number","required":true}],"outputs":[{"name":"host","type":"string","required":true}]}`},
	} {
		if resp, body := call(t, pe, http.MethodPost, server.URL+tc.path, tc.body); resp.StatusCode != http.StatusCreated {
			t.Fatalf("register %s = %d %s", tc.path, resp.StatusCode, body)
		}
	}

	dev := authenticatedClient(t, server.URL)
	status, created := requestJSON(t, dev, http.MethodPost, server.URL+"/api/v1/applications", map[string]any{"name": "Catalog Use", "subdomain": "catalog-use"})
	if status != http.StatusCreated {
		t.Fatalf("create Application: %d %v", status, created)
	}
	base := server.URL + "/api/v1/applications/" + created["application"].(map[string]any)["key"].(string) + "/environments/staging"
	scoreWith := func(resources map[string]any) map[string]any {
		score := previewScore("api")
		score["resources"] = resources
		return score
	}
	preview := func(resources map[string]any) (int, map[string]any) {
		return requestJSON(t, dev, http.MethodPost, base+"/score-preview", map[string]any{"workloadId": "api", "action": "deploy", "runId": "r1", "scoreAfter": scoreWith(resources)})
	}
	matched := func(view map[string]any) map[string]string {
		out := map[string]string{}
		for _, m := range view["matches"].([]any) {
			match := m.(map[string]any)
			out[match["definitionKey"].(string)] = match["descriptor"].(string)
		}
		return out
	}
	status, fast := preview(map[string]any{"db": map[string]any{"type": "postgres", "class": "fast", "params": map[string]any{"database": "shop", "username": "shop"}}})
	if status != http.StatusOK {
		t.Fatalf("preview fast: %d %v", status, fast)
	}
	if _, ok := matched(fast)["postgres-fast"]; !ok {
		t.Fatalf("new Definition not matched: %v", matched(fast))
	}
	status, standard := preview(map[string]any{"db": map[string]any{"type": "postgres", "params": map[string]any{"database": "shop", "username": "shop"}}})
	if _, ok := matched(standard)["postgres-fast"]; status != http.StatusOK || ok {
		t.Fatalf("class default must keep the seeded Definition: %d %v", status, matched(standard))
	}

	// The new Type's input schema validates Score params in planning at once
	// (score stage), but runtime drivers stay fixed: valid params then fail
	// only because no Definition can provision the new Type (graph stage).
	for _, params := range []map[string]any{{"size": "big"}, {}, {"size": 1, "extra": true}} {
		status, out := preview(map[string]any{"c": map[string]any{"type": "cache", "params": params}})
		if status != http.StatusBadRequest || !strings.Contains(toJSON(t, out), "Resource Type contracts") {
			t.Fatalf("invalid cache params %v: %d %v", params, status, out)
		}
	}
	status, rejected := preview(map[string]any{"c": map[string]any{"type": "cache", "params": map[string]any{"size": 2}}})
	if status != http.StatusBadRequest || !strings.Contains(toJSON(t, rejected), "Resource Graph cannot be built or matched") {
		t.Fatalf("cache without a Definition: %d %v", status, rejected)
	}
}

func TestUC02UC04ManagementHTTPContract(t *testing.T) {
	server, _ := newServer(t, "", approvedCluster{})
	pe := authenticatedClientAs(t, server.URL, "platform-engineer")
	dev := authenticatedClient(t, server.URL)
	typeBody := `{"key":"queue","inputs":[],"outputs":[{"name":"url","type":"string"}]}`
	for _, path := range []string{"/api/v1/resource-types", "/api/v1/resource-definitions", "/api/v1/connections/kubernetes"} {
		for name, body := range map[string]string{
			"unknown field": `{"key":"x","unexpected":true}`,
			"trailing":      `{"key":"x"} {}`,
			"null root":     `null`,
			"array root":    `[]`,
			"malformed":     `{"key":`,
		} {
			if resp, out := call(t, pe, http.MethodPost, server.URL+path, body); resp.StatusCode != http.StatusBadRequest {
				t.Errorf("%s %s = %d %s", path, name, resp.StatusCode, out)
			}
		}
		// Connection bodies carry a kubeconfig of up to 1 MiB, so their JSON
		// envelope is bounded at 8 MiB to allow escaping.
		limit := 1 << 20
		if path == "/api/v1/connections/kubernetes" {
			limit = 8 << 20
		}
		if resp, _ := call(t, pe, http.MethodPost, server.URL+path, `{"key":"`+strings.Repeat("x", limit)+`"}`); resp.StatusCode != http.StatusRequestEntityTooLarge {
			t.Errorf("%s oversize = %d", path, resp.StatusCode)
		}
		if resp, _ := call(t, dev, http.MethodPost, server.URL+path, typeBody); resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s developer = %d", path, resp.StatusCode)
		}
		if resp, _ := call(t, newJarClient(t), http.MethodPost, server.URL+path, typeBody); resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s anonymous = %d", path, resp.StatusCode)
		}
	}
	// Developers read Resource Types (UC-16 editor); Definitions and
	// Connections stay Platform Engineer/Admin.
	if resp, _ := call(t, dev, http.MethodGet, server.URL+"/api/v1/resource-types", ""); resp.StatusCode != http.StatusOK {
		t.Errorf("developer type list = %d", resp.StatusCode)
	}
	for _, path := range []string{"/api/v1/resource-definitions", "/api/v1/connections"} {
		if resp, _ := call(t, dev, http.MethodGet, server.URL+path, ""); resp.StatusCode != http.StatusForbidden {
			t.Errorf("developer %s = %d", path, resp.StatusCode)
		}
	}
	if resp, body := call(t, pe, http.MethodPost, server.URL+"/api/v1/resource-types", `{"key":"broken","outputs":[{"name":"url","type":"object"}]}`); resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, "unsupported type") {
		t.Errorf("invalid schema = %d %s", resp.StatusCode, body)
	}
	concurrentRegistration(t, server.URL, pe)
}

func TestUC02UC04ConcurrentRegistrationOnPostgres(t *testing.T) {
	databaseURL := persistencetest.FreshPostgresDatabase(t)
	opts := seed.Defaults()
	app, err := bootstrap.Build(context.Background(), bootstrap.Options{Seed: opts, Adapters: bootstrap.AdapterFake, DatabaseURL: databaseURL, ConnectionVerifierOverride: approvedCluster{}})
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
func concurrentRegistration(t *testing.T, baseURL string, client *http.Client) {
	t.Helper()
	for _, tc := range []struct{ path, list, a, b string }{
		{"/api/v1/resource-types", "/api/v1/resource-types", `{"key":"race-type","outputs":[{"name":"host","type":"string"}]}`, `{"key":"race-type","inputs":[{"name":"size","type":"number"}],"outputs":[{"name":"host","type":"string"}]}`},
		{"/api/v1/resource-definitions", "/api/v1/resource-definitions", strings.Replace(strings.Replace(fastPostgres, `"postgres-fast"`, `"race-def"`, 1), `,"connectionKey":"fast-cluster"`, ``, 1), strings.Replace(strings.Replace(strings.Replace(fastPostgres, `"postgres-fast"`, `"race-def"`, 1), `,"connectionKey":"fast-cluster"`, ``, 1), `{"class":"fast"}`, `{"class":"slow"},{}`, 1)},
		{"/api/v1/connections/kubernetes", "/api/v1/connections", `{"key":"race-cluster","clusterId":"race-a","kubeContext":"ctx-a"}`, `{"key":"race-cluster","clusterId":"race-b","kubeContext":"ctx-b"}`},
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
		want := map[bool]string{true: "race-a", false: "race-b"}[winner == 0]
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
		default:
			if !strings.Contains(body, want) || strings.Contains(body, map[bool]string{true: "race-b", false: "race-a"}[winner == 0]) {
				t.Fatalf("connection winner %d not stored: %s", winner, body)
			}
		}
	}
}
