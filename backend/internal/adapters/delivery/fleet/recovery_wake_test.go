package fleet

import (
	"encoding/json"
	port "orchestrator/internal/ports/delivery"
	"testing"
)

func TestRecoveryWakeCannotChangeDesiredDeploymentOrForceReplay(t *testing.T) {
	i := port.Intent{Owner: port.Owner{OperationID: "operation", Instance: "recovery-owner"}}
	bd := map[string]any{"metadata": map[string]any{"uid": "original-uid", "resourceVersion": "42", "annotations": map[string]any{"business": "retained"}}, "spec": map[string]any{"deploymentID": "exact-pending-manifest:options", "forceSyncGeneration": float64(7)}, "status": map[string]any{"appliedDeploymentID": "prior-manifest:options"}}
	patch, pending, err := pendingRecoveryPatch(bd, i)
	if err != nil || !pending {
		t.Fatal("pending receipt not admitted", err)
	}
	var operations []struct {
		Op, Path string
		Value    any
	}
	if json.Unmarshal(patch, &operations) != nil || len(operations) != 4 {
		t.Fatal("invalid guarded patch")
	}
	for _, op := range operations {
		if op.Op == "test" {
			continue
		}
		if op.Op != "add" || op.Path != "/metadata/annotations" {
			t.Fatal("recovery writes deployment state", op.Path)
		}
		annotations := op.Value.(map[string]any)
		if annotations["business"] != "retained" || annotations["orchestrator.io/recovery-observation"] != "operation/recovery-owner" {
			t.Fatal("annotation scope lost")
		}
	}
	bd["status"].(map[string]any)["appliedDeploymentID"] = bd["spec"].(map[string]any)["deploymentID"]
	if patch, pending, err := pendingRecoveryPatch(bd, i); err != nil || pending || patch != nil {
		t.Fatal("already applied deployment was woken")
	}
	bd["status"] = map[string]any{}
	delete(bd["metadata"].(map[string]any), "uid")
	if _, _, err := pendingRecoveryPatch(bd, i); err == nil {
		t.Fatal("missing incarnation proof admitted")
	}
}
