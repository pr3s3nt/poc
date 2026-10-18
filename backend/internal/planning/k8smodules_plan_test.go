package planning_test

import (
	"errors"
	"orchestrator/internal/seed/seedtest"
	"strings"
	"testing"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/adapters/terraform/registry"
	"orchestrator/internal/adapters/terraform/registry/registrytest"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/planning"
	"orchestrator/internal/planning/score"
	"orchestrator/internal/seed"
)

func k8sTerraformDefinition(key, typ string, ref tfmodule.SourceRef, variables map[string]any, inputs []string, outputs []string) resource.Definition {
	inputMapping := map[string]any{}
	for _, name := range inputs {
		inputMapping[name] = map[string]any{"param": name}
	}
	outputMapping := map[string]any{}
	for _, name := range outputs {
		outputMapping[name] = name
	}
	return resource.Definition{
		Key: key, Name: key, Version: 1, ResourceTypeKey: typ, DriverType: resource.DriverTerraform,
		DriverInputs: map[string]any{"values": map[string]any{
			"source": ref.Map(), "variables": variables, "input_mapping": inputMapping, "output_mapping": outputMapping,
			"provider_bindings": map[string]any{"kubernetes": map[string]any{"target": tfmodule.TargetResource, "resource_ref": "k8s-cluster.internal#environments.@app.@env"}},
		}},
		Criteria: []resource.Criterion{{}},
	}
}

// k8sRequest plans one backend workload that uses a shared PostgreSQL and a
// shared Redis, both in-cluster Terraform modules over the REAL registry
// bundles, with the test fixture that binds a Redis metadata annotation to the
// shared PostgreSQL output.
func k8sRequest(t *testing.T, annotation bool) planning.Request {
	t.Helper()
	var annotations map[string]any
	if annotation {
		annotations = map[string]any{"example.test/postgres-host": "${resources['postgres.default#shared.acceptance-db'].outputs.host}"}
	}
	return k8sRequestWith(t, annotations, nil)
}

// k8sRequestWith plans the same graph with arbitrary Redis annotations and an
// optional adjustment of the catalog (for example a Type classification).
func k8sRequestWith(t *testing.T, annotations map[string]any, adjust func(*planning.Catalog)) planning.Request {
	t.Helper()
	annotation := annotations != nil
	refs, err := registry.PackagedRefs()
	if err != nil {
		t.Fatal(err)
	}
	o := seedtest.Defaults()
	o.RunID = "run-test"
	nsOut := "${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}"
	redisVars := map[string]any{"namespace": nsOut}
	if annotation {
		redisVars["annotations"] = annotations
	}
	catalog := planning.Catalog{Types: map[string]resource.Type{}, Accounts: map[string]application.Connection{
		o.ConnectionKey: {Key: o.ConnectionKey, Kind: application.ConnectionKubernetes, OrganizationKey: o.OrganizationKey, Status: application.ConnectionReady, Version: 2},
	}}
	for _, typ := range seed.ResourceTypes() {
		catalog.Types[typ.Key] = typ
	}
	catalog.Types["redis"] = resource.Type{Key: "redis", Outputs: []resource.OutputField{
		{Name: "host", Type: "string", Required: true}, {Name: "port", Type: "number", Required: true}, {Name: "username", Type: "string", Required: true},
		{Name: "password", Type: "string", Required: true, Secret: true}, {Name: "tls", Type: "bool", Required: true}}}
	for _, d := range seed.ResourceDefinitions(o) {
		if d.Key != "namespace-kubernetes" && d.Key != "namespace-kubernetes-internal" && d.Key != "postgres-internal-statefulset" {
			catalog.Definitions = append(catalog.Definitions, d)
		}
	}
	catalog.Definitions = append(catalog.Definitions,
		k8sTerraformDefinition("namespace-k8s-module", "k8s-namespace", refs["k8s-namespace"], map[string]any{"name": "${context.env.namespace}"}, nil, []string{"name"}),
		k8sTerraformDefinition("postgres-k8s-module", "postgres", refs["postgres-k8s"], map[string]any{"namespace": nsOut}, []string{"database", "username"}, []string{"host", "port", "database", "username", "password"}),
		k8sTerraformDefinition("redis-k8s-module", "redis", refs["redis-k8s"], redisVars, nil, []string{"host", "port", "username", "password", "tls"}),
	)
	if adjust != nil {
		adjust(&catalog)
	}
	scoreMap := seed.AcceptanceScores(o)["backend"]
	resources := scoreMap["resources"].(map[string]any)
	resources["cache"] = map[string]any{"type": "redis", "class": "default", "id": "acceptance-cache"}
	doc, err := score.FromMap(scoreMap)
	if err != nil {
		t.Fatal(err)
	}
	return planning.Request{
		OrganizationKey: o.OrganizationKey,
		App:             application.Application{Key: o.ApplicationKey, OrganizationKey: o.OrganizationKey, Name: o.ApplicationKey, Version: 1},
		Env: environment.Environment{Key: o.EnvironmentKey, ApplicationKey: o.ApplicationKey, Name: o.EnvironmentName, Type: o.EnvironmentType, NamespaceIdentity: o.NamespaceIdentity, Version: 1,
			InfrastructureScope: environment.ScopeEnvironment},
		BaseSet: environment.NewDocument(), After: doc, WorkloadID: "backend", RunID: o.RunID, Catalog: catalog,
		Terraform: registrytest.Inspector(t, store.New()),
	}
}

