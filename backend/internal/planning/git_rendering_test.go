package planning

import (
	"errors"
	"strings"
	"testing"

	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/templatesource"
)

func gitRecord(id, revision string) templatesource.Bundle {
	return templatesource.Bundle{ID: id, OrganizationKey: "org",
		Source:         templatesource.Source{Kind: "git", URL: "https://github.com/pr3s3nt/poc.git", Path: "backend/template-sources/standard", Revision: revision, ContentSHA256: strings.Repeat(revision[:1], 64)},
		EngineContract: templatesource.EngineContract, RendererVersion: templatesource.RendererVersion, RendererBinary: strings.Repeat("f", 64),
		Contract: templatesource.Contract{InputContract: templatesource.InputContract, Defaults: map[string]any{},
			InputsSchema: map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"replicas": map[string]any{"type": "integer", "default": 1.0}, "team": map[string]any{"type": "string"}}}}}
}

const gitA = "11111111-1111-4111-8111-111111111111"
const gitB = "22222222-2222-4222-8222-222222222222"

// UC-05 RB-01: the plan pins the Git source, engine and effective inputs from
// snapshot metadata; a bundle- or input-only change changes the selection
// (a pending workload update) without changing infrastructure intent.
func TestGitRenderingPinsSourceAndEffectiveInputs(t *testing.T) {
	req := testRequest(t, "internal-k8s", "backend")
	dropWorkloadDefinitions(&req)
	def := renderDefinition("render", "internal-k8s")
	def.Variables()[resource.VariableTemplateSource] = gitA
	req.Catalog.Definitions = append(req.Catalog.Definitions, def)
	req.Catalog.TemplateSources = map[string]templatesource.Bundle{gitA: gitRecord(gitA, strings.Repeat("a", 40)), gitB: gitRecord(gitB, strings.Repeat("b", 40))}
	svc := NewRequiredRendererService(testBundles())
	planA, err := svc.Plan(req)
	if err != nil {
		t.Fatal(err)
	}
	selection := planA.Rendering["backend"]
	if !selection.Bundle.Git() || selection.Bundle.Source.Revision != strings.Repeat("a", 40) || selection.Bundle.EngineContract != "template-v1" || selection.Inputs["replicas"] != 1.0 || selection.InputsHash == "" {
		t.Fatalf("selection = %+v", selection)
	}
	def.Variables()[resource.VariableTemplateInputs] = map[string]any{"team": "payments"}
	req.Catalog.Definitions[len(req.Catalog.Definitions)-1] = def
	withInputs, err := svc.Plan(req)
	if err != nil {
		t.Fatal(err)
	}
	if withInputs.Rendering["backend"].InputsHash == selection.InputsHash || withInputs.PlanHash == planA.PlanHash {
		t.Fatal("input-only change must change the pinned selection")
	}
	def.Variables()[resource.VariableTemplateSource] = gitB
	planB, err := svc.Plan(req)
	if err != nil {
		t.Fatal(err)
	}
	if planB.Rendering["backend"].Bundle.Source.Revision != strings.Repeat("b", 40) {
		t.Fatal("bundle change not pinned")
	}
	if len(planB.Matches) != len(planA.Matches) || len(planB.Batches) != len(planA.Batches) {
		t.Fatal("render selection changed infrastructure intent")
	}
	// A missing (foreign or deleted) record fails before resources, no fallback.
	delete(req.Catalog.TemplateSources, gitB)
	_, err = svc.Plan(req)
	var staged *StageError
	if !errors.As(err, &staged) || staged.Reason != ReasonRendererUnavailable {
		t.Fatalf("missing record: %v", err)
	}
	// Inputs the schema does not admit also fail before resources.
	def.Variables()[resource.VariableTemplateSource] = gitA
	def.Variables()[resource.VariableTemplateInputs] = map[string]any{"replicas": "3"}
	if _, err := svc.Plan(req); !errors.As(err, &staged) || staged.Reason != ReasonRendererUnavailable {
		t.Fatalf("invalid inputs: %v", err)
	}
}
