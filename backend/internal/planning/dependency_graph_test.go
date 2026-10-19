package planning_test

import (
	"errors"
	"strings"
	"testing"

	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning"
)

// ADR-017 graph rules: one scoped descriptor is one node, authored sources that
// disagree are rejected, flags keep the UC-06 BR-08 meaning when a consumer
// appears only in a later expansion round, and a cycle anywhere in the full graph
// fails before any call.

const (
	chainType  = "chain"
	chainClass = "default"
)

func chainTypes(req *planning.Request) {
	req.Catalog.Types[chainType] = resource.Type{Key: chainType,
		Inputs:  []resource.InputField{{Name: "label", Type: "string"}},
		Outputs: []resource.OutputField{{Name: "value", Type: "string"}}}
}

func echoDefinition(key string, variables map[string]any, provision map[string]resource.ProvisionRule, criteria ...resource.Criterion) resource.Definition {
	return resource.Definition{Key: key, Name: key, Version: 1, ResourceTypeKey: chainType, DriverType: resource.DriverEcho,
		DriverInputs: map[string]any{"values": map[string]any{"variables": variables}}, Provision: provision, Criteria: criteria}
}

func chainRequest(t *testing.T, workloadRules map[string]resource.ProvisionRule, extra ...resource.Definition) planning.Request {
	t.Helper()
	defs := defaultCatalog(t)
	defs[2].Provision = workloadRules
	req := placedRequest(t, frontendScore(), append(defs, extra...)...)
	chainTypes(&req)
	return req
}

func chainNode(id string) string {
	return chainType + "." + chainClass + "#environments.shop.dev-" + id
}
func chainRule(id string) string {
	return chainType + "." + chainClass + "#environments.@app.@env-" + id
}

func nodeCount(plan *planning.Plan, descriptor string) int {
	count := 0
	for _, node := range plan.Graph.Nodes {
		if node.Descriptor == descriptor {
			count++
		}
	}
	return count
}

func TestDependencyGraph_SharedDescriptorFromTwoSourcesIsOneNodeWhenParamsAgree(t *testing.T) {
	// Two parents name the same companion with identical params: one scoped node, not two.
	rule := resource.ProvisionRule{Params: map[string]any{"label": "same"}}
	left := echoDefinition("chain-left", nil, map[string]resource.ProvisionRule{chainRule("shared"): rule}, resource.Criterion{Class: "left"})
	right := echoDefinition("chain-right", nil, map[string]resource.ProvisionRule{chainRule("shared"): rule}, resource.Criterion{Class: "right"})
	leaf := echoDefinition("chain-leaf", nil, nil, resource.Criterion{Class: chainClass})
	req := chainRequest(t, map[string]resource.ProvisionRule{
		"chain.left#environments.@app.@env-left":   {IsDependent: false},
		"chain.right#environments.@app.@env-right": {IsDependent: false},
	}, left, right, leaf)
	plan, err := placedPlan(t, req)
	if err != nil {
		t.Fatal(err)
	}
	if got := nodeCount(plan, chainNode("shared")); got != 1 {
		t.Fatalf("the shared companion must be one node, found %d: %+v", got, plan.Graph.Nodes)
	}
	again, err := placedPlan(t, req)
	if err != nil || again.PlanHash != plan.PlanHash {
		t.Fatalf("a fixed-point expansion must be deterministic: %v", err)
	}
}

func TestDependencyGraph_ConflictingParamsForOneDescriptorAreRejected(t *testing.T) {
	left := echoDefinition("chain-left", nil, map[string]resource.ProvisionRule{chainRule("shared"): {Params: map[string]any{"label": "one"}}}, resource.Criterion{Class: "left"})
	right := echoDefinition("chain-right", nil, map[string]resource.ProvisionRule{chainRule("shared"): {Params: map[string]any{"label": "two"}}}, resource.Criterion{Class: "right"})
	leaf := echoDefinition("chain-leaf", nil, nil, resource.Criterion{Class: chainClass})
	req := chainRequest(t, map[string]resource.ProvisionRule{
		"chain.left#environments.@app.@env-left":   {},
		"chain.right#environments.@app.@env-right": {},
	}, left, right, leaf)
	plan, err := placedPlan(t, req)
	var staged *planning.StageError
	if plan != nil || !errors.As(err, &staged) || staged.Reason != planning.ReasonResourceConflict || !errors.Is(err, planning.ErrNodeConflict) {
		t.Fatalf("plan=%v err=%v", plan, err)
	}
	message, _ := planning.PublicMessage(err)
	if !strings.Contains(message, "same resource with different params") || strings.Contains(message, "chain-left") || strings.Contains(message, "shared") {
		t.Fatalf("the public message must be fixed and name no catalog content: %q", message)
	}
}

