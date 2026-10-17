package tfexec_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"orchestrator/internal/application/tfexec"
	"orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/terraformexec"
)

func mustExecute(t *testing.T, h *harness, ctx context.Context, req tfexec.ExecuteRequest) tfexec.Result {
	t.Helper()
	res, err := h.svc.Execute(ctx, req)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	return res
}

// CREATE then redeploy with an in-place UPDATE: same instance, state key,
// physical name and password; durable acknowledged state, no plaintext secret
// in the database, runtime outputs checked, sensitive flags kept.
func TestCreateThenUpdateKeepsIdentityNameAndPassword(t *testing.T) {
	h := newHarness(t)
	ctx, op := h.claim("proc-a")
	created := mustExecute(t, h, ctx, h.request("run-1", h.mod, inputs("db1", "k")))
	if created.Action != tfresource.ActionCreate || created.Outputs["port"] != float64(5432) || created.Outputs["host"] == "" || !created.Sensitive["password"] || created.Sensitive["host"] {
		t.Fatalf("create result %+v", created)
	}
	password, _ := created.Outputs["password"].(string)
	if len(password) != 32 {
		t.Fatalf("generated password shape")
	}
	inst, found := h.instance(ctx, defaultDescriptor, defaultScope())
	if !found || inst.Status != tfresource.InstanceReady || inst.ModuleBundleID != h.mod.bundle.ID {
		t.Fatalf("instance %+v", inst)
	}
	name, _ := inst.PhysicalIntent["name"].(string)
	if !strings.HasPrefix(name, "orc-") || inst.StateKey != h.identity(defaultDescriptor, defaultScope()).StateKey() {
		t.Fatalf("stable name/key: %q %q", name, inst.StateKey)
	}
	attempts := h.attempts(inst.ID)
	if len(attempts) != 1 || attempts[0].Status != tfresource.AttemptSucceeded || attempts[0].Ownership != tfresource.CreatedByRun || attempts[0].Action != tfresource.ActionCreate ||
		attempts[0].Stage != tfexec.StageReady || attempts[0].StateSerial == nil || len(attempts[0].ObservedPhysicalIDs) == 0 {
		t.Fatalf("attempt %+v", attempts)
	}
	// State is durable, encrypted and carries no plaintext secret; the lock is released.
	if h.count(`SELECT count(*) FROM terraform_states WHERE resource_instance_id=$1::uuid`, inst.ID) != 1 || h.count(`SELECT count(*) FROM terraform_state_locks`) != 0 {
		t.Fatal("state or lock rows")
	}
	if h.count(`SELECT count(*) FROM terraform_states WHERE position(convert_to($1,'UTF8') in ciphertext) > 0`, password) != 0 {
		t.Fatal("plaintext password found in ciphertext")
	}
	if h.count(`SELECT count(*) FROM terraform_resource_attempts WHERE position($1 in module_snapshot::text) > 0 OR position($1 in target_snapshot::text) > 0`, password) != 0 ||
		h.count(`SELECT count(*) FROM terraform_resource_instances WHERE position($1 in physical_intent::text) > 0`, password) != 0 {
		t.Fatal("password stored in database metadata")
	}
	// The provider binder received no host fallback and ran before state.
	if len(h.binder.calls) != 1 {
		t.Fatalf("binder calls %d", len(h.binder.calls))
	}
	h.release(op, environment.OpSucceeded)

	// Redeploy in a later run with a changed in-place input.
	ctx2, op2 := h.claim("proc-b")
	updated := mustExecute(t, h, ctx2, h.request("run-2", h.mod, inputs("db2", "k")))
	if updated.Action != tfresource.ActionUpdate || updated.InstanceID != created.InstanceID || updated.StateKey != created.StateKey || updated.Outputs["database"] != "db2" || updated.Outputs["password"] != password {
		t.Fatalf("update result %+v", updated)
	}
	inst2, _ := h.instance(ctx2, defaultDescriptor, defaultScope())
	if inst2.PhysicalIntent["name"] != name || inst2.Status != tfresource.InstanceReady {
		t.Fatalf("identity changed: %+v", inst2)
	}
	attempts = h.attempts(inst.ID)
	if len(attempts) != 2 || attempts[1].Ownership != tfresource.UpdatedByRun || attempts[1].Action != tfresource.ActionUpdate || attempts[1].Status != tfresource.AttemptSucceeded {
		t.Fatalf("update attempts %+v", attempts)
	}
	if got := h.count(`SELECT count(*) FROM terraform_resource_instances`); got != 1 {
		t.Fatalf("redeploy created %d instances", got)
	}
	h.release(op2, environment.OpSucceeded)

	// A third run with identical inputs is a no-op that keeps everything.
	ctx3, _ := h.claim("proc-c")
	again := mustExecute(t, h, ctx3, h.request("run-3", h.mod, inputs("db2", "k")))
	if again.Outputs["password"] != password || again.InstanceID != created.InstanceID {
		t.Fatalf("no-op redeploy changed identity: %+v", again)
	}
}

