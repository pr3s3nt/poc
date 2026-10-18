package e2e

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"orchestrator/internal/adapters/credentialmemory"
	ct "orchestrator/internal/application/connection/connectiontest"
	"orchestrator/internal/bootstrap"
	"orchestrator/internal/seed"
	"orchestrator/internal/seed/seedtest"
)

// newCatalogFreeServer builds the fake-adapter product with the catalog-free
// seed (no Types, Definitions or Connections) and an always-approving
// Kubernetes verifier for Internal accounts. Setup only: nothing is deployed.
func newCatalogFreeServer(t *testing.T) (*httptest.Server, seed.Options) {
	t.Helper()
	options := seedtest.Defaults()
	options.Catalog = seed.CatalogNone
	options.RunID = "run-provision"
	app, err := bootstrap.Build(context.Background(), bootstrap.Options{
		Seed: options, Adapters: bootstrap.AdapterFake, ConnectionCredentialsOverride: credentialmemory.New(),
		KubeconfigVerifierOverride: approvedCluster{},
	})
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	server := httptest.NewServer(app.Server)
	t.Cleanup(server.Close)
	return server, options
}

// TestV02ProvisionStoredThroughHTTPDrivesPreviewPlan proves that the provision
// rules a Platform Engineer stores through the current Resource Definition
// contract (UC-03 BR-26) are the ones the planner consumes in a real Score
// Preview (UC-05/UC-06 BR-08): companion descriptor and params, both
// is_dependent states, both match_dependents states, consumer -> provider edges
// and provider-first batches. Nothing is deployed and no executor runs; the
// Preview only reads the stored configuration.
func TestV02ProvisionStoredThroughHTTPDrivesPreviewPlan(t *testing.T) {
	// Catalog-free server, like the V02 recorded scenario: the Types, the
	// Definitions and the Environment connection are authored through the API.
	server, opts := newCatalogFreeServer(t)
	pe := authenticatedClientAs(t, server.URL, "platform-engineer")
	dev := authenticatedClient(t, server.URL)
	api := server.URL + "/api/v1"

	register := func(typ map[string]any) {
		t.Helper()
		if status, out := requestJSON(t, pe, http.MethodPost, api+"/resource-types", typ); status != http.StatusCreated {
			t.Fatalf("register type %v: %d %v", typ["key"], status, out)
		}
	}
	field := func(name, typ string, required bool) map[string]any {
		return map[string]any{"name": name, "type": typ, "required": required}
	}
	register(map[string]any{"key": "workload", "inputs": []any{}, "outputs": []any{}})
	register(map[string]any{"key": "k8s-namespace", "inputs": []any{}, "outputs": []any{field("name", "string", true)}})
	register(map[string]any{"key": "k8s-cluster", "inputs": []any{}, "outputs": []any{field("name", "string", true), field("endpoint", "string", false), field("cluster", "any", true), field("kubeContext", "string", true), field("kubeconfig", "string", false)}})
	register(map[string]any{"key": "postgres", "inputs": []any{field("database", "string", true), field("username", "string", true), field("labels", "any", false)},
		"outputs": []any{field("host", "string", true), field("port", "number", true), field("database", "string", true), field("username", "string", true), field("password", "string", true)}})
	outputs := []any{}
	for _, o := range seed.RedisType().Outputs {
		outputs = append(outputs, field(o.Name, o.Type, o.Required))
	}
	register(map[string]any{"key": "redis", "inputs": []any{}, "outputs": outputs})

	// The Environment connection is a fake-verified Internal account (setup only).
	status, account := requestJSON(t, pe, http.MethodPost, api+"/driver-accounts/internal", map[string]any{"name": "Provision lab", "kubeconfig": ct.Single("ctx", "https://cluster.example", "synthetic-token-0123456789"), "context": "ctx"})
	if status != http.StatusCreated {
		t.Fatalf("internal account: %d %v", status, account)
	}
	// ADR-016: Definitions place the Application; the Environment carries no connection.
	cluster := map[string]any{"key": "cluster-lab", "name": "Cluster lab", "resourceType": "k8s-cluster", "driverType": "existing-cluster", "driverAccount": account["key"],
		"driverInputs": map[string]any{"values": map[string]any{"variables": map[string]any{}}}, "criteria": []any{map[string]any{"class": "internal", "app_id": opts.ApplicationKey}}}
	if status, out := requestJSON(t, pe, http.MethodPost, api+"/resource-definitions", cluster); status != http.StatusCreated {
		t.Fatalf("register cluster definition: %d %v", status, out)
	}
	workload := map[string]any{"key": "workload-lab", "name": "Workload lab", "resourceType": "workload", "driverType": "score-k8s",
		"driverInputs": map[string]any{"values": map[string]any{"variables": map[string]any{"render_bundle": "score-k8s-internal-v1", "namespace": "${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}", "cluster": "${resources['k8s-cluster.internal#environments.@app.@env'].outputs.cluster}"}}}, "criteria": []any{map[string]any{}}}
	if status, out := requestJSON(t, pe, http.MethodPost, api+"/resource-definitions", workload); status != http.StatusCreated {
		t.Fatalf("register workload definition: %d %v", status, out)
	}
	namespace := k8sModuleDefinition("ns-lab", "k8s-namespace", "k8s-namespace", map[string]any{"name": "${context.env.namespace}"}, nil, []string{"name"}, []any{map[string]any{}})
	if status, out := requestJSON(t, pe, http.MethodPost, api+"/resource-definitions", namespace); status != http.StatusCreated {
		t.Fatalf("register namespace definition: %d %v", status, out)
	}
	// Companion Definition: the verified in-cluster PostgreSQL module, mapping the nested labels param.
	companion := k8sModuleDefinition("pg-companion", "postgres", "postgres-k8s", map[string]any{"namespace": namespaceOutput}, []string{"database", "username", "labels"}, postgresOutputs, []any{map[string]any{"class": "companion"}})
	if status, out := requestJSON(t, pe, http.MethodPost, api+"/resource-definitions", companion); status != http.StatusCreated {
		t.Fatalf("register companion definition: %d %v", status, out)
	}

	const (
		depOnly   = "postgres.companion#shared.dep-only"
		matchOnly = "postgres.companion#shared.match-only"
		plain     = "postgres.companion#shared.plain"
		parent    = "redis.prov#modules.api.externals.cache"
		consumer  = "workload.default#modules.api"
	)
	paramsFor := func(db string) map[string]any {
		return map[string]any{"database": db, "username": "app", "labels": map[string]any{"tier": "db", "team": db}}
	}
	parentBody := func(provision map[string]any) map[string]any {
		def := k8sModuleDefinition("redis-prov", "redis", "redis-k8s", map[string]any{"namespace": namespaceOutput}, nil, []string{"host", "port", "username", "password", "tls"}, []any{map[string]any{"class": "prov"}})
		def["provision"] = provision
		return def
	}
	initial := map[string]any{
		depOnly:   map[string]any{"is_dependent": true, "params": paramsFor("dep")},
		matchOnly: map[string]any{"match_dependents": true, "params": paramsFor("match")},
		plain:     map[string]any{"params": paramsFor("plain")},
	}
	status, created := requestJSON(t, pe, http.MethodPost, api+"/resource-definitions", parentBody(initial))
	if status != http.StatusCreated {
		t.Fatalf("register parent definition: %d %v", status, created)
	}
	storedDefinition := func() map[string]any {
		t.Helper()
		_, list := requestJSON(t, pe, http.MethodGet, api+"/resource-definitions", nil)
		for _, item := range list["resourceDefinitions"].([]any) {
			if def := item.(map[string]any); def["key"] == "redis-prov" {
				return def
			}
		}
		t.Fatal("redis-prov is not stored")
		return nil
	}
	if got := storedDefinition()["provision"]; !reflect.DeepEqual(got, any(initial)) {
		t.Fatalf("stored provision = %v, want %v", got, initial)
	}

	key := opts.ApplicationKey
	score := map[string]any{
		"apiVersion": "score.dev/v1b1", "metadata": map[string]any{"name": "api"},
		"containers": map[string]any{"main": map[string]any{"image": "example.invalid/api:v1"}},
		"resources":  map[string]any{"cache": map[string]any{"type": "redis", "class": "prov"}},
	}
	type plan struct {
		edges   map[[3]string]bool
		params  map[string][]string
		batches [][]string
	}
	preview := func() plan {
		t.Helper()
		status, out := requestJSON(t, dev, http.MethodPost, api+"/applications/"+key+"/environments/staging/score-preview",
			map[string]any{"workloadId": "api", "action": "deploy", "runId": "run-provision", "scoreAfter": score})
		if status != http.StatusOK {
			t.Fatalf("preview: %d %v", status, out)
		}
		got := plan{edges: map[[3]string]bool{}, params: map[string][]string{}}
		for _, e := range out["graph"].(map[string]any)["edges"].([]any) {
			edge := e.(map[string]any)
			got.edges[[3]string{edge["consumer"].(string), edge["provider"].(string), edge["reason"].(string)}] = true
		}
		for _, n := range out["graph"].(map[string]any)["nodes"].([]any) {
			node := n.(map[string]any)
			keys := []string{}
			for _, k := range node["paramKeys"].([]any) {
				keys = append(keys, k.(string))
			}
			got.params[node["descriptor"].(string)] = keys
		}
		for _, b := range out["batches"].([]any) {
			batch := []string{}
			for _, d := range b.([]any) {
				batch = append(batch, d.(string))
			}
			got.batches = append(got.batches, batch)
		}
		return got
	}
	batchOf := func(p plan, descriptor string) int {
		for i, batch := range p.batches {
			for _, d := range batch {
				if d == descriptor {
					return i
				}
			}
		}
		return -1
	}
	const isDependent, matchDependents = "co-provision-is-dependent", "co-provision-match-dependents"

	first := preview()
	for _, companionID := range []string{depOnly, matchOnly, plain} {
		if !reflect.DeepEqual(first.params[companionID], []string{"database", "labels", "username"}) {
			t.Fatalf("%s params = %v, want database, labels and username", companionID, first.params[companionID])
		}
		if batchOf(first, companionID) < 0 {
			t.Fatalf("%s is not scheduled: %v", companionID, first.batches)
		}
	}
	for edge, want := range map[[3]string]bool{
		{depOnly, parent, isDependent}:               true,  // is_dependent true: companion -> parent
		{matchOnly, parent, isDependent}:             false, // is_dependent absent
		{plain, parent, isDependent}:                 false,
		{consumer, depOnly, matchDependents}:         false, // match_dependents absent
		{consumer, matchOnly, matchDependents}:       true,  // match_dependents true: consumer -> companion
		{consumer, plain, matchDependents}:           false,
		{parent, depOnly, isDependent}:               false, // never provider -> consumer
		{matchOnly, consumer, matchDependents}:       false,
		{depOnly, matchOnly, matchDependents}:        true, // the is_dependent companion now consumes the parent too
		{depOnly, plain, matchDependents}:            false,
		{matchOnly, depOnly, matchDependents}:        false,
		{consumer, parent, "deployment-set-private"}: true,
	} {
		if first.edges[edge] != want {
			t.Fatalf("edge %v present = %v, want %v", edge, first.edges[edge], want)
		}
	}
	if batchOf(first, depOnly) <= batchOf(first, parent) {
		t.Fatalf("is_dependent: parent must be scheduled first: %v", first.batches)
	}
	if batchOf(first, depOnly) <= batchOf(first, matchOnly) {
		t.Fatalf("match_dependents: companion must be scheduled before its consumers: %v", first.batches)
	}

	// Toggle the flags through a version-CAS edit: the next Preview follows.
	toggled := map[string]any{
		depOnly:   map[string]any{"match_dependents": true, "params": paramsFor("dep")},
		matchOnly: map[string]any{"is_dependent": true, "params": paramsFor("match")},
		plain:     map[string]any{"params": paramsFor("plain")},
	}
	before := storedDefinition()
	edit := parentBody(toggled)
	edit["version"] = before["version"]
	if status, out := requestJSON(t, pe, http.MethodPut, api+"/resource-definitions/redis-prov", edit); status != http.StatusOK {
		t.Fatalf("edit provision: %d %v", status, out)
	}
	after := storedDefinition()
	if after["version"].(float64) != before["version"].(float64)+1 || !reflect.DeepEqual(after["provision"], any(toggled)) {
		t.Fatalf("stored edit = version %v provision %v", after["version"], after["provision"])
	}
	for _, field := range []string{"key", "name", "resourceType", "driverType", "driverInputs", "criteria"} {
		if !reflect.DeepEqual(before[field], after[field]) {
			t.Fatalf("editing provision changed %s: %v -> %v", field, before[field], after[field])
		}
	}
	if status, _ := requestJSON(t, pe, http.MethodPut, api+"/resource-definitions/redis-prov", edit); status != http.StatusConflict {
		t.Fatalf("stale version edit = %d, want 409", status)
	}
	second := preview()
	for edge, want := range map[[3]string]bool{
		{depOnly, parent, isDependent}:         false,
		{matchOnly, parent, isDependent}:       true,
		{consumer, depOnly, matchDependents}:   true,
		{consumer, matchOnly, matchDependents}: false,
		{matchOnly, depOnly, matchDependents}:  true,
		{depOnly, matchOnly, matchDependents}:  false,
	} {
		if second.edges[edge] != want {
			t.Fatalf("after toggle: edge %v present = %v, want %v", edge, second.edges[edge], want)
		}
	}
	if batchOf(second, matchOnly) <= batchOf(second, parent) || batchOf(second, matchOnly) <= batchOf(second, depOnly) {
		t.Fatalf("after toggle: provider-first batches broken: %v", second.batches)
	}

	// Two companions that both set both flags consume each other: the planner
	// rejects the cycle at the schedule stage (planner reference, no invention).
	cyclic := parentBody(map[string]any{
		depOnly:   map[string]any{"is_dependent": true, "match_dependents": true, "params": paramsFor("dep")},
		matchOnly: map[string]any{"is_dependent": true, "match_dependents": true, "params": paramsFor("match")},
	})
	cyclic["version"] = after["version"]
	if status, out := requestJSON(t, pe, http.MethodPut, api+"/resource-definitions/redis-prov", cyclic); status != http.StatusOK {
		t.Fatalf("store cyclic provision: %d %v", status, out)
	}
	status, out := requestJSON(t, dev, http.MethodPost, api+"/applications/"+key+"/environments/staging/score-preview",
		map[string]any{"workloadId": "api", "action": "deploy", "runId": "run-cycle", "scoreAfter": score})
	if params, _ := out["params"].(map[string]any); status != http.StatusBadRequest || out["code"] != "PLANNING_REJECTED" || params["stage"] != "schedule" {
		t.Fatalf("cycle preview = %d %v, want PLANNING_REJECTED at the schedule stage", status, out)
	}
}

