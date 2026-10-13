package tfexec_test

import (
	"errors"
	"strings"
	"testing"

	"orchestrator/internal/application/tfexec"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/ports/terraformexec"
)

// createInstance runs a CREATE under its own claim and releases it.
func (h *harness) createInstance(run, descriptor string, scope resource.Scope, providers []string) tfexec.Result {
	h.t.Helper()
	ctx, op := h.claim("proc-" + run)
	req := h.request(run, h.mod, inputs("db-"+run, "k"))
	req.Descriptor, req.Scope = descriptor, scope
	req.Identity = h.identity(descriptor, scope)
	req.ProviderIDs = providers
	res, err := h.svc.Execute(ctx, req)
	if err != nil {
		h.t.Fatalf("create %s: %v", descriptor, err)
	}
	h.release(op, environment.OpSucceeded)
	return res
}

func scopeOf(id string) resource.Scope {
	return resource.Scope{Type: resource.ScopeWorkload, ID: "app.staging." + id}
}

// DESTROY with the original pinned module/target/state: terraform destroys,
// the empty terminal state stays as a tombstone, absence is proven, history is
// retained, and the identity can be created again later.
func TestDestroyKeepsTombstoneAndAudit(t *testing.T) {
	h := newHarness(t)
	res := h.createInstance("run-1", defaultDescriptor, defaultScope(), nil)
	ctx, op := h.claim("proc-clean")
	out, err := h.svc.Destroy(ctx, tfexec.DestroyRequest{InstanceID: res.InstanceID, OrganizationKey: h.orgKey, RunID: "cleanup-" + op.ID})
	if err != nil || !out.Destroyed || len(out.Remaining) != 0 || out.Stage != tfexec.StageDestroyed {
		t.Fatalf("destroy: %+v %v", out, err)
	}
	inst, _ := h.instance(ctx, defaultDescriptor, defaultScope())
	attempts := h.attempts(inst.ID)
	last := attempts[len(attempts)-1]
	if inst.Status != tfresource.InstanceDestroyed || last.Action != tfresource.ActionDestroy || last.Status != tfresource.AttemptSucceeded || last.Stage != tfexec.StageDestroyed || last.StateSerial == nil {
		t.Fatalf("destroyed instance %+v attempt %+v", inst, last)
	}
	// The terminal empty state is retained (never purged).
	if h.count(`SELECT count(*) FROM terraform_states WHERE resource_instance_id=$1::uuid`, inst.ID) != 1 || h.count(`SELECT count(*) FROM terraform_resource_instances`) != 1 || h.count(`SELECT count(*) FROM terraform_state_locks`) != 0 {
		t.Fatal("tombstone or lock rows")
	}
	// Idempotent.
	if again, err := h.svc.Destroy(ctx, tfexec.DestroyRequest{InstanceID: res.InstanceID, OrganizationKey: h.orgKey, RunID: "cleanup-" + op.ID}); err != nil || !again.Destroyed {
		t.Fatalf("second destroy: %+v %v", again, err)
	}
	h.release(op, environment.OpSucceeded)
	// The same identity can be created again over the tombstone; it is never
	// classified CREATED_BY_RUN because prior state exists.
	ctx2, _ := h.claim("proc-again")
	again, err := h.svc.Execute(ctx2, h.request("run-9", h.mod, inputs("db-new", "k")))
	if err != nil || again.InstanceID != res.InstanceID || again.Action != tfresource.ActionCreate {
		t.Fatalf("recreate: %+v %v", again, err)
	}
	attempts = h.attempts(inst.ID)
	if attempts[len(attempts)-1].Ownership != tfresource.UpdatedByRun {
		t.Fatalf("recreate ownership %+v", attempts[len(attempts)-1])
	}
}

// Destroy uses the historical pinned target: a credential for another
// account or region is refused before any effect.
func TestDestroyUsesOriginalTargetNotCurrentMatching(t *testing.T) {
	h := newHarness(t)
	res := h.createInstance("run-1", defaultDescriptor, defaultScope(), nil)
	ctx, op := h.claim("proc-clean")
	h.binder.account = "999999999999"
	opens := h.runner.opens
	_, err := h.svc.Destroy(ctx, tfexec.DestroyRequest{InstanceID: res.InstanceID, OrganizationKey: h.orgKey, RunID: "cleanup-" + op.ID})
	if !errors.Is(err, tfexec.ErrBinding) || h.runner.opens != opens {
		t.Fatalf("other account: %v", err)
	}
	h.binder.account = "123456789012"
	h.binder.region = "eu-west-1"
	if _, err := h.svc.Destroy(ctx, tfexec.DestroyRequest{InstanceID: res.InstanceID, OrganizationKey: h.orgKey, RunID: "cleanup-" + op.ID}); !errors.Is(err, tfexec.ErrBinding) {
		t.Fatalf("other region: %v", err)
	}
	inst, _ := h.instance(ctx, defaultDescriptor, defaultScope())
	if inst.Status != tfresource.InstanceReady {
		t.Fatalf("refused destroy changed status to %s", inst.Status)
	}
}

