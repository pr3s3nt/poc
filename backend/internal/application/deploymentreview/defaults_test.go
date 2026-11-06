package deploymentreview

import (
	"orchestrator/internal/domain/environment"
	"strings"
	"testing"
)

func TestEffectiveDefaultsProjectionKeepsProvenanceAndRedactsProfileCommands(t *testing.T) {
	sentinel := "private-command-sentinel"
	spec := environment.ModuleSpec{Containers: map[string]environment.Container{"main": {Image: "image", ReadinessProbe: &environment.Probe{Type: "exec", Command: []string{sentinel}}}}, Execution: &environment.WorkloadExecution{Version: 1, Kind: "Deployment", Template: environment.TemplatePin{ID: "example", Version: "1.0.0", SHA256: strings.Repeat("a", 64)}, TemplateDefaults: &environment.TemplateDefaultsSnapshot{Environment: "staging", Profile: &environment.RuntimeDefaults{Resources: &environment.ContainerResourceRequirements{Requests: &environment.ComputeResources{CPU: "357m"}}, ReadinessProbe: &environment.Probe{Type: "exec", Command: []string{sentinel}}}}}}
	side := defaultsProjection(spec, spec.Execution.TemplateDefaults.Profile)
	if strings.Contains(side.Text, sentinel) || !strings.Contains(side.Text, "357m") || !strings.Contains(side.Text, "template") || !strings.Contains(side.Text, "<redacted>") {
		t.Fatal("unsafe or incomplete effective projection", side.Text)
	}
}
