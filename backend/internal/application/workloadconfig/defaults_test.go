package workloadconfig

import (
	"context"
	"encoding/json"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/templatesource"
	"orchestrator/internal/planning/workloadspec"
	"orchestrator/internal/platform/ids"
	"strings"
	"testing"
)

func TestDefaultsDesiredSaveResolveAndAppliedReopenAreLossless(t *testing.T) {
	svc, st := paramsWorkloadService(t)
	template := templatesource.WorkloadTemplate{ID: "example", Version: "1.0.0", Kind: "Deployment", Labels: map[string]string{"en": "Example"}, InputsSchema: map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}, Defaults: map[string]any{}, RuntimeDefaults: &environment.RuntimeDefaults{Resources: &environment.ContainerResourceRequirements{Requests: &environment.ComputeResources{CPU: "357m"}}, Environments: map[string]*environment.RuntimeDefaults{"production": {Resources: &environment.ContainerResourceRequirements{Requests: &environment.ComputeResources{CPU: "719m"}}}}}}
	record := templatesource.Bundle{ID: ids.New(), OrganizationKey: "acme", Source: templatesource.Source{Kind: "git", URL: "https://example.com/templates.git", Path: ".", Revision: strings.Repeat("a", 40), ContentSHA256: strings.Repeat("b", 64)}, EngineContract: templatesource.EngineContract, RendererVersion: templatesource.RendererVersion, RendererBinary: strings.Repeat("c", 64), Contract: templatesource.Contract{InputContract: templatesource.InputContract, InputsSchema: map[string]any{}, Defaults: map[string]any{}, Manifest: templatesource.Manifest{WorkloadCapabilities: []string{templatesource.TemplateDefaultsV1, templatesource.CustomExecutionV1}, WorkloadTemplates: []templatesource.WorkloadTemplate{template}}}}
	if _, err := st.InsertTemplateSource(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	raw := map[string]any{}
	_ = json.Unmarshal([]byte(`{"apiVersion":"orchestrator.dev/workload/v1","metadata":{"name":"web"},"replicas":0,"containers":{"main":{"image":"example/web:1","securityContext":{"runAsNonRoot":false,"runAsUser":0}}},"workloadExecution":{"version":1,"kind":"Deployment","automountServiceAccountToken":false,"revisionHistoryLimit":0}}`), &raw)
	execution := raw["workloadExecution"].(map[string]any)
	execution["template"] = environment.TemplatePin{ID: template.ID, Version: template.Version, SHA256: template.SHA256()}
	view, err := svc.Save(ctx, "app", "staging", "web", raw, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Workloads) != 1 {
		t.Fatal(view)
	}
	// Parse -> fragment -> applied reconstruction retains overrides, never resolved values.
	saved := view.Workloads[0].Workload
	doc, err := workloadspec.FromMap(saved)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := doc.Fragment(map[string]resource.Type{})
	if err != nil {
		t.Fatal(err)
	}
	set := environment.NewDocument()
	set.Modules["web"] = fragment.Module
	reopened, err := ReconstructVerified("web", fragment.Module, set, map[string]resource.Type{})
	if err != nil {
		t.Fatal(err)
	}
	again, err := workloadspec.FromMap(reopened)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(doc.Map())
	b, _ := json.Marshal(again.Map())
	if string(a) != string(b) {
		t.Fatalf("override changed:\n%s\n%s", a, b)
	}
	effective, sources, err := svc.ResolveDefaults(ctx, "app", "staging", saved)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(effective)
	if string(encoded) == string(a) || sources["containers.main.resources.requests.cpu"] != "template" {
		t.Fatal("resolution missing", sources)
	}
	forgedRaw, _ := json.Marshal(saved)
	var forged map[string]any
	_ = json.Unmarshal(forgedRaw, &forged)
	forged["workloadExecution"].(map[string]any)["templateDefaults"].(map[string]any)["profile"].(map[string]any)["resources"].(map[string]any)["requests"].(map[string]any)["cpu"] = "999m"
	if _, _, err := svc.ResolveDefaults(ctx, "app", "staging", forged); err == nil {
		t.Fatal("forged frozen template profile accepted")
	}
	if _, _, err := svc.ResolveDefaults(ctx, "app", "production", saved); err == nil {
		t.Fatal("cross environment policy accepted")
	}
	if _, present := raw["containers"].(map[string]any)["main"].(map[string]any)["resources"]; present {
		t.Fatal("resolution mutated draft")
	}
}