// A destructive UPDATE (replace) is rejected after the refreshed plan and
// before apply: nothing changes in state or provider objects.
func TestDestructiveUpdateFailsBeforeApply(t *testing.T) {
	h := newHarness(t)
	ctx, op := h.claim("proc-a")
	mustExecute(t, h, ctx, h.request("run-1", h.mod, inputs("db1", "k")))
	h.release(op, environment.OpSucceeded)
	inst, _ := h.instance(ctx, defaultDescriptor, defaultScope())
	var serialBefore int64
	if err := h.conn().QueryRow(context.Background(), `SELECT serial FROM terraform_states WHERE resource_instance_id=$1::uuid`, inst.ID).Scan(&serialBefore); err != nil {
		t.Fatal(err)
	}

	ctx2, _ := h.claim("proc-b")
	appliesBefore := h.runner.applies
	_, err := h.svc.Execute(ctx2, h.request("run-2", h.mod, inputs("db1", "changed-trigger")))
	if !errors.Is(err, tfexec.ErrDestructivePlan) || errors.Is(err, tfexec.ErrNeedsAttention) {
		t.Fatalf("destructive update: %v", err)
	}
	if h.runner.applies != appliesBefore {
		t.Fatal("apply ran for a destructive plan")
	}
	var serialAfter int64
	if err := h.conn().QueryRow(context.Background(), `SELECT serial FROM terraform_states WHERE resource_instance_id=$1::uuid`, inst.ID).Scan(&serialAfter); err != nil || serialAfter != serialBefore {
		t.Fatalf("state changed by a rejected plan: %d -> %d (%v)", serialBefore, serialAfter, err)
	}
	attempts := h.attempts(inst.ID)
	last := attempts[len(attempts)-1]
	if last.Status != tfresource.AttemptFailed || last.SafeErrorCode != "DESTRUCTIVE_PLAN" || last.Stage != tfexec.StagePlanned {
		t.Fatalf("rejected attempt %+v", last)
	}
	inst, _ = h.instance(ctx2, defaultDescriptor, defaultScope())
	if inst.Status != tfresource.InstanceReady {
		t.Fatalf("a rejected update must keep the instance READY, got %s", inst.Status)
	}
}

// Class, module or target replacement is never an ordinary execution.
func TestModuleAndTargetReplacementIsRejectedBeforeEffects(t *testing.T) {
	h := newHarness(t)
	ctx, op := h.claim("proc-a")
	mustExecute(t, h, ctx, h.request("run-1", h.mod, inputs("db1", "k")))
	h.release(op, environment.OpSucceeded)
	other := h.registerModule("pg2", moduleHCL)
	mods := &fixtureModules{h: h, bundles: map[string]module{h.mod.bundle.ID: h.mod, other.bundle.ID: other}}
	svc := h.newService(mods)
	inst, _ := h.instance(ctx, defaultDescriptor, defaultScope())

	ctx2, op2 := h.claim("proc-b")
	opensBefore, attemptsBefore := h.runner.opens, len(h.attempts(inst.ID))
	if _, err := svc.Execute(ctx2, h.request("run-2", other, inputs("db1", "k"))); !errors.Is(err, tfexec.ErrReplacementRequired) {
		t.Fatalf("module replacement: %v", err)
	}
	// A different account behind the same descriptor is a target replacement.
	h.binder.account = "999999999999"
	if _, err := svc.Execute(ctx2, h.request("run-2", h.mod, inputs("db1", "k"))); !errors.Is(err, tfexec.ErrReplacementRequired) {
		t.Fatalf("target replacement: %v", err)
	}
	h.binder.account = "123456789012"
	if h.runner.opens != opensBefore || len(h.attempts(inst.ID)) != attemptsBefore {
		t.Fatal("a rejected replacement ran Terraform or wrote an attempt")
	}
	h.release(op2, environment.OpFailed)
}

