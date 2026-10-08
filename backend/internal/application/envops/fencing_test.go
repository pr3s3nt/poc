package envops_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"orchestrator/internal/adapters/postgres"
	"orchestrator/internal/adapters/store"
	"orchestrator/internal/application/envops"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/persistence/persistencetest"
)

func seedEnvironment(t *testing.T, st persistence.Store) string {
	t.Helper()
	ctx := context.Background()
	if err := st.SaveOrganization(ctx, application.Organization{ID: ids.New(), Key: "acme", Name: "Acme"}); err != nil {
		t.Fatal(err)
	}
	appID := ids.New()
	if err := st.SaveApplication(ctx, application.Application{ID: appID, Key: appID, OrganizationKey: "acme", Name: "Fence", Subdomain: "fence-" + appID[:8], Version: 1}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveEnvironment(ctx, environment.Environment{ID: ids.New(), Key: "staging", ApplicationID: appID, ApplicationKey: appID, Name: "s", Type: "staging", NamespaceIdentity: "app-" + appID + "-staging", Version: 1}); err != nil {
		t.Fatal(err)
	}
	return appID
}

// runFencing drives two independent managers (two backend processes) over two
// Store handles: the old owner stalls, recovery takes the claim, and the old
// owner then tries to write and to release.
func runFencing(t *testing.T, storeA, storeB persistence.Store) {
	ctx := context.Background()
	appKey := seedEnvironment(t, storeA)
	oldManager, newManager := envops.NewManager(storeA), envops.NewManager(storeB)
	// Real default timing: heartbeat 5s, stale 30s. The old owner "stalls" by
	// never heartbeating (its goroutine is stopped via a tiny heartbeat).
	oldManager.SetTiming(time.Hour, 50*time.Millisecond)
	newManager.SetTiming(5*time.Second, 50*time.Millisecond)

	oldLease, err := oldManager.Begin(ctx, envops.Claim{ApplicationKey: appKey, EnvironmentKey: "staging", Kind: environment.OpDeploy})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := newManager.Begin(ctx, envops.Claim{ApplicationKey: appKey, EnvironmentKey: "staging", Kind: environment.OpDeploy}); !errors.Is(err, persistence.ErrEnvironmentBusy) {
		t.Fatalf("second manager must be busy: %v", err)
	}
	time.Sleep(120 * time.Millisecond)
	if n, err := newManager.Sweep(ctx); err != nil || n != 1 {
		t.Fatalf("sweep: %d %v", n, err)
	}
	// Still held after interruption: no TTL release.
	if _, err := newManager.Begin(ctx, envops.Claim{ApplicationKey: appKey, EnvironmentKey: "staging", Kind: environment.OpDeploy}); !errors.Is(err, persistence.ErrEnvironmentBusy) {
		t.Fatalf("interrupted claim released by time: %v", err)
	}
	if _, err := storeB.BeginRecovery(ctx, oldLease.Operation.ID, "new-owner", ""); !errors.Is(err, persistence.ErrRecoveryUnconfirmed) {
		t.Fatalf("recovery without confirmation: %v", err)
	}
	recovered, err := storeB.BeginRecovery(ctx, oldLease.Operation.ID, "new-owner", "operator")
	if err != nil {
		t.Fatal(err)
	}
	newLease := newManager.Adopt(ctx, recovered)
	if err := newLease.Stage("RECOVERING", nil); err != nil {
		t.Fatalf("recovery lease cannot heartbeat: %v", err)
	}

	// The stalled old owner resumes. Its FIRST action is End, before any
	// heartbeat or Stage could have noticed the loss (lost is still false).
	if oldLease.Lost() {
		t.Fatal("precondition: the old owner has not yet noticed the loss")
	}
	if err := oldLease.End(environment.OpSucceeded, ""); !errors.Is(err, persistence.ErrOperationLost) {
		t.Fatalf("old owner End must not release: %v", err)
	}
	if env, _ := storeB.GetEnvironment(ctx, appKey, "staging"); !env.Busy() {
		t.Fatal("the stale End released the recovering owner's claim")
	}
	if op, _ := storeB.GetOperation(ctx, oldLease.Operation.ID); op.Status != environment.OpRecovering {
		t.Fatalf("stale End changed the operation: %+v", op)
	}
	// Every owner-only write of the old owner is fenced, across both stores.
	oldCtx, cancelOld := oldLease.Detached()
	defer cancelOld()
	if err := storeA.SetPublicRoutesPending(oldCtx, appKey, "staging", true); !errors.Is(err, persistence.ErrOperationLost) {
		t.Fatalf("old owner write: %v", err)
	}
	if err := storeA.SaveTransition(oldCtx, environment.Transition{ID: ids.New(), OperationID: oldLease.Operation.ID, ApplicationKey: appKey, EnvironmentKey: "staging", Status: environment.TransitionRunning, Stage: environment.StagePreflight, Mode: environment.ModeDeployNew}); !errors.Is(err, persistence.ErrOperationLost) {
		t.Fatalf("old owner transition write: %v", err)
	}
	if err := oldLease.Stage("late", nil); !errors.Is(err, persistence.ErrOperationLost) {
		t.Fatalf("old owner stage: %v", err)
	}
	if _, err := oldManager.Begin(ctx, envops.Claim{ApplicationKey: appKey, EnvironmentKey: "staging", Kind: environment.OpDeploy}); !errors.Is(err, persistence.ErrEnvironmentBusy) {
		t.Fatalf("concurrent execution after stale End: %v", err)
	}
	if err := storeB.SetPublicRoutesPending(newLease.Ctx, appKey, "staging", true); err != nil {
		t.Fatalf("new owner write: %v", err)
	}

	// The recovery owner itself crashes (no heartbeat): the RECOVERING claim is
	// flagged INTERRUPTED, stays held, and a second recovery takes a higher fence.
	time.Sleep(120 * time.Millisecond)
	sweeper := envops.NewManager(storeA)
	sweeper.SetTiming(time.Hour, 50*time.Millisecond)
	if n, err := sweeper.Sweep(ctx); err != nil || n != 1 {
		t.Fatalf("stale recovery owner must be interrupted: %d %v", n, err)
	}
	if env, _ := storeA.GetEnvironment(ctx, appKey, "staging"); !env.Busy() {
		t.Fatal("an interrupted recovery must keep the claim")
	}
	second, err := storeA.BeginRecovery(ctx, oldLease.Operation.ID, "second-recoverer", "operator")
	if err != nil || second.Fence != recovered.Fence+1 {
		t.Fatalf("second recovery: %+v %v", second, err)
	}
	if err := storeB.SetPublicRoutesPending(newLease.Ctx, appKey, "staging", false); !errors.Is(err, persistence.ErrOperationLost) {
		t.Fatalf("first recovery owner must be fenced by the second: %v", err)
	}
	finalLease := envops.NewManager(storeA).Adopt(ctx, second)
	if err := finalLease.End(environment.OpRecovered, "recovered"); err != nil {
		t.Fatal(err)
	}
	if env, _ := storeA.GetEnvironment(ctx, appKey, "staging"); env.Busy() {
		t.Fatal("final recovery End must free the environment")
	}
}

func TestStaleOwnerCannotWriteOrReleaseAfterRecoveryJSON(t *testing.T) {
	st := store.New()
	runFencing(t, st, st)
}

func TestStaleOwnerCannotWriteOrReleaseAfterRecoveryAcrossPostgresPools(t *testing.T) {
	url := persistencetest.FreshPostgresDatabase(t)
	a, err := postgres.Open(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := postgres.Open(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	runFencing(t, a, b)
}

func TestLeaseContextEnforcesTheRecordedDeadline(t *testing.T) {
	st := store.New()
	appKey := seedEnvironment(t, st)
	m := envops.NewManager(st)
	lease, err := m.Begin(context.Background(), envops.Claim{ApplicationKey: appKey, EnvironmentKey: "staging", Kind: environment.OpDeploy, Deadline: 60 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-lease.Ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("executor context outlived the recorded deadline")
	}
	if err := lease.End(environment.OpSucceeded, ""); err != nil {
		t.Fatal(err)
	}
	ops, _ := st.ListOperations(context.Background(), appKey, "staging", 1)
	if ops[0].Status != environment.OpFailed || ops[0].Failure != "the operation exceeded its deadline" {
		t.Fatalf("deadline must be surfaced, not reported as success: %+v", ops[0])
	}
}

func runSuspend(t *testing.T, st persistence.Store) {
	ctx := context.Background()
	appKey := seedEnvironment(t, st)
	m := envops.NewManager(st)
	lease, err := m.Begin(ctx, envops.Claim{ApplicationKey: appKey, EnvironmentKey: "staging", Kind: environment.OpTransition})
	if err != nil {
		t.Fatal(err)
	}
	if err := lease.Suspend("compensation incomplete"); err != nil {
		t.Fatal(err)
	}
	op, held, _ := st.ActiveOperation(ctx, appKey, "staging")
	if !held || op.Status != environment.OpInterrupted || op.Failure != "compensation incomplete" {
		t.Fatalf("a suspended claim must stay held and visible: %+v", op)
	}
	if _, err := m.Begin(ctx, envops.Claim{ApplicationKey: appKey, EnvironmentKey: "staging", Kind: environment.OpDeploy}); !errors.Is(err, persistence.ErrEnvironmentBusy) {
		t.Fatalf("suspended claim released: %v", err)
	}
	// The suspended owner can no longer release; explicit recovery can proceed.
	if err := lease.End(environment.OpFailed, ""); err != nil {
		t.Fatalf("End after Suspend must be a no-op: %v", err)
	}
	if _, err := st.BeginRecovery(ctx, op.ID, "recoverer", "operator"); err != nil {
		t.Fatal(err)
	}
}

func TestSuspendKeepsTheClaimHeldJSON(t *testing.T) { runSuspend(t, store.New()) }

func TestSuspendKeepsTheClaimHeldPostgres(t *testing.T) {
	st, err := postgres.Open(context.Background(), persistencetest.FreshPostgresDatabase(t))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	runSuspend(t, st)
}

// The ticker (not a manual Stage) keeps an adopted recovery lease alive under
// its own owner identity, and recovery of an EXPIRED original operation gets a
// fresh bounded deadline instead of an already-cancelled context.
func TestAdoptedRecoveryLeaseHeartbeatsAndExpiredOperationsRecover(t *testing.T) {
	st := store.New()
	ctx := context.Background()
	appKey := seedEnvironment(t, st)
	owner := envops.NewManager(st)
	owner.SetTiming(time.Hour, 20*time.Millisecond)
	lease, err := owner.Begin(ctx, envops.Claim{ApplicationKey: appKey, EnvironmentKey: "staging", Kind: environment.OpDeploy, Deadline: 40 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(80 * time.Millisecond)
	if lease.Ctx.Err() == nil {
		t.Fatal("the original context should have expired")
	}
	recoverer := envops.NewManager(st)
	recoverer.SetTiming(30*time.Millisecond, time.Hour)
	if n, _ := recoverer.Sweep(ctx); n != 0 {
		// stale threshold is an hour for this manager; mark via the owner's.
		t.Fatalf("unexpected sweep %d", n)
	}
	if n, _ := owner.Sweep(ctx); n != 1 {
		t.Fatalf("owner sweep: %d", n)
	}
	recovered, err := st.BeginRecovery(ctx, lease.Operation.ID, "r", "operator")
	if err != nil {
		t.Fatal(err)
	}
	if !recovered.Deadline.After(time.Now().Add(time.Minute)) || recovered.Detail["originalDeadline"] == nil {
		t.Fatalf("recovery needs a fresh recorded deadline and keeps the original for audit: %+v", recovered)
	}
	adopted := recoverer.Adopt(ctx, recovered)
	if adopted.Ctx.Err() != nil {
		t.Fatal("an expired original operation must be recoverable")
	}
	first, _ := st.GetOperation(ctx, recovered.ID)
	time.Sleep(200 * time.Millisecond)
	later, _ := st.GetOperation(ctx, recovered.ID)
	if !later.HeartbeatAt.After(first.HeartbeatAt) || later.Status != environment.OpRecovering {
		t.Fatalf("the ticker must advance the persisted heartbeat under the adopted owner: %v -> %v (%s)", first.HeartbeatAt, later.HeartbeatAt, later.Status)
	}
	if err := adopted.End(environment.OpRecovered, ""); err != nil {
		t.Fatal(err)
	}
}

// With the DEFAULT 5s heartbeat, an adopted recovery lease stays alive well past
// one interval; nothing cancels it.
func TestDefaultTimingRecoveryLeaseSurvivesBeyondOneHeartbeat(t *testing.T) {
	if testing.Short() {
		t.Skip("waits for real default heartbeat intervals")
	}
	st := store.New()
	ctx := context.Background()
	appKey := seedEnvironment(t, st)
	owner := envops.NewManager(st)
	owner.SetTiming(time.Hour, 10*time.Millisecond)
	lease, err := owner.Begin(ctx, envops.Claim{ApplicationKey: appKey, EnvironmentKey: "staging", Kind: environment.OpDeploy})
	if err != nil {
		t.Fatal(err)
	}
	_ = lease
	time.Sleep(30 * time.Millisecond)
	_, _ = owner.Sweep(ctx)
	recovered, err := st.BeginRecovery(ctx, lease.Operation.ID, "default-recoverer", "operator")
	if err != nil {
		t.Fatal(err)
	}
	adopted := envops.NewManager(st).Adopt(ctx, recovered) // default 5s heartbeat / 30s stale
	before, _ := st.GetOperation(ctx, recovered.ID)
	time.Sleep(6 * time.Second)
	after, _ := st.GetOperation(ctx, recovered.ID)
	if adopted.Lost() || adopted.Ctx.Err() != nil || !after.HeartbeatAt.After(before.HeartbeatAt) {
		t.Fatalf("recovery lease died or did not heartbeat at default timing: lost=%v err=%v %v -> %v", adopted.Lost(), adopted.Ctx.Err(), before.HeartbeatAt, after.HeartbeatAt)
	}
	if err := adopted.End(environment.OpRecovered, ""); err != nil {
		t.Fatal(err)
	}
}

type outageStore struct {
	persistence.Store
	down atomic.Bool
}

func (o *outageStore) HeartbeatOperation(ctx context.Context, owner persistence.Owner, stage string, detail map[string]any) error {
	if o.down.Load() {
		return errors.New("storage outage")
	}
	return o.Store.HeartbeatOperation(ctx, owner, stage, detail)
}

// When the owner can no longer prove its authority (storage outage), it stops
// its external work and does not release the claim.
func TestStorageOutageStopsExecutionAndKeepsTheClaim(t *testing.T) {
	inner := store.New()
	st := &outageStore{Store: inner}
	appKey := seedEnvironment(t, inner)
	m := envops.NewManager(st)
	m.SetTiming(10*time.Millisecond, 80*time.Millisecond)
	lease, err := m.Begin(context.Background(), envops.Claim{ApplicationKey: appKey, EnvironmentKey: "staging", Kind: environment.OpDeploy})
	if err != nil {
		t.Fatal(err)
	}
	st.down.Store(true)
	select {
	case <-lease.Ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("work continued under unverifiable authority")
	}
	st.down.Store(false)
	if !lease.Lost() {
		t.Fatal("the lease must report lost authority")
	}
	if err := lease.End(environment.OpSucceeded, ""); !errors.Is(err, persistence.ErrOperationLost) {
		t.Fatalf("End must not report success: %v", err)
	}
	if env, _ := inner.GetEnvironment(context.Background(), appKey, "staging"); !env.Busy() {
		t.Fatal("the claim must stay held")
	}
}