func TestDependencyGraph_LateConsumerStillReceivesMatchDependentsEdge(t *testing.T) {
	// workload -> (provision) head; head's Definition provisions tail whose Definition
	// references the namespace only after it is matched in a LATER round. The namespace
	// Definition's match_dependents companion must still become a provider of tail.
	const (
		ns   = "k8s-namespace.default#environments.shop.dev"
		pool = "chain.pool#environments.shop.dev-pool"
		tail = "chain.tail#environments.shop.dev-tail"
	)
	head := echoDefinition("chain-head", nil, map[string]resource.ProvisionRule{"chain.tail#environments.@app.@env-tail": {}}, resource.Criterion{Class: "head"})
	tailDef := echoDefinition("chain-tail", map[string]any{"target": "${resources['" + ns + "'].outputs.name}"}, nil, resource.Criterion{Class: "tail"})
	poolDef := echoDefinition("chain-pool", nil, nil, resource.Criterion{Class: "pool"})
	req := chainRequest(t, map[string]resource.ProvisionRule{"chain.head#environments.@app.@env-head": {}}, head, tailDef, poolDef)
	for i, def := range req.Catalog.Definitions {
		if def.Key == "namespace-module" {
			req.Catalog.Definitions[i].Provision = map[string]resource.ProvisionRule{"chain.pool#environments.@app.@env-pool": {MatchDependents: true}}
		}
	}
	plan, err := placedPlan(t, req)
	if err != nil {
		t.Fatal(err)
	}
	if !hasEdge(plan, tail, ns, planning.ReasonReference) {
		t.Fatalf("tail must reference the namespace: %+v", plan.Graph.Edges)
	}
	if !hasEdge(plan, tail, pool, planning.ReasonMatchDependent) || !hasEdge(plan, workloadNode, pool, planning.ReasonMatchDependent) {
		t.Fatalf("every consumer of the namespace, including the late one, must depend on the match_dependents companion: %+v", plan.Graph.Edges)
	}
	if batchOf(plan, pool) >= batchOf(plan, tail) {
		t.Fatalf("the companion must run before its late consumer: %v", plan.Batches)
	}
	if hasEdge(plan, pool, ns, planning.ReasonDependent) {
		t.Fatalf("match_dependents alone must not make the companion depend on its parent: %+v", plan.Graph.Edges)
	}
}

func TestDependencyGraph_ReferenceCycleBetweenResourcesFailsAtScheduleBeforeAnyCall(t *testing.T) {
	const (
		nodeAClass = "chain.a#environments.shop.dev-a"
		nodeBClass = "chain.b#environments.shop.dev-b"
	)
	a := echoDefinition("chain-a", map[string]any{"peer": "${resources['" + nodeBClass + "'].outputs.value}"}, nil, resource.Criterion{Class: "a"})
	b := echoDefinition("chain-b", map[string]any{"peer": "${resources['" + nodeAClass + "'].outputs.value}"}, nil, resource.Criterion{Class: "b"})
	req := chainRequest(t, map[string]resource.ProvisionRule{
		"chain.a#environments.@app.@env-a": {},
		"chain.b#environments.@app.@env-b": {},
	}, a, b)
	plan, err := placedPlan(t, req)
	var staged *planning.StageError
	if plan != nil || !errors.As(err, &staged) || staged.Stage != planning.StageSchedule || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("a mutual reference must fail at the schedule stage before any call: plan=%v err=%v", plan, err)
	}
}

func TestDependencyGraph_ReferenceToUndeclaredOutputFailsAtTheReferenceStage(t *testing.T) {
	const ns = "k8s-namespace.default#environments.shop.dev"
	broken := echoDefinition("chain-broken", map[string]any{"target": "${resources['" + ns + "'].outputs.missing}"}, nil, resource.Criterion{Class: chainClass})
	req := chainRequest(t, map[string]resource.ProvisionRule{chainRule("one"): {}}, broken)
	_, err := placedPlan(t, req)
	var staged *planning.StageError
	if !errors.As(err, &staged) || staged.Stage != planning.StageReference {
		t.Fatalf("an output the provider Type does not declare must fail at the reference stage: %v", err)
	}
}

func TestDependencyGraph_ProviderBindingOrdersButDoesNotSelectAnAccount(t *testing.T) {
	plan, err := placedPlan(t, placedRequest(t, frontendScore(), defaultCatalog(t)...))
	if err != nil {
		t.Fatal(err)
	}
	// The namespace is ordered after its cluster by the provider-binding edge ...
	if !hasEdge(plan, namespaceNode, clusterNode, planning.ReasonProviderBinding) {
		t.Fatalf("edges = %+v", plan.Graph.Edges)
	}
	// ... and the account comes only from the cluster Definition's own match, never from the binding.
	pins := plan.Terraform[0].ProviderBindings
	if len(pins) != 1 || pins[0].ConnectionKey != plan.Matches[clusterNode].DriverAccount || pins[0].AccountVersion != plan.Matches[clusterNode].AccountVersion {
		t.Fatalf("provider pin must mirror the cluster match: %+v vs %+v", pins, plan.Matches[clusterNode])
	}
	for _, node := range plan.Graph.Nodes {
		if node.Descriptor == namespaceNode {
			if got := node.Bindings["provider.kubernetes"]; got != clusterNode {
				t.Fatalf("binding = %q", got)
			}
		}
	}
}
