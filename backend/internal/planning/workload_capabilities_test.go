package planning

import (
	"errors"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/templatesource"
	"strings"
	"testing"
)

func TestCustomExecutionRequiresPinnedSourceCapabilitiesBeforePlanningResources(t *testing.T) {
	req := testRequest(t, "internal-k8s", "backend")
	dropWorkloadDefinitions(&req)
	def := renderDefinition("custom", "internal-k8s")
	def.Variables()[resource.VariableTemplateSource] = gitA
	req.Catalog.Definitions = append(req.Catalog.Definitions, def)
	source := gitRecord(gitA, strings.Repeat("a", 40))
	source.Contract.Manifest.WorkloadCapabilities = []string{templatesource.CustomExecutionV1, templatesource.PodVolumesV1, templatesource.ContainerLifecycleV1, templatesource.HostPathVolumesV1}
	req.Catalog.TemplateSources = map[string]templatesource.Bundle{gitA: source}
	req.After.Execution = &environment.WorkloadExecution{Version: 1, Kind: "Deployment", Volumes: []environment.PodVolume{{Name: "shared", HostPath: &environment.HostPathVolume{Path: "/var/local/app", Type: "Directory"}}}, VolumeMounts: []environment.VolumeMount{{Volume: "shared", Container: "main", Path: "/shared"}}}
	req.After.Containers["prepare"] = environment.Container{Role: "init", InitOrder: 1, Image: "prepare:1"}
	svc := NewRequiredRendererService(testBundles())
	plan, err := svc.Plan(req)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Rendering["backend"].Bundle.ID != gitA {
		t.Fatal("custom execution lost matched source pin")
	}
	for _, missing := range source.Contract.Manifest.WorkloadCapabilities {
		broken := source.Clone()
		broken.Contract.Manifest.WorkloadCapabilities = nil
		for _, name := range source.Contract.Manifest.WorkloadCapabilities {
			if name != missing {
				broken.Contract.Manifest.WorkloadCapabilities = append(broken.Contract.Manifest.WorkloadCapabilities, name)
			}
		}
		req.Catalog.TemplateSources[gitA] = broken
		if _, err := svc.Plan(req); !errors.Is(err, environment.ErrExecutionUnavailable) {
			t.Fatalf("missing %s accepted: %v", missing, err)
		}
	}
}

func TestProbeCapabilitiesAreCheckedWithoutExecutionPreset(t *testing.T) {
	req := testRequest(t, "internal-k8s", "backend")
	dropWorkloadDefinitions(&req)
	def := renderDefinition("custom", "internal-k8s")
	def.Variables()[resource.VariableTemplateSource] = gitA
	req.Catalog.Definitions = append(req.Catalog.Definitions, def)
	source := gitRecord(gitA, strings.Repeat("a", 40))
	req.Catalog.TemplateSources = map[string]templatesource.Bundle{gitA: source}
	c := req.After.Containers["main"]
	c.ReadinessProbe = &environment.Probe{Type: "tcp", Port: 80}
	req.After.Containers["main"] = c
	svc := NewRequiredRendererService(testBundles())
	if _, err := svc.Plan(req); !errors.Is(err, environment.ErrExecutionUnavailable) {
		t.Fatalf("missing probe capability accepted: %v", err)
	}
	source.Contract.Manifest.WorkloadCapabilities = []string{templatesource.ContainerProbesV1}
	req.Catalog.TemplateSources[gitA] = source
	if _, err := svc.Plan(req); err != nil {
		t.Fatal(err)
	}
}
