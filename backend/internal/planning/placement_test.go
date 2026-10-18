package planning_test

import (
	"errors"
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
)

// ADR-016 placement: Definitions and typed references decide the target of a
// workload; the Environment carries no deployment Connection and no builtin,
// profile or Environment account is a fallback.

const (
	nsRef      = "${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}"
	clusterRef = "${resources['k8s-cluster.internal#environments.@app.@env'].outputs.cluster}"
	bundleID   = "score-k8s-internal-v1"
)

func placedBundles() map[string]resource.RenderBundle {
	return map[string]resource.RenderBundle{bundleID: {ID: bundleID, Version: "0.15.0", BinaryDigest: "sha256:bin", Digest: "sha256:bundle"}}
}

func placedAccounts() map[string]application.Connection {
	return map[string]application.Connection{
		"acct-a": {Key: "acct-a", Kind: application.ConnectionKubernetes, OrganizationKey: "acme", Status: application.ConnectionReady, Version: 3},
		"acct-b": {Key: "acct-b", Kind: application.ConnectionKubernetes, OrganizationKey: "acme", Status: application.ConnectionReady, Version: 7},
	}
}

func clusterDefinition(key, account string, criteria ...resource.Criterion) resource.Definition {
	return resource.Definition{Key: key, Name: key, Version: 2, ResourceTypeKey: planning.TypeCluster, DriverType: resource.DriverExistingCluster,
		DriverAccount: account, DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{}}}, Criteria: criteria}
}

func namespaceDefinition(t *testing.T, key, clusterClass string, criteria ...resource.Criterion) resource.Definition {
	t.Helper()
	refs, err := registry.PackagedRefs()
	if err != nil {
		t.Fatal(err)
	}
	return resource.Definition{Key: key, Name: key, Version: 1, ResourceTypeKey: planning.TypeNamespace, DriverType: resource.DriverTerraform,
		DriverInputs: map[string]any{"values": map[string]any{
			"source": refs["k8s-namespace"].Map(), "variables": map[string]any{"name": "${context.env.namespace}"},
			"input_mapping": map[string]any{}, "output_mapping": map[string]any{"name": "name"},
			"provider_bindings": map[string]any{"kubernetes": map[string]any{"target": tfmodule.TargetResource, "resource_ref": "k8s-cluster." + clusterClass + "#environments.@app.@env"}},
		}}, Criteria: criteria}
}

func workloadDefinition(key string, variables map[string]any, criteria ...resource.Criterion) resource.Definition {
	merged := map[string]any{"render_bundle": bundleID, "namespace": nsRef, "cluster": clusterRef}
	for k, v := range variables {
		if v == nil {
			delete(merged, k)
			continue
		}
		merged[k] = v
	}
	return resource.Definition{Key: key, Name: key, Version: 4, ResourceTypeKey: planning.TypeWorkload, DriverType: resource.DriverScoreK8s,
		DriverInputs: map[string]any{"values": map[string]any{"variables": merged}}, Criteria: criteria}
}

func placedRequest(t *testing.T, doc map[string]any, defs ...resource.Definition) planning.Request {
	t.Helper()
	parsed, err := score.FromMap(doc)
	if err != nil {
		t.Fatal(err)
	}
	types := map[string]resource.Type{
		planning.TypeWorkload:  {Key: planning.TypeWorkload},
		planning.TypeNamespace: {Key: planning.TypeNamespace, Outputs: []resource.OutputField{{Name: "name", Type: "string", Required: true}}},
		planning.TypeCluster: {Key: planning.TypeCluster, Outputs: []resource.OutputField{
			{Name: "name", Type: "string"}, {Name: "cluster", Type: "any", Required: true}}},
	}
	return planning.Request{
		OrganizationKey: "acme",
		App:             application.Application{Key: "shop", OrganizationKey: "acme", Name: "shop", Version: 1},
		// No ConnectionKey, Profile or Environment account: placement is Definition-driven.
		Env:        environment.Environment{Key: "dev", ApplicationKey: "shop", Name: "dev", Type: "development", NamespaceIdentity: "shop-dev", Version: 1},
		BaseSet:    environment.NewDocument(),
		After:      parsed,
		WorkloadID: "web",
		RunID:      "run-1",
		Catalog:    planning.Catalog{Types: types, Definitions: defs, Accounts: placedAccounts()},
		Terraform:  registrytest.Inspector(t, store.New()),
	}
}

