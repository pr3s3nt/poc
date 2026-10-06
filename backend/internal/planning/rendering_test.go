package planning

import (
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning/score"
	"strings"
	"testing"
)

func TestRendererMatchingPinsContentAndPreservesInfrastructure(t *testing.T) {
	req := testRequest(t, application.ProfileInternalK8s, "backend")
	bundle := resource.RenderBundle{ID: "installed", Version: "0.15.0", BinaryDigest: "binary", Digest: "bundle"}
	svc := NewService(map[string]resource.RenderBundle{"installed": bundle})
	native, err := svc.Plan(req)
	if err != nil {
		t.Fatal(err)
	}
	def := resource.Definition{Key: "render", ResourceTypeKey: "workload", ExecutionProfile: "internal-k8s", DriverType: resource.DriverScoreK8s, DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{"render_bundle": "installed"}}}, Criteria: []resource.Criterion{{}}}
	req.Catalog.Definitions = append(req.Catalog.Definitions, def)
	plan, err := svc.Plan(req)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Rendering["backend"].DefinitionKey != "render" || plan.PlanHash == native.PlanHash {
		t.Fatal("rendering intent not pinned")
	}
	if len(plan.Matches) != len(native.Matches) || len(plan.Batches) != len(native.Batches) {
		t.Fatal("render selection affected infrastructure")
	}
	if _, err := NewService().Plan(req); err == nil {
		t.Fatal("missing bundle accepted")
	}
	def.Key = "specific"
	def.Criteria = []resource.Criterion{{ResourceID: "modules.backend", Class: "default"}}
	req.Catalog.Definitions = append(req.Catalog.Definitions, def)
	specific, err := svc.Plan(req)
	if err != nil {
		t.Fatal(err)
	}
	if specific.Rendering["backend"].DefinitionKey != "specific" {
		t.Fatal("weighted matching failed")
	}
	def.Key = "tied"
	req.Catalog.Definitions = append(req.Catalog.Definitions, def)
	if _, err := svc.Plan(req); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("tie: %v", err)
	}
}
func TestRenderingBundleChangeAndWorkloadDependencyAreRejected(t *testing.T) {
	req := testRequest(t, application.ProfileInternalK8s, "backend")
	def := resource.Definition{Key: "render", ResourceTypeKey: "workload", ExecutionProfile: "internal-k8s", DriverType: resource.DriverScoreK8s, SourceFingerpr: "old-bundle", DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{"render_bundle": "installed"}}}, Criteria: []resource.Criterion{{}}}
	req.Catalog.Definitions = append(req.Catalog.Definitions, def)
	if _, err := NewService(map[string]resource.RenderBundle{"installed": {ID: "installed", Version: "0.15.0", BinaryDigest: "binary", Digest: "changed"}}).Plan(req); err == nil {
		t.Fatal("changed fingerprint accepted")
	}
}

func TestWorkloadRenderingTypeCannotBeScoreDependency(t *testing.T) {
	req := testRequest(t, application.ProfileInternalK8s, "backend")
	req.After.Resources["renderer"] = score.ResourceSpec{Type: "workload"}
	if _, err := NewService().Plan(req); err == nil {
		t.Fatal("renderer became an infrastructure dependency")
	}
}