// FAKE refresh session: objects that the provider still reports are listed as
// remaining IDs, the attempt fails with PHYSICAL_RESOURCE_REMAINS, state and
// history are retained and a retry is possible.
func TestFakeProviderStillHasObjectsKeepsRemainingIDs(t *testing.T) {
	h := newHarness(t)
	res := h.createInstance("run-1", defaultDescriptor, defaultScope(), nil)
	// Pretend terraform_data are provider objects: the fake refresh reports no drift.
	h.runner.onResources = func(list []terraformexec.StateResource) []terraformexec.StateResource {
		out := make([]terraformexec.StateResource, 0, len(list))
		for _, r := range list {
			r.Provider = "registry.terraform.io/hashicorp/aws"
			out = append(out, r)
		}
		return out
	}
	ctx, op := h.claim("proc-clean")
	out, err := h.svc.Destroy(ctx, tfexec.DestroyRequest{InstanceID: res.InstanceID, OrganizationKey: h.orgKey, RunID: "cleanup-" + op.ID})
	if err == nil || out.Destroyed || len(out.Remaining) != 2 {
		t.Fatalf("remaining objects: %+v %v", out, err)
	}
	inst, _ := h.instance(ctx, defaultDescriptor, defaultScope())
	attempts := h.attempts(inst.ID)
	last := attempts[len(attempts)-1]
	if inst.Status != tfresource.InstanceFailed || last.Status != tfresource.AttemptFailed || last.SafeErrorCode != "PHYSICAL_RESOURCE_REMAINS" || len(last.ObservedPhysicalIDs) != 2 || !strings.HasPrefix(last.ObservedPhysicalIDs[0], "terraform_data.") {
		t.Fatalf("remaining state %+v %+v", inst, last)
	}
}

// A killed destroy is an unknown outcome and holds.
func TestDestroyUnknownOutcomeHolds(t *testing.T) {
	h := newHarness(t)
	res := h.createInstance("run-1", defaultDescriptor, defaultScope(), nil)
	h.runner.onApply = func(int, terraformexec.Session) error {
		return &terraformexec.StepError{Step: "apply", Class: terraformexec.ClassTimeout, Outcome: terraformexec.OutcomeUnknown, Stopped: true}
	}
	ctx, op := h.claim("proc-clean")
	_, err := h.svc.Destroy(ctx, tfexec.DestroyRequest{InstanceID: res.InstanceID, OrganizationKey: h.orgKey, RunID: "cleanup-" + op.ID})
	if !errors.Is(err, tfexec.ErrNeedsAttention) {
		t.Fatalf("killed destroy: %v", err)
	}
	inst, _ := h.instance(ctx, defaultDescriptor, defaultScope())
	if inst.Status != tfresource.InstanceNeedsAttention {
		t.Fatalf("status %s", inst.Status)
	}
	if _, err := h.svc.Destroy(ctx, tfexec.DestroyRequest{InstanceID: res.InstanceID, OrganizationKey: h.orgKey, RunID: "cleanup-" + op.ID}); !errors.Is(err, tfexec.ErrInstanceHeld) {
		t.Fatalf("retry while held: %v", err)
	}
}

// A readiness failure leaves state that the independent cleanup can destroy.
func TestCleanupCanDestroyAfterReadinessFailure(t *testing.T) {
	h := newHarness(t)
	h.ready.err = terraformexec.ErrNotReady
	ctx, op := h.claim("proc-a")
	if _, err := h.svc.Execute(ctx, h.request("run-1", h.mod, inputs("db1", "k"))); !errors.Is(err, terraformexec.ErrNotReady) {
		t.Fatalf("readiness: %v", err)
	}
	h.release(op, environment.OpFailed)
	inst, _ := h.instance(ctx, defaultDescriptor, defaultScope())
	ctx2, op2 := h.claim("proc-clean")
	out, err := h.svc.Destroy(ctx2, tfexec.DestroyRequest{InstanceID: inst.ID, OrganizationKey: h.orgKey, RunID: "cleanup-" + op2.ID})
	if err != nil || !out.Destroyed {
		t.Fatalf("cleanup after readiness failure: %+v %v", out, err)
	}
}
