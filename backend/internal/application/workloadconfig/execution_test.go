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
