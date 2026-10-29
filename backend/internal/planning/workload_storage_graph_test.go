package planning

import (
	"strings"
	"testing"

	"orchestrator/internal/domain/environment"
	"orchestrator/internal/planning/score"
)

// A shared claim mount has no output placeholder. It must still create a graph
// edge and a binding, or provisioning/order/reference counting misses the claim.
func TestSharedStorageMountCreatesDependencyWithoutEnvironmentPlaceholder(t *testing.T) {
	doc := environment.NewDocument()
	doc.Shared["images"] = environment.ResourceEntry{Type: "persistent-storage"}
	doc.Modules["app"] = environment.Module{Spec: environment.ModuleSpec{Execution: &environment.WorkloadExecution{Version: 1, Kind: "StatefulSet", Template: environment.TemplatePin{ID: "stateful", Version: "1.0.0", SHA256: strings.Repeat("a", 64)}, Mounts: []environment.StorageMount{{Resource: "shared.images", Container: "main", Path: "/data"}}}}}
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

func TestSharedMountSurvivesUnrelatedDeltaAndOtherConsumerRemoval(t *testing.T) {
	mounted := func() environment.Module {
		return environment.Module{Profile: environment.ModuleProfile, Spec: environment.ModuleSpec{Containers: map[string]environment.Container{"main": {Image: "busybox:1.37"}}, Execution: &environment.WorkloadExecution{Version: 1, Kind: "StatefulSet", Template: environment.TemplatePin{ID: "stateful", Version: "1.0.0", SHA256: strings.Repeat("a", 64)}, Mounts: []environment.StorageMount{{Resource: "shared.images", Container: "main", Path: "/data"}}}}}
	}
	data := environment.ResourceEntry{Type: "persistent-storage", Class: "default"}
	base := environment.NewDocument()
	base.Modules["app"] = mounted()
	base.Shared["images"] = data
	worker := environment.Module{Profile: environment.ModuleProfile, Spec: environment.ModuleSpec{Containers: map[string]environment.Container{"main": {Image: "busybox:1.37"}}}}
	added, err := (DeltaBuilder{}).BuildHumanitecDelta(base, "worker", nil, &score.Fragment{WorkloadID: "worker", Module: worker})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := added.Candidate.Shared["images"]; !ok {
		t.Fatal("unrelated workload addition dropped mounted shared storage")
	}
	before := &score.Fragment{WorkloadID: "app", Module: base.Modules["app"], Shared: map[string]environment.ResourceEntry{"images": data}}
	added.Candidate.Modules["second"] = mounted()
	removed, err := (DeltaBuilder{}).BuildHumanitecDelta(added.Candidate, "app", before, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := removed.Candidate.Shared["images"]; !ok {
		t.Fatal("removing one consumer dropped another consumer's mount")
	}
	before = &score.Fragment{WorkloadID: "second", Module: removed.Candidate.Modules["second"], Shared: map[string]environment.ResourceEntry{"images": data}}
	last, err := (DeltaBuilder{}).BuildHumanitecDelta(removed.Candidate, "second", before, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := last.Candidate.Shared["images"]; ok {
		t.Fatal("last mount removal did not release desired reference")
	}
}
