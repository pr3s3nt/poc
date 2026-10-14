package tfexec_test

import (
	"context"
	"errors"
	"orchestrator/internal/adapters/postgres"
	"orchestrator/internal/domain/environment"
	"testing"
	"time"

	"orchestrator/internal/application/envops"
	"orchestrator/internal/application/resourcecleanup"
	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/ports/terraformexec"
)

// These tests run the I04-08 admission over the REAL tfexec cleanup engine:
// real disposable PostgreSQL (three roles), the real encrypted state backend
// and the actual Terraform CLI destroying fixture `terraform_data` modules.
// The provider binder and the readiness check stay the harness FAKES (no
// cloud, no cluster). This is the real-engine counterpart of the HTTP tests
// that use the labelled scripted engine.

func (h *harness) deletionService() *resourcecleanup.Service {
	h.t.Helper()
	svc := resourcecleanup.New(h.store, envops.NewManager(h.store))
	svc.SetState(h.state)
	svc.SetEngine(h.svc)
	return svc
}

func (h *harness) cleanupScope() resourcecleanup.Scope {
	return resourcecleanup.Scope{OrganizationKey: h.orgKey, ApplicationKey: h.appKey, EnvironmentKey: "staging"}
}

var peActor = resourcecleanup.Actor{Username: "platform-engineer", MayDelete: true}

func waitTerminal(t *testing.T, svc *resourcecleanup.Service, sc resourcecleanup.Scope, id string) resourcecleanup.CleanupView {
	t.Helper()
	svc.Wait()
	view, err := svc.GetCleanup(context.Background(), sc, id)
	if err != nil {
		t.Fatal(err)
	}
	return view
}

