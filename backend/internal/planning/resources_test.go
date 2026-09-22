package planning

import (
	"encoding/json"
	"reflect"
	"testing"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/planning/score"
	"orchestrator/internal/seed"
)

// backendScoreWithResources returns the seeded backend Score whose main
// container declares the given requirements, replacing any sample value.
func backendScoreWithResources(t *testing.T, resources map[string]any) *score.Document {
	t.Helper()
	raw := seed.AcceptanceScores(seedOptions())["backend"]
	b, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	var tree map[string]any
	if err := json.Unmarshal(b, &tree); err != nil {
		t.Fatal(err)
	}
	main := tree["containers"].(map[string]any)["main"].(map[string]any)
	if resources == nil {
		delete(main, "resources")
	} else {
		main["resources"] = resources
	}
	doc, err := score.FromMap(tree)
	if err != nil {
		t.Fatalf("score: %v", err)
	}
	return doc
}

func requirementsJSON(t *testing.T, r *environment.ContainerResourceRequirements) string {
	t.Helper()
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestPlan_PreservesContainerResourceRequirements(t *testing.T) {
	declared := map[string]any{
		"requests": map[string]any{"cpu": "100m", "memory": "128Mi"},
		"limits":   map[string]any{"cpu": "0.5", "memory": "512Mi"},
	}
	want := `{"requests":{"cpu":"100m","memory":"128Mi"},"limits":{"cpu":"0.5","memory":"512Mi"}}`

	req := testRequest(t, application.ProfileInternalK8s, "backend")
	req.After = backendScoreWithResources(t, declared)
	plan, err := NewService().Plan(req)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	// Candidate Deployment Set.
	candidate := plan.CandidateSet.Modules["backend"].Spec.Containers["main"].Resources
	if got := requirementsJSON(t, candidate); got != want {
		t.Fatalf("candidate set requirements:\nwant %s\ngot  %s", want, got)
	}
	// Delta modules.add carries the full module, including requirements.
	added := plan.Delta.Modules.Add["backend"].Spec.Containers["main"].Resources
	if got := requirementsJSON(t, added); got != want {
		t.Fatalf("delta requirements:\nwant %s\ngot  %s", want, got)
	}
	// base + delta reproduces the requirements.
	applied, err := ApplyHumanitecDelta(req.BaseSet, plan.Delta)
	if err != nil {
		t.Fatalf("apply delta: %v", err)
	}
	if got := requirementsJSON(t, applied.Modules["backend"].Spec.Containers["main"].Resources); got != want {
		t.Fatalf("applied delta requirements:\nwant %s\ngot  %s", want, got)
	}
}

// Container requirements belong to the workload module; they never become
// Resource Graph nodes or change graph identity.
func TestPlan_ContainerResourcesAddNoGraphNodes(t *testing.T) {
	without := testRequest(t, application.ProfileInternalK8s, "backend")
	without.After = backendScoreWithResources(t, nil)
	with := testRequest(t, application.ProfileInternalK8s, "backend")
	with.After = backendScoreWithResources(t, map[string]any{
		"requests": map[string]any{"cpu": "100m", "memory": "128Mi"},
		"limits":   map[string]any{"cpu": "500m", "memory": "512Mi"},
	})
	a, err := NewService().Plan(without)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	b, err := NewService().Plan(with)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if !reflect.DeepEqual(a.Graph, b.Graph) {
		t.Fatal("container resources must not change the Resource Graph")
	}
	if !reflect.DeepEqual(a.Batches, b.Batches) {
		t.Fatal("container resources must not change provision batches")
	}
	for _, node := range b.Graph.Nodes {
		if node.ResourceType == "cpu" || node.ResourceType == "memory" {
			t.Fatalf("unexpected compute resource node %s", node.Descriptor)
		}
	}
}

// Changing one declared value produces a module-relative replace patch whose
// value is the exact Score string.
func TestPlan_ContainerResourceChangeIsModuleRelativePatch(t *testing.T) {
	svc := NewService()
	first := testRequest(t, application.ProfileInternalK8s, "backend")
	first.After = backendScoreWithResources(t, map[string]any{
		"requests": map[string]any{"cpu": "100m"},
		"limits":   map[string]any{"memory": "512Mi"},
	})
	firstPlan, err := svc.Plan(first)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	next := testRequest(t, application.ProfileInternalK8s, "backend")
	next.BaseSet = firstPlan.CandidateSet
	next.Before = first.After
	next.After = backendScoreWithResources(t, map[string]any{
		"requests": map[string]any{"cpu": "250m"},
		"limits":   map[string]any{"memory": "512Mi", "cpu": "1"},
	})
	plan, err := svc.Plan(next)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	want := []deployment.JSONPatchOperation{
		{Op: deployment.PatchAdd, Path: "/spec/containers/main/resources/limits/cpu", Value: "1"},
		{Op: deployment.PatchReplace, Path: "/spec/containers/main/resources/requests/cpu", Value: "250m"},
	}
	if plan.Delta.Modules == nil || !reflect.DeepEqual(plan.Delta.Modules.Update["backend"], want) {
		t.Fatalf("unexpected delta: %s", deltaJSON(t, plan.Delta))
	}
	got := plan.CandidateSet.Modules["backend"].Spec.Containers["main"].Resources
	if s := requirementsJSON(t, got); s != `{"requests":{"cpu":"250m"},"limits":{"cpu":"1","memory":"512Mi"}}` {
		t.Fatalf("candidate requirements: %s", s)
	}
}