func frontendScore(extra ...func(map[string]any)) map[string]any {
	doc := map[string]any{
		"apiVersion": "score.dev/v1b1", "metadata": map[string]any{"name": "web"},
		"containers": map[string]any{"main": map[string]any{"image": "example.invalid/frontend:1"}},
	}
	for _, mutate := range extra {
		mutate(doc)
	}
	return doc
}

func defaultCatalog(t *testing.T) []resource.Definition {
	return []resource.Definition{
		clusterDefinition("cluster-a", "acct-a", resource.Criterion{Class: "internal"}),
		namespaceDefinition(t, "namespace-module", "internal", resource.Criterion{Class: "default"}),
		workloadDefinition("workload-render", nil, resource.Criterion{}),
	}
}

func placedPlan(t *testing.T, req planning.Request) (*planning.Plan, error) {
	t.Helper()
	return planning.NewRequiredRendererService(placedBundles()).Plan(req)
}

const (
	clusterNode   = "k8s-cluster.internal#environments.shop.dev"
	namespaceNode = "k8s-namespace.default#environments.shop.dev"
	workloadNode  = "workload.default#modules.web"
)

func TestPlacement_DefinitionsDecideTargetWithoutEnvironmentConnection(t *testing.T) {
	req := placedRequest(t, frontendScore(), defaultCatalog(t)...)
	plan, err := placedPlan(t, req)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Target.ConnectionKey != "" || plan.Target.Profile != "" {
		t.Fatalf("an Environment target leaked into the plan: %+v", plan.Target)
	}
	cluster := plan.Matches[clusterNode]
	if cluster.DefinitionKey != "cluster-a" || cluster.DriverAccount != "acct-a" || cluster.AccountVersion != 3 || cluster.DefinitionVersion != 2 {
		t.Fatalf("cluster match = %+v", cluster)
	}
	work := plan.Placements[workloadNode]
	if work.Cluster != clusterNode || work.Namespace != namespaceNode || work.Account != "acct-a" || work.AccountVersion != 3 || work.ClusterDefinition != "cluster-a" || work.Source != planning.PlacementWorkloadReference {
		t.Fatalf("workload placement = %+v", work)
	}
	ns := plan.Placements[namespaceNode]
	if ns.Cluster != clusterNode || ns.Source != planning.PlacementProviderBinding || ns.Account != "acct-a" {
		t.Fatalf("namespace placement = %+v", ns)
	}
	if !hasEdge(plan, workloadNode, namespaceNode, planning.ReasonReference) || !hasEdge(plan, workloadNode, clusterNode, planning.ReasonReference) || !hasEdge(plan, namespaceNode, clusterNode, planning.ReasonProviderBinding) {
		t.Fatalf("edges = %+v", plan.Graph.Edges)
	}
	if batchOf(plan, clusterNode) != 0 || batchOf(plan, namespaceNode) != 1 || batchOf(plan, workloadNode) != -1 {
		t.Fatalf("provider-first batches = %v", plan.Batches)
	}
	if pins := plan.Terraform; len(pins) != 1 || len(pins[0].ProviderBindings) != 1 || pins[0].ProviderBindings[0].ConnectionKey != "acct-a" || pins[0].ProviderBindings[0].ConnectionKind != "KUBERNETES" || pins[0].ProviderBindings[0].AccountVersion != 3 || pins[0].ProviderBindings[0].Provider != clusterNode {
		t.Fatalf("namespace provider pin = %+v", plan.Terraform)
	}
	selection := plan.Rendering["web"]
	if selection.DefinitionKey != "workload-render" || selection.Class != "default" || selection.Bundle.ID != bundleID {
		t.Fatalf("rendering = %+v", selection)
	}
	req2 := placedRequest(t, frontendScore(), defaultCatalog(t)...)
	req2.Terraform = req.Terraform // one registry: the bundle ID is part of the pin
	again, err := placedPlan(t, req2)
	if err != nil || again.PlanHash != plan.PlanHash {
		t.Fatalf("hash not deterministic: %v", err)
	}
}