func hasEdge(plan *planning.Plan, consumer, provider, reason string) bool {
	for _, e := range plan.Graph.Edges {
		if strings.HasPrefix(e.Consumer, consumer) && e.Provider == provider && string(e.Reason) == reason {
			return true
		}
	}
	return false
}

func batchOf(plan *planning.Plan, descriptor string) int {
	for i, batch := range plan.Batches {
		for _, d := range batch {
			if d == descriptor {
				return i
			}
		}
	}
	return -1
}

// The in-cluster modules plan over their REAL verified bundles: provider
// (cluster) -> namespace -> PostgreSQL/Redis, the shared descriptors are the
// ones the form uses, and the fixture annotation reference creates a Redis ->
// PostgreSQL edge to the SAME shared PostgreSQL node (never a second one).
func TestKubernetesModulesPlanWithRealBundles(t *testing.T) {
	const (
		cluster = "k8s-cluster.internal#environments.acceptance.dev"
		ns      = "k8s-namespace.default#environments.acceptance.dev"
		pg      = "postgres.default#shared.acceptance-db"
		redis   = "redis.default#shared.acceptance-cache"
	)
	plan, err := planning.NewService(placedBundles()).Plan(k8sRequest(t, true))
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, n := range plan.Graph.Nodes {
		counts[strings.SplitN(n.Descriptor, ".", 2)[0]]++
	}
	if counts["postgres"] != 1 || counts["redis"] != 1 || counts["k8s-namespace"] != 1 {
		t.Fatalf("node counts %v", counts)
	}
	for _, want := range [][3]string{
		{ns, cluster, "provider-binding"}, {pg, cluster, "provider-binding"}, {redis, cluster, "provider-binding"},
		{pg, ns, "resource-reference"}, {redis, ns, "resource-reference"}, {redis, pg, "resource-reference"},
	} {
		if !hasEdge(plan, want[0], want[1], want[2]) {
			t.Errorf("missing %s edge %s -> %s", want[2], want[0], want[1])
		}
	}
	if !(batchOf(plan, cluster) < batchOf(plan, ns) && batchOf(plan, ns) < batchOf(plan, pg) && batchOf(plan, pg) < batchOf(plan, redis)) {
		t.Fatalf("batches %v", plan.Batches)
	}
	byDefinition := map[string]planning.TerraformContract{}
	for _, c := range plan.Terraform {
		byDefinition[c.DefinitionKey] = c
	}
	redisContract := byDefinition["redis-k8s-module"]
	if redisContract.Module != "redis-k8s" || len(redisContract.ProviderBindings) != 1 || redisContract.ProviderBindings[0].Kind != tfmodule.BindingKubernetes ||
		len(redisContract.SecretOutputs) != 1 || redisContract.SecretOutputs[0] != "password" {
		t.Fatalf("redis contract %+v", redisContract)
	}
	if len(byDefinition) != 3 {
		t.Fatalf("contracts %v", byDefinition)
	}

	// Without the fixture annotation the production graph has no Redis ->
	// PostgreSQL dependency: it is a test graph, not a product requirement.
	plain, err := planning.NewService(placedBundles()).Plan(k8sRequest(t, false))
	if err != nil {
		t.Fatal(err)
	}
	if hasEdge(plain, redis, pg, "resource-reference") {
		t.Fatal("Redis depends on PostgreSQL without the fixture reference")
	}
	if !hasEdge(plain, redis, ns, "resource-reference") {
		t.Fatal("Redis lost its namespace dependency")
	}
}

