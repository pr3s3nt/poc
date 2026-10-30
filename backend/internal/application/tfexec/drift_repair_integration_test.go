package tfexec_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"orchestrator/internal/application/tfexec"
	"orchestrator/internal/domain/drift"
	"orchestrator/internal/domain/environment"
)

// Actual PostgreSQL/encryption/Terraform planning, with a clearly FAKE drift
// observation. This proves admission fences, not provider repair conformance.
func TestRepairRejectsChangedPrivateValuesBeforeApplyDespiteSamePublicSummary(t *testing.T) {
	h := newHarness(t)
	ctx, op := h.claim("create-baseline")
	created := mustExecute(t, h, ctx, h.request("create-baseline", h.mod, inputs("database", "stable")))
	h.release(op, environment.OpSucceeded)
	digest := strings.Repeat("a", 64)
	h.runner.onDrift = func(drift.Report) (drift.Report, error) {
		return drift.Report{Complete: true, Applyable: true, Proposed: []drift.Finding{{Address: "terraform_data.db", Type: "terraform_data", Actions: []string{"update"}, Paths: []string{"/input"}}}, PrivateChangeDigest: digest}, nil
	}
	ctx, preview := h.claim("repair-preview")
	prepared, err := h.svc.PrepareInstanceRepair(ctx, h.orgID, created.InstanceID)
	if err != nil || len(prepared.Blockers) > 0 || prepared.ArtifactID == "" {
		t.Fatal("repair fixture preparation failed", err)
	}
	publicBefore, _ := json.Marshal(prepared.Report)
	if strings.Contains(string(publicBefore), digest) || strings.Contains(string(publicBefore), prepared.ArtifactID) {
		t.Fatal("private repair proof escaped review")
	}
	h.release(preview, environment.OpSucceeded)
	digest = strings.Repeat("b", 64)
	publicAfter, _ := json.Marshal(drift.Report{Complete: true, Applyable: true, Proposed: prepared.Report.Proposed, PrivateChangeDigest: digest})
	if string(publicBefore) != string(publicAfter) {
		t.Fatal("fixture no longer proves equal public summaries")
	}
	ctx, confirm := h.claim("repair-confirm")
	before := h.runner.applies
	if err := h.svc.RepairInstance(ctx, h.orgID, created.InstanceID, prepared.ArtifactID); !errors.Is(err, tfexec.ErrRepairStale) {
		t.Fatal("changed private repair values admitted", err)
	}
	if h.runner.applies != before || h.count(`SELECT count(*) FROM terraform_state_locks`) != 0 {
		t.Fatal("rejected repair applied or leaked its lock")
	}
	h.release(confirm, environment.OpFailed)
}

func TestRepairPreviewBlocksUnchangedActualPlanWithoutInventingAnApply(t *testing.T) {
	h := newHarness(t)
	ctx, op := h.claim("baseline")
	created := mustExecute(t, h, ctx, h.request("baseline", h.mod, inputs("database", "stable")))
	h.release(op, environment.OpSucceeded)
	ctx, review := h.claim("preview-no-drift")
	before := h.runner.applies
	prepared, err := h.svc.PrepareInstanceRepair(ctx, h.orgID, created.InstanceID)
	if err != nil || len(prepared.Blockers) == 0 || prepared.ArtifactID != "" || h.runner.applies != before {
		t.Fatal("unchanged plan admitted as repair", err)
	}
	h.release(review, environment.OpSucceeded)
}
