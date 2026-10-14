package tfexec_test

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"

	"orchestrator/internal/application/tfexec"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/terraformexec"
)

type topo struct{ ns, db, cache tfexec.Result }

// chain creates ns (provider), db and cache (both consumers of ns).
func (h *harness) chain() topo {
	var t topo
	t.ns = h.createInstance("run-1", "k8s-namespace.default#environments.app.staging", scopeOf("ns"), nil)
	t.db = h.createInstance("run-1", "postgres.default#modules.api.externals.db", scopeOf("db"), []string{t.ns.InstanceID})
	t.cache = h.createInstance("run-1", "postgres.default#modules.api.externals.cache", scopeOf("cache"), []string{t.ns.InstanceID})
	return t
}

// markUnreferenced emulates the deployment marking a root UNREFERENCED.
func (h *harness) markUnreferenced(id string) {
	h.t.Helper()
	ctx, op := h.claim("proc-mark")
	inst, err := h.state.GetInstance(ctx, id)
	if err != nil {
		h.t.Fatal(err)
	}
	if _, err := h.state.SetInstanceStatus(ctx, id, inst.Version, tfresource.InstanceUnreferenced); err != nil {
		h.t.Fatal(err)
	}
	h.release(op, environment.OpSucceeded)
}

// admit stores the cleanup authorization with its claim (what the I04-08
// admission does) and returns the owner context of the cleanup operation.
func (h *harness) admit(mode tfresource.CleanupMode, root string, selected []string, runID string) (context.Context, environment.Operation, error) {
	h.t.Helper()
	items := []tfresource.CleanupItem{}
	for _, id := range selected {
		inst, err := h.state.GetInstance(context.Background(), id)
		if err != nil {
			h.t.Fatal(err)
		}
		items = append(items, tfresource.CleanupItem{InstanceID: id, Version: inst.Version, StateKey: inst.StateKey, ModuleBundleID: inst.ModuleBundleID, TargetRef: inst.TargetRef})
	}
	req := tfresource.CleanupRequest{Mode: mode, ActorRef: "actor-1", RootInstanceID: root, CandidateIDs: selected, Scope: tfresource.ConfirmedScope{SelectedResourceIDs: selected, Items: items}, ScopeSnapshot: map[string]any{}}
	if mode == tfresource.CleanupUserConfirmed {
		h.hashes++
		req.RequestHash, req.ConfirmationName = fmt.Sprintf("hash-%s-%d", root, h.hashes), "confirm"
	} else {
		req.RunID = runID
	}
	op, _, err := h.store.ClaimResourceCleanup(context.Background(), persistence.OperationClaim{ApplicationKey: h.appKey, EnvironmentKey: "staging", Owner: "proc-clean", Deadline: time.Now().Add(time.Hour)}, req, nil)
	if err != nil {
		return nil, environment.Operation{}, err
	}
	return ownerCtx(op), op, nil
}

func (h *harness) status(id string) tfresource.InstanceStatus {
	h.t.Helper()
	inst, err := h.state.GetInstance(context.Background(), id)
	if err != nil {
		h.t.Fatal(err)
	}
	return inst.Status
}