// Secret sources never reach a nonsecret module input at planning time: the
// PostgreSQL Resource Type secret password is refused in Redis annotations
// (direct, nested, interpolated), and a MODULE-only secret (Type says nonsecret,
// the module marks the output sensitive) is refused the same way. The
// nonsecret host fixture keeps planning.
func TestKubernetesModulesPlanRejectsSecretOutputsInNonsecretInputs(t *testing.T) {
	password := "${resources['postgres.default#shared.acceptance-db'].outputs.password}"
	host := "${resources['postgres.default#shared.acceptance-db'].outputs.host}"
	values := map[string]map[string]any{
		"direct":       {"example.test/k": password},
		"interpolated": {"example.test/k": "pw=" + password + ";host=" + host},
		"nested":       {"example.test/k": map[string]any{"deeper": []any{password}}},
	}
	classifications := map[string]func(*planning.Catalog){
		"Resource Type secret": nil, // the seeded postgres Type marks password secret
		"module-only secret": func(c *planning.Catalog) {
			typ := c.Types["postgres"]
			outputs := append([]resource.OutputField(nil), typ.Outputs...)
			for i := range outputs {
				outputs[i].Secret = false
			}
			typ.Outputs = outputs
			c.Types["postgres"] = typ
		},
	}
	for class, adjust := range classifications {
		for name, annotations := range values {
			t.Run(class+"/"+name, func(t *testing.T) {
				_, err := planning.NewService(placedBundles()).Plan(k8sRequestWith(t, annotations, adjust))
				if !errors.Is(err, planning.ErrSecretInput) {
					t.Fatalf("not rejected: %v", err)
				}
				if strings.Contains(err.Error(), "${") {
					t.Fatalf("the error echoes the reference: %v", err)
				}
			})
		}
	}
	// A nonsecret output of the same provider is accepted, even next to text.
	if _, err := planning.NewService(placedBundles()).Plan(k8sRequestWith(t, map[string]any{"example.test/postgres-host": "db=" + host}, nil)); err != nil {
		t.Fatalf("nonsecret host fixture: %v", err)
	}
}

// The product seed's terraform-k8s catalog (not a test-local helper) plans the
// same provider -> namespace -> PostgreSQL/Redis graph over the real bundles.
func TestSeedTerraformK8sCatalogPlansOverRealBundles(t *testing.T) {
	req := k8sRequestWith(t, nil, func(c *planning.Catalog) {
		o := seedtest.Defaults()
		o.Catalog = seed.CatalogTerraformK8s
		c.Definitions = seed.ResourceDefinitions(o)
		for _, typ := range seed.ResourceTypesFor(o) {
			c.Types[typ.Key] = typ
		}
	})
	plan, err := planning.NewService(placedBundles()).Plan(req)
	if err != nil {
		t.Fatal(err)
	}
	const (
		cluster = "k8s-cluster.internal#environments.acceptance.dev"
		ns      = "k8s-namespace.default#environments.acceptance.dev"
		pg      = "postgres.default#shared.acceptance-db"
		redis   = "redis.default#shared.acceptance-cache"
	)
	for _, want := range [][3]string{{ns, cluster, "provider-binding"}, {pg, ns, "resource-reference"}, {redis, ns, "resource-reference"}} {
		if !hasEdge(plan, want[0], want[1], want[2]) {
			t.Errorf("missing %s edge %s -> %s", want[2], want[0], want[1])
		}
	}
	got := map[string]bool{}
	for _, c := range plan.Terraform {
		got[c.DefinitionKey] = true
	}
	for _, key := range []string{"namespace-k8s-module", "postgres-k8s-module", "redis-k8s-module"} {
		if !got[key] {
			t.Errorf("Terraform contract for %s missing: %v", key, got)
		}
	}
}