// An unknown outcome (killed apply) holds NEEDS_ATTENTION: no duplicate
// CREATE can start, the instance and attempt stay held.
func TestUnknownOutcomeHoldsAndForbidsDuplicateCreate(t *testing.T) {
	h := newHarness(t)
	h.runner.onApply = func(int, terraformexec.Session) error {
		return &terraformexec.StepError{Step: "apply", Class: terraformexec.ClassCancelled, Outcome: terraformexec.OutcomeUnknown, Stopped: true}
	}
	ctx, _ := h.claim("proc-a")
	_, err := h.svc.Execute(ctx, h.request("run-1", h.mod, inputs("db1", "k")))
	if !errors.Is(err, tfexec.ErrNeedsAttention) {
		t.Fatalf("unknown outcome: %v", err)
	}
	inst, _ := h.instance(ctx, defaultDescriptor, defaultScope())
	attempts := h.attempts(inst.ID)
	if inst.Status != tfresource.InstanceNeedsAttention || len(attempts) != 1 || attempts[0].Status != tfresource.AttemptNeedsAttention || attempts[0].Stage != tfexec.StageApplying || attempts[0].SafeErrorCode != "CANCELLED" {
		t.Fatalf("held state: %+v %+v", inst, attempts)
	}
	// Another execution (same or later operation) is refused before any process.
	h.runner.onApply = nil
	opens := h.runner.opens
	if _, err := h.svc.Execute(ctx, h.request("run-1", h.mod, inputs("db1", "k"))); !errors.Is(err, tfexec.ErrInstanceHeld) {
		t.Fatalf("duplicate create: %v", err)
	}
	if h.runner.opens != opens || len(h.attempts(inst.ID)) != 1 {
		t.Fatal("a held instance started another attempt")
	}
}

// A clean failure of a mutating step proves nothing unless the backend
// acknowledged state: with no acknowledged state the instance is held whatever
// the error class, no second apply ever runs, and the claim is marked held. A
// failure whose state IS acknowledged stays a normal, cleanup-eligible failure
// that a later run can repair without a duplicate create.
func TestMissingStateAfterApplyAttemptFailsClosed(t *testing.T) {
	for _, class := range []terraformexec.Class{terraformexec.ClassFailed, terraformexec.ClassAuth, terraformexec.ClassInput} {
		h := newHarness(t)
		h.runner.onApply = func(int, terraformexec.Session) error {
			return &terraformexec.StepError{Step: "apply", Class: class, Outcome: terraformexec.OutcomeConfirmed, Stopped: true}
		}
		ctx, _ := h.claimHeld("proc-a")
		_, err := h.svc.Execute(ctx, h.request("run-1", h.mod, inputs("db1", "k")))
		if !errors.Is(err, tfexec.ErrNeedsAttention) || !persistence.Held(ctx) || h.runner.applies != 1 {
			t.Fatalf("%s without acknowledged state: %v held=%v applies=%d", class, err, persistence.Held(ctx), h.runner.applies)
		}
		inst, _ := h.instance(ctx, defaultDescriptor, defaultScope())
		if inst.Status != tfresource.InstanceNeedsAttention {
			t.Fatalf("%s: status %s", class, inst.Status)
		}
		h.runner.onApply = nil
		if _, err := h.svc.Execute(ctx, h.request("run-1", h.mod, inputs("db1", "k"))); !errors.Is(err, tfexec.ErrInstanceHeld) || h.runner.applies != 1 {
			t.Fatalf("%s: duplicate create after a hold: %v", class, err)
		}
	}
}

