package workloadconfig

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning/score"
)

func TestExecutionRoundTripThroughAppliedSet(t *testing.T) {
	for _, kind := range []string{"Deployment", "StatefulSet", "CronJob"} {
		t.Run(kind, func(t *testing.T) {
			execution := &environment.WorkloadExecution{Version: 1, Kind: kind, Template: environment.TemplatePin{ID: "custom", Version: "1.2.3", SHA256: strings.Repeat("a", 64)}}
			if kind == "CronJob" {
				zero := int32(0)
				execution.Schedule = &environment.Schedule{Expression: "0 0 * * *", TimeZone: "UTC", ConcurrencyPolicy: "Forbid", BackoffLimit: &zero}
			}
			document := score.Document{APIVersion: score.SupportedAPIVersion, Metadata: score.Metadata{Name: "example"}, Containers: map[string]environment.Container{"main": {Image: "example:v1"}}, Execution: execution}
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
			restored, err := ReconstructScore("example", set.Modules["example"], set)
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := score.FromMap(restored)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(parsed.Execution, execution) {
				t.Fatalf("execution lost: %#v", parsed.Execution)
			}
			fragment.Module.Spec.Execution.Template.ID = "changed"
			if document.Execution.Template.ID != "custom" {
				t.Fatal("fragment aliases Score execution")
			}
		})
	}
}

func TestExecutionMountAliasSurvivesSharedResourceRoundTrip(t *testing.T) {
	execution := &environment.WorkloadExecution{Version: 1, Kind: "StatefulSet", Template: environment.TemplatePin{ID: "stateful", Version: "1.0.0", SHA256: strings.Repeat("a", 64)}, Mounts: []environment.StorageMount{{Resource: "photos", Container: "main", Path: "/data"}}}
	doc := score.Document{APIVersion: score.SupportedAPIVersion, Metadata: score.Metadata{Name: "web"}, Containers: map[string]environment.Container{"main": {Image: "web:v1"}}, Execution: execution, Resources: map[string]score.ResourceSpec{"photos": {Type: "storage", ID: "shared-photos"}}}
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
	restored, err := ReconstructScore("web", fragment.Module, set, aliases)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := score.FromMap(restored)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Execution.Mounts[0].Resource != "photos" {
		t.Fatalf("alias lost: %v", parsed.Execution.Mounts)
	}
}
