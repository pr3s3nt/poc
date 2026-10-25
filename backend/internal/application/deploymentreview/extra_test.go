package deploymentreview

import (
	"strings"
	"testing"

	"orchestrator/internal/ports/execution"
)

// ADR-024: bundle objects of other kinds are reviewed with metadata and a
// sanitized spec; environment values and data payloads never appear.
func TestProjectManifestsShowsExtraObjectsRedacted(t *testing.T) {
	manifests := []execution.Manifest{
		{APIVersion: "autoscaling/v2", Kind: "HorizontalPodAutoscaler", Name: "web", Namespace: "ns", Object: map[string]any{
			"metadata": map[string]any{"name": "web", "labels": map[string]any{"orchestrator.io/workload": "web"}},
			"spec":     map[string]any{"minReplicas": 2.0, "maxReplicas": 7.0},
		}},
		{APIVersion: "v1", Kind: "ConfigMap", Name: "web-files", Namespace: "ns", Object: map[string]any{
			"metadata": map[string]any{"name": "web-files"}, "data": map[string]any{"app.yaml": "password: hunter2-sentinel"},
		}},
		{APIVersion: "batch/v1", Kind: "Job", Name: "web-migrate", Namespace: "ns", Object: map[string]any{
			"metadata": map[string]any{"name": "web-migrate"},
			"spec": map[string]any{"template": map[string]any{"spec": map[string]any{"containers": []any{map[string]any{
				"name": "migrate", "image": "app:1", "env": []any{map[string]any{"name": "DB", "value": "plain-sentinel"}},
			}}}}},
		}},
		{APIVersion: "v1", Kind: "ServiceAccount", Name: "web-vault", Namespace: "ns", Auxiliary: true, Object: map[string]any{}},
	}
	out := ProjectManifests(manifests)
	if _, ok := out["ServiceAccount/web-vault"]; ok {
		t.Fatal("platform auxiliaries are not reviewed")
	}
	hpa := out["HorizontalPodAutoscaler/web"]
	if !strings.Contains(hpa.Text, "maxReplicas: 7") || !strings.Contains(hpa.Text, "orchestrator.io/workload: web") {
		t.Fatalf("HPA projection: %s", hpa.Text)
	}
	for key, side := range out {
		if strings.Contains(side.Text, "sentinel") {
			t.Fatalf("%s leaks a value: %s", key, side.Text)
		}
	}
	if !strings.Contains(out["ConfigMap/web-files"].Text, "app.yaml: <redacted>") {
		t.Fatalf("ConfigMap keys stay visible: %s", out["ConfigMap/web-files"].Text)
	}
}