func TestKnownAcknowledgedFailureStaysCleanupEligibleAndRepairable(t *testing.T) {
	h := newHarness(t)
	h.runner.onApply = func(n int, real terraformexec.Session) error {
		_ = real.Apply(context.Background())
		return &terraformexec.StepError{Step: "apply", Class: terraformexec.ClassFailed, Outcome: terraformexec.OutcomeConfirmed, Stopped: true}
	}
	ctx, op := h.claimHeld("proc-a")
	_, err := h.svc.Execute(ctx, h.request("run-1", h.mod, inputs("db1", "k")))
	if err == nil || errors.Is(err, tfexec.ErrNeedsAttention) || persistence.Held(ctx) {
		t.Fatalf("acknowledged failure must be an ordinary failure: %v held=%v", err, persistence.Held(ctx))
	}
	inst, _ := h.instance(ctx, defaultDescriptor, defaultScope())
	if inst.Status != tfresource.InstanceFailed || h.count(`SELECT count(*) FROM terraform_states WHERE resource_instance_id=$1::uuid`, inst.ID) != 1 {
		t.Fatalf("status %s", inst.Status)
	}
	h.release(op, environment.OpFailed)
	// A later run repairs it in place: UPDATE over the retained state, never a second create.
	h.runner.onApply = nil
	ctx2, _ := h.claim("proc-b")
	res, err := h.svc.Execute(ctx2, h.request("run-2", h.mod, inputs("db1", "k")))
	if err != nil || res.Action != tfresource.ActionUpdate || res.InstanceID != inst.ID {
		t.Fatalf("repair: %+v %v", res, err)
	}
}

// After the one permitted retry, a SECOND transient apply failure with a
// create the state does not account for holds too (no third apply).
func TestSecondTransientFailureWithUnaccountedCreateHolds(t *testing.T) {
	h := newHarness(t)
	transient := &terraformexec.StepError{Step: "apply", Class: terraformexec.ClassTransient, Outcome: terraformexec.OutcomeConfirmed, Stopped: true}
	planCalls := 0
	h.runner.onPlan = func(plan terraformexec.Plan, mode terraformexec.PlanMode) (terraformexec.Plan, error) {
		planCalls++
		if planCalls == 2 { // the replan after the retry plans one more create
			plan.Changes = append(plan.Changes, terraformexec.ResourceChange{Address: "terraform_data.ghost", Type: "terraform_data", Actions: []string{"create"}})
		}
		return plan, nil
	}
	h.runner.onApply = func(n int, real terraformexec.Session) error {
		if n == 1 {
			_ = real.Apply(context.Background()) // acknowledged and fully accounted: retry allowed
		}
		return transient
	}
	ctx, _ := h.claimHeld("proc-a")
	_, err := h.svc.Execute(ctx, h.request("run-1", h.mod, inputs("db1", "k")))
	if !errors.Is(err, tfexec.ErrNeedsAttention) || !persistence.Held(ctx) || h.runner.applies != 2 {
		t.Fatalf("second transient failure: %v held=%v applies=%d", err, persistence.Held(ctx), h.runner.applies)
	}
}

// Failed state push: the state Terraform kept locally goes to the private
// recovery store with an audited handle; the instance is held. If even the
// recovery store fails the workspace is retained.
func TestStatePushFailureKeepsRecoveryArtifactAndHolds(t *testing.T) {
	for _, storeFails := range []bool{false, true} {
		h := newHarness(t)
		h.recover.fail = storeFails
		h.runner.recoveryState = []byte(`{"version":4,"serial":3,"lineage":"x","resources":[]}`)
		h.runner.onApply = func(int, terraformexec.Session) error {
			return &terraformexec.StepError{Step: "apply", Class: terraformexec.ClassBackend, Outcome: terraformexec.OutcomeConfirmed, Stopped: true}
		}
		ctx, _ := h.claim("proc-a")
		_, err := h.svc.Execute(ctx, h.request("run-1", h.mod, inputs("db1", "k")))
		if !errors.Is(err, tfexec.ErrNeedsAttention) {
			t.Fatalf("failed state push: %v", err)
		}
		inst, _ := h.instance(ctx, defaultDescriptor, defaultScope())
		attempts := h.attempts(inst.ID)
		if inst.Status != tfresource.InstanceNeedsAttention || attempts[0].Status != tfresource.AttemptNeedsAttention || attempts[0].SafeErrorCode != "STATE_BACKEND" {
			t.Fatalf("state: %+v %+v", inst, attempts)
		}
		want := "recovery-" + attempts[0].ID
		if storeFails {
			want = "retained-" + attempts[0].ID
		}
		if attempts[0].RecoveryHandle != want {
			t.Fatalf("recovery handle %q, want %q (store fails: %v)", attempts[0].RecoveryHandle, want, storeFails)
		}
		if !storeFails {
			saved, err := readFile(h.recover.dir, want)
			if err != nil || !strings.Contains(string(saved), `"serial":3`) {
				t.Fatalf("recovery artifact: %v", err)
			}
		}
	}
}

