package tfexec_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"orchestrator/internal/adapters/postgres"
	"orchestrator/internal/application/statebackend"
	"orchestrator/internal/application/tfexec"
	deliveryhttp "orchestrator/internal/delivery/http"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/terraformexec"
)

// These tests combine the ACTUAL Terraform CLI, real PostgreSQL and the real
// HTTP state backend (destroy really empties the acknowledged state) with a
// FAKE physical provider seam: the refresh-only plan's drift report stands in
// for the cloud/cluster API that says whether an object still exists, and
// terraform_data objects are relabelled as provider objects. Live proof is a
// later gate.

// providerSeam scripts what the physical provider reports.
type providerSeam struct {
	queries int
	// gone lists the addresses the provider no longer has; apiErr makes the
	// query itself fail.
	gone   []string
	apiErr error
}

func (p *providerSeam) install(h *harness) {
	h.runner.onResources = func(list []terraformexec.StateResource) []terraformexec.StateResource {
		out := make([]terraformexec.StateResource, 0, len(list))
		for _, r := range list {
			r.Provider = "registry.terraform.io/hashicorp/aws"
			out = append(out, r)
		}
		return out
	}
	h.runner.onPlan = func(plan terraformexec.Plan, mode terraformexec.PlanMode) (terraformexec.Plan, error) {
		if mode == terraformexec.PlanRefreshOnly {
			p.queries++
			if p.apiErr != nil {
				return terraformexec.Plan{}, p.apiErr
			}
			plan.Drift = append([]string(nil), p.gone...)
		}
		return plan, nil
	}
}

var both = []string{"terraform_data.db", "terraform_data.pw"}

func destroyReq(h *harness, id string, op string) tfexec.DestroyRequest {
	return tfexec.DestroyRequest{InstanceID: id, OrganizationKey: h.orgKey, RunID: "cleanup-" + op}
}

// A retry after the terminal state is already empty still queries the ORIGINAL
// objects and never declares success until the provider proves them gone.
func TestRetryAfterEmptiedStateStillQueriesTheOriginalObjects(t *testing.T) {
	h := newHarness(t)
	res := h.createInstance("run-1", defaultDescriptor, defaultScope(), nil)
	seam := &providerSeam{}
	seam.install(h)

	// 1: destroy exit 0 empties the state; the provider still has the objects.
	ctx, op := h.claim("proc-clean-1")
	out, err := h.svc.Destroy(ctx, destroyReq(h, res.InstanceID, op.ID))
	if err == nil || out.Destroyed || len(out.Remaining) != 2 || seam.queries < 1 {
		t.Fatalf("first destroy: %+v %v queries=%d", out, err, seam.queries)
	}
	if h.status(res.InstanceID) != tfresource.InstanceFailed {
		t.Fatalf("status %s", h.status(res.InstanceID))
	}
	h.release(op, environment.OpFailed)
	afterFirst := seam.queries

	// 2: the backend state is empty now; the objects are STILL there. The
	// provider must be queried again and the instance must not be DESTROYED.
	ctx2, op2 := h.claim("proc-clean-2")
	out, err = h.svc.Destroy(ctx2, destroyReq(h, res.InstanceID, op2.ID))
	if err == nil || out.Destroyed || len(out.Remaining) != 2 || seam.queries <= afterFirst {
		t.Fatalf("retry with empty state: %+v %v queries=%d", out, err, seam.queries)
	}
	if h.status(res.InstanceID) == tfresource.InstanceDestroyed {
		t.Fatal("an empty terminal state was taken as proof of absence")
	}
	h.release(op2, environment.OpFailed)
	afterSecond := seam.queries

	// 3: the provider proves the original IDs gone: success.
	seam.gone = both
	ctx3, op3 := h.claim("proc-clean-3")
	out, err = h.svc.Destroy(ctx3, destroyReq(h, res.InstanceID, op3.ID))
	if err != nil || !out.Destroyed || len(out.Remaining) != 0 || seam.queries <= afterSecond {
		t.Fatalf("proven absence: %+v %v queries=%d", out, err, seam.queries)
	}
	if h.status(res.InstanceID) != tfresource.InstanceDestroyed {
		t.Fatal("not destroyed after proof")
	}
	// One verification handle spans the cycle; its attempts audit the IDs.
	handles := map[string]bool{}
	for _, a := range h.attempts(res.InstanceID) {
		if a.Action == tfresource.ActionDestroy {
			handle, _ := a.ModuleSnapshot["verifyHandle"].(string)
			handles[handle] = true
			if len(a.ObservedPhysicalIDs) != 2 && a.Status != tfresource.AttemptSucceeded {
				t.Fatalf("destroy attempt lost the original IDs: %+v", a)
			}
		}
	}
	if len(handles) != 1 {
		t.Fatalf("verification handles %v", handles)
	}
}

// A failing verification API after the state was emptied never lets a retry
// skip the query.
func TestVerificationFailureThenRetryCannotSkipTheQuery(t *testing.T) {
	h := newHarness(t)
	res := h.createInstance("run-1", defaultDescriptor, defaultScope(), nil)
	seam := &providerSeam{apiErr: &terraformexec.StepError{Step: "plan", Class: terraformexec.ClassTransient, Outcome: terraformexec.OutcomeConfirmed, Stopped: true}}
	seam.install(h)
	ctx, op := h.claim("proc-clean-1")
	out, err := h.svc.Destroy(ctx, destroyReq(h, res.InstanceID, op.ID))
	if err == nil || out.Destroyed || len(out.Remaining) != 2 {
		t.Fatalf("API failure: %+v %v", out, err)
	}
	first := seam.queries
	h.release(op, environment.OpFailed)
	// The API still fails: still no success, and the query is attempted again.
	ctx2, op2 := h.claim("proc-clean-2")
	out, err = h.svc.Destroy(ctx2, destroyReq(h, res.InstanceID, op2.ID))
	if err == nil || out.Destroyed || len(out.Remaining) != 2 || seam.queries <= first {
		t.Fatalf("retry with a failing API: %+v %v queries %d->%d", out, err, first, seam.queries)
	}
	h.release(op2, environment.OpFailed)
	// The API works and the objects are gone: success only now.
	seam.apiErr, seam.gone = nil, both
	ctx3, op3 := h.claim("proc-clean-3")
	if out, err = h.svc.Destroy(ctx3, destroyReq(h, res.InstanceID, op3.ID)); err != nil || !out.Destroyed {
		t.Fatalf("proven: %+v %v", out, err)
	}
}

