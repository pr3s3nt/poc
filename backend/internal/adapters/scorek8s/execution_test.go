package scorek8s

import (
	"context"
	"testing"

	"orchestrator/internal/domain/environment"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/renderengine"
)

func TestExecutionRendersPrimaryControllersAndApprovedMounts(t *testing.T) {
	files := readBundle(t, standardDir)
	bundle, err := renderengine.Validate(files)
	if err != nil {
		t.Fatal(err)
	}
	renderer, selections := gitRenderer(t, map[string]map[string][]byte{"standard": files})
	for _, id := range []string{"backend", "stateful", "scheduled"} {
		t.Run(id, func(t *testing.T) {
			req := gitRequest(selections["standard"])
			var chosen int
			for i, template := range bundle.Contract.Manifest.WorkloadTemplates {
				if template.ID == id {
					chosen = i
				}
			}
			template := bundle.Contract.Manifest.WorkloadTemplates[chosen]
			e := &environment.WorkloadExecution{Version: 1, Kind: template.Kind, Template: environment.TemplatePin{ID: template.ID, Version: template.Version, SHA256: template.SHA256()}, Mounts: []environment.StorageMount{{Resource: "externals.data", Container: "main", Path: "/data"}}}
			one := 1
			req.Module.Spec.Replicas = &one
			if id == "scheduled" {
				zero := int32(0)
				e.Schedule = &environment.Schedule{Expression: "0 0 * * *", TimeZone: "UTC", ConcurrencyPolicy: "Forbid", SuccessfulJobsHistoryLimit: &zero}
				req.Module.Spec.Service = nil
			}
			req.Module.Spec.Execution = e
			req.Storage = map[string]execution.StorageBinding{"externals.data": {ClaimName: "owned-data", Namespace: req.Namespace}}
			manifests, err := renderer.Render(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, m := range manifests {
				if m.Name != req.WorkloadID || m.Kind != e.Kind {
					continue
				}
				found = true
				spec := object(m.Object["spec"])
				if e.Kind == "Deployment" {
					strategy := object(spec["strategy"])
					if strategy["type"] != "Recreate" || len(strategy) != 1 {
						t.Fatalf("PVC Deployment must use Recreate without rollingUpdate: %v", strategy)
					}
				} else if _, ok := spec["strategy"]; ok {
					t.Fatalf("Deployment strategy leaked into %s", e.Kind)
				}
				if id == "scheduled" {
					if spec["successfulJobsHistoryLimit"] != float64(0) {
						t.Fatal("explicit zero policy lost")
					}
					spec = object(object(spec["jobTemplate"])["spec"])
				}
				pod := object(object(spec["template"])["spec"])
				volumes := list(pod["volumes"])
				if len(volumes) != 1 || object(object(volumes[0])["persistentVolumeClaim"])["claimName"] != "owned-data" {
					t.Fatalf("mount lost: %v", pod)
				}
			}
			if !found {
				t.Fatal("primary controller missing")
			}
			req.Storage = nil
			if _, err = renderer.Render(context.Background(), req); err == nil {
				t.Fatal("render accepted missing approved storage binding")
			}
		})
	}
}

func TestDeploymentWithoutPersistentMountKeepsRolloutPolicy(t *testing.T) {
	files := readBundle(t, standardDir)
	bundle, err := renderengine.Validate(files)
	if err != nil {
		t.Fatal(err)
	}
	renderer, selections := gitRenderer(t, map[string]map[string][]byte{"standard": files})
	for _, mode := range []string{"legacy", "stateless", "secret-file-only"} {
		t.Run(mode, func(t *testing.T) {
			req := gitRequest(selections["standard"])
			if mode != "legacy" {
				for _, template := range bundle.Contract.Manifest.WorkloadTemplates {
					if template.ID == "backend" {
						req.Module.Spec.Execution = &environment.WorkloadExecution{Version: 1, Kind: template.Kind, Template: environment.TemplatePin{ID: template.ID, Version: template.Version, SHA256: template.SHA256()}}
					}
				}
				if mode == "secret-file-only" {
					req.Module.Spec.Execution.SecretFiles = []environment.SecretFile{{Ref: "${context.uc12.GPG_KEY}", Container: "main", Path: "/keys/server.asc", Mode: 0400}}
					req.ConfigSecretName = "revision-specific"
					req.ConfigSecretKeys = map[string]map[string]string{"main": {"orchestrator-file-0": "gpg-key"}}
				}
			}
			manifests, err := renderer.Render(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, m := range manifests {
				if m.Kind == "Deployment" {
					found = true
					if strategy := object(object(m.Object["spec"])["strategy"]); len(strategy) != 0 {
						t.Fatalf("unexpected rollout policy without PVC mount: %v", strategy)
					}
				}
			}
			if !found {
				t.Fatal("Deployment missing")
			}
		})
	}
}

func TestCronJobFileSecretStaysReferenceOnlyAndNotEnvironment(t *testing.T) {
	files := readBundle(t, standardDir)
	bundle, err := renderengine.Validate(files)
	if err != nil {
		t.Fatal(err)
	}
	renderer, selections := gitRenderer(t, map[string]map[string][]byte{"standard": files})
	var templateIndex int
	for i, template := range bundle.Contract.Manifest.WorkloadTemplates {
		if template.ID == "scheduled" {
			templateIndex = i
		}
	}
	template := bundle.Contract.Manifest.WorkloadTemplates[templateIndex]
	req := gitRequest(selections["standard"])
	req.Module.Spec.Service = nil
	req.Module.Spec.Replicas = nil
	req.Module.Spec.Execution = &environment.WorkloadExecution{Version: 1, Kind: "CronJob", Template: environment.TemplatePin{ID: template.ID, Version: template.Version, SHA256: template.SHA256()}, Schedule: &environment.Schedule{Expression: "* * * * *", TimeZone: "UTC", ConcurrencyPolicy: "Forbid"}, SecretFiles: []environment.SecretFile{{Ref: "${context.uc12.GPG_KEY}", Container: "main", Path: "/keys/server.asc", Mode: 0400}}}
	req.ConfigSecretName = "revision-specific"
	req.ConfigSecretKeys = map[string]map[string]string{"main": {"orchestrator-file-0": "gpg-key"}}
	manifests, err := renderer.Render(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range manifests {
		if m.Kind != "CronJob" {
			continue
		}
		pod := object(object(object(object(m.Object["spec"])["jobTemplate"])["spec"])["template"])
		containers := list(object(pod["spec"])["containers"])
		for _, raw := range containers {
			for _, entry := range list(object(raw)["env"]) {
				if object(entry)["name"] == "orchestrator-file-0" {
					t.Fatal("file secret also emitted as environment")
				}
			}
		}
	}
}