// The Console-authored acceptance catalog (Definitions with the in-cluster
// class, as the real runner registers them) plans the shared graph
// provider -> namespace -> PostgreSQL -> Redis for the Redis-enabled
// acceptance documents. The fixture annotation references the PostgreSQL
// descriptor WITH its class (postgres.in-cluster#...), which is what gives the
// V2-A01 deletion step both a dependency to select and one to leave unchecked.
func TestConsoleAuthoredAcceptanceCatalogPlansTheDeletionFixtureGraph(t *testing.T) {
	const (
		cluster = "k8s-cluster.internal#environments.acceptance.dev"
		ns      = "k8s-namespace.default#environments.acceptance.dev"
		pg      = "postgres.in-cluster#shared.acceptance-db"
		redis   = "redis.in-cluster#shared.acceptance-cache"
	)
	req := k8sRequestWith(t, nil, func(c *planning.Catalog) {
		refs, err := registry.PackagedRefs()
		if err != nil {
			t.Fatal(err)
		}
		nsOut := "${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}"
		nsDef := k8sTerraformDefinition("namespace-k8s-module", "k8s-namespace", refs["k8s-namespace"], map[string]any{"name": "${context.env.namespace}"}, nil, []string{"name"})
		pgDef := k8sTerraformDefinition("pg-in-cluster", "postgres", refs["postgres-k8s"], map[string]any{"namespace": nsOut}, []string{"database", "username"}, []string{"host", "port", "database", "username", "password"})
		redisDef := k8sTerraformDefinition("redis-in-cluster", "redis", refs["redis-k8s"], map[string]any{
			"namespace":   nsOut,
			"annotations": map[string]any{"example.test/postgres-host": "${resources['" + pg + "'].outputs.host}"},
		}, nil, []string{"host", "port", "username", "password", "tls"})
		pgDef.Criteria = []resource.Criterion{{Class: "in-cluster"}}
		redisDef.Criteria = []resource.Criterion{{Class: "in-cluster"}}
		c.Definitions = []resource.Definition{nsDef, pgDef, redisDef}
		for _, d := range seed.ResourceDefinitions(seedtest.Defaults()) {
			if d.Key == "cluster-internal" || d.Key == "workload-score-k8s" {
				c.Definitions = append(c.Definitions, d)
			}
		}
		c.Types = map[string]resource.Type{
			"workload":      {Key: "workload"},
			"k8s-cluster":   {Key: "k8s-cluster", Outputs: []resource.OutputField{{Name: "name", Type: "string", Required: true}, {Name: "endpoint", Type: "string"}, {Name: "kubeContext", Type: "string", Required: true}, {Name: "kubeconfig", Type: "string"}}},
			"k8s-namespace": {Key: "k8s-namespace", Outputs: []resource.OutputField{{Name: "name", Type: "string", Required: true}}},
			"redis":         seed.RedisType(),
		}
		for _, typ := range seed.ResourceTypes() {
			if typ.Key == "postgres" || typ.Key == "k8s-cluster" {
				c.Types[typ.Key] = typ
			}
		}
	})
	o := seedtest.Defaults()
	doc := seed.AcceptanceScoresRedis(o, "acceptance-cache", "in-cluster", "acceptance", "staging", "r1")["backend"]
	doc["resources"].(map[string]any)["db"].(map[string]any)["class"] = "in-cluster"
	after, err := score.FromMap(doc)
	if err != nil {
		t.Fatal(err)
	}
	req.After = after
	plan, err := planning.NewService(placedBundles()).Plan(req)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range [][3]string{{ns, cluster, "provider-binding"}, {pg, ns, "resource-reference"}, {redis, ns, "resource-reference"}, {redis, pg, "resource-reference"}} {
		if !hasEdge(plan, want[0], want[1], want[2]) {
			t.Errorf("missing %s edge %s -> %s", want[2], want[0], want[1])
		}
	}
	// The wrong reference (default class) is exactly what the planner refuses.
	bad := k8sRequestWith(t, map[string]any{"example.test/postgres-host": "${resources['postgres.default#shared.acceptance-db'].outputs.host}"}, nil)
	bad.After = after
	if _, err := planning.NewService(placedBundles()).Plan(bad); err == nil {
		t.Fatal("a reference to a nonexistent descriptor was planned")
	}
}