func TestPlacement_AccountVersionAndClusterDefinitionChangeThePlanHash(t *testing.T) {
	baseReq := placedRequest(t, frontendScore(), defaultCatalog(t)...)
	base, err := placedPlan(t, baseReq)
	if err != nil {
		t.Fatal(err)
	}
	req := placedRequest(t, frontendScore(), defaultCatalog(t)...)
	req.Terraform = baseReq.Terraform
	account := req.Catalog.Accounts["acct-a"]
	account.Version++
	req.Catalog.Accounts["acct-a"] = account
	changed, err := placedPlan(t, req)
	if err != nil || changed.PlanHash == base.PlanHash {
		t.Fatalf("an account edit must make an older Preview stale: %v", err)
	}
	defs := defaultCatalog(t)
	defs[0].Version++
	defReq := placedRequest(t, frontendScore(), defs...)
	defReq.Terraform = baseReq.Terraform
	again, err := placedPlan(t, defReq)
	if err != nil || again.PlanHash == base.PlanHash {
		t.Fatalf("a cluster Definition edit must make an older Preview stale: %v", err)
	}
}

func TestPlacement_UnavailableAccountFailsWithoutFallback(t *testing.T) {
	cases := map[string]func(map[string]application.Connection){
		"missing": func(a map[string]application.Connection) { delete(a, "acct-a") },
		"wrong kind": func(a map[string]application.Connection) {
			c := a["acct-a"]
			c.Kind = application.ConnectionAWS
			a["acct-a"] = c
		},
		"foreign org": func(a map[string]application.Connection) {
			c := a["acct-a"]
			c.OrganizationKey = "other"
			a["acct-a"] = c
		},
		"not ready": func(a map[string]application.Connection) {
			c := a["acct-a"]
			c.Status = application.ConnectionVerifying
			a["acct-a"] = c
		},
		"rejected": func(a map[string]application.Connection) {
			c := a["acct-a"]
			c.Status = application.ConnectionRejected
			a["acct-a"] = c
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			req := placedRequest(t, frontendScore(), defaultCatalog(t)...)
			mutate(req.Catalog.Accounts) // acct-b stays READY: it must never replace the winner.
			plan, err := placedPlan(t, req)
			var staged *planning.StageError
			if plan != nil || !errors.Is(err, planning.ErrAccountConflict) || !errors.As(err, &staged) || staged.Reason != planning.ReasonPlacement {
				t.Fatalf("plan=%v err=%v", plan, err)
			}
			if message, ok := planning.PublicMessage(err); !ok || strings.Contains(message, "acct-a") {
				t.Fatalf("public message must be fixed and name no account: %q", message)
			}
		})
	}
}

func TestPlacement_TiedClusterDefinitionsAreAmbiguousNotResolved(t *testing.T) {
	defs := append(defaultCatalog(t), clusterDefinition("cluster-b", "acct-b", resource.Criterion{Class: "internal"}))
	plan, err := placedPlan(t, placedRequest(t, frontendScore(), defs...))
	var staged *planning.StageError
	if plan != nil || err == nil || !strings.Contains(err.Error(), "ambiguous match") || !errors.As(err, &staged) || staged.Reason != planning.ReasonResourceAmbiguous || staged.Stage != planning.StageGraph {
		t.Fatalf("plan=%v err=%v", plan, err)
	}
	if !strings.Contains(err.Error(), "cluster-a") || !strings.Contains(err.Error(), "cluster-b") {
		t.Fatalf("the tied Definitions must be named: %v", err)
	}
}

func TestPlacement_MoreSpecificClusterDefinitionWinsByNormalWeights(t *testing.T) {
	defs := append(defaultCatalog(t), clusterDefinition("cluster-b", "acct-b", resource.Criterion{Class: "internal", ApplicationID: "shop"}))
	plan, err := placedPlan(t, placedRequest(t, frontendScore(), defs...))
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.Placements[workloadNode]; got.ClusterDefinition != "cluster-b" || got.Account != "acct-b" || got.AccountVersion != 7 {
		t.Fatalf("placement = %+v", got)
	}
}

func TestPlacement_NoClusterDefinitionHasNoBuiltinFallback(t *testing.T) {
	defs := defaultCatalog(t)[1:]
	plan, err := placedPlan(t, placedRequest(t, frontendScore(), defs...))
	var staged *planning.StageError
	if plan != nil || err == nil || !strings.Contains(err.Error(), "no resource definition matches") || !errors.As(err, &staged) || staged.Reason != planning.ReasonResourceNoMatch {
		t.Fatalf("plan=%v err=%v", plan, err)
	}
}