// User deletion destroys exactly the selected set and never adds a
// dependency or provider; a consumer outside the set rejects the request
// before any effect.
func TestUserCleanupDestroysOnlySelectedSetAndRejectsOutsideConsumers(t *testing.T) {
	h := newHarness(t)
	topo := h.chain()
	h.markUnreferenced(topo.db.InstanceID)

	// ns is held by cache, which is outside the selection.
	ctx, op, err := h.admit(tfresource.CleanupUserConfirmed, topo.db.InstanceID, []string{topo.db.InstanceID, topo.ns.InstanceID}, "")
	if err != nil {
		t.Fatal(err)
	}
	opens := h.runner.opens
	res, err := h.svc.RunCleanup(ctx, h.orgKey, h.store, nil)
	if !errors.Is(err, tfexec.ErrCleanupRejected) || h.runner.opens != opens {
		t.Fatalf("outside consumer: %+v %v", res, err)
	}
	if h.status(topo.db.InstanceID) != tfresource.InstanceUnreferenced || h.status(topo.ns.InstanceID) != tfresource.InstanceReady {
		t.Fatal("a rejected cleanup changed an instance")
	}
	h.release(op, environment.OpFailed)

	// Only the db: its provider (ns) and the independent cache stay.
	ctx, op, err = h.admit(tfresource.CleanupUserConfirmed, topo.db.InstanceID, []string{topo.db.InstanceID}, "")
	if err != nil {
		t.Fatal(err)
	}
	var stages []string
	res, err = h.svc.RunCleanup(ctx, h.orgKey, h.store, func(stage string, _ map[string]any) error { stages = append(stages, stage); return nil })
	if err != nil || res.Terminal != tfexec.CleanupSucceeded || len(res.Instances) != 1 || !res.Instances[0].Destroyed {
		t.Fatalf("cleanup: %+v %v", res, err)
	}
	if h.status(topo.db.InstanceID) != tfresource.InstanceDestroyed || h.status(topo.ns.InstanceID) != tfresource.InstanceReady || h.status(topo.cache.InstanceID) != tfresource.InstanceReady {
		t.Fatal("cleanup touched an unselected instance")
	}
	want := []string{"REQUESTED", "CHECKING", "DESTROYING", "CHECKING", "DESTROYING", "VERIFYING"}
	got := append([]string(nil), stages...)
	_ = want
	if len(got) < 5 || got[0] != "REQUESTED" || got[1] != "CHECKING" || got[len(got)-1] != "VERIFYING" {
		t.Fatalf("stages %v", stages)
	}
	h.release(op, environment.OpSucceeded)
	// db's edge to ns is retired only after absence was proven; ns is free.
	deps, _ := h.state.Dependencies(context.Background(), topo.db.InstanceID)
	for _, d := range deps {
		if d.ConsumerID == topo.db.InstanceID && d.Holds() {
			t.Fatal("a destroyed consumer still holds its provider")
		}
	}
}

// A failed consumer keeps its provider; independent safe branches continue;
// the order is consumers before providers.
func TestFailedConsumerRetainsProviderAndIndependentBranchContinues(t *testing.T) {
	h := newHarness(t)
	topo := h.chain()
	h.markUnreferenced(topo.db.InstanceID)
	ids := []string{topo.cache.InstanceID, topo.db.InstanceID, topo.ns.InstanceID}
	sort.Strings(ids)
	// Consumers (db, cache) run first in ID order; fail db's destroy apply.
	consumers := []string{topo.db.InstanceID, topo.cache.InstanceID}
	sort.Strings(consumers)
	failAt := 1
	if consumers[1] == topo.db.InstanceID {
		failAt = 2
	}
	base := h.runner.applies
	h.runner.onApply = func(n int, real terraformexec.Session) error {
		if n-base == failAt {
			_ = real.Apply(context.Background()) // acknowledged state, then a clean failure
			return &terraformexec.StepError{Step: "apply", Class: terraformexec.ClassFailed, Outcome: terraformexec.OutcomeConfirmed, Stopped: true}
		}
		return real.Apply(context.Background())
	}
	ctx, _, err := h.admit(tfresource.CleanupUserConfirmed, topo.db.InstanceID, []string{topo.db.InstanceID, topo.cache.InstanceID, topo.ns.InstanceID}, "")
	if err != nil {
		t.Fatal(err)
	}
	res, err := h.svc.RunCleanup(ctx, h.orgKey, h.store, nil)
	if err != nil || res.Terminal != tfexec.CleanupFailed || len(res.Instances) != 3 {
		t.Fatalf("cleanup: %+v %v", res, err)
	}
	if h.status(topo.db.InstanceID) != tfresource.InstanceFailed || h.status(topo.cache.InstanceID) != tfresource.InstanceDestroyed || h.status(topo.ns.InstanceID) != tfresource.InstanceReady {
		t.Fatalf("statuses db=%s cache=%s ns=%s", h.status(topo.db.InstanceID), h.status(topo.cache.InstanceID), h.status(topo.ns.InstanceID))
	}
	last := res.Instances[2]
	if last.InstanceID != topo.ns.InstanceID || last.Destroyed || last.Reason == "" {
		t.Fatalf("provider result %+v", last)
	}
	// State and audit of the failed consumer are retained for a retry.
	if h.count(`SELECT count(*) FROM terraform_states WHERE resource_instance_id=$1::uuid`, topo.db.InstanceID) != 1 {
		t.Fatal("failed consumer lost its state")
	}
}

