package workloadconfig

import (
	"context"
	"encoding/json"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning/workloadspec"
	"testing"
)

func TestDefaultsDesiredSaveResolveAndAppliedReopenAreLossless(t *testing.T) {
	svc, _ := testWorkloadService(t)
	ctx := context.Background()
	raw := map[string]any{}
	_ = json.Unmarshal([]byte(`{"apiVersion":"orchestrator.dev/workload/v1","metadata":{"name":"web"},"replicas":0,"containers":{"main":{"image":"example/web:1","securityContext":{"runAsNonRoot":false,"runAsUser":0}}},"workloadExecution":{"version":1,"kind":"Deployment","defaults":{"version":"workload-defaults-v1","environment":"staging"},"automountServiceAccountToken":false,"revisionHistoryLimit":0}}`), &raw)
	view, err := svc.Save(ctx, "app", "staging", "web", raw, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Workloads) != 1 {
		t.Fatal(view)
	}
	// Parse -> fragment -> applied reconstruction retains overrides, never resolved values.
	doc, err := workloadspec.FromMap(raw)
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
	effective, sources, err := svc.ResolveDefaults(ctx, "app", "staging", raw)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(effective)
	if string(encoded) == string(a) || sources["containers.main.resources.requests.cpu"] != "environment" {
		t.Fatal("resolution missing", sources)
	}
	if _, _, err := svc.ResolveDefaults(ctx, "app", "production", raw); err == nil {
		t.Fatal("cross environment policy accepted")
	}
	if _, present := raw["containers"].(map[string]any)["main"].(map[string]any)["resources"]; present {
		t.Fatal("resolution mutated draft")
	}
}
