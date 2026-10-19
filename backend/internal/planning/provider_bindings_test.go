package planning

import (
	"strings"
	"testing"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
)

// kubernetesModuleDefinition is a Terraform postgres Definition whose module
// takes a Kubernetes provider binding and whose variables never mention the
// cluster, so only the binding can create the dependency.
func kubernetesModuleDefinition(binding map[string]any) resource.Definition {
	ref := tfmodule.SourceRef{SourceID: "platform", ModuleID: "pgk8s", Revision: "v1", Subdirectory: "pgk8s", ContentSHA256: strings.Repeat("a", 64)}
	return resource.Definition{
		Key: "postgres-k8s-module", Name: "postgres-k8s-module", Version: 1, ResourceTypeKey: "postgres", DriverType: resource.DriverTerraform,
		DriverInputs: map[string]any{"values": map[string]any{
			"source": ref.Map(), "variables": map[string]any{},
			"input_mapping":     map[string]any{"database": map[string]any{"param": "database"}, "username": map[string]any{"param": "username"}},
			"output_mapping":    map[string]any{"host": "host", "port": "port", "database": "database", "username": "username", "password": "password"},
			"provider_bindings": map[string]any{"kubernetes": binding},
		}},
		Criteria: []resource.Criterion{{Class: "default"}},
	}
}

func edgeReason(plan *Plan, consumerPrefix, provider string) (Edge, bool) {
	for _, e := range plan.Graph.Edges {
		if strings.HasPrefix(e.Consumer, consumerPrefix) && e.Provider == provider && e.Reason == ReasonProviderBinding {
			return e, true
		}
	}
	return Edge{}, false
}

func contractFor(t *testing.T, plan *Plan, definition string) TerraformContract {
	t.Helper()
	for _, c := range plan.Terraform {
		if c.DefinitionKey == definition {
			return c
		}
	}
	t.Fatalf("no Terraform contract for %s", definition)
	return TerraformContract{}
}

func seedAWSDefinition() resource.Definition {
	for _, d := range catalogOf(seedOptions()).Definitions {
		if d.Key == "postgres-aws-aurora" {
			return d
		}
	}
	panic("missing seed definition")
}

// Edges and pins are deterministic: the same inputs give the same plan hash.
func TestPlan_ProviderBindingPlanIsDeterministic(t *testing.T) {
	build := func() *Plan {
		req := testRequest(t, application.ProfileInternalK8s, "backend")
		req.Catalog.Definitions = append(req.Catalog.Definitions, kubernetesModuleDefinition(map[string]any{"target": tfmodule.TargetResource, "resource_ref": "k8s-cluster.internal#environments.@app.@env"}))
		plan, err := NewService(testBundles()).Plan(req)
		if err != nil {
			t.Fatal(err)
		}
		return plan
	}
	if a, b := build(), build(); a.PlanHash != b.PlanHash {
		t.Fatal("plan hash is not deterministic")
	}
}