// Bounded retry. A transient failure BEFORE apply (init, plan) is retried
// once. A transient failure DURING apply is retried only when acknowledged
// state proves the outcome; auth, input and unknown never retry.
func TestBoundedTransientRetry(t *testing.T) {
	transient := &terraformexec.StepError{Step: "apply", Class: terraformexec.ClassTransient, Outcome: terraformexec.OutcomeConfirmed, Stopped: true}
	// Positive: the apply really changed things and the backend acknowledged
	// the state before the CLI exited non-zero; the retry replans and finishes.
	h := newHarness(t)
	h.runner.onApply = func(n int, real terraformexec.Session) error {
		if n == 1 {
			if err := real.Apply(context.Background()); err != nil {
				return err
			}
			return transient
		}
		return real.Apply(context.Background())
	}
	ctx, _ := h.claim("proc-a")
	if _, err := h.svc.Execute(ctx, h.request("run-1", h.mod, inputs("db1", "k"))); err != nil {
		t.Fatalf("retried create with acknowledged state: %v", err)
	}
	inst, _ := h.instance(ctx, defaultDescriptor, defaultScope())
	if attempts := h.attempts(inst.ID); len(attempts) != 1 || attempts[0].Status != tfresource.AttemptSucceeded {
		t.Fatalf("a retry must stay one attempt: %+v", attempts)
	}

	// Acknowledged but still transient every time: one retry only, no hold.
	h2 := newHarness(t)
	planCalls := 0
	h2.runner.onPlan = func(plan terraformexec.Plan, mode terraformexec.PlanMode) (terraformexec.Plan, error) {
		planCalls++
		if planCalls == 2 { // the replan after the retry still has work to do
			plan.Changes = append(plan.Changes, terraformexec.ResourceChange{Address: "terraform_data.db", Type: "terraform_data", Actions: []string{"update"}})
		}
		return plan, nil
	}
	h2.runner.onApply = func(n int, real terraformexec.Session) error {
		if n == 1 {
			_ = real.Apply(context.Background())
		}
		return transient
	}
	ctx2, _ := h2.claim("proc-a")
	if _, err := h2.svc.Execute(ctx2, h2.request("run-1", h2.mod, inputs("db1", "k"))); err == nil || errors.Is(err, tfexec.ErrNeedsAttention) {
		t.Fatalf("exhausted retry: %v", err)
	}
	if h2.runner.applies != 2 {
		t.Fatalf("applies %d, want exactly 2 (one retry)", h2.runner.applies)
	}

	// Auth, input, unknown and unconfirmed-stop never retry.
	for _, step := range []*terraformexec.StepError{
		{Step: "apply", Class: terraformexec.ClassAuth, Outcome: terraformexec.OutcomeConfirmed, Stopped: true},
		{Step: "apply", Class: terraformexec.ClassInput, Outcome: terraformexec.OutcomeConfirmed, Stopped: true},
		{Step: "apply", Class: terraformexec.ClassTransient, Outcome: terraformexec.OutcomeUnknown, Stopped: true},
		{Step: "apply", Class: terraformexec.ClassTransient, Outcome: terraformexec.OutcomeConfirmed, Stopped: false},
	} {
		step := step
		h3 := newHarness(t)
		h3.runner.onApply = func(int, terraformexec.Session) error { return step }
		ctx3, _ := h3.claim("proc-a")
		_, _ = h3.svc.Execute(ctx3, h3.request("run-1", h3.mod, inputs("db1", "k")))
		if h3.runner.applies != 1 {
			t.Fatalf("%s/%s retried (applies %d)", step.Class, step.Outcome, h3.runner.applies)
		}
	}
}

