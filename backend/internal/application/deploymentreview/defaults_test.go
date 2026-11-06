package deploymentreview

import (
	"orchestrator/internal/domain/environment"
	"strings"
	"testing"
)

func TestEffectiveDefaultsProjectionKeepsProvenanceAndRedactsProfileCommands(t *testing.T) {
	sentinel := "private-command-sentinel"
	spec := environment.ModuleSpec{Containers: map[string]environment.Container{"main": {Image: "image", ReadinessProbe: &environment.Probe{Type: "exec", Command: []string{sentinel}}}}, Execution: &environment.WorkloadExecution{Version: 1, Kind: "Deployment", Defaults: &environment.DefaultsPolicy{Version: "workload-defaults-v1", Environment: "staging", Template: &environment.RuntimeDefaults{ReadinessProbe: &environment.Probe{Type: "exec", Command: []string{sentinel}}}}}}
	side := defaultsProjection(spec, nil)
	if strings.Contains(side.Text, sentinel) || !strings.Contains(side.Text, "100m") || !strings.Contains(side.Text, "environment") || !strings.Contains(side.Text, "<redacted>") {
		t.Fatal("unsafe or incomplete effective projection", side.Text)
	}
}
