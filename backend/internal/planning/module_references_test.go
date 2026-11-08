package planning

import (
	"testing"

	"orchestrator/internal/domain/environment"
)

func TestConfigMapLiteralDoesNotCreateGraphOrSharedReferences(t *testing.T) {
	text := "${secret_dir}/file\n${resources.shared.missing.host}\n${context.service.missing.http}"
	module := environment.Module{Spec: environment.ModuleSpec{
		Containers: map[string]environment.Container{"main": {Image: "test"}},
		Execution: &environment.WorkloadExecution{Version: 1, Kind: "Deployment", Volumes: []environment.PodVolume{
			{Name: "config", ConfigMap: &environment.ConfigMapVolume{Files: []environment.ConfigFile{{Name: "configure", Value: text}}}},
		}},
	}}
	b := newGraphBuilder(Context{}, Catalog{})
	if err := b.buildFromSet(environment.Document{Modules: map[string]environment.Module{"web": module}}, nil); err != nil {
		t.Fatal(err)
	}
	if len(b.graph().Edges) != 0 {
		t.Fatal("literal file created graph edges")
	}
	refs, err := sharedReferences(module)
	if err != nil || len(refs) != 0 {
		t.Fatalf("literal file retained a shared resource: %v %v", refs, err)
	}
	if module.Spec.Execution.Volumes[0].ConfigMap.Files[0].Value != text {
		t.Fatal("reference discovery changed the authored file")
	}
	module.Spec.Containers["main"] = environment.Container{Image: "test", Variables: map[string]string{"ACTUAL_REFERENCE": "${resources.missing.host}"}}
	if err := newGraphBuilder(Context{}, Catalog{}).buildFromSet(environment.Document{Modules: map[string]environment.Module{"web": module}}, nil); err == nil {
		t.Fatal("real container reference bypassed validation")
	}
}
