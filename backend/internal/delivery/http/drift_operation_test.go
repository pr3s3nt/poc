package http

import (
	"encoding/json"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/platform/ids"
	"strings"
	"testing"
)

func TestRepairOperationProjectionExposesOnlySafeContinuationHandle(t *testing.T) {
	id := ids.New()
	op := environment.Operation{Kind: environment.OpDriftRepair, Detail: map[string]any{"repairReviewId": id, "repairPreparation": true, "privatePlan": "secret-plan", "checkpoint": "private-checkpoint"}}
	view := operationViewOf(op)
	if view.RepairReviewID != id || !view.RepairPreparation {
		t.Fatal("safe continuation metadata missing")
	}
	raw, _ := json.Marshal(view)
	for _, private := range []string{"secret-plan", "privatePlan", "checkpoint", "private-checkpoint"} {
		if strings.Contains(string(raw), private) {
			t.Fatal("operation detail leaked")
		}
	}
	op.Kind = environment.OpDeploy
	view = operationViewOf(op)
	if view.RepairReviewID != "" || view.RepairPreparation {
		t.Fatal("repair metadata exposed for another operation kind")
	}
	op.Kind = environment.OpDriftRepair
	op.Detail["repairReviewId"] = "private-non-UUID"
	if operationViewOf(op).RepairReviewID != "" {
		t.Fatal("unvalidated continuation handle exposed")
	}
}
