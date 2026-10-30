package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"orchestrator/internal/domain/drift"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
)

func TestDriftScanPersistenceFencesWorkersAndRetainsUnknownOutcome(t *testing.T) {
	f := newStateFixture(t)
	ctx := context.Background()
	old := persistence.Owner{OperationID: f.op.ID, Owner: f.op.Owner, Fence: f.op.Fence}
	if err := f.state.DeleteLock(persistence.WithOwner(ctx, old), f.inst.ID, "lock-1", old.OperationID, old.Fence); err != nil {
		t.Fatal(err)
	}
	if err := f.console.ReleaseOperation(ctx, old, environment.OpSucceeded, ""); err != nil {
		t.Fatal(err)
	}
	env, err := f.console.GetEnvironment(ctx, "app", "staging")
	if err != nil {
		t.Fatal(err)
	}
	input := drift.Scan{ID: ids.New(), OrganizationID: f.inst.Identity.OrganizationID, EnvironmentID: env.ID, ApplicationKey: "app", EnvironmentKey: "staging", EnvironmentVersion: env.Version, RequestID: ids.New(), RequestDigest: strings.Repeat("a", 64), Status: drift.Queued, Result: drift.Unknown, Scope: []drift.InstancePin{{InstanceID: f.inst.ID, InstanceVersion: f.inst.Version}}, CreatedAt: time.Now().UTC()}
	queued, err := f.console.QueueDriftScan(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := f.console.QueueDriftScan(ctx, input)
	if err != nil || duplicate.ID != queued.ID {
		t.Fatal("request not idempotent", err)
	}
	changed := input
	changed.RequestDigest = strings.Repeat("b", 64)
	if _, err := f.console.QueueDriftScan(ctx, changed); !errors.Is(err, persistence.ErrVersionConflict) {
		t.Fatal("request payload reused", err)
	}
	other := input
	other.ID, other.RequestID = ids.New(), ids.New()
	if _, err := f.console.QueueDriftScan(ctx, other); !errors.Is(err, persistence.ErrEnvironmentBusy) {
		t.Fatal("duplicate active scan admitted", err)
	}
	op, err := f.console.ClaimEnvironment(ctx, persistence.OperationClaim{ID: ids.New(), ApplicationKey: "app", EnvironmentKey: "staging", Owner: "scanner-a", Kind: environment.OpDriftScan, Deadline: time.Now().Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	owner := persistence.Owner{OperationID: op.ID, Owner: op.Owner, Fence: op.Fence}
	worker := persistence.WithOwner(ctx, owner)
	running, err := f.console.StartDriftScan(worker, input.OrganizationID, input.ID, queued.Version)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.console.SuspendOperation(worker, owner, "scanner stopped"); err != nil {
		t.Fatal(err)
	}
	recovered, err := f.console.BeginRecovery(ctx, op.ID, "scanner-b", "operator")
	if err != nil {
		t.Fatal(err)
	}
	finish := time.Now().UTC()
	running.Status, running.Result, running.FinishedAt = drift.Interrupted, drift.Unknown, &finish
	if err := f.console.FinishDriftScan(worker, running, running.Version); !errors.Is(err, persistence.ErrOperationLost) {
		t.Fatal("stale scanner committed", err)
	}
	newWorker := persistence.WithOwner(ctx, persistence.Owner{OperationID: op.ID, Owner: recovered.Owner, Fence: recovered.Fence})
	if err := f.console.FinishDriftScan(newWorker, running, running.Version); err != nil {
		t.Fatal(err)
	}
	loaded, err := f.console.GetDriftScan(ctx, input.OrganizationID, env.ID, input.ID)
	if err != nil || loaded.Status != drift.Interrupted || loaded.Result != drift.Unknown {
		t.Fatal("interruption was presented clean", err)
	}
	if _, err := f.console.GetDriftScan(ctx, ids.New(), env.ID, input.ID); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatal("foreign scan exposed", err)
	}
	if err := f.console.FinishDriftScan(newWorker, running, loaded.Version); err == nil {
		t.Fatal("terminal scan rewritten")
	}
}
