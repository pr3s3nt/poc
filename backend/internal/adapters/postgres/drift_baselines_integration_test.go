package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/terraformstate"
)

func TestExecutionBaselineIsScopedImmutableAndPrivate(t *testing.T) {
	f := newStateFixture(t)
	ctx := persistence.WithOwner(context.Background(), persistence.Owner{OperationID: f.op.ID, Owner: f.op.Owner, Fence: f.op.Fence})
	attempt, err := f.state.BeginAttempt(ctx, tfresource.Attempt{InstanceID: f.inst.ID, RunID: "baseline", Action: tfresource.ActionUpdate, Ownership: tfresource.Preexisting, Stage: "INTENT", ModuleSnapshot: map[string]any{}, TargetSnapshot: map[string]any{}, Deadline: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.state.UpdateAttempt(ctx, attempt.ID, tfresource.AttemptProgress{Stage: "READY", Status: tfresource.AttemptSucceeded}); err != nil {
		t.Fatal(err)
	}
	instance, err := f.state.SetInstanceStatus(ctx, f.inst.ID, f.inst.Version, tfresource.InstanceReady)
	if err != nil {
		t.Fatal(err)
	}
	artifact := persistence.CipherArtifact{ID: ids.New(), OrganizationID: instance.Identity.OrganizationID, OwnerID: instance.ID, Purpose: "TERRAFORM_BASELINE", KeyID: "state-key", Nonce: make([]byte, 12), Ciphertext: []byte("private-encrypted-inputs"), AADVersion: 1, CreatedAt: time.Now().UTC()}
	input := terraformstate.ExecutionBaseline{ID: ids.New(), OrganizationID: artifact.OrganizationID, EnvironmentID: instance.Identity.EnvironmentID, InstanceID: instance.ID, AttemptID: attempt.ID, ModuleBundleID: instance.ModuleBundleID, InstanceVersion: instance.Version, ArtifactID: artifact.ID, CreatedAt: artifact.CreatedAt}
	baseline, err := f.state.AppendExecutionBaseline(ctx, input, artifact)
	if err != nil {
		t.Fatal(err)
	}
	if baseline.Lineage != "lin" || baseline.Serial != 1 || baseline.StateVersion != 1 {
		t.Fatal("acknowledged state pins omitted")
	}
	if _, err = f.state.AppendExecutionBaseline(ctx, input, artifact); err == nil {
		t.Fatal("immutable acknowledgement overwritten")
	}
	loaded, err := f.state.LatestExecutionBaseline(context.Background(), input.OrganizationID, input.InstanceID)
	if err != nil || loaded.ID != input.ID {
		t.Fatal("baseline not durable", err)
	}
	if _, err = f.state.LatestExecutionBaseline(context.Background(), ids.New(), input.InstanceID); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatal("foreign baseline exposed", err)
	}
	if _, err = f.state.ReadBaselineArtifact(context.Background(), ids.New(), artifact.ID); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatal("foreign private inputs exposed", err)
	}
	if _, err = f.state.GetDeliveryArtifact(context.Background(), input.OrganizationID, artifact.ID); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatal("baseline passed delivery-purpose boundary", err)
	}
	if _, err := f.console.q(context.Background()).Exec(context.Background(), `SELECT * FROM terraform_execution_baselines`); err == nil {
		t.Fatal("Console accessed private execution pins")
	}
	if _, err := f.state.q(context.Background()).Exec(context.Background(), `UPDATE terraform_execution_baselines SET serial=2 WHERE id=$1`, baseline.ID); err == nil {
		t.Fatal("State runtime rewrote immutable baseline")
	}
	if _, err = f.state.AppendExecutionBaseline(context.Background(), input, artifact); !errors.Is(err, terraformstate.ErrOwnerRequired) {
		t.Fatal("baseline append ignored claim", err)
	}
}