func TestCustomWorkloadHasNoBackendDefaultsAndOldPolicyIsRejected(t *testing.T) {
	svc, _ := testWorkloadService(t)
	raw := map[string]any{"apiVersion": workloadspec.SupportedAPIVersion, "metadata": map[string]any{"name": "web"}, "containers": map[string]any{"main": map[string]any{"image": "example:v1"}}, "workloadExecution": map[string]any{"version": 1, "kind": "Deployment"}}
	effective, sources, err := svc.ResolveDefaults(context.Background(), "app", "staging", raw)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(effective)
	if strings.Contains(string(encoded), "100m") || strings.Contains(string(encoded), "512Mi") || sources["containers.main.resources.requests.cpu"] != "" {
		t.Fatal("backend invented defaults")
	}
	raw["workloadExecution"].(map[string]any)["defaults"] = map[string]any{"version": "workload-defaults-v1", "environment": "staging"}
	if _, err := workloadspec.FromMap(raw); err == nil {
		t.Fatal("removed backend policy accepted")
	}
}

// Saved desired remains sparse even after admission freezes template metadata.
func TestTemplateScheduleAndPDBDefaultsSurviveSaveAndReopen(t *testing.T) {
	for _, tc := range []struct {
		name, kind string
		execution  map[string]any
		profile    *environment.RuntimeDefaults
		replicas   int
	}{
		{"cron", "CronJob", map[string]any{"schedule": map[string]any{"expression": "15 3 * * *"}}, &environment.RuntimeDefaults{Execution: &environment.RuntimeExecutionDefaults{Schedule: &environment.Schedule{TimeZone: "Asia/Bangkok", ConcurrencyPolicy: "Forbid"}}}, 1},
		{"pdb", "Deployment", map[string]any{"podDisruptionBudget": map[string]any{"enabled": true}}, &environment.RuntimeDefaults{Execution: &environment.RuntimeExecutionDefaults{PodDisruptionBudget: &environment.PodDisruptionBudget{MaxUnavailable: 1}}}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, st := paramsWorkloadService(t)
			template := templatesource.WorkloadTemplate{ID: "example", Version: "1.0.0", Kind: tc.kind, Labels: map[string]string{"en": "Example"}, InputsSchema: map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}, Defaults: map[string]any{}, RuntimeDefaults: tc.profile}
			record := templatesource.Bundle{ID: ids.New(), OrganizationKey: "acme", Source: templatesource.Source{Kind: "git", URL: "https://example.com/templates.git", Path: ".", Revision: strings.Repeat("a", 40), ContentSHA256: strings.Repeat("b", 64)}, EngineContract: templatesource.EngineContract, RendererVersion: templatesource.RendererVersion, RendererBinary: strings.Repeat("c", 64), Contract: templatesource.Contract{InputContract: templatesource.InputContract, InputsSchema: map[string]any{}, Defaults: map[string]any{}, Manifest: templatesource.Manifest{WorkloadCapabilities: []string{templatesource.TemplateDefaultsV1, templatesource.CustomExecutionV1, templatesource.ControllerPolicyV1}, WorkloadTemplates: []templatesource.WorkloadTemplate{template}}}}
			if _, err := st.InsertTemplateSource(context.Background(), record); err != nil {
				t.Fatal(err)
			}
			tc.execution["version"] = 1
			tc.execution["kind"] = tc.kind
			tc.execution["template"] = environment.TemplatePin{ID: template.ID, Version: template.Version, SHA256: template.SHA256()}
			raw := map[string]any{"apiVersion": workloadspec.SupportedAPIVersion, "metadata": map[string]any{"name": "web"}, "containers": map[string]any{"main": map[string]any{"image": "busybox:1.36"}}, "replicas": tc.replicas, "workloadExecution": tc.execution}
			ctx := context.Background()
			view, err := svc.Save(ctx, "app", "staging", "web", raw, 0)
			if err != nil {
				t.Fatal(err)
			}
			saved := view.Workloads[0].Workload
			doc, err := workloadspec.FromMap(saved)
			if err != nil {
				t.Fatal("reopen", err)
			}
			if err := doc.Execution.ValidateResolved(doc.Containers, tc.replicas, false, nil); err == nil {
				t.Fatal("incomplete raw intent accepted as resolved")
			}
			if _, _, err := svc.ResolveDefaults(ctx, "app", "staging", saved); err != nil {
				t.Fatal("resolve saved", err)
			}
			if _, err := svc.Save(ctx, "app", "staging", "web", saved, view.DraftVersion); err != nil {
				t.Fatal("save reopened", err)
			}
		})
	}
}
