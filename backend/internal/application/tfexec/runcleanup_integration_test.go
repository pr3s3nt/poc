package tfexec_test

import (
	"context"
	"errors"
	"testing"

	"orchestrator/internal/application/envops"
	"orchestrator/internal/application/runcleanup"
	"orchestrator/internal/application/tfexec"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/ports/terraformexec"
)

// These tests run the independent acceptance safety net (runcleanup) over the
// REAL tfexec engine: real disposable PostgreSQL (three roles), the real
// encrypted state backend and the actual Terraform CLI destroying low-cost
// terraform_data fixtures. The scenario is "the runner and the browser were
// killed after the instances were created": the safety net is a fresh service
// that knows nothing about the dead process and must reverse-destroy through
// the original pinned engine, hold on unknown outcomes and never unlock.

func (h *harness) safetyNet() *runcleanup.Service {
	h.t.Helper()
	return runcleanup.New(h.store, h.state, h.svc, h.store, envops.NewManager(h.store))
}

func (h *harness) target() runcleanup.Target {
	return runcleanup.Target{OrganizationKey: h.orgKey, ApplicationKey: h.appKey, EnvironmentKey: "staging", RunIDs: []string{"run-1"}, Generation: 0}
}

func TestSafetyNetReverseDestroysAnAbandonedRunThroughTheEngine(t *testing.T) {
	h := newHarness(t)
	topo := h.chain() // namespace <- db, cache: all CREATED_BY_RUN of run-1; the "runner" then died
	rep, err := h.safetyNet().Run(context.Background(), h.target())
	if err != nil || rep.Outcome != runcleanup.OutcomeClean || len(rep.Runs) != 1 || len(rep.Remaining) != 0 {
		t.Fatalf("report %+v %v", rep, err)
	}
	for _, id := range []string{topo.db.InstanceID, topo.cache.InstanceID, topo.ns.InstanceID} {
		if h.status(id) != tfresource.InstanceDestroyed {
			t.Fatalf("%s is %s, not DESTROYED", id, h.status(id))
		}
	}
	// Consumers first: the namespace is the last instance of the result.
	order := rep.Runs[0].Instances
	if len(order) != 3 || order[2].InstanceID != topo.ns.InstanceID {
		t.Fatalf("destroy order %+v", order)
	}
	// State stays as a tombstone (never purged) and the claim was released.
	if h.count(`SELECT count(*) FROM terraform_states`) != 3 {
		t.Fatal("state rows were removed")
	}
	if h.count(`SELECT count(*) FROM environment_operations WHERE status IN ('ACTIVE','RECOVERING','INTERRUPTED')`) != 0 {
		t.Fatal("the safety net left an active claim")
	}
	// Idempotent.
	again, err := h.safetyNet().Run(context.Background(), h.target())
	if err != nil || again.Outcome != runcleanup.OutcomeClean || len(again.Runs) != 0 {
		t.Fatalf("second run %+v %v", again, err)
	}
}

func TestSafetyNetHoldsOnAnUnknownOutcomeAndNeverResumesOrUnlocks(t *testing.T) {
	h := newHarness(t)
	topo := h.chain()
	h.runner.onApply = func(int, terraformexec.Session) error {
		return &terraformexec.StepError{Step: "apply", Class: terraformexec.ClassCancelled, Outcome: terraformexec.OutcomeUnknown}
	}
	rep, err := h.safetyNet().Run(context.Background(), h.target())
	if err != nil || rep.Outcome != runcleanup.OutcomeAttention || len(rep.Remaining) == 0 {
		t.Fatalf("report %+v %v", rep, err)
	}
	states := h.count(`SELECT count(*) FROM terraform_states`)
	// State and instances are retained (nothing destroyed, nothing purged).
	for _, id := range []string{topo.db.InstanceID, topo.cache.InstanceID, topo.ns.InstanceID} {
		if h.status(id) == tfresource.InstanceDestroyed {
			t.Fatalf("%s was destroyed despite the unknown outcome", id)
		}
	}
	// The Environment stays held (suspended): a second run is refused and does not touch the engine.
	h.runner.onApply = nil
	opens := h.runner.opens
	second, err := h.safetyNet().Run(context.Background(), h.target())
	if !errors.Is(err, runcleanup.ErrBusy) || second.Outcome != runcleanup.OutcomeBusy || h.runner.opens != opens {
		t.Fatalf("second run %+v %v opens %d->%d", second, err, opens, h.runner.opens)
	}
	if h.count(`SELECT count(*) FROM terraform_states`) != states {
		t.Fatal("state changed after the hold")
	}
}

func TestSafetyNetDoesNotTakeOverAClaimHeldByAnotherOperation(t *testing.T) {
	h := newHarness(t)
	h.chain()
	_, op := h.claimHeld("someone-else") // e.g. the killed product server left an INTERRUPTED claim
	opens := h.runner.opens
	rep, err := h.safetyNet().Run(context.Background(), h.target())
	if !errors.Is(err, runcleanup.ErrBusy) || rep.Outcome != runcleanup.OutcomeBusy || h.runner.opens != opens {
		t.Fatalf("report %+v %v", rep, err)
	}
	if active, ok, _ := h.store.ActiveOperation(context.Background(), h.appKey, "staging"); !ok || active.ID != op.ID {
		t.Fatalf("the foreign claim was disturbed: %+v", active)
	}
	h.release(op, environment.OpSucceeded)
}

