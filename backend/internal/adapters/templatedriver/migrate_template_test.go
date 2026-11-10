package templatedriver

import (
	"context"
	"testing"

	"orchestrator/internal/domain/environment"
	"orchestrator/internal/renderengine"
)

// The standard backend-migrate template adds one migration Job running the
// first container with the template's migrateArgs (ADR-042); other templates
// render no Job.
func TestStandardMigrateTemplateRendersMigrationJob(t *testing.T) {
	files := readBundle(t, standardDir)
	bundle, err := renderengine.Validate(files)
	if err != nil {
		t.Fatal(err)
	}
	renderer, selections := gitRenderer(t, map[string]map[string][]byte{"standard": files})
	jobs := func(id string) []map[string]any {
		t.Helper()
		req := gitRequest(selections["standard"])
		main := req.Module.Spec.Containers["main"]
		main.Resources = nil
		req.Module.Spec.Containers["main"] = main
		one := 1
		req.Module.Spec.Replicas = &one
		for _, template := range bundle.Contract.Manifest.WorkloadTemplates {
			if template.ID == id {
				req.Module.Spec.Execution = &environment.WorkloadExecution{Version: 1, Kind: template.Kind, Template: environment.TemplatePin{ID: template.ID, Version: template.Version, SHA256: template.SHA256()}}
			}
		}
		if req.Module.Spec.Execution == nil {
			t.Fatalf("template %s missing", id)
		}
		manifests, err := renderer.Render(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		out := []map[string]any{}
		for _, m := range manifests {
			if m.Kind == "Job" {
				if m.Name != req.WorkloadID+"-migrate" || m.APIVersion != "batch/v1" {
					t.Fatalf("job identity %s %s", m.APIVersion, m.Name)
				}
				out = append(out, m.Object)
			}
		}
		return out
	}
	if got := jobs("backend"); len(got) != 0 {
		t.Fatalf("backend template rendered a Job: %v", got)
	}
	got := jobs("backend-migrate")
	if len(got) != 1 {
		t.Fatalf("migrate template jobs = %v", got)
	}
	pod := object(object(object(got[0]["spec"])["template"])["spec"])
	containers := list(pod["containers"])
	if pod["restartPolicy"] != "Never" || len(containers) != 1 {
		t.Fatalf("job pod = %v", pod)
	}
	c := object(containers[0])
	args := list(c["args"])
	if len(args) != 1 || args[0] != "migrate" || c["readinessProbe"] != nil || c["ports"] != nil {
		t.Fatalf("job container = %v", c)
	}
	labels := object(object(got[0]["metadata"])["labels"])
	if labels["orchestrator.io/workload"] == nil {
		t.Fatalf("job is not owned by the workload: %v", labels)
	}
}
