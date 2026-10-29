package planning

import (
	"testing"

	"orchestrator/internal/domain/environment"
)

// A shared claim mount has no output placeholder. It must still create a graph
// edge and a binding, or provisioning/order/reference counting misses the claim.
func TestSharedStorageMountCreatesDependencyWithoutEnvironmentPlaceholder(t *testing.T) {
	doc := environment.NewDocument()
	doc.Shared["images"] = environment.ResourceEntry{Type: "persistent-storage"}
	doc.Modules["app"] = environment.Module{Spec: environment.ModuleSpec{Execution: &environment.WorkloadExecution{Mounts: []environment.StorageMount{{Resource: "shared.images", Container: "main", Path: "/data"}}}}}
	builder := newGraphBuilder(Context{}, Catalog{})
	if err := builder.buildFromSet(doc, nil); err != nil {
		t.Fatal(err)
	}
	graph := builder.graph()
	var consumer, provider string
	for _, node := range graph.Nodes {
		if node.Kind == NodeWorkload {
			consumer = node.Descriptor
			provider = node.Bindings["shared.images"]
		}
	}
	if provider == "" {
		t.Fatal("shared claim has no resolved binding")
	}
	for _, edge := range graph.Edges {
		if edge.Consumer == consumer && edge.Provider == provider {
			return
		}
	}
	t.Fatal("shared claim has no dependency edge")
}
