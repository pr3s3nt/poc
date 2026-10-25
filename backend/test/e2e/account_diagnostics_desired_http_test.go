package e2e

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"orchestrator/internal/adapters/credentialmemory"
	"orchestrator/internal/application/accountdiag"
	ct "orchestrator/internal/application/connection/connectiontest"
	"orchestrator/internal/bootstrap"
	"orchestrator/internal/domain/application"
	domain "orchestrator/internal/domain/diagnostics"
	probes "orchestrator/internal/ports/diagnostics"
	"orchestrator/internal/seed"
	"orchestrator/internal/seed/seedtest"
)

// The provider boundary is a recording double; everything before it is real:
// HTTP, session, the Resource Type/Definition/draft registration API, the
// planner and the Desired snapshot. The test proves that a saved, NEVER
// deployed draft is planned into exact requirements (matched Definition, its
// literal StorageClass, the placement on the diagnosed account) before any
// probe runs, and that an invalid draft is a FAIL with planner enums.

type requirementsProbe struct {
	mu   sync.Mutex
	seen *probes.Requirements
}

func (p *requirementsProbe) Plan(domain.Scope, *probes.Requirements) []probes.Check {
	return []probes.Check{{ID: "k8s.api", Group: "access"}}
}
func (p *requirementsProbe) Open(context.Context, application.Connection, domain.Scope) probes.Session {
	return &requirementsSession{p: p}
}

type requirementsSession struct{ p *requirementsProbe }

func (s *requirementsSession) Run(_ context.Context, _ probes.Check, _ map[string]domain.Finding, req *probes.Requirements, _ probes.Recorder) domain.Finding {
	s.p.mu.Lock()
	s.p.seen = req
	s.p.mu.Unlock()
	return domain.Finding{Verdict: domain.VerdictPass, Reason: "API_ANSWERED"}
}
func (s *requirementsSession) Cleanup(_ context.Context, r []domain.ProbeResource, _ string) []domain.ProbeResource {
	return r
}
func (s *requirementsSession) Close() {}