// A clean non-zero exit of a transient apply failure proves nothing about the
// provider. Without acknowledged state (or with a planned create missing from
// it) there is exactly one apply, the instance is held and the Environment
// claim is marked held: no second apply, no duplicate create.
func TestTransientApplyFailureWithoutAcknowledgedStateHoldsInsteadOfRetrying(t *testing.T) {
	transient := &terraformexec.StepError{Step: "apply", Class: terraformexec.ClassTransient, Outcome: terraformexec.OutcomeConfirmed, Stopped: true}

	// The remote call "succeeded" but no state was acknowledged.
	h := newHarness(t)
	h.runner.onApply = func(int, terraformexec.Session) error { return transient }
	ctx, _ := h.claimHeld("proc-a")
	_, err := h.svc.Execute(ctx, h.request("run-1", h.mod, inputs("db1", "k")))
	if !errors.Is(err, tfexec.ErrNeedsAttention) || !persistence.Held(ctx) {
		t.Fatalf("unacknowledged apply: %v held=%v", err, persistence.Held(ctx))
	}
	if h.runner.applies != 1 {
		t.Fatalf("applies %d, want exactly one", h.runner.applies)
	}
	inst, _ := h.instance(ctx, defaultDescriptor, defaultScope())
	attempts := h.attempts(inst.ID)
	if inst.Status != tfresource.InstanceNeedsAttention || attempts[0].SafeErrorCode != "OUTCOME_UNKNOWN" || attempts[0].Status != tfresource.AttemptNeedsAttention {
		t.Fatalf("held: %+v %+v", inst, attempts)
	}
	if _, err := h.svc.Execute(ctx, h.request("run-1", h.mod, inputs("db1", "k"))); !errors.Is(err, tfexec.ErrInstanceHeld) || h.runner.applies != 1 {
		t.Fatalf("a held instance accepted another apply: %v", err)
	}

	// State was acknowledged, but a planned create is not accounted for in it.
	h2 := newHarness(t)
	h2.runner.onPlan = func(plan terraformexec.Plan, mode terraformexec.PlanMode) (terraformexec.Plan, error) {
		plan.Changes = append(plan.Changes, terraformexec.ResourceChange{Address: "terraform_data.ghost", Type: "terraform_data", Actions: []string{"create"}})
		return plan, nil
	}
	h2.runner.onApply = func(_ int, real terraformexec.Session) error {
		if err := real.Apply(context.Background()); err != nil {
			return err
		}
		return transient
	}
	ctx2, _ := h2.claimHeld("proc-b")
	if _, err := h2.svc.Execute(ctx2, h2.request("run-1", h2.mod, inputs("db1", "k"))); !errors.Is(err, tfexec.ErrNeedsAttention) || !persistence.Held(ctx2) || h2.runner.applies != 1 {
		t.Fatalf("partial state: %v applies=%d", err, h2.runner.applies)
	}
}

// A transient failure before apply (plan) is safe to retry: it had no effect.
func TestTransientPlanFailureIsRetriedOnce(t *testing.T) {
	h := newHarness(t)
	planCalls := 0
	h.runner.onPlan = func(plan terraformexec.Plan, mode terraformexec.PlanMode) (terraformexec.Plan, error) {
		planCalls++
		if planCalls == 1 {
			return terraformexec.Plan{}, &terraformexec.StepError{Step: "plan", Class: terraformexec.ClassTransient, Outcome: terraformexec.OutcomeConfirmed, Stopped: true}
		}
		return plan, nil
	}
	ctx, _ := h.claim("proc-a")
	if _, err := h.svc.Execute(ctx, h.request("run-1", h.mod, inputs("db1", "k"))); err != nil || planCalls != 2 || h.runner.applies != 1 {
		t.Fatalf("plan retry: %v plans=%d applies=%d", err, planCalls, h.runner.applies)
	}
}

// Runtime outputs must meet the module contract and the Resource Type
// contract before readiness; deterministic mismatch leaves state usable and
// holds nothing.
func TestRuntimeOutputContractIsEnforcedBeforeReadiness(t *testing.T) {
	h := newHarness(t)
	bad := strings.Replace(moduleHCL, `output "port" { value = 5432 }`, `output "port" { value = "5432" }`, 1)
	mod := h.registerModule("pg-bad", bad)
	svc := h.newService(&fixtureModules{h: h, bundles: map[string]module{mod.bundle.ID: mod}})
	ctx, _ := h.claim("proc-a")
	_, err := svc.Execute(ctx, h.request("run-1", mod, inputs("db1", "k")))
	if !errors.Is(err, tfexec.ErrOutputContract) || errors.Is(err, tfexec.ErrNeedsAttention) {
		t.Fatalf("mistyped output: %v", err)
	}
	if len(h.ready.calls) != 0 {
		t.Fatal("readiness ran after an output contract failure")
	}
	inst, _ := h.instance(ctx, defaultDescriptor, defaultScope())
	if inst.Status != tfresource.InstanceFailed || h.count(`SELECT count(*) FROM terraform_states`) != 1 {
		t.Fatalf("failed output contract: status %s", inst.Status)
	}
}

