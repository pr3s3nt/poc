package planning

import (
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning/score"
	"orchestrator/internal/platform/canon"
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

// SourceFingerpr is registration provenance. A Definition registered under bundle
// A still plans under installed bundle B, pins B, and changes the plan hash.
func TestRenderingBundleUpgradeKeepsDefinitionAndPinsInstalledBundle(t *testing.T) {
	req := testRequest(t, application.ProfileInternalK8s, "backend")
	def := resource.Definition{Key: "render", ResourceTypeKey: "workload", ExecutionProfile: "internal-k8s", DriverType: resource.DriverScoreK8s, SourceFingerpr: "bundle-a", DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{"render_bundle": "installed"}}}, Criteria: []resource.Criterion{{}}}
	req.Catalog.Definitions = append(req.Catalog.Definitions, def)
	a := resource.RenderBundle{ID: "installed", Version: "0.15.0", BinaryDigest: "binary-a", Digest: "bundle-a"}
	b := resource.RenderBundle{ID: "installed", Version: "0.15.0", BinaryDigest: "binary-b", Digest: "bundle-b"}
	planA, err := NewService(map[string]resource.RenderBundle{"installed": a}).Plan(req)
	if err != nil {
		t.Fatal(err)
	}
	planB, err := NewService(map[string]resource.RenderBundle{"installed": b}).Plan(req)
	if err != nil {
		t.Fatalf("existing Definition rejected after bundle upgrade: %v", err)
	}
	if planA.Rendering["backend"].Bundle != a || planB.Rendering["backend"].Bundle != b {
		t.Fatal("plans must pin the installed bundle snapshot")
	}
	if planA.PlanHash == planB.PlanHash {
		t.Fatal("bundle upgrade must change the plan hash")
	}
	if planA.Rendering["backend"].DefinitionHash != planB.Rendering["backend"].DefinitionHash {
		t.Fatal("Definition content hash must not depend on the installed bundle")
	}
	changed := def
	changed.SourceFingerpr = "other"
	h1, _ := canon.Hash(def)
	h2, _ := canon.Hash(changed)
	if h1 == h2 {
		t.Fatal("registration fingerprint must stay in the Definition content hash")
	}
	// Unavailable selector ID still fails.
	if _, err := NewService(map[string]resource.RenderBundle{"other": b}).Plan(req); err == nil {
		t.Fatal("unavailable bundle ID accepted")
	}
}

func TestWorkloadRenderingTypeCannotBeScoreDependency(t *testing.T) {
	req := testRequest(t, application.ProfileInternalK8s, "backend")
	req.After.Resources["renderer"] = score.ResourceSpec{Type: "workload"}
	if _, err := NewService().Plan(req); err == nil {
		t.Fatal("renderer became an infrastructure dependency")
	}
}
