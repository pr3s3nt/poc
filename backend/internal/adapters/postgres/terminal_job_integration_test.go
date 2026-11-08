package postgres

import (
	"context"
	"orchestrator/internal/adapters/statecipher"
	appdelivery "orchestrator/internal/application/delivery"
	domain "orchestrator/internal/domain/delivery"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/platform/ids"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/persistence/persistencetest"
	"strings"
	"testing"
	"time"
)

func TestTerminalJobFailureInventorySurvivesPostgresReopen(t *testing.T) {
	ctx := context.Background()
	db := freshDatabase(t)
	st, err := Open(ctx, db.Console)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	state, err := OpenState(ctx, db.State)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	binding := persistencetest.DeliveryBindings(t, st)
	persistencetest.DeliverySelections(t, st, binding)
	env, err := st.GetEnvironment(ctx, "delivery-app", "staging")
	if err != nil {
		t.Fatal(err)
	}
	op, err := st.ClaimEnvironment(ctx, persistence.OperationClaim{ID: ids.New(), ApplicationKey: "delivery-app", EnvironmentKey: "staging", Owner: "worker", Kind: environment.OpDeploy, Deadline: time.Now().Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	ctx = persistence.WithOwner(ctx, persistence.Owner{OperationID: op.ID, Owner: op.Owner, Fence: op.Fence})
	cipher, err := statecipher.New("test", make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	artifacts := &appdelivery.Artifacts{Store: state, Cipher: cipher}
	unit := ids.New()
	ref, err := artifacts.Put(ctx, binding.OrganizationID, unit, []byte("owned job artifact"))
	if err != nil {
		t.Fatal(err)
	}
	scope := domain.Scope{OrganizationID: binding.OrganizationID, EnvironmentID: env.ID, TargetGeneration: env.TargetGeneration, Epoch: env.DeliveryEpoch, UnitKey: "job/JOB"}
	revision := ids.New()
	intent := ids.New()
	record := persistence.DeliveryExecution{UnitID: unit, OrganizationID: binding.OrganizationID, EnvironmentID: env.ID, WorkloadID: "job", Version: 1, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(), Target: execution.Target{Organization: "delivery-org", Connection: "delivery-manager", Namespace: "app"}, Revision: domain.Revision{ID: revision, Scope: scope, Phase: domain.Job, PayloadDigest: strings.Repeat("a", 64), ArtifactRef: ref, Selection: domain.Selection{Mode: domain.CD, BindingID: binding.ID, BindingVersion: binding.Version}}, Mutation: domain.Mutation{ID: intent, Scope: scope, RevisionID: revision, OperationID: op.ID, RequestID: intent, RequestDigest: strings.Repeat("b", 64), Action: domain.Submit, Status: domain.IntentPending, Owner: op.Owner, Fence: op.Fence, Version: 1}}
	record, err = st.PrepareDelivery(ctx, record)
	if err != nil {
		t.Fatal(err)
	}
	record.Mutation, err = record.Mutation.Advance(domain.IntentRunning, false)
	if err != nil {
		t.Fatal(err)
	}
	record, err = st.UpdateDeliveryExecution(ctx, record, 1)
	if err != nil {
		t.Fatal(err)
	}
	record.Receipt = port.Receipt{IntentID: intent, RevisionID: revision, PayloadDigest: record.Revision.PayloadDigest, Handle: map[string]string{port.TerminalJobFailureKey: "v1"}}
	record.Inventory = []port.AppliedObject{{ObjectRef: execution.ObjectRef{APIVersion: "batch/v1", Kind: "Job", Name: "job"}, UID: "original-job-uid"}}
	record.Mutation, err = record.Mutation.Advance(domain.IntentFailed, true)
	if err != nil {
		t.Fatal(err)
	}
	record, err = st.UpdateDeliveryExecution(ctx, record, 2)
	if err != nil {
		t.Fatal(err)
	}
	st.Close()
	reopened, err := Open(context.Background(), db.Console)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	loaded, err := reopened.GetDeliveryExecution(context.Background(), binding.OrganizationID, intent)
	if err != nil || !loaded.TerminalJobFailure() || loaded.Inventory[0].UID != "original-job-uid" {
		t.Fatal("lost terminal failure proof", err)
	}
	var successful *string
	if err = reopened.q(context.Background()).QueryRow(context.Background(), "SELECT successful_revision_id::text FROM delivery_units WHERE id=$1", unit).Scan(&successful); err != nil || successful != nil {
		t.Fatal("failed Job became successful revision", err)
	}
	retained, err := appdelivery.HasRetainedWorkload([]persistence.DeliveryExecution{loaded}, "job")
	if err != nil || !retained {
		t.Fatal("failed inventory cannot be removed", err)
	}
}
