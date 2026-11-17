package workloadconfig

import (
	"context"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/templatesource"
	"orchestrator/internal/planning/workloadspec"
	"orchestrator/internal/platform/ids"
	"reflect"
	"strings"
	"testing"
)

func TestScoreProfileResolutionIsDeterministicAndRejectsAmbiguity(t *testing.T) {
	ts := []templatesource.WorkloadTemplate{{ID: "backend", Version: "1.9.0", Kind: "Deployment"}, {ID: "backend", Version: "1.10.0", Kind: "Deployment"}, {ID: "scheduled", Version: "1.0.0", Kind: "CronJob"}}
	for _, profile := range []string{"backend", "humanitec/default-module"} {
		got, err := resolveScoreProfile(profile, ts)
		if err != nil || got.Version != "1.10.0" {
			t.Fatalf("numeric selection: %s %#v %v", profile, got, err)
		}
	}
	got, err := resolveScoreProfile("backend@1.9.0", ts)
	if err != nil || got.Version != "1.9.0" {
		t.Fatal("explicit version not preserved")
	}
	ts = append(ts, templatesource.WorkloadTemplate{ID: "frontend", Version: "1.0.0", Kind: "Deployment"})
	if _, err := resolveScoreProfile("humanitec/default-module", ts); err == nil {
		t.Fatal("ambiguous default profile accepted")
	}
	for _, profile := range []string{"unknown", "backend@9.0.0", "humanitec/default-job"} {
		if _, err := resolveScoreProfile(profile, ts); err == nil {
			t.Fatal("unavailable profile accepted")
		}
	}
}

func TestScoreAdmissionUsesSelectedSourceAndFreezesPinWithoutWrites(t *testing.T) {
	ctx := context.Background()
	svc, st := paramsWorkloadService(t)
	template := templatesource.WorkloadTemplate{ID: "backend", Version: "1.0.0", Kind: "Deployment", Labels: map[string]string{"en": "Backend"}, InputsSchema: map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}, Defaults: map[string]any{}, RuntimeDefaults: &environment.RuntimeDefaults{Resources: &environment.ContainerResourceRequirements{Requests: &environment.ComputeResources{CPU: "357m"}}}}
	bundle := templatesource.Bundle{ID: ids.New(), OrganizationKey: "acme", Source: templatesource.Source{Kind: "git", URL: "https://example.com/templates.git", Path: ".", Revision: strings.Repeat("a", 40), ContentSHA256: strings.Repeat("b", 64)}, EngineContract: templatesource.EngineContract, RendererVersion: templatesource.RendererVersion, RendererBinary: strings.Repeat("c", 64), Contract: templatesource.Contract{InputContract: templatesource.InputContract, InputsSchema: map[string]any{}, Defaults: map[string]any{}, Manifest: templatesource.Manifest{WorkloadCapabilities: []string{templatesource.TemplateDefaultsV1, templatesource.CustomExecutionV1}, WorkloadTemplates: []templatesource.WorkloadTemplate{template}}}}
	if _, err := st.InsertTemplateSource(ctx, bundle); err != nil {
		t.Fatal(err)
	}
	// A newer template in a different org-scoped source must not affect resolution.
	other := bundle
	other.ID = ids.New()
	newer := template.Clone()
	newer.Version = "9.0.0"
	other.Contract.Manifest.WorkloadTemplates = []templatesource.WorkloadTemplate{newer}
	if _, err := st.InsertTemplateSource(ctx, other); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveResourceType(ctx, "acme", resource.Type{Key: "workload"}); err != nil {
		t.Fatal(err)
	}
	def := resource.Definition{Key: "runtime", Name: "Runtime", Version: 1, ResourceTypeKey: "workload", DriverType: resource.DriverTemplate, Criteria: []resource.Criterion{{}}, DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{resource.VariableTemplateSource: bundle.ID}}}}
	if err := st.SaveResourceDefinition(ctx, "acme", def); err != nil {
		t.Fatal(err)
	}
	raw := map[string]any{"apiVersion": workloadspec.ScoreAPIVersion, "metadata": map[string]any{"name": "api"}, "containers": map[string]any{"main": map[string]any{"image": "example/api:1", "variables": map[string]any{"PGHOST": "${resources.db.host}"}}}, "resources": map[string]any{"db": map[string]any{"type": "postgres", "id": "orders-db", "params": map[string]any{"database": "orders"}}}}
	before, _ := st.GetEnvironment(ctx, "app", "staging")
	normalized, err := svc.ImportDocument(ctx, "app", "staging", raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := workloadspec.FromMap(normalized)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Execution.Template.Version != "1.0.0" || doc.Execution.Template.SHA256 != template.SHA256() {
		t.Fatal("profile escaped selected source")
	}
	if doc.Execution.TemplateDefaults == nil || doc.Containers["main"].Resources != nil {
		t.Fatal("defaults must freeze separately, not rewrite authored CPU")
	}
	after, _ := st.GetEnvironment(ctx, "app", "staging")
	if !reflect.DeepEqual(before, after) {
		t.Fatal("import mutated environment")
	}
	drafts, err := st.ListWorkloadDrafts(ctx, "app", "staging")
	if err != nil || len(drafts) != 0 {
		t.Fatal("import wrote drafts")
	}
	score, ext, err := workloadspec.ExportScore(doc)
	if err != nil {
		t.Fatal(err)
	}
	again, err := svc.ImportDocument(ctx, "app", "staging", score, ext)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(normalized, again) {
		t.Fatal("pinned export/reimport changed intent/defaults")
	}
	ext["profile"] = "backend@9.0.0"
	if _, err := svc.ImportDocument(ctx, "app", "staging", score, ext); err == nil {
		t.Fatal("profile resolved against another source")
	}
}