func TestSafetyNetNeverDestroysAnInstanceAnotherRunChanged(t *testing.T) {
	h := newHarness(t)
	h.createInstance("run-1", defaultDescriptor, defaultScope(), nil)
	// A later run updates the same instance: it is no longer purely CREATED_BY_RUN of run-1.
	ctx, op := h.claim("proc-update")
	if _, err := h.svc.Execute(ctx, h.request("run-2", h.mod, inputs("changed-by-run-2", "k"))); err != nil {
		t.Fatal(err)
	}
	h.release(op, environment.OpSucceeded)
	rep, err := h.safetyNet().Run(context.Background(), h.target())
	if err != nil || rep.Outcome != runcleanup.OutcomeAttention || len(rep.Blocked) == 0 {
		t.Fatalf("report %+v %v", rep, err)
	}
	inst, _ := h.instance(context.Background(), defaultDescriptor, defaultScope())
	if inst.Status == tfresource.InstanceDestroyed {
		t.Fatal("an instance changed by another run was destroyed")
	}
}

func TestSafetyNetReportsFailuresWithoutDroppingState(t *testing.T) {
	h := newHarness(t)
	topo := h.chain()
	base := h.runner.applies
	h.runner.onApply = func(n int, real terraformexec.Session) error {
		if n-base == 1 {
			_ = real.Apply(context.Background()) // acknowledged state, then a clean failure
			return &terraformexec.StepError{Step: "apply", Class: terraformexec.ClassFailed, Outcome: terraformexec.OutcomeConfirmed, Stopped: true}
		}
		return real.Apply(context.Background())
	}
	rep, err := h.safetyNet().Run(context.Background(), h.target())
	if err != nil || rep.Outcome != runcleanup.OutcomeAttention || len(rep.Remaining) == 0 {
		t.Fatalf("report %+v %v", rep, err)
	}
	if rep.Runs[0].Terminal != string(tfexec.CleanupFailed) {
		t.Fatalf("terminal %s", rep.Runs[0].Terminal)
	}
	// The namespace (provider of the failed consumer) is retained, never destroyed to skip its children.
	if h.status(topo.ns.InstanceID) == tfresource.InstanceDestroyed {
		t.Fatal("the namespace was destroyed although a consumer failed")
	}
	if h.count(`SELECT count(*) FROM terraform_states`) != 3 {
		t.Fatal("state rows were dropped")
	}
}

// The manifest-recorded run IDs are the only authority: instances of any other
// run in the same Environment are neither destroyed nor relabeled.
func TestSafetyNetNeverDestroysOrRelabelsInstancesItDoesNotOwn(t *testing.T) {
	h := newHarness(t)
	own := h.createInstance("run-1", defaultDescriptor, defaultScope(), nil)
	otherScope := scopeOf("retained")
	const otherDescriptor = "postgres.default#modules.api.externals.retained"
	foreign := h.createInstance("run-foreign", otherDescriptor, otherScope, nil)
	beforeForeign, ok := h.instance(context.Background(), otherDescriptor, otherScope)
	if !ok {
		t.Fatal("foreign instance missing")
	}
	opens := h.runner.opens
	rep, err := h.safetyNet().Run(context.Background(), h.target())
	if err != nil || rep.Outcome != runcleanup.OutcomeAttention {
		t.Fatalf("an unowned instance keeps the outcome at NEEDS_ATTENTION: %+v %v", rep, err)
	}
	if h.status(own.InstanceID) != tfresource.InstanceDestroyed {
		t.Fatal("the run's own instance must be destroyed")
	}
	after, _ := h.instance(context.Background(), otherDescriptor, otherScope)
	if after.Status == tfresource.InstanceDestroyed || after.Status != beforeForeign.Status || after.CreatedByRunID != "run-foreign" || after.Version != beforeForeign.Version || after.ID != foreign.InstanceID {
		t.Fatalf("a foreign instance was touched or relabeled: before=%+v after=%+v", beforeForeign, after)
	}
	blocked := false
	for _, b := range rep.Blocked {
		blocked = blocked || (b.InstanceID == foreign.InstanceID && b.Reason == "NOT_RUN_OWNED")
	}
	if !blocked || h.runner.opens == opens {
		t.Fatalf("the foreign instance must be reported NOT_RUN_OWNED: %+v", rep.Blocked)
	}
}

func TestSafetyNetWithoutRecordedRunsOrOnAnotherGenerationTouchesNothing(t *testing.T) {
	h := newHarness(t)
	own := h.createInstance("run-1", defaultDescriptor, defaultScope(), nil)
	opens := h.runner.opens
	noRuns := h.target()
	noRuns.RunIDs = nil
	rep, err := h.safetyNet().Run(context.Background(), noRuns)
	if err != nil || rep.Outcome != runcleanup.OutcomeAttention || len(rep.Runs) != 0 || len(rep.Blocked) != 1 || rep.Blocked[0].Reason != "NOT_RUN_OWNED" {
		t.Fatalf("without a recorded run nothing is provably owned: %+v %v", rep, err)
	}
	wrongGeneration := h.target()
	wrongGeneration.Generation = 4
	if rep, err = h.safetyNet().Run(context.Background(), wrongGeneration); err == nil || rep.Outcome != runcleanup.OutcomeAttention {
		t.Fatalf("a generation other than the recorded one must be refused: %+v %v", rep, err)
	}
	if h.runner.opens != opens || h.status(own.InstanceID) == tfresource.InstanceDestroyed {
		t.Fatal("no terraform effect may happen when ownership cannot be proven")
	}
}
