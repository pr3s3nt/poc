package kubernetes

import (
	"context"
	"encoding/json"
	"orchestrator/internal/ports/execution"
	"os"
	"path/filepath"
	"testing"
)

func TestRuntimeValidationReplacesCompleteObjectWithoutApplyListMerging(t *testing.T) {
	dir := t.TempDir()
	kubeconfig := filepath.Join(dir, "kubeconfig")
	if err := os.WriteFile(kubeconfig, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(dir, "calls")
	payload := filepath.Join(dir, "proposed")
	script := `#!/bin/sh
printf '%s\n' "$*" >> '` + log + `'
case "$*" in
 *'/apis/apps/v1') printf '%s' '{"resources":[{"name":"deployments","kind":"Deployment","namespaced":true}]}' ;;
 *'/apis/apps/v1/namespaces/test/deployments/web') printf '%s' '{"metadata":{"name":"web","resourceVersion":"42"}}' ;;
 *'replace --dry-run=server --validate=strict -f -') cat > '` + payload + `' ;;
 *) exit 7 ;;
esac
`
	binary := filepath.Join(dir, "kubectl")
	if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	proposed := map[string]any{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": map[string]any{"name": "web", "namespace": "test"}, "spec": map[string]any{"revisionHistoryLimit": 0, "template": map[string]any{"spec": map[string]any{"containers": []any{map[string]any{"name": "main", "ports": []any{map[string]any{"name": "http", "containerPort": 8080}}}}}}}}
	err := ValidateRuntimeObjects(context.Background(), NewCLI(binary, "", kubeconfig), execution.Target{Namespace: "test"}, []execution.Manifest{{APIVersion: "apps/v1", Kind: "Deployment", Name: "web", Namespace: "test", Object: proposed}})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(payload)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	meta := got["metadata"].(map[string]any)
	if meta["resourceVersion"] != "42" {
		t.Fatal("missing update precondition")
	}
	if _, mutated := proposed["metadata"].(map[string]any)["resourceVersion"]; mutated {
		t.Fatal("dry-run mutated deliverable object")
	}
	containers := got["spec"].(map[string]any)["template"].(map[string]any)["spec"].(map[string]any)["containers"].([]any)
	if len(containers[0].(map[string]any)["ports"].([]any)) != 1 {
		t.Fatal("dry-run merged ports")
	}
}

func TestRuntimeClaimControllerRequiresExactDeliveryOwner(t *testing.T) {
	target := execution.Target{Extra: map[string]string{"application": "app", "environment": "staging"}}
	manager := DeliveryManager{Release: "release", Namespace: "fleet-releases"}
	makeController := func() map[string]any {
		return map[string]any{"metadata": map[string]any{"labels": map[string]any{"app.kubernetes.io/managed-by": "Helm", "app.kubernetes.io/name": "web", "orchestrator.io/managed-by": "orchestrator", "orchestrator.io/application": "app", "orchestrator.io/environment": "staging"}, "annotations": map[string]any{"meta.helm.sh/release-name": "release", "meta.helm.sh/release-namespace": "fleet-releases"}}}
	}
	if !runtimeControllerOwner(makeController(), target, "web", manager) {
		t.Fatal("Admitted Fleet controller rejected")
	}
	for _, test := range []struct {
		name   string
		change func(map[string]any)
	}{
		{"foreign-release", func(c map[string]any) {
			c["metadata"].(map[string]any)["annotations"].(map[string]any)["meta.helm.sh/release-name"] = "foreign"
		}},
		{"foreign-environment", func(c map[string]any) {
			c["metadata"].(map[string]any)["labels"].(map[string]any)["orchestrator.io/environment"] = "prod"
		}},
		{"missing-platform-owner", func(c map[string]any) {
			delete(c["metadata"].(map[string]any)["labels"].(map[string]any), "orchestrator.io/managed-by")
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := makeController()
			test.change(c)
			if runtimeControllerOwner(c, target, "web", manager) {
				t.Fatal("Foreign controller admitted")
			}
		})
	}
	if runtimeControllerOwner(makeController(), target, "web", DeliveryManager{}) {
		t.Fatal("Direct admission accepted Helm ownership")
	}
}
