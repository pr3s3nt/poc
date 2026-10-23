package deploymentreview

import (
	"context"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/planning"
	"orchestrator/internal/ports/execution"
	"strings"
	"testing"
)

func TestBatchReviewCreationUsesOriginalBase(t *testing.T) {
	module := environment.Module{Spec: environment.ModuleSpec{Containers: map[string]environment.Container{"main": {Image: "app:v2"}}}}
	plan := &planning.Plan{BaseSet: environment.NewDocument(), CandidateSet: environment.Document{Modules: map[string]environment.Module{"api": module, "web": module}}, Graph: planning.Graph{Nodes: []planning.Node{{Descriptor: "workload.default#modules.api", Kind: planning.NodeWorkload, WorkloadID: "api"}, {Descriptor: "workload.default#modules.web", Kind: planning.NodeWorkload, WorkloadID: "web"}}}}
	review := (Assembler{}).Assemble(context.Background(), plan, nil, nil)
	for _, node := range review.Nodes {
		if node.Intent != "CREATE" {
			t.Fatalf("new batch workload incorrectly uses intermediate base: %+v", node)
		}
	}
}
func TestRemovedWorkloadHasExplicitAbsentAfter(t *testing.T) {
	module := environment.Module{Spec: environment.ModuleSpec{Containers: map[string]environment.Container{"main": {Image: "app:v1"}}}}
	plan := &planning.Plan{BaseSet: environment.Document{Modules: map[string]environment.Module{"api": module}}, CandidateSet: environment.NewDocument()}
	id := "workload.default#modules.api"
	old := map[string][]Artifact{id: {{ID: id + ":Deployment/api", NodeID: id, Kind: "kubernetes-manifest", Path: "Deployment/api", After: Side{Availability: "PARTIAL", Text: "kind: Deployment\n"}}}}
	review := (Assembler{}).Assemble(context.Background(), plan, nil, old)
	if len(review.Nodes) != 1 || review.Nodes[0].Intent != "REMOVE" || len(review.Artifacts) != 1 || review.Artifacts[0].After.Availability != "ABSENT" || review.Artifacts[0].Before.Text == "" {
		t.Fatalf("removal hidden: %+v", review)
	}
}
func TestProjectionRejectsNestedValuesInScalarFields(t *testing.T) {
	secret := "must-not-leak"
	side := ProjectManifests([]execution.Manifest{{Kind: "Deployment", Name: "api", Object: map[string]any{"spec": map[string]any{"replicas": map[string]any{"secret": secret}}}}})["Deployment/api"]
	if strings.Contains(side.Text, secret) {
		t.Fatal("nested invalid scalar leaked")
	}
}

func TestPreexistingClusterReviewBindsInsteadOfCreatingInfrastructure(t *testing.T) {
	plan := &planning.Plan{Graph: planning.Graph{Nodes: []planning.Node{{Descriptor: "cluster", Kind: planning.NodeResource, ResourceType: "k8s-cluster"}}}}
	got := (Assembler{}).Assemble(context.Background(), plan, []planning.ResourceIntent{{Descriptor: "cluster", Ownership: planning.OwnerPreexisting, Intent: planning.IntentCreate}}, nil)
	if len(got.Nodes) != 1 || got.Nodes[0].Intent != "BIND" {
		t.Fatalf("existing infrastructure mislabeled: %+v", got.Nodes)
	}
}