// A deterministic readiness failure after an acknowledged state is a
// retained confirmed failure (cleanup can run); it is not a hold, and the
// real readiness query is always invoked with safe inputs.
func TestReadinessFailureRetainsStateWithoutHold(t *testing.T) {
	h := newHarness(t)
	h.ready.err = terraformexec.ErrNotReady
	ctx, op := h.claim("proc-a")
	_, err := h.svc.Execute(ctx, h.request("run-1", h.mod, inputs("db1", "k")))
	if !errors.Is(err, terraformexec.ErrNotReady) || errors.Is(err, tfexec.ErrNeedsAttention) {
		t.Fatalf("readiness failure: %v", err)
	}
	inst, _ := h.instance(ctx, defaultDescriptor, defaultScope())
	attempts := h.attempts(inst.ID)
	if inst.Status != tfresource.InstanceFailed || attempts[0].Status != tfresource.AttemptFailed || attempts[0].Stage != tfexec.StageReadiness || attempts[0].SafeErrorCode != "NOT_READY" || attempts[0].Ownership != tfresource.CreatedByRun {
		t.Fatalf("readiness failure state: %+v %+v", inst, attempts)
	}
	if h.count(`SELECT count(*) FROM terraform_states WHERE resource_instance_id=$1::uuid`, inst.ID) != 1 {
		t.Fatal("state not retained")
	}
	if len(h.ready.calls) != 1 || h.ready.calls[0].ResourceType != "postgres" || len(h.ready.calls[0].Resources) == 0 || h.ready.calls[0].Deadline.IsZero() {
		t.Fatalf("readiness request %+v", h.ready.calls)
	}
	// An unavailable readiness query is not an unknown apply outcome either.
	h.release(op, environment.OpFailed)
	h.ready.err = errors.New("query failed with SECRET-DETAIL")
	ctx2, _ := h.claim("proc-b")
	_, err = h.svc.Execute(ctx2, h.request("run-2", h.mod, inputs("db1", "k")))
	if err == nil || errors.Is(err, tfexec.ErrNeedsAttention) || strings.Contains(err.Error(), "SECRET-DETAIL") {
		t.Fatalf("unavailable readiness: %v", err)
	}
}

// Without the claim owner, or with a stale fence after recovery took the
// claim, nothing runs: no Terraform process, no attempt.
func TestClaimAndStaleFenceAreRequiredBeforeEffects(t *testing.T) {
	h := newHarness(t)
	if _, err := h.svc.Execute(context.Background(), h.request("run-1", h.mod, inputs("db1", "k"))); !errors.Is(err, tfexec.ErrNoClaim) {
		t.Fatalf("no claim: %v", err)
	}
	ctx, op := h.claim("proc-a")
	if _, err := h.store.MarkInterrupted(context.Background(), farFuture()); err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.BeginRecovery(context.Background(), op.ID, "proc-recover", "operator confirmed stop"); err != nil {
		t.Fatal(err)
	}
	opens := h.runner.opens
	if _, err := h.svc.Execute(ctx, h.request("run-1", h.mod, inputs("db1", "k"))); err == nil {
		t.Fatal("a stale owner executed")
	}
	if h.runner.opens != opens || h.count(`SELECT count(*) FROM terraform_resource_attempts`) != 0 || h.count(`SELECT count(*) FROM terraform_states`) != 0 {
		t.Fatal("a stale owner changed state")
	}
}

// Pins are checked against the module's providers before anything runs.
func TestProviderPinsMustMatchModuleProviders(t *testing.T) {
	h := newHarness(t)
	req := h.request("run-1", h.mod, inputs("db1", "k"))
	req.Pins = []tfmodule.ProviderPin{{Name: "aws", Kind: tfmodule.BindingAWS, Target: tfmodule.TargetEnvironment, ConnectionKey: "aws-account", Region: "us-east-1"}}
	ctx, _ := h.claim("proc-a")
	if _, err := h.svc.Execute(ctx, req); !errors.Is(err, tfexec.ErrBinding) {
		t.Fatalf("pin without provider: %v", err)
	}
	if len(h.binder.calls) != 0 || h.runner.opens != 0 {
		t.Fatal("a mismatched pin reached the binder or Terraform")
	}
}