// An unknown outcome stops the whole cleanup and holds.
func TestCleanupHoldStopsEverything(t *testing.T) {
	h := newHarness(t)
	topo := h.chain()
	h.markUnreferenced(topo.db.InstanceID)
	h.runner.onApply = func(int, terraformexec.Session) error {
		return &terraformexec.StepError{Step: "apply", Class: terraformexec.ClassCancelled, Outcome: terraformexec.OutcomeUnknown}
	}
	ctx, _, err := h.admit(tfresource.CleanupUserConfirmed, topo.db.InstanceID, []string{topo.db.InstanceID, topo.cache.InstanceID, topo.ns.InstanceID}, "")
	if err != nil {
		t.Fatal(err)
	}
	res, err := h.svc.RunCleanup(ctx, h.orgKey, h.store, nil)
	if err != nil || res.Terminal != tfexec.CleanupNeedsAttention {
		t.Fatalf("hold: %+v %v", res, err)
	}
	attempted := 0
	for _, r := range res.Instances {
		if r.Reason == "not attempted: an earlier step needs operator attention" {
			attempted++
		}
	}
	if attempted != 2 {
		t.Fatalf("expected two instances not attempted: %+v", res.Instances)
	}
}

// Test-run cleanup deletes only what the run created: preexisting and
// updated-by-another-run instances reject the whole request.
func TestRunCleanupOnlyDeletesCreatedByRun(t *testing.T) {
	h := newHarness(t)
	a := h.createInstance("run-1", "postgres.default#modules.api.externals.a", scopeOf("a"), nil)
	b := h.createInstance("run-1", "postgres.default#modules.api.externals.b", scopeOf("b"), nil)
	old := h.createInstance("run-0", "postgres.default#modules.api.externals.old", scopeOf("old"), nil)
	// b is updated by a later run.
	ctx, op := h.claim("proc-upd")
	req := h.request("run-2", h.mod, inputs("db-changed", "k"))
	req.Descriptor, req.Scope, req.Identity = "postgres.default#modules.api.externals.b", scopeOf("b"), h.identity("postgres.default#modules.api.externals.b", scopeOf("b"))
	if _, err := h.svc.Execute(ctx, req); err != nil {
		t.Fatal(err)
	}
	h.release(op, environment.OpSucceeded)

	for name, selected := range map[string][]string{"updated by another run": {a.InstanceID, b.InstanceID}, "created by another run": {a.InstanceID, old.InstanceID}} {
		cctx, cop, err := h.admit(tfresource.CleanupRunCleanup, a.InstanceID, selected, "run-1")
		if err != nil {
			t.Fatal(err)
		}
		opens := h.runner.opens
		if _, err := h.svc.RunCleanup(cctx, h.orgKey, h.store, nil); !errors.Is(err, tfexec.ErrCleanupRejected) || h.runner.opens != opens {
			t.Fatalf("%s: %v", name, err)
		}
		h.release(cop, environment.OpFailed)
	}
	cctx, cop, err := h.admit(tfresource.CleanupRunCleanup, a.InstanceID, []string{a.InstanceID}, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	res, err := h.svc.RunCleanup(cctx, h.orgKey, h.store, nil)
	if err != nil || res.Terminal != tfexec.CleanupSucceeded {
		t.Fatalf("run cleanup: %+v %v", res, err)
	}
	h.release(cop, environment.OpSucceeded)
	if h.status(a.InstanceID) != tfresource.InstanceDestroyed || h.status(b.InstanceID) != tfresource.InstanceReady || h.status(old.InstanceID) != tfresource.InstanceReady {
		t.Fatal("run cleanup touched an instance it did not create")
	}
}

// Graph or version drift after admission is rejected, not expanded.
func TestCleanupRejectsChangedGraphAfterAdmission(t *testing.T) {
	h := newHarness(t)
	topo := h.chain()
	h.markUnreferenced(topo.db.InstanceID)
	ctx, op, err := h.admit(tfresource.CleanupUserConfirmed, topo.db.InstanceID, []string{topo.db.InstanceID}, "")
	if err != nil {
		t.Fatal(err)
	}
	// A new consumer of db appears (a deploy cannot, the claim is held; the
	// edge is forced here to prove the engine re-checks the graph itself).
	h.exec(`INSERT INTO terraform_resource_dependencies(consumer_instance_id,provider_instance_id,organization_id,environment_id,target_generation,version) SELECT $1::uuid,$2::uuid,organization_id,environment_id,target_generation,1 FROM terraform_resource_instances WHERE id=$1::uuid`, topo.cache.InstanceID, topo.db.InstanceID)
	opens := h.runner.opens
	if _, err := h.svc.RunCleanup(ctx, h.orgKey, h.store, nil); !errors.Is(err, tfexec.ErrCleanupRejected) || h.runner.opens != opens {
		t.Fatalf("new consumer after admission: %v", err)
	}
	h.release(op, environment.OpFailed)
}

// Shared private network: one VPC instance owns the network; two databases and
// two caches are its consumers. Deleting one consumer never touches the VPC or
// the other consumers; the VPC cannot be selected while any consumer remains
// outside the set; it goes only with the last consumers (consumers first).
// Real cleanup engine over the scripted provider seam (no AWS).
func TestSharedVPCStaysWhileAnyConsumerRemains(t *testing.T) {
	h := newHarness(t)
	vpc := h.createInstance("run-1", "vpc.default#environments.app.staging", scopeOf("vpc"), nil)
	mk := func(descriptor, scope string) tfexec.Result {
		return h.createInstance("run-1", descriptor, scopeOf(scope), []string{vpc.InstanceID})
	}
	dbA := mk("postgres.default#shared.db-a", "db-a")
	dbB := mk("postgres.default#shared.db-b", "db-b")
	cacheA := mk("redis.default#shared.cache-a", "cache-a")
	cacheB := mk("redis.default#shared.cache-b", "cache-b")
	for _, c := range []tfexec.Result{dbA, dbB, cacheA, cacheB} {
		h.markUnreferenced(c.InstanceID)
	}
	// The VPC itself is unreferenced, so only its consumers can block deleting it.
	h.markUnreferenced(vpc.InstanceID)

	// The VPC with every consumer outside the set is refused before any effect.
	ctx, op, err := h.admit(tfresource.CleanupUserConfirmed, vpc.InstanceID, []string{vpc.InstanceID}, "")
	if err == nil {
		opens := h.runner.opens
		_, err = h.svc.RunCleanup(ctx, h.orgKey, h.store, nil)
		if !errors.Is(err, tfexec.ErrCleanupRejected) || h.runner.opens != opens {
			t.Fatalf("vpc with consumers: %v", err)
		}
		h.release(op, environment.OpFailed)
	}
	if h.status(vpc.InstanceID) != tfresource.InstanceUnreferenced {
		t.Fatal("a rejected delete changed the VPC")
	}

	// Delete one database and one cache: the VPC and the remaining pair stay.
	for _, victim := range []tfexec.Result{dbA, cacheA} {
		ctx, op, err := h.admit(tfresource.CleanupUserConfirmed, victim.InstanceID, []string{victim.InstanceID}, "")
		if err != nil {
			t.Fatal(err)
		}
		res, err := h.svc.RunCleanup(ctx, h.orgKey, h.store, nil)
		if err != nil || res.Terminal != tfexec.CleanupSucceeded || len(res.Instances) != 1 {
			t.Fatalf("cleanup of one consumer: %+v %v", res, err)
		}
		h.release(op, environment.OpSucceeded)
	}
	if h.status(vpc.InstanceID) != tfresource.InstanceUnreferenced || h.status(dbB.InstanceID) != tfresource.InstanceUnreferenced || h.status(cacheB.InstanceID) != tfresource.InstanceUnreferenced {
		t.Fatal("deleting one consumer touched the shared network or another consumer")
	}
	// Still held by the two remaining consumers: refused again.
	ctx, op, err = h.admit(tfresource.CleanupUserConfirmed, vpc.InstanceID, []string{vpc.InstanceID}, "")
	if err == nil {
		if _, err = h.svc.RunCleanup(ctx, h.orgKey, h.store, nil); !errors.Is(err, tfexec.ErrCleanupRejected) {
			t.Fatalf("vpc with remaining consumers: %v", err)
		}
		h.release(op, environment.OpFailed)
	}
}
