package fleet

import (
	domain "orchestrator/internal/domain/delivery"
	"testing"
)

func TestModifiedDeploymentHealthTracksRuntimeIndependently(t *testing.T) {
	o := map[string]any{"metadata": map[string]any{"generation": float64(2)}, "spec": map[string]any{"replicas": float64(2)}, "status": map[string]any{"observedGeneration": float64(2), "readyReplicas": float64(2), "updatedReplicas": float64(2), "availableReplicas": float64(2)}}
	if h := objectHealth("Deployment", o); h != domain.Healthy {
		t.Fatal(h)
	}
	status := o["status"].(map[string]any)
	status["observedGeneration"] = float64(1)
	if h := objectHealth("Deployment", o); h != domain.Progressing {
		t.Fatal("old health generation admitted", h)
	}
	status["observedGeneration"] = float64(2)
	status["availableReplicas"] = float64(1)
	if h := objectHealth("Deployment", o); h != domain.Progressing {
		t.Fatal("incomplete rollout admitted", h)
	}
	status["conditions"] = []any{map[string]any{"type": "Progressing", "status": "False"}}
	if h := objectHealth("Deployment", o); h != domain.Degraded {
		t.Fatal("terminal rollout failure hidden", h)
	}
}
func TestRetriedJobPodFailureIsNotTerminalJobFailure(t *testing.T) {
	o := map[string]any{"status": map[string]any{"failed": float64(1), "succeeded": float64(1)}}
	if h := objectHealth("Job", o); h != domain.Healthy {
		t.Fatal("successful retry was marked failed", h)
	}
	o["status"].(map[string]any)["conditions"] = []any{map[string]any{"type": "Failed", "status": "True"}}
	if h := objectHealth("Job", o); h != domain.Degraded {
		t.Fatal(h)
	}
}