// TestV02ProvisionRegistrationPolicyHTTP covers the provision rules the
// registration contract already refuses (UC-03 BR-26, V02-01): a malformed
// descriptor, an unregistered Type, workload rendering as companion and a rule
// that is not an object. Refused writes leave the stored Definition unchanged;
// nested typed params and explicit empty values are stored as the contract
// keeps them.
func TestV02ProvisionRegistrationPolicyHTTP(t *testing.T) {
	server, _ := newServer(t, "", approvedCluster{})
	pe := authenticatedClientAs(t, server.URL, "platform-engineer")
	api := server.URL + "/api/v1/resource-definitions"
	body := func(provision any) map[string]any {
		def := postgresModuleDefinition("pg-policy-prov", map[string]any{"namespace": namespaceOutput}, []any{map[string]any{"class": "policy"}})
		def["provision"] = provision
		return def
	}
	for name, provision := range map[string]any{
		"unknown type":      map[string]any{"nope.default#shared.x": map[string]any{}},
		"workload":          map[string]any{"workload.default#shared.x": map[string]any{}},
		"malformed":         map[string]any{"Bad Descriptor!": map[string]any{}},
		"empty descriptor":  map[string]any{"": map[string]any{}},
		"rule not object":   map[string]any{"k8s-namespace.default#shared.x": "yes"},
		"flag not boolean":  map[string]any{"k8s-namespace.default#shared.x": map[string]any{"is_dependent": "yes"}},
		"unknown rule key":  map[string]any{"k8s-namespace.default#shared.x": map[string]any{"depends": true}},
		"params not object": map[string]any{"k8s-namespace.default#shared.x": map[string]any{"params": []any{"a"}}},
	} {
		if status, out := requestJSON(t, pe, http.MethodPost, api, body(provision)); status != http.StatusBadRequest {
			t.Errorf("%s: %d %v, want 400", name, status, out)
		}
	}
	_, stored := requestJSON(t, pe, http.MethodGet, api, nil)
	for _, item := range stored["resourceDefinitions"].([]any) {
		if item.(map[string]any)["key"] == "pg-policy-prov" {
			t.Fatal("a refused registration was stored")
		}
	}
	nested := map[string]any{
		"k8s-namespace.default#shared.nested": map[string]any{
			"is_dependent": true, "match_dependents": false,
			"params": map[string]any{"labels": map[string]any{"tier": "db", "tags": []any{"a", map[string]any{"deep": true}}}, "count": float64(3)},
		},
		// Explicit defaults: both flags false and empty params.
		"k8s-namespace.default#shared.empty": map[string]any{"is_dependent": false, "match_dependents": false, "params": map[string]any{}},
	}
	status, created := requestJSON(t, pe, http.MethodPost, api, body(nested))
	if status != http.StatusCreated {
		t.Fatalf("nested params registration: %d %v", status, created)
	}
	want := map[string]any{
		"k8s-namespace.default#shared.nested": map[string]any{
			"is_dependent": true, // explicit false is the omitted default (omitempty), true stays
			"params":       nested["k8s-namespace.default#shared.nested"].(map[string]any)["params"],
		},
		"k8s-namespace.default#shared.empty": map[string]any{}, // explicit false flags and empty params normalize to the empty rule
	}
	if !reflect.DeepEqual(created["provision"], any(want)) {
		t.Fatalf("stored provision = %v, want %v", created["provision"], want)
	}
	// A refused edit leaves version and provision untouched.
	bad := body(map[string]any{"nope.default#shared.x": map[string]any{}})
	bad["version"] = created["version"]
	if status, _ := requestJSON(t, pe, http.MethodPut, api+"/pg-policy-prov", bad); status != http.StatusBadRequest {
		t.Fatalf("refused edit = %d, want 400", status)
	}
	_, list := requestJSON(t, pe, http.MethodGet, api, nil)
	for _, item := range list["resourceDefinitions"].([]any) {
		if def := item.(map[string]any); def["key"] == "pg-policy-prov" && (def["version"] != created["version"] || !reflect.DeepEqual(def["provision"], any(want))) {
			t.Fatalf("a refused edit changed the stored Definition: %v", def)
		}
	}
}
