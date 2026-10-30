package kubernetes

import (
	"encoding/json"
	"errors"
	"orchestrator/internal/ports/execution"
	"strings"
	"testing"
)

func TestOwnershipTransferPatchTouchesMetadataOnlyAndChecksIncarnation(t *testing.T) {
	target := execution.Target{Namespace: "app", Extra: map[string]string{"application": "app", "environment": "staging"}}
	object := map[string]any{"metadata": map[string]any{"uid": "admitted-uid", "resourceVersion": "10", "labels": map[string]any{"app.kubernetes.io/managed-by": "orchestrator", "orchestrator.io/application": "app", "orchestrator.io/environment": "staging", "business": "keep"}, "annotations": map[string]any{"business": "keep"}}, "spec": map[string]any{"template": map[string]any{"metadata": map[string]any{"labels": map[string]any{"business": "keep"}}}}}
	before, _ := json.Marshal(object)
	fleet := DeliveryManager{Release: "owned-release", Namespace: "fleet-releases"}
	raw, err := ownershipPatch(object, target, "admitted-uid", DeliveryManager{}, fleet)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(object)
	if string(before) != string(after) {
		t.Fatal("mutated live source")
	}
	var ops []struct {
		Op, Path string
		Value    any
	}
	if json.Unmarshal(raw, &ops) != nil || len(ops) != 4 {
		t.Fatal("invalid patch")
	}
	if ops[0].Op != "test" || ops[0].Path != "/metadata/uid" || ops[1].Path != "/metadata/resourceVersion" {
		t.Fatal("incarnation guards absent")
	}
	for _, op := range ops {
		if op.Path != "/metadata/uid" && op.Path != "/metadata/resourceVersion" && op.Path != "/metadata/labels" && op.Path != "/metadata/annotations" {
			t.Fatal("spec mutation in transfer")
		}
	}
	md := object["metadata"].(map[string]any)
	md["labels"] = ops[2].Value
	md["annotations"] = ops[3].Value
	if raw, err := ownershipPatch(object, target, "admitted-uid", DeliveryManager{}, fleet); err != nil || len(raw) != 0 {
		t.Fatal("acknowledged adoption not idempotent", err)
	}
	raw, err = ownershipPatch(object, target, "admitted-uid", fleet, DeliveryManager{})
	if err != nil {
		t.Fatal(err)
	}
	if json.Unmarshal(raw, &ops) != nil {
		t.Fatal("invalid release patch")
	}
	labels := ops[2].Value.(map[string]any)
	a := ops[3].Value.(map[string]any)
	if labels["business"] != "keep" || a["business"] != "keep" || labels["app.kubernetes.io/managed-by"] != "orchestrator" || a["meta.helm.sh/release-name"] != nil {
		t.Fatal("business metadata lost or controller ownership retained")
	}
	if _, err := ownershipPatch(object, target, "replacement-uid", fleet, DeliveryManager{}); !errors.Is(err, ErrDeliveryOwnership) {
		t.Fatal("replacement adopted", err)
	}
	foreign := DeliveryManager{Release: "foreign", Namespace: "fleet-releases"}
	if _, err := ownershipPatch(object, target, "admitted-uid", foreign, DeliveryManager{}); !errors.Is(err, ErrDeliveryOwnership) {
		t.Fatal("foreign controller released", err)
	}
}

func TestMetadataFieldReleasePreservesBusinessOwnersAndAllValues(t *testing.T) {
	fields := []any{map[string]any{"manager": "kubectl-patch", "operation": "Update", "fieldsV1": map[string]any{"f:metadata": map[string]any{"f:annotations": map[string]any{"f:orchestrator.io/delivery-revision": map[string]any{}, "f:business-note": map[string]any{}}, "f:labels": map[string]any{"f:app.kubernetes.io/managed-by": map[string]any{}, "f:business": map[string]any{}}}, "f:spec": map[string]any{"f:replicas": map[string]any{}}}}, map[string]any{"manager": "fleetagent", "operation": "Apply", "fieldsV1": map[string]any{"f:spec": map[string]any{"f:template": map[string]any{}}}}}
	before, _ := json.Marshal(fields)
	released, changed, err := releaseMetadataFields(fields)
	if err != nil || !changed {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(released)
	text := string(raw)
	for _, key := range []string{"f:replicas", "f:template", "f:business-note", "f:business", "fleetagent"} {
		if !strings.Contains(text, key) {
			t.Fatal("business ownership erased", key)
		}
	}
	if strings.Contains(text, "f:orchestrator.io/delivery-revision") || strings.Contains(text, "f:app.kubernetes.io/managed-by") {
		t.Fatal("handoff metadata owner retained")
	}
	after, _ := json.Marshal(fields)
	if string(before) != string(after) {
		t.Fatal("input ownership record mutated")
	}
	if _, changed, err := releaseMetadataFields(released); err != nil || changed {
		t.Fatal("metadata ownership release was not idempotent", err)
	}
}
