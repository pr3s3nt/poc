package http

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/seed/seedtest"
	"strings"
	"testing"
)

func TestWorkloadDefaultsEndpointIsScopedPureAndPreservesDesired(t *testing.T) {
	_, server, store := bundleTestServer(t, nil)
	opts := seedtest.Defaults()
	if err := store.SaveEnvironment(context.Background(), environment.Environment{Key: "staging", ApplicationKey: opts.ApplicationKey, NamespaceIdentity: "fixture-staging", Version: 1}); err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/applications/" + opts.ApplicationKey + "/environments/staging/workloads/resolve-defaults"
	body := `{"workload":{"name":"web","image":"example:v1","replicas":0,"workloadExecution":{"version":1,"kind":"Deployment","defaults":{"version":"workload-defaults-v1","environment":"staging"},"automountServiceAccountToken":false,"revisionHistoryLimit":0}}}`
	client := bundleClient(t, server.URL, "developer")
	before, err := store.GetEnvironment(context.Background(), opts.ApplicationKey, "staging")
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Post(server.URL+path, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatalf("%d %s", response.StatusCode, raw)
	}
	var result struct {
		Workload map[string]any
		Sources  map[string]string
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	e := result.Workload["workloadExecution"].(map[string]any)
	if result.Workload["replicas"] != float64(0) || e["automountServiceAccountToken"] != false || e["revisionHistoryLimit"] != float64(0) || result.Sources["containers.main.resources.requests.cpu"] != "environment" {
		t.Fatal("resolver semantics", string(raw))
	}
	after, err := store.GetEnvironment(context.Background(), opts.ApplicationKey, "staging")
	if err != nil {
		t.Fatal(err)
	}
	drafts, err := store.ListWorkloadDrafts(context.Background(), opts.ApplicationKey, "staging")
	if err != nil || len(drafts) != 0 || after.DraftVersion != before.DraftVersion || after.Version != before.Version {
		t.Fatal("resolution changed desired state")
	}
	for _, invalid := range []string{`{}`, `{"workload":null}`, `{"workload":{},"unknown":true}`} {
		r, err := client.Post(server.URL+path, "application/json", strings.NewReader(invalid))
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		if r.StatusCode != http.StatusBadRequest {
			t.Fatalf("invalid resolver envelope returned %d", r.StatusCode)
		}
	}
	foreign := bundleClient(t, server.URL, "other-admin")
	reply, err := foreign.Post(server.URL+path, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	reply.Body.Close()
	if reply.StatusCode == http.StatusOK {
		t.Fatal("foreign org resolved workload")
	}
}
