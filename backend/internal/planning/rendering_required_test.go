package planning

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
)

// dropWorkloadDefinitions removes the seeded workload Definitions so a test supplies its own.
func dropWorkloadDefinitions(req *Request) {
	var kept []resource.Definition
	for _, def := range req.Catalog.Definitions {
		if def.ResourceTypeKey != TypeWorkload {
			kept = append(kept, def)
		}
	}
	req.Catalog.Definitions = kept
}

func renderDefinition(key, profile string, criteria ...resource.Criterion) resource.Definition {
	if len(criteria) == 0 {
		criteria = []resource.Criterion{{}}
	}
	variables := map[string]any{"render_bundle": "installed"}
	if profile == string(application.ProfileInternalK8s) {
		// Definition-placed Applications reference the typed namespace and cluster outputs (ADR-016).
		variables["namespace"] = "${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}"
		variables["cluster"] = "${resources['k8s-cluster.internal#environments.@app.@env'].outputs.cluster}"
	}
	return resource.Definition{Key: key, Name: key, Version: 1, ResourceTypeKey: "workload", DriverType: resource.DriverScoreK8s,
		DriverInputs: map[string]any{"values": map[string]any{"variables": variables}}, Criteria: criteria}
}

var testBundle = resource.RenderBundle{ID: "installed", Version: "0.15.0", BinaryDigest: "binary", Digest: "bundle"}

func rendererReason(t *testing.T, err error) string {
	t.Helper()
	var staged *StageError
	if !errors.As(err, &staged) {
		t.Fatalf("expected a staged planning error, got %v", err)
	}
	return staged.Reason
}

// UC-05 BR-09 / UC-06 BR-17: both profiles need exactly one compatible
// score-k8s Definition; absence, tie or an uninstalled bundle fail in Plan,
// before any resource is provisioned, and never fall back to a native renderer.
func TestRequiredRendererRejectsMissingTiedAndUnavailableBeforeResources(t *testing.T) {
	bundles := map[string]resource.RenderBundle{"installed": testBundle}
	for _, profile := range []application.ExecutionProfile{application.ProfileInternalK8s, application.ProfileAWSEKS} {
		t.Run(string(profile), func(t *testing.T) {
			req := testRequest(t, profile, "backend")
			dropWorkloadDefinitions(&req)
			if profile == application.ProfileAWSEKS {
				// The additive planner still plans natively without a Definition (reference compatibility).
				if plan, err := NewService(testBundles()).Plan(req); err != nil || len(plan.Rendering) != 0 {
					t.Fatalf("additive planner must stay native: %v %v", plan, err)
				}
			}
			required := NewRequiredRendererService(bundles)
			if _, err := required.Plan(req); rendererReason(t, err) != ReasonRendererNoMatch {
				t.Fatalf("no Definition: %v", err)
			}
			// A non-matching criterion is no match either; there is no profile guard.
			req.Catalog.Definitions = append(req.Catalog.Definitions, renderDefinition("other-class", string(profile), resource.Criterion{Class: "special"}))
			if _, err := required.Plan(req); rendererReason(t, err) != ReasonRendererNoMatch {
				t.Fatalf("wrong class: %v", err)
			}
			req.Catalog.Definitions = append(req.Catalog.Definitions, renderDefinition("render", string(profile)))
			plan, err := required.Plan(req)
			if err != nil {
				t.Fatal(err)
			}
			selected := plan.Rendering["backend"]
			if selected.DefinitionKey != "render" || selected.DriverType != resource.DriverScoreK8s || selected.Bundle != testBundle || selected.DefinitionHash == "" {
				t.Fatalf("selection not pinned: %+v", selected)
			}
			req.Catalog.Definitions = append(req.Catalog.Definitions, renderDefinition("render-tie", string(profile)))
			if _, err := required.Plan(req); rendererReason(t, err) != ReasonRendererAmbiguous {
				t.Fatalf("tie: %v", err)
			}
			// A more specific Definition wins a former tie; weights and ties follow the Definition matcher.
			req.Catalog.Definitions = append(req.Catalog.Definitions, renderDefinition("render-specific", string(profile), resource.Criterion{ResourceID: "modules.backend", Class: "default"}))
			if plan, err = required.Plan(req); err != nil || plan.Rendering["backend"].DefinitionKey != "render-specific" {
				t.Fatalf("specific: %v", err)
			}
			if _, err := NewRequiredRendererService(map[string]resource.RenderBundle{}).Plan(req); rendererReason(t, err) != ReasonRendererUnavailable {
				t.Fatalf("bundle not installed: %v", err)
			}
		})
	}
}

// The required renderer never changes the infrastructure part of the plan.
func TestRequiredRendererKeepsInfrastructurePlanAndHidesParamsFromIntents(t *testing.T) {
	req := testRequest(t, application.ProfileAWSEKS, "backend")
	dropWorkloadDefinitions(&req)
	req.Terraform = pinnedInspector{stubInspector{}}
	req.Catalog.Definitions = append(req.Catalog.Definitions, renderDefinition("render", "aws-eks"))
	native, err := NewService(map[string]resource.RenderBundle{"installed": testBundle}).Plan(req)
	if err != nil {
		t.Fatal(err)
	}
	required, err := NewRequiredRendererService(map[string]resource.RenderBundle{"installed": testBundle}).Plan(req)
	if err != nil {
		t.Fatal(err)
	}
	if native.PlanHash != required.PlanHash || len(native.Matches) != len(required.Matches) || len(native.Batches) != len(required.Batches) {
		t.Fatal("required mode must plan identically when a Definition matches")
	}
	intents := required.ResourceIntents(nil)
	if len(intents) == 0 {
		t.Fatal("expected resource intents")
	}
	seen := map[string]planningIntent{}
	for _, intent := range intents {
		seen[intent.Descriptor] = planningIntent{intent.ResourceType, intent.Class, intent.Intent, intent.DefinitionKey}
		if intent.Ownership != OwnerOrchestrator || intent.Intent != IntentCreate {
			t.Fatalf("new managed resource expected: %+v", intent)
		}
		if intent.DriverType == string(resource.DriverTerraform) && (intent.Module == nil || intent.Module.Revision == "" || intent.Module.ContentSHA256 == "") {
			t.Fatalf("Terraform intent must show the pinned module: %+v", intent)
		}
	}
	if len(seen) != len(intents) {
		t.Fatal("duplicate descriptors in the projection")
	}
	raw, _ := json.Marshal(intents)
	for _, forbidden := range []string{"params", "password", "kubeconfig", "secret", "state", "inputs", "token"} {
		if strings.Contains(strings.ToLower(string(raw)), `"`+forbidden) {
			t.Fatalf("intent projection exposes %q: %s", forbidden, raw)
		}
	}
}

type planningIntent struct{ typ, class, intent, definition string }

// pinnedInspector adds the verified source pin to the stub contract, as the
// registry inspector does.
type pinnedInspector struct{ stubInspector }

func (p pinnedInspector) Contract(def resource.Definition) (ModuleContract, error) {
	contract, err := p.stubInspector.Contract(def)
	if err != nil {
		return contract, err
	}
	inputs, err := tfmodule.ParseDriverInputs(def.DriverValues())
	if err != nil {
		return contract, err
	}
	source := inputs.Source
	contract.Source, contract.BundleID = &source, "bundle-"+source.ModuleID
	return contract, nil
}
