package templatedriver

import (
	"context"
	"encoding/json"
	"testing"

	"orchestrator/internal/domain/environment"
	"orchestrator/internal/renderengine"
)

func TestCompatibleSecurityFallbackPreservesIntentAcrossControllersAndRoles(t *testing.T) {
	files := readBundle(t, standardDir)
	bundle, err := renderengine.Validate(files)
	if err != nil {
		t.Fatal(err)
	}
	r, pins := gitRenderer(t, map[string]map[string][]byte{"standard": files})
	for _, kind := range []string{"Deployment", "StatefulSet", "CronJob"} {
		for _, preset := range []bool{false, true} {
			t.Run(kind+map[bool]string{false: "/custom", true: "/preset"}[preset], func(t *testing.T) {
				req := gitRequest(pins["standard"])
				one := 1
				req.Module.Spec.Replicas = &one
				req.Module.Spec.Service = nil
				req.Module.Spec.Execution = &environment.WorkloadExecution{Version: 1, Kind: kind}
				if kind == "CronJob" {
					req.Module.Spec.Execution.Schedule = &environment.Schedule{Expression: "0 0 * * *", TimeZone: "UTC", ConcurrencyPolicy: "Forbid"}
				}
				if preset {
					for _, template := range bundle.Contract.Manifest.WorkloadTemplates {
						if template.Kind == kind && template.ID != "nginx-readonly" {
							req.Module.Spec.Execution.Template = environment.TemplatePin{ID: template.ID, Version: template.Version, SHA256: template.SHA256()}
							req.Module.Spec.Execution.TemplateDefaults = &environment.TemplateDefaultsSnapshot{Environment: "staging", Profile: template.RuntimeDefaults.ForEnvironment("staging")}
							break
						}
					}
				}
				req.Module.Spec.Containers = map[string]environment.Container{}
				for _, c := range []string{"main", "setup", "proxy"} {
					var container environment.Container
					raw := `{"image":"example/app:1","securityContext":{"runAsNonRoot":true,"runAsUser":1000,"allowPrivilegeEscalation":false,"readOnlyRootFilesystem":false}}`
					if c == "setup" {
						raw = `{"image":"example/init:1","role":"init","initOrder":1}`
					}
					if c == "proxy" {
						raw = `{"image":"example/proxy:1","role":"sidecar","initOrder":2}`
					}
					if err := json.Unmarshal([]byte(raw), &container); err != nil {
						t.Fatal(err)
					}
					req.Module.Spec.Containers[c] = container
				}
				req.PlainEnv = nil
				req.SecretEnv = nil
				req.ConfigSecretName = ""
				req.ConfigSecretKeys = nil
				manifests, err := r.Render(context.Background(), req)
				if err != nil {
					t.Fatal(err)
				}
				for _, m := range manifests {
					if m.Kind != kind {
						continue
					}
					s := object(m.Object["spec"])
					if kind == "CronJob" {
						s = object(object(s["jobTemplate"])["spec"])
					}
					pod := object(object(s["template"])["spec"])
					if _, ok := pod["automountServiceAccountToken"]; ok {
						t.Fatal("invented token policy")
					}
					if _, ok := pod["securityContext"]; ok {
						t.Fatal("invented Pod security policy")
					}
					for _, field := range []string{"containers", "initContainers"} {
						for _, raw := range list(pod[field]) {
							c := object(raw)
							security := object(c["securityContext"])
							if security["privileged"] != false {
								t.Fatalf("%s lacks fallback: %v", c["name"], security)
							}
							if c["name"] == "main" {
								if security["runAsUser"] != float64(1000) || security["runAsNonRoot"] != true || security["allowPrivilegeEscalation"] != false || security["readOnlyRootFilesystem"] != false {
									t.Fatal("explicit intent changed", security)
								}
							} else if len(security) != 1 {
								t.Fatal("invented restrictions", security)
							}
						}
					}
				}
			})
		}
	}
}
