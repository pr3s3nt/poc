package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"orchestrator/internal/adapters/statecipher"
	appdelivery "orchestrator/internal/application/delivery"
	"orchestrator/internal/domain/delivery"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/platform/ids"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/persistence/persistencetest"
)

func TestDeliveryExecutionArtifactsSurvivePostgresReopen(t *testing.T) {
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
	b := persistencetest.DeliveryBindings(t, st)
	persistencetest.DeliverySelections(t, st, b)
	env, err := st.GetEnvironment(ctx, "delivery-app", "staging")
	if err != nil {
		t.Fatal(err)
	}
	op, err := st.ClaimEnvironment(ctx, persistence.OperationClaim{ID: ids.New(), ApplicationKey: "delivery-app", EnvironmentKey: "staging", Owner: "worker", Kind: environment.OpDeploy, Deadline: time.Now().Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	ctx = persistence.WithOwner(ctx, persistence.Owner{OperationID: op.ID, Owner: op.Owner, Fence: op.Fence})
	cipher, err := statecipher.New("delivery-test", make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	artifacts := &appdelivery.Artifacts{Store: state, Cipher: cipher}
	unit := ids.New()
	ref, err := artifacts.Put(ctx, b.OrganizationID, unit, []byte("private payload"))
	if err != nil {
		t.Fatal(err)
	}
	scope := delivery.Scope{OrganizationID: b.OrganizationID, EnvironmentID: env.ID, UnitKey: "app/rollout"}
	revisionID := ids.New()
	record := persistence.DeliveryExecution{UnitID: unit, EnvironmentID: env.ID, OrganizationID: b.OrganizationID, WorkloadID: "app", Version: 1, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(), Target: execution.Target{Organization: "delivery-org", Connection: "delivery-manager", Namespace: "app"}, Revision: delivery.Revision{ID: revisionID, Scope: scope, Phase: delivery.Rollout, PayloadDigest: strings.Repeat("a", 64), ArtifactRef: ref, Selection: delivery.Selection{Mode: delivery.CD, BindingID: b.ID, BindingVersion: 1}}, Mutation: delivery.Mutation{ID: ids.New(), Scope: scope, RevisionID: revisionID, OperationID: op.ID, RequestID: "request-1", RequestDigest: strings.Repeat("b", 64), Action: delivery.Submit, Status: delivery.IntentPending, Owner: op.Owner, Fence: op.Fence, Version: 1}}
	created, err := st.PrepareDelivery(ctx, record)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := st.PrepareDelivery(ctx, record)
	if err != nil || duplicate.Mutation.ID != created.Mutation.ID {
		t.Fatal("duplicate intent", err)
	}
	created.Mutation, err = created.Mutation.Advance(delivery.IntentRunning, false)
	if err != nil {
		t.Fatal(err)
	}
	running, err := st.UpdateDeliveryExecution(ctx, created, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpdateDeliveryExecution(ctx, created, 1); !errors.Is(err, persistence.ErrVersionConflict) {
		t.Fatal("stale mutation accepted", err)
	}
	state.Close()
	st.Close()
	reopened, err := Open(context.Background(), db.Console)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	loaded, err := reopened.GetDeliveryExecution(context.Background(), b.OrganizationID, record.Mutation.ID)
	if err != nil || loaded.Mutation.Status != delivery.IntentRunning || loaded.Revision.ArtifactRef != ref {
		t.Fatal("lost running intent", err)
	}
	if _, err := reopened.GetDeliveryExecution(context.Background(), ids.New(), record.Mutation.ID); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatal("foreign intent exposed", err)
	}
	reopenedState, err := OpenState(context.Background(), db.State)
	if err != nil {
		t.Fatal(err)
	}
	defer reopenedState.Close()
	artifacts.Store = reopenedState
	payload, err := artifacts.Get(context.Background(), b.OrganizationID, unit, ref)
	if err != nil || string(payload) != "private payload" {
		t.Fatal("artifact lost", err)
	}
	if _, err := artifacts.Get(context.Background(), b.OrganizationID, ids.New(), ref); err == nil {
		t.Fatal("foreign unit decrypted")
	}
	running.Mutation, err = running.Mutation.Advance(delivery.IntentUnknown, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.UpdateDeliveryExecution(context.Background(), running, 2); !errors.Is(err, persistence.ErrOperationLost) {
		t.Fatal("unowned update", err)
	}
	// Each history entry carries its own intent timestamp, not the unit's first
	// creation time. Cleanup must distinguish a newer withdrawal from old Submit.
	loaded.Receipt = port.Receipt{IntentID: loaded.Mutation.ID, RevisionID: loaded.Revision.ID, PayloadDigest: loaded.Revision.PayloadDigest, Handle: map[string]string{"release": "owned-release"}}
	loaded.Inventory = []persistence.DeliveryObject{{ObjectRef: execution.ObjectRef{APIVersion: "apps/v1", Kind: "Deployment", Name: "app"}, UID: ids.New(), Release: "owned-release"}}
	loaded.Mutation, err = loaded.Mutation.Advance(delivery.IntentAcknowledged, true)
	if err != nil {
		t.Fatal(err)
	}
	ack, err := reopened.UpdateDeliveryExecution(ctx, loaded, 2)
	if err != nil {
		t.Fatal(err)
	}
	withdraw := ack
	withdraw.Revision.ID = ids.New()
	withdraw.Mutation = delivery.Mutation{ID: ids.New(), Scope: scope, RevisionID: withdraw.Revision.ID, OperationID: op.ID, RequestID: "withdraw", RequestDigest: strings.Repeat("c", 64), Action: delivery.Withdraw, Status: delivery.IntentPending, Owner: op.Owner, Fence: op.Fence, Version: 1}
	withdraw.Version = 1
	withdraw.CreatedAt = record.CreatedAt.Add(time.Second).Truncate(time.Microsecond)
	withdraw.UpdatedAt = withdraw.CreatedAt
	prepared, err := reopened.PrepareDelivery(ctx, withdraw)
	if err != nil {
		t.Fatal(err)
	}
	if !prepared.CreatedAt.Equal(withdraw.CreatedAt) || !prepared.CreatedAt.After(ack.CreatedAt) || prepared.Receipt.RevisionID != ack.Revision.ID || len(prepared.Inventory) != 1 || prepared.Inventory[0].UID != ack.Inventory[0].UID {
		t.Fatal("withdrawal lost ordering or source incarnation proof")
	}
	lease := persistence.GitPublicationLease{Destination: strings.Repeat("d", 64), OrganizationID: b.OrganizationID, Owner: persistence.Owner{OperationID: op.ID, Owner: op.Owner, Fence: op.Fence}}
	if err := reopened.ClaimGitPublication(ctx, lease); err != nil {
		t.Fatal(err)
	}
	owner, _ := persistence.OwnerFrom(ctx)
	if err := reopened.SuspendOperation(ctx, owner, "transfer fixture interrupted"); err != nil {
		t.Fatal(err)
	}
	recovery, err := reopened.BeginRecovery(context.Background(), op.ID, "recovery-worker", "operator")
	if err != nil {
		t.Fatal(err)
	}
	recoveredCtx := persistence.WithOwner(context.Background(), persistence.Owner{OperationID: recovery.ID, Owner: recovery.Owner, Fence: recovery.Fence})
	resumed, err := reopened.ResumeDeliveryExecution(recoveredCtx, b.OrganizationID, prepared.Mutation.ID, prepared.Mutation.Version)
	if err != nil || resumed.Mutation.Fence != recovery.Fence || resumed.Mutation.Owner != recovery.Owner || resumed.Revision != prepared.Revision {
		t.Fatal("transfer intent recovery lost immutable artifact or fence", err)
	}
	if err := reopened.RecoverGitPublication(recoveredCtx, b.OrganizationID, lease.Destination); err != nil {
		t.Fatal("publication lease recovery failed", err)
	}
	lease.Owner = persistence.Owner{OperationID: recovery.ID, Owner: recovery.Owner, Fence: recovery.Fence}
	if err := reopened.CheckGitPublication(recoveredCtx, lease); err != nil {
		t.Fatal(err)
	}
	// Observer leases are independent of execution ownership and cannot replace
	// an active mutator, publish payloads, or acknowledge a stale worker.
	now := time.Now().UTC().Truncate(time.Microsecond)
	poll := persistence.DeliveryPollLease{OrganizationID: b.OrganizationID, UnitID: ack.UnitID, IntentID: ack.Mutation.ID, Owner: "observer-one", LeaseUntil: now.Add(time.Minute), NextPollAt: now.Add(30 * time.Second)}
	if _, err := reopened.ClaimDeliveryPoll(context.Background(), poll, now); err == nil {
		t.Fatal("observer admitted during mutation")
	}
	if err := reopened.ReleaseOperation(recoveredCtx, lease.Owner, environment.OpRecovered, ""); err != nil {
		t.Fatal(err)
	}
	first, err := reopened.ClaimDeliveryPoll(context.Background(), poll, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.ClaimDeliveryPoll(context.Background(), poll, now); err == nil {
		t.Fatal("duplicate poll admitted")
	}
	later := now.Add(61 * time.Second)
	poll.Owner, poll.LeaseUntil, poll.NextPollAt = "observer-two", later.Add(time.Minute), later.Add(30*time.Second)
	second, err := reopened.ClaimDeliveryPoll(context.Background(), poll, later)
	if err != nil || second.Fence <= first.Fence {
		t.Fatal("expired poll was not fenced", err)
	}
	observation := persistence.DeliveryObservation{ID: ids.New(), UnitID: ack.UnitID, Sequence: 1, StoredAt: later, Observation: delivery.Observation{Scope: ack.Revision.Scope, IntendedRevision: ack.Revision.ID, ObservedRevision: ack.Revision.ID, PayloadDigest: ack.Revision.PayloadDigest, Complete: true, Sync: delivery.InSync, Health: delivery.Healthy, Drift: delivery.NoDrift, ObservedAt: later}}
	if err := reopened.SaveDeliveryObservation(context.Background(), first, observation, later); err == nil {
		t.Fatal("stale observer wrote")
	}
	if err := reopened.SaveDeliveryObservation(context.Background(), second, observation, later); err != nil {
		t.Fatal(err)
	}
	observations, err := reopened.ListDeliveryObservations(context.Background(), b.OrganizationID, env.ID, 0, 0, "", 50)
	if err != nil || len(observations) != 1 || observations[0].Observation.IntendedRevision != ack.Revision.ID {
		t.Fatal("observation not retained", err)
	}
	foreign, err := reopened.ListDeliveryObservations(context.Background(), ids.New(), env.ID, 0, 0, "", 50)
	if err != nil || len(foreign) != 0 {
		t.Fatal("foreign observation exposed", err)
	}
	// Expiring read-only polls do not exclude the next mutator. A confirmed
	// stopped Submit can be rebound for observation without becoming PENDING.
	nextOp, err := reopened.ClaimEnvironment(context.Background(), persistence.OperationClaim{ID: ids.New(), ApplicationKey: "delivery-app", EnvironmentKey: "staging", Owner: "submit-worker", Kind: environment.OpDeploy, Deadline: time.Now().Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	nextOwner := persistence.Owner{OperationID: nextOp.ID, Owner: nextOp.Owner, Fence: nextOp.Fence}
	nextCtx := persistence.WithOwner(context.Background(), nextOwner)
	submission := ack
	submission.Revision.ID = ids.New()
	submission.Mutation = delivery.Mutation{ID: ids.New(), Scope: scope, RevisionID: submission.Revision.ID, OperationID: nextOp.ID, RequestID: ids.New(), RequestDigest: strings.Repeat("e", 64), Action: delivery.Submit, Status: delivery.IntentPending, Owner: nextOp.Owner, Fence: nextOp.Fence, Version: 1}
	submission.Version = 1
	submission.CreatedAt = time.Now().UTC()
	submission.UpdatedAt = submission.CreatedAt
	submission, err = reopened.PrepareDelivery(nextCtx, submission)
	if err != nil {
		t.Fatal(err)
	}
	submission.Mutation, _ = submission.Mutation.Advance(delivery.IntentRunning, false)
	submission, err = reopened.UpdateDeliveryExecution(nextCtx, submission, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened.SuspendOperation(nextCtx, nextOwner, "executor stopped"); err != nil {
		t.Fatal(err)
	}
	resolving, err := reopened.BeginRecovery(context.Background(), nextOp.ID, "read-only-recovery", "operator")
	if err != nil {
		t.Fatal(err)
	}
	resolveCtx := persistence.WithOwner(context.Background(), persistence.Owner{OperationID: resolving.ID, Owner: resolving.Owner, Fence: resolving.Fence})
	if _, err := reopened.ResumeDeliveryExecution(resolveCtx, b.OrganizationID, submission.Mutation.ID, submission.Mutation.Version); err == nil {
		t.Fatal("ordinary Submit reset to PENDING")
	}
	rebound, err := reopened.RebindDeliverySubmission(resolveCtx, b.OrganizationID, submission.Mutation.ID, submission.Mutation.Version)
	if err != nil || rebound.Mutation.Status != delivery.IntentRunning || rebound.Mutation.Fence != resolving.Fence || rebound.Revision != submission.Revision {
		t.Fatal("submission observation rebind changed intent", err)
	}
	if _, err := reopened.RebindDeliverySubmission(nextCtx, b.OrganizationID, submission.Mutation.ID, rebound.Mutation.Version); err == nil {
		t.Fatal("stopped worker regained ownership")
	}

}
