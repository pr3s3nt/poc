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
func TestUC02UC04RegistrationIsUsableWithoutRestart(t *testing.T) {
	server, _ := newServer(t, "", approvedCluster{})
	pe := authenticatedClientAs(t, server.URL, "platform-engineer")
	for _, tc := range []struct{ path, body string }{
		{"/api/v1/connections/kubernetes", uploadBody("Fast cluster")},
		{"/api/v1/resource-definitions", fastPostgres},
		{"/api/v1/resource-types", `{"key":"cache","inputs":[{"name":"size","type":"number","required":true}],"outputs":[{"name":"host","type":"string","required":true}]}`},
	} {
		if resp, body := call(t, pe, http.MethodPost, server.URL+tc.path, tc.body); resp.StatusCode != http.StatusCreated {
			t.Fatalf("register %s = %d %s", tc.path, resp.StatusCode, body)
		}
	}

	dev := authenticatedClient(t, server.URL)
	created := createConfiguredApplication(t, dev, server.URL, "Catalog Use", "catalog-use", "fast-cluster")
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
		if status != http.StatusBadRequest || out["code"] != "INVALID_SCORE" || toJSON(t, out["params"]) != `{"stage":"score"}` {
			t.Fatalf("invalid cache params %v: %d %v", params, status, out)
		}
	}
	status, rejected := preview(map[string]any{"c": map[string]any{"type": "cache", "params": map[string]any{"size": 2}}})
	if status != http.StatusBadRequest || rejected["code"] != "PLANNING_REJECTED" || toJSON(t, rejected["params"]) != `{"stage":"graph"}` {
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
	if resp, body := call(t, pe, http.MethodPost, server.URL+"/api/v1/resource-types", `{"key":"broken","outputs":[{"name":"url","type":"object"}]}`); resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, `"code":"INVALID_REQUEST"`) || !strings.Contains(body, `"field":"type.schema"`) || strings.Contains(body, "unsupported type") {
		t.Errorf("invalid schema = %d %s", resp.StatusCode, body)
	}
	concurrentRegistration(t, server.URL, pe)
}

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
		{"/api/v1/resource-definitions", "/api/v1/resource-definitions", postgresBody("race-def", "", []any{map[string]any{"class": "fast"}}), postgresBody("race-def", "", []any{map[string]any{"class": "slow"}, map[string]any{}})},
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
func TestPublicRegistrationRejectsExistingClusterDriver(t *testing.T) {
	server, _ := newServer(t, "", approvedCluster{})
	pe := authenticatedClientAs(t, server.URL, "platform-engineer")
	if resp, body := call(t, pe, http.MethodPost, server.URL+"/api/v1/connections/kubernetes", uploadBody("Fast cluster")); resp.StatusCode != http.StatusCreated {
		t.Fatalf("connection = %d %s", resp.StatusCode, body)
	}
	list := func() string {
		resp, body := call(t, pe, http.MethodGet, server.URL+"/api/v1/resource-definitions", "")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("list = %d %s", resp.StatusCode, body)
		}
		return body
	}
	before := list()
	for name, key := range map[string]string{"arbitrary key": "cluster-fast", "reserved key": "builtin-existing-cluster"} {
		for _, connection := range []string{`,"connectionKey":"fast-cluster"`, ``, `,"connectionKey":"missing"`} {
			body := `{"key":"` + key + `","resourceType":"k8s-cluster","executionProfile":"internal-k8s","driverType":"existing-cluster"` + connection + `,` +
				`"driverInputs":{"values":{"variables":{"name":"${context.connection.cluster}","kubeContext":"${context.connection.context}"}}},"criteria":[{"class":"internal"}]}`
			if resp, out := call(t, pe, http.MethodPost, server.URL+"/api/v1/resource-definitions", body); resp.StatusCode != http.StatusBadRequest {
				t.Errorf("%s %q: status %d %s, want 400", name, connection, resp.StatusCode, out)
			}
		}
	}
	reserved := postgresBody("builtin-existing-cluster", "fast-cluster", []any{map[string]any{"class": "fast"}})
	if resp, out := call(t, pe, http.MethodPost, server.URL+"/api/v1/resource-definitions", reserved); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("reserved key with kubernetes driver: %d %s, want 400", resp.StatusCode, out)
	}
	if after := list(); after != before {
		t.Fatalf("definitions changed after rejected registrations")
	}
}
