package deploymentreview

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"orchestrator/internal/adapters/scorek8s"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning"
)

func TestSelectedScoreK8sRendersSymbolicReview(t *testing.T) {
	path, err := exec.LookPath("score-k8s")
	if err != nil {
		t.Skip("installed score-k8s unavailable")
	}
	renderer, err := scorek8s.New(context.Background(), path, nil)
	if err != nil {
		t.Fatal(err)
	}
	module := environment.Module{Profile: environment.ModuleProfile, Spec: environment.ModuleSpec{
		Containers: map[string]environment.Container{"main": {Image: "example/preview-namespace:v1", Variables: map[string]string{"DATABASE_HOST": "${resources.db.outputs.host}", "API_TOKEN": "opaque-literal-credential"}}},
		Service:    &environment.Service{Ports: map[string]environment.Port{"http": {Port: 8080}}},
	}}
	plan := &planning.Plan{
		PlanHash:     "fixed-hash",
		Graph:        planning.Graph{Nodes: []planning.Node{{Descriptor: "workload.backend", Kind: planning.NodeWorkload, WorkloadID: "backend"}}},
		CandidateSet: environment.Document{Modules: map[string]environment.Module{"backend": module}},
		BaseSet:      environment.NewDocument(),
		Rendering:    map[string]resource.RenderingSelection{"backend": {DefinitionKey: "render", DefinitionHash: "pin", DriverType: resource.DriverScoreK8s, Bundle: renderer.Bundles()[scorek8s.BundleID]}},
		Placements:   map[string]planning.Placement{"workload.backend": {Namespace: "k8s-namespace.default#environments.env.staging"}},
	}
	review := (Assembler{Renderer: renderer}).Assemble(context.Background(), plan, nil, nil)
	if len(review.Nodes) != 1 || review.Nodes[0].RendererDigest != renderer.Bundles()[scorek8s.BundleID].Digest || review.Nodes[0].DefinitionKey != "render" {
		t.Fatal("selected renderer pin missing")
	}
	got := map[string]Side{}
	for _, artifact := range review.Artifacts {
		got[artifact.Path] = artifact.After
	}
	for _, key := range []string{"Deployment/backend", "Service/backend"} {
		if got[key].Availability != "PARTIAL" || got[key].Text == "" {
			t.Fatalf("%s unavailable: %#v", key, got[key])
		}
		if strings.Contains(got[key].Text, "opaque-literal-credential") || strings.Contains(got[key].Text, "orchestrator-preview-unknown") {
			t.Fatalf("%s leaked variable value: %s", key, got[key].Text)
		}
	}
	for _, side := range got {
		if strings.Contains(side.Text, "k8s-namespace.default#") || !strings.Contains(side.Text, "<namespace: chờ cấp phát>") {
			t.Fatalf("namespace is not symbolic: %s", side.Text)
		}
	}
	if !strings.Contains(got["Deployment/backend"].Text, "example/preview-namespace:v1") {
		t.Fatal("selected renderer lost ordinary workload shape")
	}
}