func TestPlacement_WorkloadWithoutTypedReferencesFails(t *testing.T) {
	defs := []resource.Definition{defaultCatalog(t)[0], defaultCatalog(t)[1], {
		Key: "workload-render", Name: "w", Version: 1, ResourceTypeKey: planning.TypeWorkload, DriverType: resource.DriverScoreK8s,
		DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{"render_bundle": bundleID}}}, Criteria: []resource.Criterion{{}},
	}}
	_, err := placedPlan(t, placedRequest(t, frontendScore(), defs...))
	var staged *planning.StageError
	if !errors.As(err, &staged) || staged.Stage != planning.StageCatalog || !strings.Contains(err.Error(), "needs both the namespace and cluster references") {
		t.Fatalf("err=%v", err)
	}
}

func TestPlacement_NamespaceProviderMustMatchWorkloadCluster(t *testing.T) {
	defs := []resource.Definition{
		clusterDefinition("cluster-a", "acct-a", resource.Criterion{Class: "internal"}),
		clusterDefinition("cluster-b", "acct-b", resource.Criterion{Class: "other"}),
		// The namespace is created on the other cluster while the workload names the internal one.
		namespaceDefinition(t, "namespace-module", "other", resource.Criterion{Class: "default"}),
		workloadDefinition("workload-render", nil, resource.Criterion{}),
	}
	plan, err := placedPlan(t, placedRequest(t, frontendScore(), defs...))
	var staged *planning.StageError
	if plan != nil || !errors.As(err, &staged) || staged.Reason != planning.ReasonPlacement || !strings.Contains(err.Error(), "contradicts") {
		t.Fatalf("plan=%v err=%v", plan, err)
	}
}

func TestPlacement_TwoClustersInOneCandidateSetAreUnsupportedBeforeExecution(t *testing.T) {
	defs := []resource.Definition{
		clusterDefinition("cluster-a", "acct-a", resource.Criterion{Class: "internal"}),
		clusterDefinition("cluster-b", "acct-b", resource.Criterion{Class: "other"}),
		namespaceDefinition(t, "namespace-module", "internal", resource.Criterion{Class: "default"}),
		namespaceDefinition(t, "namespace-second", "other", resource.Criterion{Class: "second"}),
		workloadDefinition("workload-render", nil, resource.Criterion{}),
	}
	doc := frontendScore(func(d map[string]any) {
		d["resources"] = map[string]any{"extra": map[string]any{"type": "k8s-namespace", "class": "second"}}
	})
	plan, err := placedPlan(t, placedRequest(t, doc, defs...))
	var staged *planning.StageError
	if plan != nil || !errors.As(err, &staged) || staged.Reason != planning.ReasonMultiCluster {
		t.Fatalf("plan=%v err=%v", plan, err)
	}
	if message, _ := planning.PublicMessage(err); !strings.Contains(message, "more than one Kubernetes cluster") {
		t.Fatalf("message = %q", message)
	}
}

func TestPlacement_ClassSelectsWorkloadDefinitionAndDefaultsWhenMissing(t *testing.T) {
	defs := append(defaultCatalog(t),
		workloadDefinition("workload-gold", nil, resource.Criterion{Class: "gold"}))
	defs[2] = workloadDefinition("workload-render", nil, resource.Criterion{Class: "default"})
	plain, err := placedPlan(t, placedRequest(t, frontendScore(), defs...))
	if err != nil || plain.Rendering["web"].DefinitionKey != "workload-render" || plain.Rendering["web"].Class != "default" {
		t.Fatalf("default class: %+v %v", plain, err)
	}
	gold, err := placedPlan(t, placedRequest(t, frontendScore(func(d map[string]any) { d["metadata"] = map[string]any{"name": "web", "class": "gold"} }), defs...))
	if err != nil {
		t.Fatal(err)
	}
	if gold.Rendering["web"].DefinitionKey != "workload-gold" || gold.Rendering["web"].Class != "gold" || gold.CandidateSet.Modules["web"].Class != "gold" {
		t.Fatalf("explicit class: %+v", gold.Rendering["web"])
	}
	if _, ok := gold.Graph.Node("workload.gold#modules.web"); !ok {
		t.Fatalf("workload node must carry the class: %+v", gold.Graph.Nodes)
	}
	if gold.PlanHash == plain.PlanHash {
		t.Fatal("a class change must invalidate an older Preview")
	}
	_, err = placedPlan(t, placedRequest(t, frontendScore(func(d map[string]any) { d["metadata"] = map[string]any{"name": "web", "class": "silver"} }), defs...))
	var staged *planning.StageError
	if !errors.As(err, &staged) || staged.Reason != planning.ReasonRendererNoMatch {
		t.Fatalf("an unmatched class must not fall back to default: %v", err)
	}
}

