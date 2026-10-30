package tfexec_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"orchestrator/internal/domain/environment"
)

func TestAcknowledgedBaselineScanPreservesCanonicalStateAndPrivateInputs(t *testing.T) {
	h := newHarness(t)
	ctx, op := h.claim("baseline-create")
	created := mustExecute(t, h, ctx, h.request("baseline-create", h.mod, inputs("acknowledged-db", "stable")))
	baseline, err := h.state.LatestExecutionBaseline(ctx, h.orgID, created.InstanceID)
	if err != nil {
		t.Fatal(err)
	}
	if baseline.InstanceVersion <= 0 || baseline.StateVersion <= 0 || baseline.Lineage == "" {
		t.Fatal("execution baseline omitted acknowledged pins")
	}
	artifact, err := h.state.ReadBaselineArtifact(ctx, h.orgID, baseline.ArtifactID)
	if err != nil {
		t.Fatal(err)
	}
	password := created.Outputs["password"].(string)
	if bytes.Contains(artifact.Ciphertext, []byte(password)) {
		t.Fatal("plaintext input secret persisted")
	}
	var before, after []byte
	if err := h.conn().QueryRow(context.Background(), `SELECT row_to_json(s)::text FROM terraform_states s WHERE resource_instance_id=$1`, created.InstanceID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	h.release(op, environment.OpSucceeded)
	ctx, scan := h.claim("read-only-drift-scan")
	applies := h.runner.applies
	report, err := h.svc.InspectInstance(ctx, h.orgID, created.InstanceID)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Complete || len(report.External) != 0 || len(report.Proposed) != 0 || len(report.OutputNames) != 0 {
		t.Fatal("unchanged baseline did not yield a complete clean report")
	}
	if h.runner.applies != applies {
		t.Fatal("read-only scan applied")
	}
	if err := h.conn().QueryRow(context.Background(), `SELECT row_to_json(s)::text FROM terraform_states s WHERE resource_instance_id=$1`, created.InstanceID).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) || h.count(`SELECT count(*) FROM terraform_state_locks`) != 0 {
		t.Fatal("scan wrote canonical state or leaked its lock")
	}
	if h.count(`SELECT count(*) FROM terraform_execution_baselines WHERE resource_instance_id=$1`, created.InstanceID) != 1 {
		t.Fatal("scan replaced the execution baseline")
	}
	public, err := json.Marshal(report)
	if err != nil || strings.Contains(string(public), password) || strings.Contains(string(public), report.PrivateChangeDigest) {
		t.Fatal("public scan exposed private input or secret-dependent digest")
	}
	h.release(scan, environment.OpSucceeded)
}
