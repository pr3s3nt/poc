package workloadconfig

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning/workloadspec"
)

func TestExecutionRoundTripThroughAppliedSet(t *testing.T) {
	for _, kind := range []string{"Deployment", "StatefulSet", "CronJob"} {
		t.Run(kind, func(t *testing.T) {
			execution := &environment.WorkloadExecution{Version: 1, Kind: kind, Template: environment.TemplatePin{ID: "custom", Version: "1.2.3", SHA256: strings.Repeat("a", 64)}}
			if kind == "CronJob" {
				zero := int32(0)
				execution.Schedule = &environment.Schedule{Expression: "0 0 * * *", TimeZone: "UTC", ConcurrencyPolicy: "Forbid", BackoffLimit: &zero}
			}
			document := workloadspec.Document{APIVersion: workloadspec.SupportedAPIVersion, Metadata: workloadspec.Metadata{Name: "example"}, Containers: map[string]environment.Container{"main": {Image: "example:v1"}}, Execution: execution}
			fragment, err := document.Fragment(map[string]resource.Type{})
			if err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(environment.Document{Modules: map[string]environment.Module{"example": fragment.Module}, Shared: map[string]environment.ResourceEntry{}})
			if err != nil {
				t.Fatal(err)
			}
			var set environment.Document
			if err = json.Unmarshal(raw, &set); err != nil {
				t.Fatal(err)
			}
			restored, err := ReconstructWorkload("example", set.Modules["example"], set)
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := workloadspec.FromMap(restored)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(parsed.Execution, execution) {
				t.Fatalf("execution lost: %#v", parsed.Execution)
			}
			fragment.Module.Spec.Execution.Template.ID = "changed"
			if document.Execution.Template.ID != "custom" {
				t.Fatal("fragment aliases Workload execution")
			}
		})
	}
}

func TestExecutionMountAliasSurvivesSharedResourceRoundTrip(t *testing.T) {
	execution := &environment.WorkloadExecution{Version: 1, Kind: "StatefulSet", Template: environment.TemplatePin{ID: "stateful", Version: "1.0.0", SHA256: strings.Repeat("a", 64)}, Mounts: []environment.StorageMount{{Resource: "photos", Container: "main", Path: "/data"}}}
	doc := workloadspec.Document{APIVersion: workloadspec.SupportedAPIVersion, Metadata: workloadspec.Metadata{Name: "web"}, Containers: map[string]environment.Container{"main": {Image: "web:v1"}}, Execution: execution, Resources: map[string]workloadspec.ResourceSpec{"photos": {Type: "storage", ID: "shared-photos"}}}
	types := map[string]resource.Type{"storage": {Key: "storage", Outputs: []resource.OutputField{{Name: "claimName", Type: "string"}, {Name: "namespace", Type: "string"}}}}
	fragment, err := doc.Fragment(types)
	if err != nil {
		t.Fatal(err)
	}
	if fragment.Module.Spec.Execution.Mounts[0].Resource != "shared.shared-photos" {
		t.Fatal("shared binding not normalized")
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	var tree map[string]any
	_ = json.Unmarshal(raw, &tree)
	aliases := KeepResourceAliases(nil, environment.Document{Modules: map[string]environment.Module{"web": fragment.Module}}, &doc)["web"]
	set := environment.Document{Modules: map[string]environment.Module{"web": fragment.Module}, Shared: fragment.Shared}
	restored, err := ReconstructWorkload("web", fragment.Module, set, aliases)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := workloadspec.FromMap(restored)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Execution.Mounts[0].Resource != "photos" {
		t.Fatalf("alias lost: %v", parsed.Execution.Mounts)
	}
}

func TestCustomVolumeAndLifecycleSaveImportAndAppliedRoundTrip(t *testing.T) {
	svc, _ := testWorkloadService(t)
	raw := testWorkload("api", nil, nil)
	raw["containers"] = map[string]any{"main": map[string]any{"image": "app:1"}, "prepare": map[string]any{"image": "init:1", "role": "init", "initOrder": 2}, "agent": map[string]any{"image": "agent:1", "role": "sidecar", "initOrder": 1}}
	raw["workloadExecution"] = map[string]any{"version": 1, "kind": "Deployment", "volumes": []any{map[string]any{"name": "shared", "emptyDir": map[string]any{"medium": "Memory", "sizeLimit": "128Mi"}}, map[string]any{"name": "config", "configMap": map[string]any{"files": []any{map[string]any{"name": "app.conf", "value": "line one\n${LITERAL}"}}}}}, "volumeMounts": []any{map[string]any{"volume": "shared", "container": "prepare", "path": "/shared", "readOnly": false}, map[string]any{"volume": "config", "container": "agent", "path": "/config/app.conf", "readOnly": true, "subPath": "app.conf"}}}
	executionIntent := raw["workloadExecution"].(map[string]any)
	executionIntent["volumes"] = append(executionIntent["volumes"].([]any), map[string]any{"name": "node-data", "hostPath": map[string]any{"path": "/var/local/app", "type": "Directory"}})
	for _, name := range []string{"main", "prepare", "agent"} {
		executionIntent["volumeMounts"] = append(executionIntent["volumeMounts"].([]any), map[string]any{"volume": "node-data", "container": name, "path": "/node", "readOnly": name != "main"})
	}
	if _, err := svc.Save(context.Background(), "app", "staging", "api", raw, 0); err != nil {
		t.Fatal(err)
	}
	view, err := svc.List(context.Background(), "app", "staging")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := workloadspec.FromMap(view.Workloads[0].Workload)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := doc.Fragment(map[string]resource.Type{})
	if err != nil {
		t.Fatal(err)
	}
	set := environment.Document{Modules: map[string]environment.Module{"api": fragment.Module}}
	restored, err := ReconstructWorkload("api", fragment.Module, set)
	if err != nil {
		t.Fatal(err)
	}
	again, err := workloadspec.FromMap(restored)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(again.Execution, doc.Execution) || !reflect.DeepEqual(again.Containers, doc.Containers) {
		t.Fatalf("round-trip changed runtime intent")
	}
}

func TestSecretFilesOnlyDeclareEnvironmentOnAppliedReconstruction(t *testing.T) {
	module := environment.Module{Profile: environment.ModuleProfile, Spec: environment.ModuleSpec{
		Containers: map[string]environment.Container{"main": {Image: "app:1"}},
		Execution:  &environment.WorkloadExecution{Version: 1, Kind: "Deployment", SecretFiles: []environment.SecretFile{{Ref: "${context.uc12.TOKEN}", Container: "main", Path: "/secrets/token"}}},
	}}
	set := environment.Document{Modules: map[string]environment.Module{"app": module}}
	raw, err := ReconstructWorkload("app", module, set)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := workloadspec.FromMap(raw)
	if err != nil {
		t.Fatal("Reconstructed file-only workload cannot be previewed", err)
	}
	if parsed.Resources["env"].Type != "environment" || parsed.Execution.SecretFiles[0].Ref != "${resources.env.TOKEN}" {
		t.Fatal("Secret resource alias lost")
	}
	fragment, err := parsed.Fragment(map[string]resource.Type{})
	if err != nil {
		t.Fatal(err)
	}
	if fragment.Module.Spec.Execution.SecretFiles[0].Ref != "${context.uc12.TOKEN}" {
		t.Fatal("Applied secret reference changed")
	}
}
