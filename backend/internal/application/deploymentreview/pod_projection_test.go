package deploymentreview

import (
	"orchestrator/internal/ports/execution"
	"strings"
	"testing"
)

func TestPreviewProjectsInitSidecarAndVolumeSourcesWithoutSecretBytes(t *testing.T) {
	secret := "preview-secret-byte-sentinel"
	pod := map[string]any{"containers": []any{map[string]any{"name": "app", "image": "app:1", "volumeMounts": []any{map[string]any{"name": "shared", "mountPath": "/data"}}}}, "initContainers": []any{map[string]any{"name": "sidecar", "image": "agent:1", "restartPolicy": "Always", "env": []any{map[string]any{"name": "SECRET", "value": secret}}, "volumeMounts": []any{map[string]any{"name": "key", "mountPath": "/keys/key", "subPath": "value", "readOnly": true}}, "livenessProbe": map[string]any{"exec": map[string]any{"command": []any{secret}}}}}, "volumes": []any{map[string]any{"name": "shared", "emptyDir": map[string]any{"medium": "Memory", "sizeLimit": "64Mi"}}, map[string]any{"name": "key", "secret": map[string]any{"secretName": "approved-revision", "items": []any{map[string]any{"key": "approved-key", "path": "value"}}}}}}
	pod["volumes"] = append(pod["volumes"].([]any), map[string]any{"name": "node", "hostPath": map[string]any{"path": "/var/local/app", "type": "Directory"}})
	for _, kind := range []string{"Deployment", "StatefulSet", "CronJob"} {
		t.Run(kind, func(t *testing.T) {
			spec := map[string]any{"template": map[string]any{"spec": pod}}
			if kind == "CronJob" {
				spec = map[string]any{"schedule": "0 0 * * *", "timeZone": "UTC", "jobTemplate": map[string]any{"spec": spec}}
			}
			m := execution.Manifest{APIVersion: "apps/v1", Kind: kind, Name: "app", Namespace: "ns", Object: map[string]any{"spec": spec}}
			text := ProjectManifests([]execution.Manifest{m})[kind+"/app"].Text
			for _, want := range []string{"initContainers", "restartPolicy: Always", "emptyDir", "volumeMounts", "approved-revision", "hostPath", "/var/local/app", "type: Directory"} {
				if !strings.Contains(text, want) {
					t.Fatalf("preview omitted %s", want)
				}
			}
			if strings.Contains(text, secret) {
				t.Fatal("secret bytes leaked in init/probe projection")
			}
			if kind == "CronJob" && !strings.Contains(text, "jobTemplate") {
				t.Fatal("scheduled Pod lost")
			}
		})
	}
}