func TestResourceCleanupRealEngineDestroysSelectedDependencyOnly(t *testing.T) {
	h := newHarness(t)
	svc := h.deletionService()
	sc := h.cleanupScope()
	// db consumes ns and cfg; ns has no other consumer, cfg is left unticked.
	ns := h.createInstance("run-1", "k8s-namespace.default#environments.app.staging", scopeOf("ns"), nil)
	cfg := h.createInstance("run-1", "k8s-secret.default#environments.app.cfg", scopeOf("cfg"), nil)
	db := h.createInstance("run-1", "postgres.default#modules.api.externals.db", scopeOf("db"), []string{ns.InstanceID, cfg.InstanceID})
	for _, id := range []string{db.InstanceID, ns.InstanceID, cfg.InstanceID} {
		h.markUnreferenced(id)
	}

	pv, err := svc.PrepareDeletion(context.Background(), sc, peActor, db.InstanceID)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if len(pv.Candidates) != 2 || !pv.Root.Selectable {
		t.Fatalf("preview: %+v", pv)
	}
	// A wrong name (case differs) never reaches the engine.
	opens := h.runner.opens
	if _, err := svc.CleanupResources(context.Background(), sc, peActor, db.InstanceID, resourcecleanup.Input{Token: pv.Token, ConfirmationName: "POSTGRES.default#modules.api.externals.db", Selected: []string{db.InstanceID, ns.InstanceID}}); !errors.Is(err, resourcecleanup.ErrConfirmationMismatch) || h.runner.opens != opens {
		t.Fatalf("wrong name: %v", err)
	}
	adm, err := svc.CleanupResources(context.Background(), sc, peActor, db.InstanceID, resourcecleanup.Input{Token: pv.Token, ConfirmationName: pv.ConfirmationName, Selected: []string{db.InstanceID, ns.InstanceID}})
	if err != nil {
		t.Fatalf("admission: %v", err)
	}
	view := waitTerminal(t, svc, sc, adm.OperationID)
	if view.Status != "SUCCEEDED" || !view.AllVerified || view.Terminal != "SUCCEEDED" || len(view.Resources) != 2 {
		t.Fatalf("operation: %+v", view)
	}
	for _, r := range view.Resources {
		if r.Outcome != "DESTROYED" || !r.Verified {
			t.Fatalf("resource: %+v", r)
		}
	}
	if h.status(db.InstanceID) != tfresource.InstanceDestroyed || h.status(ns.InstanceID) != tfresource.InstanceDestroyed {
		t.Fatal("the confirmed set was not destroyed")
	}
	// The unticked candidate is preserved exactly (state, status, no destroy attempt).
	if h.status(cfg.InstanceID) != tfresource.InstanceUnreferenced {
		t.Fatalf("the unselected candidate changed: %s", h.status(cfg.InstanceID))
	}
	for _, a := range h.attempts(cfg.InstanceID) {
		if a.Action == tfresource.ActionDestroy {
			t.Fatal("the unselected candidate was destroyed")
		}
	}
	// The Environment claim was released; the tombstone and audit stay.
	if _, ok, _ := h.store.ActiveOperation(context.Background(), h.appKey, "staging"); ok {
		t.Fatal("the Environment is still claimed")
	}
	if h.count(`SELECT count(*) FROM terraform_states WHERE resource_instance_id=$1::uuid`, db.InstanceID) != 1 {
		t.Fatal("terminal state tombstone missing")
	}
	detail, err := svc.GetResource(context.Background(), sc, db.InstanceID)
	if err != nil || detail.Status != "DESTROYED" {
		t.Fatalf("detail after deletion: %+v %v", detail, err)
	}

	// Replay after the REAL cleanup retired the graph: the exact actor, token and
	// selected set return the same operation from persistence alone - on the same
	// service, on a reopened Console connection, past the preview expiry and with
	// the execution services unavailable - and no second destroy ever runs.
	opens, destroys := h.runner.opens, h.count(`SELECT count(*) FROM terraform_resource_attempts WHERE action='DESTROY'`)
	same := resourcecleanup.Input{Token: pv.Token, ConfirmationName: pv.ConfirmationName, Selected: []string{ns.InstanceID, db.InstanceID}}
	again, err := svc.CleanupResources(context.Background(), sc, peActor, db.InstanceID, same)
	if err != nil || again.OperationID != adm.OperationID || !again.Replayed {
		t.Fatalf("replay on the same service: %+v %v", again, err)
	}
	reopened, err := postgres.Open(context.Background(), h.db.Console)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	cold := resourcecleanup.New(reopened, envops.NewManager(reopened)) // no State login, no engine
	cold.SetClock(func() time.Time { return time.Now().Add(24 * 365 * time.Hour) })
	cold1, err := cold.CleanupResources(context.Background(), sc, peActor, db.InstanceID, same)
	if err != nil || cold1.OperationID != adm.OperationID || !cold1.Replayed {
		t.Fatalf("replay after reopening, past expiry, without execution services: %+v %v", cold1, err)
	}
	if view, err := cold.GetCleanup(context.Background(), sc, adm.OperationID); err != nil || !view.AllVerified || view.Status != "SUCCEEDED" {
		t.Fatalf("the reopened service lost the operation: %+v %v", view, err)
	}
	// A changed selection of an admitted token, a wrong name and a Developer are
	// still refused, and a new token for the retired graph is refused by planning.
	for name, in := range map[string]resourcecleanup.Input{
		"smaller set": {Token: pv.Token, ConfirmationName: pv.ConfirmationName, Selected: []string{db.InstanceID}},
		"grown set":   {Token: pv.Token, ConfirmationName: pv.ConfirmationName, Selected: []string{db.InstanceID, ns.InstanceID, cfg.InstanceID}},
	} {
		if _, err := cold.CleanupResources(context.Background(), sc, peActor, db.InstanceID, in); !errors.Is(err, resourcecleanup.ErrStalePreview) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := cold.CleanupResources(context.Background(), sc, peActor, db.InstanceID, resourcecleanup.Input{Token: pv.Token, ConfirmationName: "wrong", Selected: same.Selected}); !errors.Is(err, resourcecleanup.ErrConfirmationMismatch) {
		t.Errorf("wrong name on a replay: %v", err)
	}
	if _, err := cold.CleanupResources(context.Background(), sc, resourcecleanup.Actor{Username: "platform-engineer"}, db.InstanceID, same); !errors.Is(err, resourcecleanup.ErrForbidden) {
		t.Errorf("a Developer replay: %v", err)
	}
	if _, err := cold.CleanupResources(context.Background(), sc, resourcecleanup.Actor{Username: "someone-else", MayDelete: true}, db.InstanceID, same); err == nil {
		t.Error("another actor replayed the token")
	}
	if h.runner.opens != opens || h.count(`SELECT count(*) FROM terraform_resource_attempts WHERE action='DESTROY'`) != destroys {
		t.Fatal("a replay ran a second destroy")
	}
}

func TestResourceCleanupRealEngineFailureDoesNotClaimSuccess(t *testing.T) {
	h := newHarness(t)
	svc := h.deletionService()
	sc := h.cleanupScope()
	ns := h.createInstance("run-1", "k8s-namespace.default#environments.app.staging", scopeOf("ns"), nil)
	db := h.createInstance("run-1", "postgres.default#modules.api.externals.db", scopeOf("db"), []string{ns.InstanceID})
	h.markUnreferenced(db.InstanceID)
	h.markUnreferenced(ns.InstanceID)
	pv, err := svc.PrepareDeletion(context.Background(), sc, peActor, db.InstanceID)
	if err != nil {
		t.Fatal(err)
	}
	// The consumer's destroy fails once it has started: its provider stays.
	base := h.runner.applies
	h.runner.onApply = func(n int, real terraformexec.Session) error {
		if n-base == 1 {
			_ = real.Apply(context.Background()) // acknowledged state, then a clean failure
			return &terraformexec.StepError{Step: "apply", Class: terraformexec.ClassFailed, Outcome: terraformexec.OutcomeConfirmed, Stopped: true}
		}
		return real.Apply(context.Background())
	}
	adm, err := svc.CleanupResources(context.Background(), sc, peActor, db.InstanceID, resourcecleanup.Input{Token: pv.Token, ConfirmationName: pv.ConfirmationName, Selected: []string{db.InstanceID, ns.InstanceID}})
	if err != nil {
		t.Fatal(err)
	}
	view := waitTerminal(t, svc, sc, adm.OperationID)
	if view.Status == "SUCCEEDED" || view.AllVerified {
		t.Fatalf("a failed cleanup is reported as done: %+v", view)
	}
	if h.status(ns.InstanceID) == tfresource.InstanceDestroyed {
		t.Fatal("the provider of a failed consumer was destroyed")
	}
	for _, r := range view.Resources {
		if r.Verified && h.status(r.ID) != tfresource.InstanceDestroyed {
			t.Fatalf("verified without persisted absence: %+v", r)
		}
	}
}

func TestResourceCleanupRealEngineUnknownOutcomeHoldsTheEnvironment(t *testing.T) {
	h := newHarness(t)
	svc := h.deletionService()
	sc := h.cleanupScope()
	db := h.createInstance("run-1", "postgres.default#modules.api.externals.db", scopeOf("db"), nil)
	h.markUnreferenced(db.InstanceID)
	pv, err := svc.PrepareDeletion(context.Background(), sc, peActor, db.InstanceID)
	if err != nil {
		t.Fatal(err)
	}
	h.runner.onApply = func(int, terraformexec.Session) error {
		return &terraformexec.StepError{Step: "apply", Class: terraformexec.ClassCancelled, Outcome: terraformexec.OutcomeUnknown}
	}
	adm, err := svc.CleanupResources(context.Background(), sc, peActor, db.InstanceID, resourcecleanup.Input{Token: pv.Token, ConfirmationName: pv.ConfirmationName, Selected: []string{db.InstanceID}})
	if err != nil {
		t.Fatal(err)
	}
	view := waitTerminal(t, svc, sc, adm.OperationID)
	if view.Status != "INTERRUPTED" || view.Terminal != "NEEDS_ATTENTION" || view.AllVerified {
		t.Fatalf("operation: %+v", view)
	}
	if _, ok, _ := h.store.ActiveOperation(context.Background(), h.appKey, "staging"); !ok {
		t.Fatal("an unknown outcome released the Environment")
	}
	// The held Environment refuses every further preview and admission.
	if _, err := svc.PrepareDeletion(context.Background(), sc, peActor, db.InstanceID); err == nil {
		t.Fatal("a held Environment accepted a new deletion preview")
	}
}

func TestMarkUnreferencedMirrorsOnlyReadyInstancesUnderTheClaim(t *testing.T) {
	h := newHarness(t)
	db := h.createInstance("run-1", defaultDescriptor, defaultScope(), nil)
	if h.status(db.InstanceID) != tfresource.InstanceReady {
		t.Fatalf("setup status %s", h.status(db.InstanceID))
	}
	// Without an Environment claim nothing is written.
	if err := h.svc.MarkUnreferenced(context.Background(), 0, defaultDescriptor, string(defaultScope().Type), defaultScope().ID); err == nil || h.status(db.InstanceID) != tfresource.InstanceReady {
		t.Fatalf("marked without a claim: %v", err)
	}
	ctx, op := h.claim("proc-mark")
	if err := h.svc.MarkUnreferenced(ctx, 0, defaultDescriptor, string(defaultScope().Type), defaultScope().ID); err != nil {
		t.Fatal(err)
	}
	if h.status(db.InstanceID) != tfresource.InstanceUnreferenced {
		t.Fatalf("status %s", h.status(db.InstanceID))
	}
	// Idempotent; an unknown identity and another generation are left alone.
	for _, gen := range []int64{0, 7} {
		if err := h.svc.MarkUnreferenced(ctx, gen, defaultDescriptor, string(defaultScope().Type), defaultScope().ID); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.svc.MarkUnreferenced(ctx, 0, "postgres.default#modules.nope.externals.db", string(defaultScope().Type), "x"); err != nil {
		t.Fatal(err)
	}
	// Only READY instances move: a FAILED one keeps its status.
	failed, _ := h.instance(ctx, defaultDescriptor, defaultScope())
	if _, err := h.state.SetInstanceStatus(ctx, failed.ID, failed.Version, tfresource.InstanceFailed); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.MarkUnreferenced(ctx, 0, defaultDescriptor, string(defaultScope().Type), defaultScope().ID); err != nil || h.status(db.InstanceID) != tfresource.InstanceFailed {
		t.Fatalf("a FAILED instance was changed: %s %v", h.status(db.InstanceID), err)
	}
	h.release(op, environment.OpSucceeded)
}
