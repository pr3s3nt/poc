package kubernetes

import (
	"encoding/json"
	"testing"
)

func TestSelfHealOwnershipReclaimPreservesDelegationAndOtherControllers(t *testing.T) {
	manual := map[string]any{"manager": "kubectl-patch", "operation": "Update", "fieldsV1": map[string]any{"f:spec": map[string]any{"f:replicas": map[string]any{}, "f:injected": map[string]any{}, "f:template": map[string]any{"f:spec": map[string]any{"f:containers": map[string]any{`k:{"name":"app"}`: map[string]any{"f:image": map[string]any{}, "f:injected": map[string]any{}}, `k:{"name":"sidecar"}`: map[string]any{"f:image": map[string]any{}}}}}}, "f:status": map[string]any{"f:replicas": map[string]any{}}}}
	apply := map[string]any{"manager": "another-declarative-controller", "operation": "Apply", "fieldsV1": map[string]any{"f:spec": map[string]any{"f:replicas": map[string]any{}}}}
	hpa := map[string]any{"manager": "kube-controller-manager", "operation": "Update", "fieldsV1": map[string]any{"f:spec": map[string]any{"f:replicas": map[string]any{}}}}
	desired := map[string]any{"spec": map[string]any{"replicas": float64(1), "template": map[string]any{"spec": map[string]any{"containers": []any{map[string]any{"name": "app", "image": "admitted:v1"}}}}}}
	got, changed, err := manualDesiredFields([]any{manual, apply, hpa}, desired)
	if err != nil || !changed || len(got) != 3 {
		t.Fatal("reclaim not scoped", err)
	}
	fields := got[0].(map[string]any)["fieldsV1"].(map[string]any)
	spec := fields["f:spec"].(map[string]any)
	if _, exists := spec["f:replicas"]; exists {
		t.Fatal("manual declared replicas ownership retained")
	}
	if spec["f:injected"] == nil || fields["f:status"] == nil {
		t.Fatal("undeclared or status ownership changed")
	}
	containers := spec["f:template"].(map[string]any)["f:spec"].(map[string]any)["f:containers"].(map[string]any)
	app := containers[`k:{"name":"app"}`].(map[string]any)
	if _, exists := app["f:image"]; exists {
		t.Fatal("declared container image retained")
	}
	if app["f:injected"] == nil || containers[`k:{"name":"sidecar"}`] == nil {
		t.Fatal("injected container fields changed")
	}
	for index, want := range []any{apply, hpa} {
		before, _ := json.Marshal(want)
		after, _ := json.Marshal(got[index+1])
		if string(before) != string(after) {
			t.Fatal("another controller ownership changed")
		}
	}
	delete(desired["spec"].(map[string]any), "replicas")
	got, _, err = manualDesiredFields([]any{manual}, desired)
	if err != nil || got[0].(map[string]any)["fieldsV1"].(map[string]any)["f:spec"].(map[string]any)["f:replicas"] == nil {
		t.Fatal("delegated replicas reclaimed")
	}
}