// UC-06 BR-08 keeps its meaning when the parent is a workload Definition: neither flag adds no
// ordering edge, is_dependent adds child->parent, match_dependents adds every parent consumer->child.
func companionPlan(t *testing.T, rules map[string]resource.ProvisionRule) (*planning.Plan, error) {
	t.Helper()
	defs := defaultCatalog(t)
	defs[2].Provision = rules
	return placedPlan(t, placedRequest(t, frontendScore(), defs...))
}

const (
	companionA = "k8s-namespace.default#environments.@app.@env-a"
	companionB = "k8s-namespace.default#environments.@app.@env-b"
	nodeA      = "k8s-namespace.default#environments.shop.dev-a"
	nodeB      = "k8s-namespace.default#environments.shop.dev-b"
)

func edgesTouching(plan *planning.Plan, descriptor string) []planning.Edge {
	var out []planning.Edge
	for _, e := range plan.Graph.Edges {
		if e.Consumer == descriptor || e.Provider == descriptor {
			out = append(out, e)
		}
	}
	return out
}

func TestPlacement_WorkloadProvisionFlagsKeepBR08Semantics(t *testing.T) {
	// false/false: the companion exists with its placement but no edge orders it against the workload.
	plan, err := companionPlan(t, map[string]resource.ProvisionRule{companionA: {}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := plan.Placements[nodeA]; !ok || plan.Matches[nodeA].DefinitionKey == "" {
		t.Fatalf("companion not planned: %+v", plan.Placements)
	}
	for _, e := range edgesTouching(plan, nodeA) {
		if e.Consumer == workloadNode || e.Provider == workloadNode || e.Reason == planning.ReasonDependent || e.Reason == planning.ReasonMatchDependent {
			t.Fatalf("an empty rule must add no workload edge: %+v", e)
		}
	}
	if len(plan.AfterWorkloads) != 0 {
		t.Fatalf("nothing follows the workload: %v", plan.AfterWorkloads)
	}

	// is_dependent: the companion depends on the workload and is scheduled after it.
	plan, err = companionPlan(t, map[string]resource.ProvisionRule{companionA: {IsDependent: true}})
	if err != nil {
		t.Fatal(err)
	}
	if !hasEdge(plan, nodeA, workloadNode, planning.ReasonDependent) {
		t.Fatalf("edges = %+v", plan.Graph.Edges)
	}
	if got := plan.AfterWorkloads[nodeA]; len(got) != 1 || got[0] != workloadNode {
		t.Fatalf("afterWorkloads = %v", plan.AfterWorkloads)
	}
	if batchOf(plan, nodeA) < 0 {
		t.Fatalf("a post-workload resource must still be listed in the batches: %v", plan.Batches)
	}

	// match_dependents: every consumer of the workload also depends on the companion.
	// A is a consumer of the workload (is_dependent); B is the match_dependents companion.
	plan, err = companionPlan(t, map[string]resource.ProvisionRule{companionA: {IsDependent: true}, companionB: {MatchDependents: true}})
	if err != nil {
		t.Fatal(err)
	}
	if !hasEdge(plan, nodeA, nodeB, planning.ReasonMatchDependent) || !hasEdge(plan, nodeA, workloadNode, planning.ReasonDependent) {
		t.Fatalf("edges = %+v", plan.Graph.Edges)
	}
	if batchOf(plan, nodeB) >= batchOf(plan, nodeA) {
		t.Fatalf("B must be scheduled before its dependent consumer A: %v", plan.Batches)
	}
	if _, post := plan.AfterWorkloads[nodeB]; post {
		t.Fatalf("B depends on nothing that follows the workload: %v", plan.AfterWorkloads)
	}

	// both flags on the same rule: the companion depends on the workload and the other consumers on it.
	plan, err = companionPlan(t, map[string]resource.ProvisionRule{companionA: {IsDependent: true}, companionB: {IsDependent: true, MatchDependents: true}})
	if err != nil {
		t.Fatal(err)
	}
	if !hasEdge(plan, nodeB, workloadNode, planning.ReasonDependent) || !hasEdge(plan, nodeA, nodeB, planning.ReasonMatchDependent) {
		t.Fatalf("edges = %+v", plan.Graph.Edges)
	}
	if batchOf(plan, nodeB) >= batchOf(plan, nodeA) {
		t.Fatalf("A depends on B: %v", plan.Batches)
	}
	for _, node := range []string{nodeA, nodeB} {
		if len(plan.AfterWorkloads[node]) != 1 {
			t.Fatalf("%s must follow the workload: %v", node, plan.AfterWorkloads)
		}
	}
	// fixed point: the same input plans to the same hash.
	defs := defaultCatalog(t)
	defs[2].Provision = map[string]resource.ProvisionRule{companionA: {IsDependent: true}, companionB: {IsDependent: true, MatchDependents: true}}
	req := placedRequest(t, frontendScore(), defs...)
	first, err := placedPlan(t, req)
	if err != nil {
		t.Fatal(err)
	}
	second, err := placedPlan(t, req)
	if err != nil || second.PlanHash != first.PlanHash {
		t.Fatalf("plan must be deterministic: %v", err)
	}
}

func TestPlacement_WorkloadDependencyCyclesFailBeforeAnySideEffect(t *testing.T) {
	var staged *planning.StageError
	// The workload references its namespace; making that namespace depend on the workload closes a cycle.
	_, err := companionPlan(t, map[string]resource.ProvisionRule{"k8s-namespace.default#environments.@app.@env": {IsDependent: true}})
	if !errors.As(err, &staged) || staged.Stage != planning.StageSchedule || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("a cycle through the workload must fail at the schedule stage: %v", err)
	}
	// A rule that makes the workload depend on itself is a self cycle, not a dropped edge.
	_, err = companionPlan(t, map[string]resource.ProvisionRule{workloadNode: {IsDependent: true}})
	if !errors.As(err, &staged) || staged.Stage != planning.StageSchedule || !strings.Contains(err.Error(), "depends on itself") {
		t.Fatalf("a self dependency must fail: %v", err)
	}
	// A companion that is also a match_dependents consumer of itself is not a cycle (skipped), but a
	// companion pair that depend on each other through the workload are.
	_, err = companionPlan(t, map[string]resource.ProvisionRule{
		companionA: {IsDependent: true},
		companionB: {IsDependent: true, MatchDependents: true},
		"k8s-namespace.default#environments.@app.@env-c": {MatchDependents: true},
	})
	if err != nil {
		t.Fatalf("acyclic combinations must plan: %v", err)
	}
}

func TestPlacement_EnvironmentProviderTargetIsRetiredForDefinitionPlacedEnvironments(t *testing.T) {
	defs := defaultCatalog(t)
	values := defs[1].DriverInputs["values"].(map[string]any)
	values["provider_bindings"] = map[string]any{"kubernetes": map[string]any{"target": tfmodule.TargetEnvironment}}
	_, err := placedPlan(t, placedRequest(t, frontendScore(), defs...))
	if err == nil || !strings.Contains(err.Error(), "retired") {
		t.Fatalf("err=%v", err)
	}
}

func TestPlacement_ANamespaceThatReferencesTheClusterOutputIsPlacedByThatReference(t *testing.T) {
	defs := defaultCatalog(t)
	// A non-Terraform namespace Definition has no provider binding; its own cluster reference places it.
	defs[1] = resource.Definition{Key: "namespace-driver", Name: "ns", Version: 1, ResourceTypeKey: planning.TypeNamespace, DriverType: resource.DriverKubernetes, Criteria: []resource.Criterion{{Class: "default"}},
		DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{"name": "${context.env.namespace}", "cluster": clusterRef}}}}
	plan, err := placedPlan(t, placedRequest(t, frontendScore(), defs...))
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.Placements[namespaceNode]; got.Cluster != clusterNode || got.Source != planning.PlacementReference || got.Account != "acct-a" {
		t.Fatalf("namespace placement = %+v", got)
	}
	if got := plan.Placements[workloadNode]; got.Namespace != namespaceNode || got.Cluster != clusterNode {
		t.Fatalf("workload placement = %+v", got)
	}
}
