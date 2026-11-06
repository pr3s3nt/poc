package templatedriver

import (
	"context"
	"fmt"
	"orchestrator/internal/adapters/kubernetes"
	"strings"
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
			// Runtime tests inherit the selected template profile rather than incidental limits from gitRequest.
			runtimeContainer := req.Module.Spec.Containers["main"]
			runtimeContainer.Resources = nil
			req.Module.Spec.Containers["main"] = runtimeContainer
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
			// Runtime tests inherit the selected template profile rather than incidental limits from gitRequest.
			runtimeContainer := req.Module.Spec.Containers["main"]
			runtimeContainer.Resources = nil
			req.Module.Spec.Containers["main"] = runtimeContainer
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
					strategy := object(object(m.Object["spec"])["strategy"])
					if mode == "legacy" {
						if len(strategy) != 0 {
							t.Fatalf("legacy acquired rollout policy: %v", strategy)
						}
					} else if strategy["type"] != "RollingUpdate" || object(strategy["rollingUpdate"])["maxSurge"] != float64(1) {
						t.Fatalf("template rollout policy lost: %v", strategy)
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
	// Runtime tests inherit the selected template profile rather than incidental limits from gitRequest.
	runtimeContainer := req.Module.Spec.Containers["main"]
	runtimeContainer.Resources = nil
	req.Module.Spec.Containers["main"] = runtimeContainer
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

func TestTemplateResourceReferencesAreInputData(t *testing.T) {
	files := withResources(t, `{{ toJson (list (dict "apiVersion" "v1" "kind" "ConfigMap" "metadata" (dict "name" "database-endpoint") "data" (dict "host" .Resources.databaseHost))) }}`)
	r, selections := gitRenderer(t, map[string]map[string][]byte{"references": files})
	req := gitRequest(selections["references"])
	req.ResourceReferences = map[string]any{"databaseHost": "database.internal"}
	manifests, err := r.Render(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, manifest := range manifests {
		if manifest.Name == "database-endpoint" {
			found = true
			if object(manifest.Object["data"])["host"] != "database.internal" {
				t.Fatal("resolved output lost")
			}
		}
	}
	if !found {
		t.Fatal("missing ConfigMap")
	}
}

func TestCustomLifecycleAndAllVolumeSourcesAcrossControllers(t *testing.T) {
	files := readBundle(t, standardDir)
	renderer, selections := gitRenderer(t, map[string]map[string][]byte{"standard": files})
	for _, kind := range []string{"Deployment", "StatefulSet", "CronJob"} {
		t.Run(kind, func(t *testing.T) {
			req := gitRequest(selections["standard"])
			// Runtime tests inherit the selected template profile rather than incidental limits from gitRequest.
			runtimeContainer := req.Module.Spec.Containers["main"]
			runtimeContainer.Resources = nil
			req.Module.Spec.Containers["main"] = runtimeContainer
			one := 1
			req.Module.Spec.Replicas = &one
			req.Module.Spec.Containers["aaa-prepare"] = environment.Container{Image: "busybox:1", Role: "init", InitOrder: 20, Command: []string{"sh", "-c"}, Args: []string{"cp /config/app.conf /shared/app.conf"}}
			req.Module.Spec.Containers["zzz-agent"] = environment.Container{Image: "busybox:1", Role: "sidecar", InitOrder: 10, Args: []string{"sleep", "3600"}}
			e := &environment.WorkloadExecution{Version: 1, Kind: kind, Volumes: []environment.PodVolume{{Name: "shared", EmptyDir: &environment.EmptyDirVolume{Medium: "Memory", SizeLimit: "64Mi"}}, {Name: "config", ConfigMap: &environment.ConfigMapVolume{Files: []environment.ConfigFile{{Name: "app.conf", Value: "literal\n${DO_NOT_EXPAND}"}}}}, {Name: "node", HostPath: &environment.HostPathVolume{Path: "/var/local/app", Type: "Directory"}}}}
			if kind == "CronJob" {
				e.Schedule = &environment.Schedule{Expression: "0 0 * * *", TimeZone: "UTC", ConcurrencyPolicy: "Forbid"}
				req.Module.Spec.Service = nil
			}
			req.Module.Spec.Execution = e
			req.Storage = map[string]execution.StorageBinding{"externals.data": {ClaimName: "kept-data", Namespace: req.Namespace}}
			req.ConfigSecretKeys = map[string]map[string]string{}
			req.ConfigSecretName = "approved-revision"
			for i, name := range []string{"main", "aaa-prepare", "zzz-agent"} {
				e.Mounts = append(e.Mounts, environment.StorageMount{Resource: "externals.data", Container: name, Path: "/data", SubPath: "files"})
				e.VolumeMounts = append(e.VolumeMounts, environment.VolumeMount{Volume: "shared", Container: name, Path: "/shared"}, environment.VolumeMount{Volume: "config", Container: name, Path: "/config/app.conf", SubPath: "app.conf", ReadOnly: true}, environment.VolumeMount{Volume: "node", Container: name, Path: "/node", ReadOnly: i != 0})
				e.SecretFiles = append(e.SecretFiles, environment.SecretFile{Ref: "${context.uc12.KEY}", Container: name, Path: "/keys/key", Mode: 0400})
				req.ConfigSecretKeys[name] = map[string]string{fmt.Sprintf("orchestrator-file-%d", i): "approved-key"}
			}
			manifests, err := renderer.Render(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			var pod map[string]any
			var configName string
			for _, m := range manifests {
				if m.Kind == "ConfigMap" {
					configName = m.Name
					if object(m.Object["data"])["app.conf"] != "literal\n${DO_NOT_EXPAND}" {
						t.Fatal("config file changed")
					}
				}
				if m.Kind == kind {
					spec := object(m.Object["spec"])
					if kind == "CronJob" {
						spec = object(object(spec["jobTemplate"])["spec"])
					}
					pod = object(object(spec["template"])["spec"])
				}
			}
			if configName == "" || pod == nil {
				t.Fatal("missing owned ConfigMap or controller")
			}
			if len(list(pod["containers"])) != 1 {
				t.Fatal("non-app containers leaked into app lifecycle")
			}
			init := list(pod["initContainers"])
			if len(init) != 2 || object(init[0])["name"] != "zzz-agent" || object(init[0])["restartPolicy"] != "Always" || object(init[1])["name"] != "aaa-prepare" {
				t.Fatal("init/sidecar order or lifecycle lost")
			}
			for _, raw := range podContainers(pod) {
				c := object(raw)
				if len(list(c["volumeMounts"])) != 5 {
					t.Fatal("not all volume sources mounted")
				}
				for _, rawEnv := range list(c["env"]) {
					if strings.HasPrefix(stringOrEmpty(object(rawEnv)["name"]), "orchestrator-file-") {
						t.Fatal("file Secret became env variable")
					}
				}
			}
			changed := req
			changed.Module.Spec.Execution = e.Clone()
			changed.Module.Spec.Execution.Volumes[1].ConfigMap.Files[0].Value = "new config"
			if object(configMaps(changed)["config"])["name"] == configName {
				t.Fatal("config revision did not change Pod ref")
			}
			// Verify the guards against mutations of a successfully rendered primary Pod.
			delivery, err := kubernetes.PrepareDelivery(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			var host map[string]any
			for _, raw := range list(pod["volumes"]) {
				if h := object(object(raw)["hostPath"]); h != nil {
					host = h
				}
			}
			if host == nil || host["path"] != "/var/local/app" || host["type"] != "Directory" {
				t.Fatal("host path source lost")
			}
			for _, field := range []string{"path", "type"} {
				old := host[field]
				host[field] = "changed"
				if guard(req, delivery, withoutAuxiliary(manifests)) == nil {
					t.Fatalf("accepted changed host path %s", field)
				}
				host[field] = old
			}
			for _, c := range podContainers(pod) {
				for _, raw := range list(object(c)["volumeMounts"]) {
					m := object(raw)
					if m["mountPath"] != "/node" {
						continue
					}
					old := m["readOnly"]
					m["readOnly"] = !old.(bool)
					if guard(req, delivery, withoutAuxiliary(manifests)) == nil {
						t.Fatal("accepted changed host mount mode")
					}
					m["readOnly"] = old
				}
			}
			object(init[0])["restartPolicy"] = "Never"
			if guard(req, delivery, withoutAuxiliary(manifests)) == nil {
				t.Fatal("accepted changed sidecar lifecycle")
			}
		})
	}
}
func withoutAuxiliary(in []execution.Manifest) []execution.Manifest {
	out := []execution.Manifest{}
	for _, m := range in {
		if !m.Auxiliary {
			out = append(out, m)
		}
	}
	return out
}

func TestExtendedProbeIntentOverridesDefaults(t *testing.T) {
	files := readBundle(t, standardDir)
	renderer, selections := gitRenderer(t, map[string]map[string][]byte{"standard": files})
	req := gitRequest(selections["standard"])
	// Runtime tests inherit the selected template profile rather than incidental limits from gitRequest.
	runtimeContainer := req.Module.Spec.Containers["main"]
	runtimeContainer.Resources = nil
	req.Module.Spec.Containers["main"] = runtimeContainer
	zero, one, two := 0, 1, 2
	c := req.Module.Spec.Containers["main"]
	c.ReadinessProbe = &environment.Probe{Type: "tcp", Port: 5432, InitialDelaySeconds: &zero, PeriodSeconds: &two}
	c.LivenessProbe = &environment.Probe{Type: "exec", Command: []string{"/bin/check", "--ready"}, TimeoutSeconds: &one}
	c.StartupProbe = &environment.Probe{Path: "/start", Port: 8443, Scheme: "HTTPS", FailureThreshold: &two}
	c.Ports = []environment.ContainerPort{{Name: "dns", ContainerPort: 5353, Protocol: "UDP"}}
	req.Module.Spec.Containers["main"] = c
	got, err := renderer.Render(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range got {
		if m.Kind != "Deployment" {
			continue
		}
		pod := object(object(object(m.Object["spec"])["template"])["spec"])
		main := object(list(pod["containers"])[0])
		p := object(main["readinessProbe"])
		if toNumber(p["initialDelaySeconds"]) != 0 || toNumber(p["periodSeconds"]) != 2 || object(p["tcpSocket"])["port"] != float64(5432) {
			t.Fatal("explicit TCP probe lost")
		}
		if object(main["livenessProbe"])["exec"] == nil || object(object(main["startupProbe"])["httpGet"])["scheme"] != "HTTPS" {
			t.Fatal("probe actions lost")
		}
	}
}