func desiredServer(t *testing.T, provider probes.Provider) (*httptest.Server, seed.Options) {
	t.Helper()
	options := seedtest.Defaults()
	options.Catalog = seed.CatalogNone
	options.RunID = "run-desired"
	app, err := bootstrap.Build(context.Background(), bootstrap.Options{
		Seed: options, Adapters: bootstrap.AdapterFake, ConnectionCredentialsOverride: credentialmemory.New(), KubeconfigVerifierOverride: approvedCluster{},
		DiagnosticProvidersOverride: map[string]probes.Provider{"INTERNAL": provider}, DiagnosticConfigOverride: &accountdiag.Config{SweepEvery: time.Hour},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	server := httptest.NewServer(app.Server)
	t.Cleanup(server.Close)
	return server, options
}

func TestSavedUndeployedDraftIsPlannedIntoExactRequirementsBeforeAnyProbe(t *testing.T) {
	probe := &requirementsProbe{}
	server, opts := desiredServer(t, probe)
	pe := authenticatedClientAs(t, server.URL, "platform-engineer")
	api := server.URL + "/api/v1"
	field := func(name, typ string, required bool) map[string]any {
		return map[string]any{"name": name, "type": typ, "required": required}
	}
	post := func(path string, body map[string]any) map[string]any {
		t.Helper()
		status, out := requestJSON(t, pe, http.MethodPost, api+path, body)
		if status != http.StatusCreated && status != http.StatusOK {
			t.Fatalf("POST %s: %d %v", path, status, out)
		}
		return out
	}
	post("/resource-types", map[string]any{"key": "workload", "inputs": []any{}, "outputs": []any{}})
	post("/resource-types", map[string]any{"key": "k8s-namespace", "inputs": []any{}, "outputs": []any{field("name", "string", true)}})
	post("/resource-types", map[string]any{"key": "k8s-cluster", "inputs": []any{}, "outputs": []any{field("name", "string", true), field("endpoint", "string", false), field("cluster", "any", true), field("kubeContext", "string", true), field("kubeconfig", "string", false)}})
	post("/resource-types", map[string]any{"key": "postgres", "inputs": []any{field("database", "string", true), field("username", "string", true)},
		"outputs": []any{field("host", "string", true), field("port", "number", true), field("database", "string", true), field("username", "string", true), field("password", "string", true)}})
	post("/resource-types", map[string]any{"key": "redis", "inputs": []any{}, "outputs": []any{field("host", "string", true)}})
	account := post("/driver-accounts/internal", map[string]any{"name": "Desired lab", "kubeconfig": ct.Single("ctx", "https://cluster.example", "synthetic-token-0123456789"), "context": "ctx"})
	key, version := account["key"].(string), int64(account["version"].(float64))
	post("/resource-definitions", map[string]any{"key": "cluster-lab", "name": "Cluster lab", "resourceType": "k8s-cluster", "driverType": "existing-cluster", "driverAccount": key,
		"driverInputs": map[string]any{"values": map[string]any{"variables": map[string]any{}}}, "criteria": []any{map[string]any{"class": "internal", "app_id": opts.ApplicationKey}}})
	post("/resource-definitions", map[string]any{"key": "workload-lab", "name": "Workload lab", "resourceType": "workload", "driverType": "score-k8s",
		"driverInputs": map[string]any{"values": map[string]any{"variables": map[string]any{"render_bundle": "score-k8s-internal-v1", "namespace": namespaceOutput, "cluster": "${resources['k8s-cluster.internal#environments.@app.@env'].outputs.cluster}"}}},
		"criteria":     []any{map[string]any{}}})
	post("/resource-definitions", k8sModuleDefinition("ns-lab", "k8s-namespace", "k8s-namespace", map[string]any{"name": "${context.env.namespace}"}, nil, []string{"name"}, []any{map[string]any{}}))
	post("/resource-definitions", postgresModuleDefinition("pg-fast", map[string]any{"image": "postgres:17-alpine", "storage": "2Gi", "storage_class": `"fast"`, "namespace": namespaceOutput}, []any{map[string]any{}}))

	// A draft is saved and NOT deployed.
	draft := map[string]any{"apiVersion": "score.dev/v1b1", "metadata": map[string]any{"name": "api"},
		"containers": map[string]any{"main": map[string]any{"image": "example.invalid/api:v1"}},
		"service":    map[string]any{"ports": map[string]any{"http": map[string]any{"port": 8080}}, "publicRoutes": []any{map[string]any{"path": "/", "port": "http"}}},
		"resources":  map[string]any{"db": map[string]any{"type": "postgres", "class": "default", "params": map[string]any{"database": "app", "username": "app"}}}}
	base := api + "/applications/" + opts.ApplicationKey + "/environments/staging"
	if status, out := requestJSON(t, pe, http.MethodPut, base+"/workloads/api", map[string]any{"score": draft, "version": 0}); status != http.StatusOK {
		t.Fatalf("save draft: %d %v", status, out)
	}

	start := func(uses ...string) map[string]any {
		t.Helper()
		status, out := requestJSON(t, pe, http.MethodPost, api+"/driver-accounts/"+key+"/diagnostics", map[string]any{"version": version, "purpose": "USE_CASE", "mode": "CONDITIONS", "useCases": uses, "application": opts.ApplicationKey, "environment": "staging"})
		if status != http.StatusAccepted {
			t.Fatalf("start: %d %v", status, out)
		}
		id := out["id"].(string)
		for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(25 * time.Millisecond) {
			_, got := requestJSON(t, pe, http.MethodGet, api+"/driver-accounts/"+key+"/diagnostics/"+id, nil)
			if got["status"] == "COMPLETED" {
				return got
			}
		}
		t.Fatal("report never completed")
		return nil
	}
	byID := func(report map[string]any) map[string]map[string]any {
		out := map[string]map[string]any{}
		for _, f := range report["findings"].([]any) {
			m := f.(map[string]any)
			out[m["id"].(string)] = m
		}
		return out
	}

	report := start("database")
	findings := byID(report)
	plan := findings["uc.preview.requirements"]
	if plan == nil || plan["verdict"] != "PASS" || plan["reason"] != "PLAN_RESOLVED" {
		t.Fatalf("the saved draft was not planned before deployment: %v", plan)
	}
	probe.mu.Lock()
	req := probe.seen
	probe.mu.Unlock()
	if req == nil || !req.Resolved || req.Workloads != 1 || req.PublicRoutes != 1 || len(req.Resources) == 0 {
		t.Fatalf("requirements: %+v", req)
	}
	var pg *probes.ResourceNeed
	for i := range req.Resources {
		if req.Resources[i].Type == "postgres" {
			pg = &req.Resources[i]
		}
	}
	if pg == nil || pg.Definition != "pg-fast" || !pg.Packaged || pg.Module != "postgres-k8s" || !pg.Storage.Required || !pg.Storage.Resolved || pg.Storage.Class != "fast" || pg.Cluster != key {
		t.Fatalf("the matched Definition's exact storage class and placement were not derived: %+v", pg)
	}
	if got := findings["uc.database.requirements"]; got == nil || strings.Contains(got["reason"].(string), "NO_RESOURCE") {
		t.Fatalf("database requirement: %v", got)
	}
	scope := report["scope"].(map[string]any)["context"].(map[string]any)
	if scope["applicationKey"] != opts.ApplicationKey || scope["environmentKey"] != "staging" {
		t.Fatalf("context: %v", scope)
	}

	// A draft whose resource no Definition implements is an observed FAIL with planner enums, not an empty context;
	// a selection that needs no plan is not failed by it.
	bad := map[string]any{"apiVersion": "score.dev/v1b1", "metadata": map[string]any{"name": "api"}, "containers": map[string]any{"main": map[string]any{"image": "x:1"}},
		"resources": map[string]any{"cache": map[string]any{"type": "redis", "class": "default"}}}
	_, view := requestJSON(t, pe, http.MethodGet, base+"/workloads", nil)
	draftVersion := int64(view["draftVersion"].(float64))
	if status, out := requestJSON(t, pe, http.MethodPut, base+"/workloads/api", map[string]any{"score": bad, "version": draftVersion}); status != http.StatusOK {
		t.Fatalf("save invalid draft: %d %v", status, out)
	}
	rejected := byID(start("database"))["uc.preview.requirements"]
	if rejected["verdict"] != "FAIL" || rejected["reason"] != "PLANNING_REJECTED" || rejected["facts"].(map[string]any)["planReason"] == nil {
		t.Fatalf("a rejected draft must be a FAIL with planner enums: %v", rejected)
	}
	secretsOnly := byID(start("secrets"))
	if _, planned := secretsOnly["uc.preview.requirements"]; planned || secretsOnly["uc.secrets.workload"]["verdict"] == "PASS" {
		t.Fatalf("a secrets-only selection must neither plan the draft nor pass workload auth: %v", secretsOnly)
	}
}
