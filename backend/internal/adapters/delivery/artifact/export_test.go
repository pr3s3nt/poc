package artifact

import (
	"encoding/json"
	"testing"
)

func TestExportPreservesBusinessAndPodTemplateWithoutMutatingSource(t *testing.T) {
	source := map[string]any{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": map[string]any{"name": "app", "namespace": "staging", "labels": map[string]any{"app.kubernetes.io/managed-by": "Helm", "orchestrator.io/managed-by": "orchestrator", "business": "keep"}, "annotations": map[string]any{"meta.helm.sh/release-name": "old", "meta.helm.sh/release-namespace": "releases", "helm.sh/resource-policy": "keep", "orchestrator.io/delivery-revision": "old", "business": "keep"}}, "spec": map[string]any{"template": map[string]any{"metadata": map[string]any{"labels": map[string]any{"orchestrator.io/deployment-id": "original"}}, "spec": map[string]any{"containers": []any{map[string]any{"image": "app:v1"}}}}}}
	before, _ := json.Marshal(source)
	exported, err := Export([]map[string]any{source})
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(source)
	if string(before) != string(after) {
		t.Fatal("source artifact mutated")
	}
	specBefore, _ := json.Marshal(source["spec"])
	specAfter, _ := json.Marshal(exported[0].Object["spec"])
	if string(specBefore) != string(specAfter) {
		t.Fatal("transfer altered pod template or business spec")
	}
	md := exported[0].Object["metadata"].(map[string]any)
	a := md["annotations"].(map[string]any)
	labels := md["labels"].(map[string]any)
	if len(a) != 1 || a["business"] != "keep" || labels["business"] != "keep" || labels["app.kubernetes.io/managed-by"] != "orchestrator" {
		t.Fatal("ownership export corrupted metadata")
	}
}
