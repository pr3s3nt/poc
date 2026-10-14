package statebackend_test

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"orchestrator/internal/adapters/postgres"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/terraformstate"
)

// TestFailedWriteRollsBack: a write without a running attempt, a PLAN
// capability, or a transaction that fails after the state commit leaves the
// state, its serial/version and the attempt untouched.
func TestFailedWriteRollsBack(t *testing.T) {
	f := newFixture(t)
	ctx, _ := f.claim(f.store, f.appKey, "staging", "proc-a")
	inst := f.instance(ctx, "redis.in-cluster#cache", "run-1")
	tok := f.token(ctx, inst, tfresource.CapabilityCreate)
	plan := f.token(ctx, inst, tfresource.CapabilityPlan)
	call(t, f.base, "LOCK", inst.StateKey, tok, "", lockBody("l"))
	if code, _ := call(t, f.base, http.MethodPost, inst.StateKey, tok, "l", stateDoc("lin", 1, marker)); code != http.StatusConflict {
		t.Fatalf("write without write-ahead attempt: %d", code)
	}
	if f.count(`SELECT count(*) FROM terraform_states`) != 0 {
		t.Fatal("state committed without attempt")
	}
	att, err := f.attempt(ctx, inst, "run-1", tfresource.ActionCreate, tfresource.CreatedByRun)
	if err != nil {
		t.Fatal(err)
	}
	if code, _ := call(t, f.base, http.MethodPost, inst.StateKey, plan, "l", stateDoc("lin", 1, marker)); code != http.StatusForbidden {
		t.Fatalf("PLAN capability wrote: %d", code)
	}
	if code, _ := call(t, f.base, http.MethodGet, inst.StateKey, plan, "", nil); code != http.StatusNoContent {
		t.Fatalf("PLAN capability read: %d", code)
	}
	if code, _ := call(t, f.base, http.MethodPost, inst.StateKey, tok, "l", stateDoc("lin", 1, marker)); code != http.StatusOK {
		t.Fatalf("write: %d", code)
	}
	// The state commit succeeds inside a transaction that then fails.
	boom := errors.New("later step failed")
	err = f.state.Transact(context.Background(), func(txCtx context.Context) error {
		if err := f.svc.Write(txCtx, tok, inst.StateKey, "l", stateDoc("lin", 2, marker)); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("outer failure: %v", err)
	}
	if f.count(`SELECT count(*) FROM terraform_states WHERE serial=1 AND version=1`) != 1 {
		t.Fatal("failed write advanced serial/version")
	}
	if f.count(`SELECT count(*) FROM terraform_resource_attempts WHERE id=$1::uuid AND state_serial=1`, att.ID) != 1 {
		t.Fatal("failed write advanced the attempt serial")
	}
	// The next write still succeeds from the committed revision.
	if code, _ := call(t, f.base, http.MethodPost, inst.StateKey, tok, "l", stateDoc("lin", 2, marker)); code != http.StatusOK {
		t.Fatalf("write after rollback: %d", code)
	}
}

// TestDependencyGraphAndAttemptsAreDurableAudit: edges recorded under the
// owner hold the provider until the consumer is destroyed or applied without
// them; attempts are write-ahead, classified, immutable and retained; a
// destroyed instance keeps its terminal state and history.
func TestDependencyGraphAndAttemptsAreDurableAudit(t *testing.T) {
	f := newFixture(t)
	ctx, op := f.claim(f.store, f.appKey, "staging", "proc-a")
	ns := f.instance(ctx, "namespace.in-cluster#ns", "run-1")
	redis := f.instance(ctx, "redis.in-cluster#cache", "run-1")
	pg := f.instance(ctx, "postgres.in-cluster#db", "run-1")

	updated, err := f.state.RecordDependencies(ctx, redis.ID, redis.Version, []string{ns.ID})
	if err != nil || updated.Version != redis.Version+1 {
		t.Fatalf("record edge: %v", err)
	}
	if _, err := f.state.RecordDependencies(ctx, redis.ID, redis.Version, []string{pg.ID}); !errors.Is(err, persistence.ErrVersionConflict) {
		t.Fatalf("stale consumer version: %v", err)
	}
	if again, err := f.state.RecordDependencies(ctx, redis.ID, updated.Version, []string{ns.ID}); err != nil || again.Version != updated.Version {
		t.Fatalf("re-recording an existing edge changed the graph version: %v", err)
	}
	if _, err := f.state.RecordDependencies(ctx, redis.ID, updated.Version, []string{redis.ID}); !errors.Is(err, tfresource.ErrInvalid) {
		t.Fatalf("self edge: %v", err)
	}
	redis = updated
	if _, err := f.state.RecordDependencies(context.Background(), pg.ID, pg.Version, []string{ns.ID}); !errors.Is(err, terraformstate.ErrOwnerRequired) {
		t.Fatalf("edge without claim owner: %v", err)
	}

	// A failed consumer keeps holding its provider; retirement is refused.
	redis, err = f.state.SetInstanceStatus(ctx, redis.ID, redis.Version, tfresource.InstanceFailed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.state.RetireDependencies(ctx, redis.ID, redis.Version, []string{ns.ID}); !errors.Is(err, persistence.ErrImmutable) {
		t.Fatalf("failed consumer edge retired: %v", err)
	}
	if err := f.execErr(`DELETE FROM terraform_resource_dependencies WHERE consumer_instance_id=$1::uuid`, redis.ID); err == nil {
		t.Fatal("database removed the edge of a failed consumer")
	}
	if err := f.execErr(`UPDATE terraform_resource_dependencies SET provider_instance_id=$2::uuid WHERE consumer_instance_id=$1::uuid`, redis.ID, pg.ID); err == nil {
		t.Fatal("edge identity changed")
	}
	deps, err := f.state.Dependencies(context.Background(), ns.ID)
	if err != nil || len(deps) != 1 || !deps[0].Holds() || deps[0].Version != redis.Version-1 {
		t.Fatalf("provider edges: %+v %v", deps, err)
	}

	// Attempts: write-ahead, classification kept on retry, one RUNNING at a time.
	a1, err := f.attempt(ctx, redis, "run-1", tfresource.ActionCreate, tfresource.CreatedByRun)
	if err != nil || a1.Number != 1 || a1.Status != tfresource.AttemptRunning {
		t.Fatalf("attempt 1: %+v %v", a1, err)
	}
	if _, err := f.attempt(ctx, redis, "run-1", tfresource.ActionCreate, tfresource.CreatedByRun); !errors.Is(err, persistence.ErrVersionConflict) {
		t.Fatalf("second running attempt: %v", err)
	}
	if _, err := f.state.UpdateAttempt(ctx, a1.ID, tfresource.AttemptProgress{Stage: "APPLY", Status: tfresource.AttemptFailed, ObservedPhysicalIDs: []string{"redis-stable"}, SafeErrorCode: "PROVIDER_TIMEOUT"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.state.UpdateAttempt(ctx, a1.ID, tfresource.AttemptProgress{Stage: "APPLY", Status: tfresource.AttemptSucceeded}); !errors.Is(err, persistence.ErrImmutable) {
		t.Fatalf("final attempt rewritten: %v", err)
	}
	if _, err := f.attempt(ctx, redis, "run-1", tfresource.ActionCreate, tfresource.UpdatedByRun); !errors.Is(err, tfresource.ErrInvalid) {
		t.Fatalf("retry changed ownership: %v", err)
	}
	a2, err := f.attempt(ctx, redis, "run-1", tfresource.ActionCreate, tfresource.CreatedByRun)
	if err != nil || a2.Number != 2 {
		t.Fatalf("retry attempt: %+v %v", a2, err)
	}
	if _, err := f.attempt(ctx, pg, "run-other", tfresource.ActionUpdate, tfresource.CreatedByRun); !errors.Is(err, tfresource.ErrInvalid) {
		t.Fatalf("another run claimed CREATED_BY_RUN: %v", err)
	}
	for name, sql := range map[string]string{
		"module snapshot": `UPDATE terraform_resource_attempts SET module_snapshot='{}' WHERE id=$1::uuid`,
		"ownership":       `UPDATE terraform_resource_attempts SET ownership='PREEXISTING' WHERE id=$1::uuid`,
		"delete":          `DELETE FROM terraform_resource_attempts WHERE id=$1::uuid`,
	} {
		if err := f.execErr(sql, a1.ID); err == nil {
			t.Errorf("attempt %s changed", name)
		}
	}

	// Destroy writes an empty terminal state; state, attempts and edges stay.
	tok := f.token(ctx, redis, tfresource.CapabilityDestroy)
	call(t, f.base, "LOCK", redis.StateKey, tok, "", lockBody("d"))
	if code, _ := call(t, f.base, http.MethodPost, redis.StateKey, tok, "d", stateDoc("lin", 1, marker)); code != http.StatusOK {
		t.Fatalf("apply state: %d", code)
	}
	if code, _ := call(t, f.base, http.MethodPost, redis.StateKey, tok, "d", []byte(`{"version":4,"serial":2,"lineage":"lin","resources":[]}`)); code != http.StatusOK {
		t.Fatalf("terminal state: %d", code)
	}
	call(t, f.base, "UNLOCK", redis.StateKey, tok, "", lockBody("d"))
	if err := f.execErr(`DELETE FROM terraform_states WHERE resource_instance_id=$1::uuid`, redis.ID); err == nil {
		t.Fatal("terminal state purged")
	}
	cur, _ := f.state.GetInstance(context.Background(), redis.ID)
	if cur, err = f.state.SetInstanceStatus(ctx, redis.ID, cur.Version, tfresource.InstanceDestroyed); err != nil {
		t.Fatal(err)
	}
	deps, _ = f.state.Dependencies(context.Background(), ns.ID)
	if len(deps) != 1 || deps[0].Holds() {
		t.Fatalf("destroyed consumer still holds or edge lost: %+v", deps)
	}
	if retired, err := f.state.RetireDependencies(ctx, redis.ID, cur.Version, []string{ns.ID}); err != nil || retired.Version != cur.Version+1 {
		t.Fatalf("retire after absence: %v", err)
	}
	if hist, _ := f.state.ListAttempts(context.Background(), redis.ID); len(hist) != 2 || hist[0].ModuleSnapshot["bundle"] != f.bundle || hist[0].TargetSnapshot["connection"] != "k8s-4f" {
		t.Fatalf("historical attempts: %+v", hist)
	}
	if orig, _ := f.state.GetInstance(context.Background(), redis.ID); orig.TargetRef["namespace"] != "orch-v2-test" || orig.PhysicalIntent["name"] != "redis-stable" {
		t.Fatal("original target/physical intent lost")
	}
	if f.count(`SELECT count(*) FROM terraform_states WHERE resource_instance_id=$1::uuid AND serial=2`, redis.ID) != 1 {
		t.Fatal("terminal state not retained")
	}
	_ = op
}

// TestReleaseRaceWithLockAdmission: two connections race state-lock admission
// against Environment claim release. Never may a released claim leave a state
// lock behind.
func TestReleaseRaceWithLockAdmission(t *testing.T) {
	f := newFixture(t)
	second, err := postgres.Open(context.Background(), f.db.Console)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	ctx, _ := f.claim(f.store, f.appKey, "staging", "proc-a")
	inst := f.instance(ctx, "redis.in-cluster#cache", "run-1")
	f.release(f.store, ctx)
	sawLockWin, sawReleaseWin := false, false
	for round := 0; round < 20; round++ {
		ctx, op := f.claim(f.store, f.appKey, "staging", "proc-a")
		tok := f.token(ctx, inst, tfresource.CapabilityCreate)
		owner := persistence.Owner{OperationID: op.ID, Owner: op.Owner, Fence: op.Fence}
		var wg sync.WaitGroup
		start := make(chan struct{})
		var lockCode int
		var releaseErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			lockCode, _ = call(t, f.base, "LOCK", inst.StateKey, tok, "", lockBody("race"))
		}()
		go func() {
			defer wg.Done()
			<-start
			releaseErr = second.ReleaseOperation(context.Background(), owner, environment.OpSucceeded, "")
		}()
		close(start)
		wg.Wait()
		released := f.count(`SELECT count(*) FROM environment_operations WHERE id=$1::uuid AND status='SUCCEEDED'`, op.ID) == 1
		locked := f.count(`SELECT count(*) FROM terraform_state_locks WHERE environment_operation_id=$1::uuid`, op.ID) == 1
		if released && locked {
			t.Fatalf("round %d: released claim left a state lock", round)
		}
		switch {
		case lockCode == http.StatusOK:
			sawLockWin = true
			if !errors.Is(releaseErr, persistence.ErrStateLocksHeld) || released {
				t.Fatalf("round %d: lock won but release: %v", round, releaseErr)
			}
			if code, _ := call(t, f.base, "UNLOCK", inst.StateKey, tok, "", lockBody("race")); code != http.StatusOK {
				t.Fatalf("unlock: %d", code)
			}
			f.release(f.store, ctx)
		case releaseErr == nil:
			sawReleaseWin = true
			if lockCode != http.StatusUnauthorized && lockCode != http.StatusForbidden {
				t.Fatalf("round %d: lock after release: %d", round, lockCode)
			}
		default:
			t.Fatalf("round %d: lock %d release %v", round, lockCode, releaseErr)
		}
	}
	t.Logf("lock won: %v, release won: %v", sawLockWin, sawReleaseWin)
}

func (f *fixture) release(st persistence.Store, ctx context.Context) {
	f.t.Helper()
	owner, _ := persistence.OwnerFrom(ctx)
	if err := st.ReleaseOperation(context.Background(), owner, environment.OpSucceeded, ""); err != nil {
		f.t.Fatalf("release: %v", err)
	}
}

// TestCleanupClaimStoresExactSelectionScoped: the claim persists exactly the
// selected set with the root, never adds a dependency, replays only within the
// same Environment/root/selection, and the admitted scope is immutable.
func TestCleanupClaimStoresExactSelectionScoped(t *testing.T) {
	f := newFixture(t)
	ctx, _ := f.claim(f.store, f.appKey, "staging", "proc-a")
	ns := f.instance(ctx, "namespace.in-cluster#ns", "run-1")
	root := f.instance(ctx, "redis.in-cluster#cache", "run-1")
	root, _ = f.state.RecordDependencies(ctx, root.ID, root.Version, []string{ns.ID})
	f.release(f.store, ctx)
	prodCtx, _ := f.claim(f.store, f.appKey, "prod", "proc-b")
	foreign := f.instance(prodCtx, "redis.in-cluster#cache", "run-1")
	f.release(f.store, prodCtx)

	item := func(i tfresource.Instance) tfresource.CleanupItem {
		return tfresource.CleanupItem{InstanceID: i.ID, Version: i.Version, StateKey: i.StateKey, ModuleBundleID: i.ModuleBundleID, TargetRef: i.TargetRef}
	}
	req := tfresource.CleanupRequest{Mode: tfresource.CleanupUserConfirmed, ActorRef: "user-1", RootInstanceID: root.ID, CandidateIDs: []string{ns.ID}, RequestHash: "hash-1", ConfirmationName: "redis.in-cluster#cache",
		Scope: tfresource.ConfirmedScope{SelectedResourceIDs: []string{root.ID}, Items: []tfresource.CleanupItem{item(root)}}, ScopeSnapshot: map[string]any{"envVersion": 1}}
	claim := persistence.OperationClaim{ApplicationKey: f.appKey, EnvironmentKey: "staging", Owner: "proc-c", Deadline: time.Now().Add(time.Hour)}

	if _, err := f.store.ClaimEnvironment(context.Background(), persistence.OperationClaim{ApplicationKey: f.appKey, EnvironmentKey: "staging", Owner: "x", Kind: environment.OpResourceCleanup, Deadline: time.Now().Add(time.Hour)}); err == nil {
		t.Fatal("cleanup claimed without a confirmed scope")
	}
	stale := req
	stale.Scope.Items = []tfresource.CleanupItem{item(root)}
	stale.Scope.Items[0].Version--
	if _, _, err := f.store.ClaimResourceCleanup(context.Background(), claim, stale, nil); !errors.Is(err, persistence.ErrVersionConflict) {
		t.Fatalf("stale pinned version: %v", err)
	}
	outside := req
	outside.CandidateIDs = []string{foreign.ID}
	outside.Scope = tfresource.ConfirmedScope{SelectedResourceIDs: []string{root.ID, foreign.ID}, Items: []tfresource.CleanupItem{item(root), item(foreign)}}
	if _, _, err := f.store.ClaimResourceCleanup(context.Background(), claim, outside, nil); !errors.Is(err, tfresource.ErrInvalid) {
		t.Fatalf("foreign instance admitted: %v", err)
	}

	op, replayed, err := f.store.ClaimResourceCleanup(context.Background(), claim, req, nil)
	if err != nil || replayed || op.Kind != environment.OpResourceCleanup {
		t.Fatalf("claim: %v", err)
	}
	stored, err := f.store.CleanupRequest(context.Background(), op.ID)
	if err != nil || !tfresource.SameSelection(stored.Scope.SelectedResourceIDs, []string{root.ID}) || len(stored.Scope.Items) != 1 {
		t.Fatalf("stored selection added items: %+v %v", stored.Scope, err)
	}
	again, replayed, err := f.store.ClaimResourceCleanup(context.Background(), claim, req, nil)
	if err != nil || !replayed || again.ID != op.ID {
		t.Fatalf("replay: %v", err)
	}
	grown := req
	grown.Scope = tfresource.ConfirmedScope{SelectedResourceIDs: []string{root.ID, ns.ID}, Items: []tfresource.CleanupItem{item(root), item(ns)}}
	if _, _, err := f.store.ClaimResourceCleanup(context.Background(), claim, grown, nil); !errors.Is(err, terraformstate.ErrCleanupSelectionChanged) {
		t.Fatalf("changed selection after admission: %v", err)
	}
	// Same actor and token hash against another Environment never returns the
	// staging operation.
	foreignReq := req
	foreignReq.RootInstanceID = foreign.ID
	foreignReq.CandidateIDs = nil
	foreignReq.Scope = tfresource.ConfirmedScope{SelectedResourceIDs: []string{foreign.ID}, Items: []tfresource.CleanupItem{item(foreign)}}
	prodClaim := claim
	prodClaim.EnvironmentKey = "prod"
	if got, _, err := f.store.ClaimResourceCleanup(context.Background(), prodClaim, foreignReq, nil); !errors.Is(err, terraformstate.ErrCleanupSelectionChanged) || got.ID == op.ID {
		t.Fatalf("cross-scope replay: %v", err)
	}
	for name, sql := range map[string]string{
		"grow scope": `UPDATE environment_operations SET confirmed_scope=jsonb_set(confirmed_scope,'{selectedResourceIds}',confirmed_scope->'selectedResourceIds'||to_jsonb($2::text)) WHERE id=$1::uuid`,
		"kind":       `UPDATE environment_operations SET kind='DEPLOY' WHERE id=$1::uuid AND $2::text IS NOT NULL`,
	} {
		if err := f.execErr(sql, op.ID, ns.ID); err == nil {
			t.Errorf("admitted cleanup %s changed", name)
		}
	}
	if err := f.execErr(`INSERT INTO environment_operations(id,environment_id,kind,owner_instance,status,pins,started_at,updated_at,heartbeat_at,deadline,authorization_mode,actor_ref,root_resource_instance_id,confirmed_scope,scope_snapshot,run_id) SELECT gen_random_uuid(),environment_id,'RESOURCE_CLEANUP','o','SUCCEEDED','{}',now(),now(),now(),now(),'RUN_CLEANUP','runner',$1::uuid,jsonb_build_object('selectedResourceIds',jsonb_build_array($2::text)),'{}','run-1' FROM terraform_resource_instances WHERE id=$1::uuid`, root.ID, ns.ID); err == nil {
		t.Fatal("database accepted a cleanup scope without its root")
	}
}
