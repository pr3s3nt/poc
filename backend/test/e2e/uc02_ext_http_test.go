package e2e

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"orchestrator/internal/adapters/credentialmemory"
	"orchestrator/internal/adapters/postgres/postgrestest"
	"orchestrator/internal/bootstrap"
	"orchestrator/internal/seed/seedtest"
)

// UC-02 EXT (issue #52) over HTTP: a Platform Engineer adds optional outputs to
// an existing Type; Developers cannot; invalid or repeated names change nothing.
func TestUC02AddResourceTypeOutputsHTTP(t *testing.T) {
	server, _ := newServer(t, "")
	pe := authenticatedClientAs(t, server.URL, "platform-engineer")
	dev := authenticatedClientAs(t, server.URL, "developer")
	add := func(client *http.Client, key, body string) (int, string) {
		resp, text := call(t, client, http.MethodPost, server.URL+"/api/v1/resource-types/"+key+"/outputs", body)
		return resp.StatusCode, text
	}
	if status, _ := add(dev, "postgres", `{"outputs":[{"name":"dsn","type":"string","secret":true}]}`); status != http.StatusForbidden {
		t.Fatalf("developer: %d", status)
	}
	status, body := add(pe, "postgres", `{"outputs":[{"name":"dsn","type":"string","secret":true}]}`)
	if status != http.StatusOK || !strings.Contains(body, `"name":"dsn"`) || !strings.Contains(body, `"key":"postgres"`) {
		t.Fatalf("add: %d %s", status, body)
	}
	for name, tc := range map[string]struct {
		key, body string
		want      int
	}{
		"required flag is not accepted": {"postgres", `{"outputs":[{"name":"other","type":"string","required":true}]}`, http.StatusBadRequest},
		"unknown field":                 {"postgres", `{"outputs":[{"name":"other","type":"string"}],"inputs":[]}`, http.StatusBadRequest},
		"no output":                     {"postgres", `{"outputs":[]}`, http.StatusBadRequest},
		"unsupported type":              {"postgres", `{"outputs":[{"name":"other","type":"object"}]}`, http.StatusBadRequest},
		"existing name":                 {"postgres", `{"outputs":[{"name":"host","type":"string"}]}`, http.StatusConflict},
		"added name":                    {"postgres", `{"outputs":[{"name":"dsn","type":"string","secret":true}]}`, http.StatusConflict},
		"workload":                      {"workload", `{"outputs":[{"name":"other","type":"string"}]}`, http.StatusBadRequest},
		"missing type":                  {"no-such-type", `{"outputs":[{"name":"other","type":"string"}]}`, http.StatusNotFound},
	} {
		if status, text := add(pe, tc.key, tc.body); status != tc.want {
			t.Errorf("%s: %d %s", name, status, text)
		}
	}
	_, list := call(t, dev, http.MethodGet, server.URL+"/api/v1/resource-types", "")
	if strings.Count(list, `"name":"dsn"`) != 1 || strings.Contains(list, `"name":"other"`) {
		t.Fatalf("catalog after additions: %s", list)
	}
}

// Two concurrent additions of the same name on PostgreSQL: one 200, one 409,
// and the name is stored once.
func TestUC02AddResourceTypeOutputsConcurrentOnPostgres(t *testing.T) {
	databaseURL := postgrestest.Fresh(t).Console
	app, err := bootstrap.Build(context.Background(), bootstrap.Options{Seed: seedtest.Defaults(), Adapters: bootstrap.AdapterFake, DatabaseURL: databaseURL, KubeconfigVerifierOverride: approvedCluster{}, ConnectionCredentialsOverride: credentialmemory.New()})
	if err != nil {
		t.Fatal(err)
	}
	if closer, ok := app.Store.(interface{ Close() }); ok {
		t.Cleanup(closer.Close)
	}
	server := httptest.NewServer(app.Server)
	t.Cleanup(server.Close)
	client := authenticatedClientAs(t, server.URL, "platform-engineer")
	statuses := make([]int, 2)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i, body := range []string{`{"outputs":[{"name":"dsn","type":"string","secret":true}]}`, `{"outputs":[{"name":"dsn","type":"string"}]}`} {
		wg.Add(1)
		go func(i int, body string) {
			defer wg.Done()
			<-start
			req, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/resource-types/postgres/outputs", strings.NewReader(body))
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
	if !(statuses[0] == http.StatusOK && statuses[1] == http.StatusConflict) && !(statuses[1] == http.StatusOK && statuses[0] == http.StatusConflict) {
		t.Fatalf("statuses %v, want one 200 and one 409", statuses)
	}
	_, list := call(t, client, http.MethodGet, server.URL+"/api/v1/resource-types", "")
	if strings.Count(list, `"name":"dsn"`) != 1 {
		t.Fatalf("the added name must be stored once: %s", list)
	}
}