// ADR-014: every create/update attempt is linked to its Deployment
// (`terraform_resource_attempts.deployment_id`), and the Console reads the
// safe stage/attempt/status projection of it, never the private columns.
func TestAttemptsLinkTheDeploymentAndFeedTheSafeConsoleProjection(t *testing.T) {
	h := newHarness(t)
	ctx, _ := h.claim("proc-a")
	env, err := h.store.GetEnvironment(context.Background(), h.appKey, "staging")
	if err != nil {
		t.Fatal(err)
	}
	record := deployment.Deployment{ID: ids.New(), EnvironmentID: env.ID, OrganizationKey: h.orgKey, ApplicationKey: h.appKey, EnvironmentKey: "staging", ExecutionProfile: "internal-k8s", Action: deployment.ActionDeploy, WorkloadID: "api", ActorRef: "test", Status: deployment.StatusPlanning, BaseEnvironmentVersion: env.Version, StartedAt: time.Now().UTC()}
	if err := h.store.SaveDeployment(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	req := h.request("run-1", h.mod, inputs("db1", "k"))
	req.DeploymentID = record.ID
	mustExecute(t, h, ctx, req)

	inst, found := h.instance(ctx, defaultDescriptor, defaultScope())
	if !found {
		t.Fatal("no instance")
	}
	attempts := h.attempts(inst.ID)
	if len(attempts) != 1 || attempts[0].DeploymentID != record.ID {
		t.Fatalf("the attempt must carry its Deployment: %+v", attempts)
	}
	progress, err := h.store.ListDeploymentAttempts(context.Background(), record.ID)
	if err != nil || len(progress) != 1 {
		t.Fatalf("progress %+v %v", progress, err)
	}
	p := progress[0]
	if p.Descriptor != defaultDescriptor || p.Stage != tfexec.StageReady || p.Status != string(tfresource.AttemptSucceeded) || p.Action != string(tfresource.ActionCreate) || p.ModuleID != "pg" {
		t.Fatalf("projection %+v", p)
	}
}

// The Driver Account version is an operational CAS counter: a rename, a
// verification or a same-target credential replacement raises it without moving
// the physical destination. A fresh plan after such a change reuses the same
// instance, while a changed account or region still requires replacement.
func TestAccountVersionOnlyChangeKeepsTheInstanceButTargetChangeReplacesIt(t *testing.T) {
	h := newHarness(t)
	aws := h.mod
	aws.bundle.Providers = map[string]tfmodule.ProviderRequirement{"aws": {Source: "hashicorp/aws", Version: "5.0.0", Binding: tfmodule.BindingAWS}}
	mods := &fixtureModules{h: h, bundles: map[string]module{aws.bundle.ID: aws}}
	svc := h.newService(mods)
	pin := func(account string, version int64, region string) []tfmodule.ProviderPin {
		return []tfmodule.ProviderPin{{Name: "aws", Kind: tfmodule.BindingAWS, Target: "account", ConnectionKey: account, AccountVersion: version, Region: region}}
	}
	run := func(id string, pins []tfmodule.ProviderPin) (tfexec.Result, error) {
		req := h.request(id, aws, inputs("db1", "k"))
		req.Pins = pins
		ctx, op := h.claim("proc-" + id)
		defer h.release(op, environment.OpSucceeded)
		return svc.Execute(ctx, req)
	}
	if _, err := run("run-1", pin("aws-account", 1, "us-east-1")); err != nil {
		t.Fatal(err)
	}
	first, _ := h.instance(context.Background(), defaultDescriptor, defaultScope())
	if _, err := run("run-2", pin("aws-account", 7, "us-east-1")); err != nil {
		t.Fatalf("a version-only account change must reuse the instance: %v", err)
	}
	second, ok := h.instance(context.Background(), defaultDescriptor, defaultScope())
	if !ok || second.ID != first.ID || second.StateKey != first.StateKey {
		t.Fatalf("instance identity moved: %+v -> %+v", first, second)
	}
	for _, changed := range [][]tfmodule.ProviderPin{pin("aws-other", 7, "us-east-1"), pin("aws-account", 7, "eu-west-1")} {
		if _, err := run("run-3", changed); !errors.Is(err, tfexec.ErrReplacementRequired) {
			t.Fatalf("a changed account or region must require replacement: %v", err)
		}
	}
}