// The historical source, target, IDs and the private artifact survive a
// restart of the backend and executor.
func TestVerificationSurvivesBackendAndExecutorRestart(t *testing.T) {
	h := newHarness(t)
	res := h.createInstance("run-1", defaultDescriptor, defaultScope(), nil)
	seam := &providerSeam{}
	seam.install(h)
	ctx, op := h.claim("proc-clean-1")
	if _, err := h.svc.Destroy(ctx, destroyReq(h, res.InstanceID, op.ID)); err == nil {
		t.Fatal("the first verification should fail")
	}
	h.release(op, environment.OpFailed)
	handle := ""
	for _, a := range h.attempts(res.InstanceID) {
		if a.Action == tfresource.ActionDestroy {
			handle, _ = a.ModuleSnapshot["verifyHandle"].(string)
		}
	}
	if _, err := os.Stat(filepath.Join(h.recover.dir, handle)); err != nil || handle == "" {
		t.Fatalf("no private artifact %q: %v", handle, err)
	}

	// Restart: new database pool, backend service, listener, executor and a
	// new store handle over the same private directory.
	h.state.Close()
	state, err := postgres.OpenState(context.Background(), h.db.State)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(state.Close)
	backend, err := statebackend.New(state, h.cipher)
	if err != nil {
		t.Fatal(err)
	}
	l, err := deliveryhttp.ServeTerraformStateLoopback(deliveryhttp.NewTerraformStateHandler(backend))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = l.Close(c)
	})
	h.state, h.stateRepo, h.backend, h.base = state, state, backend, l.Address
	h.recover = &recoveryDir{dir: h.recover.dir}
	svc := h.newService(nil)

	queries := seam.queries
	seam.gone = both
	ctx2, op2 := h.claim("proc-clean-2")
	out, err := svc.Destroy(ctx2, destroyReq(h, res.InstanceID, op2.ID))
	if err != nil || !out.Destroyed || seam.queries != queries+1 {
		t.Fatalf("after restart: %+v %v queries %d->%d", out, err, queries, seam.queries)
	}
}

// A missing or unreadable verification artifact after destroy effects, or one
// that cannot be stored before them, fails closed.
func TestMissingOrUnreadableVerificationArtifactFailsClosed(t *testing.T) {
	// Cannot be stored: the destroy is refused before any effect.
	h := newHarness(t)
	res := h.createInstance("run-1", defaultDescriptor, defaultScope(), nil)
	seam := &providerSeam{}
	seam.install(h)
	h.recover.failVerify = true
	ctx, op := h.claim("proc-a")
	applies := h.runner.applies
	if _, err := h.svc.Destroy(ctx, destroyReq(h, res.InstanceID, op.ID)); err == nil || h.runner.applies != applies {
		t.Fatalf("unstorable artifact: %v applies %d->%d", err, applies, h.runner.applies)
	}
	if h.status(res.InstanceID) != tfresource.InstanceReady {
		t.Fatalf("status %s", h.status(res.InstanceID))
	}
	h.release(op, environment.OpFailed)
	h.recover.failVerify = false

	// A first destroy that emptied the state, then the artifact is lost.
	ctx2, op2 := h.claim("proc-b")
	if _, err := h.svc.Destroy(ctx2, destroyReq(h, res.InstanceID, op2.ID)); err == nil {
		t.Fatal("expected failed verification")
	}
	h.release(op2, environment.OpFailed)
	entries, _ := os.ReadDir(h.recover.dir)
	for _, e := range entries {
		if len(e.Name()) > 7 && e.Name()[:7] == "verify-" {
			_ = os.Remove(filepath.Join(h.recover.dir, e.Name()))
		}
	}
	queries := seam.queries
	seam.gone = both
	ctx3, op3 := h.claimHeld("proc-c")
	out, err := h.svc.Destroy(ctx3, destroyReq(h, res.InstanceID, op3.ID))
	if !errors.Is(err, tfexec.ErrNeedsAttention) || !persistence.Held(ctx3) || out.Destroyed || len(out.Remaining) == 0 || seam.queries != queries {
		t.Fatalf("lost artifact: %+v %v held=%v queries %d->%d", out, err, persistence.Held(ctx3), queries, seam.queries)
	}
	if h.status(res.InstanceID) != tfresource.InstanceNeedsAttention {
		t.Fatalf("status %s", h.status(res.InstanceID))
	}
}

func TestUnreadableVerificationArtifactHolds(t *testing.T) {
	h := newHarness(t)
	res := h.createInstance("run-1", defaultDescriptor, defaultScope(), nil)
	(&providerSeam{}).install(h)
	h.recover.unreadable = true
	ctx, op := h.claimHeld("proc-a")
	_, err := h.svc.Destroy(ctx, destroyReq(h, res.InstanceID, op.ID))
	if !errors.Is(err, tfexec.ErrNeedsAttention) || !persistence.Held(ctx) {
		t.Fatalf("unreadable artifact: %v", err)
	}
}
