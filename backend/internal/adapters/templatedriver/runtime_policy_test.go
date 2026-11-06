package templatedriver

import (
	"context"
	"encoding/json"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/templatesource"
	"orchestrator/internal/renderengine"
	"testing"
)

func TestRuntimePolicyAllControllersAndPDB(t *testing.T) {
	r, pins := gitRenderer(t, map[string]map[string][]byte{"standard": readBundle(t, standardDir)})
	for _, kind := range []string{"Deployment", "StatefulSet", "CronJob"} {
		t.Run(kind, func(t *testing.T) {
			req := gitRequest(pins["standard"])
			raw := `{"version":1,"kind":"` + kind + `","defaults":{"version":"workload-defaults-v1","environment":"production"},"automountServiceAccountToken":false,"podSecurityContext":{"fsGroup":0},"revisionHistoryLimit":0}`
			var e environment.WorkloadExecution
			_ = json.Unmarshal([]byte(raw), &e)
			if kind == "Deployment" {
				two := 2
				req.Module.Spec.Replicas = &two
				e.PodDisruptionBudget = &environment.PodDisruptionBudget{Enabled: true, MaxUnavailable: 1, UnhealthyPodEvictionPolicy: "AlwaysAllow"}
			}
			if kind == "StatefulSet" {
				one := 1
				req.Module.Spec.Replicas = &one
				e.VolumeClaimTemplates = []environment.ClaimTemplate{{Name: "data", Size: "1Gi", AccessModes: []string{"ReadWriteOnce"}}}
				e.ClaimMounts = []environment.ClaimMount{{Claim: "data", Container: "main", Path: "/data"}}
			}
			if kind == "CronJob" {
				one := 1
				req.Module.Spec.Replicas = &one
				e.RevisionHistoryLimit = nil
				e.Schedule = &environment.Schedule{Expression: "0 0 * * *", TimeZone: "Asia/Bangkok", ConcurrencyPolicy: "Forbid"}
				req.Module.Spec.Service = nil
			}
			req.Module.Spec.Execution = &e
			c := req.Module.Spec.Containers["main"]
			f := false
			zero := int64(0)
			c.Resources = nil
			c.SecurityContext = &environment.ContainerSecurityContext{RunAsNonRoot: &f, RunAsUser: &zero}
			req.Module.Spec.Containers["main"] = c
			manifests, err := r.Render(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			pdb := false
			for _, m := range manifests {
				if m.Kind == "PodDisruptionBudget" {
					pdb = true
				}
				if m.Kind != kind {
					continue
				}
				s := object(m.Object["spec"])
				if kind == "CronJob" {
					if s["failedJobsHistoryLimit"] != float64(3) {
						t.Fatal("history default lost")
					}
					s = object(object(s["jobTemplate"])["spec"])
				} else if s["revisionHistoryLimit"] != float64(0) {
					t.Fatal("zero lost")
				}
				p := object(object(s["template"])["spec"])
				if p["automountServiceAccountToken"] != false {
					t.Fatal("token flag lost")
				}
				if object(p["securityContext"])["fsGroup"] != float64(0) {
					t.Fatal("fsGroup zero lost")
				}
			}
			if pdb != (kind == "Deployment") {
				t.Fatal("unexpected PDB presence")
			}
		})
	}
}

func TestPinnedNginxProfileSuppliesImageServiceAndWritablePaths(t *testing.T) {
	files := readBundle(t, standardDir)
	bundle, err := renderengine.Validate(files)
	if err != nil {
		t.Fatal(err)
	}
	r, pins := gitRenderer(t, map[string]map[string][]byte{"standard": files})
	var template templatesource.WorkloadTemplate
	for _, t := range bundle.Contract.Manifest.WorkloadTemplates {
		if t.ID == "nginx-readonly" {
			template = t
		}
	}
	req := gitRequest(pins["standard"])
	req.WorkloadID = "nginx"
	req.Module.Spec = environment.ModuleSpec{Containers: map[string]environment.Container{"main": {}}, Execution: &environment.WorkloadExecution{Version: 1, Kind: "Deployment", Template: environment.TemplatePin{ID: template.ID, Version: template.Version, SHA256: template.SHA256()}, Defaults: &environment.DefaultsPolicy{Version: "workload-defaults-v1", Environment: "staging"}}}
	req.PlainEnv = nil
	req.SecretEnv = nil
	req.ConfigSecretName = ""
	req.ConfigSecretKeys = nil
	req.ImagePullSecret = ""
	manifests, err := r.Render(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range manifests {
		if m.Kind == "Deployment" {
			p := object(object(object(m.Object["spec"])["template"])["spec"])
			c := object(list(p["containers"])[0])
			if c["image"] != template.RuntimeDefaults.Image || len(list(c["volumeMounts"])) != 2 || object(c["securityContext"])["readOnlyRootFilesystem"] != true {
				t.Fatal(c)
			}
		}
		if m.Kind == "Service" {
			found = true
		}
	}
	if !found {
		t.Fatal("profile Service missing")
	}
	if req.Module.Spec.Containers["main"].Image != "" || req.Module.Spec.Service != nil {
		t.Fatal("render mutated desired")
	}
}
