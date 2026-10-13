package tfexec_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"orchestrator/internal/application/tfexec"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/terraformexec"
	"orchestrator/internal/ports/terraformstate"
)

// ---- namespace content inspection (FAKE inspector, real destroy path) ----

// asNamespace makes the FAKE state resources look like a namespace with an own
// Secret, so the namespace content policy runs on the real destroy path.
func asNamespace(h *harness) {
	h.runner.onResources = func(list []terraformexec.StateResource) []terraformexec.StateResource {
		out := make([]terraformexec.StateResource, 0, len(list))
		for _, r := range list {
			switch r.Address {
			case "terraform_data.db":
				r.Type, r.Provider, r.ID, r.UID = "kubernetes_namespace_v1", "registry.terraform.io/hashicorp/kubernetes", "orc-ns", "uid-ns"
			case "terraform_data.pw":
				r.Type, r.Provider, r.ID, r.UID = "kubernetes_secret_v1", "registry.terraform.io/hashicorp/kubernetes", "orc-ns/pw", "uid-pw"
			}
			out = append(out, r)
		}
		return out
	}
}

func TestNamespaceDestroyRefusesForeignContentsBeforeAnyProviderCall(t *testing.T) {
	h := newHarness(t)
	res := h.createInstance("run-1", defaultDescriptor, defaultScope(), nil)
	asNamespace(h)
	appliesBefore := h.runner.applies
	h.inspector.objects = []terraformexec.NamespaceObject{
		{Kind: "Secret", Name: "pw", UID: "uid-pw", DataKeys: []string{"v"}}, // own (state, exact UID)
		{Kind: "Pod", Name: "stranger", UID: "u1"},                           // foreign, not in the graph
		{Kind: "PersistentVolumeClaim", Name: "customer-data", UID: "u2"},
		{Kind: "Secret", Name: "token", UID: "u3"},
		{Kind: "Endpoints", Name: "custom", UID: "u4", Managers: []string{"kubectl-create"}}, // standalone foreign Endpoints
	}
	ctx, op := h.claim("proc-clean")
	out, err := h.svc.Destroy(ctx, tfexec.DestroyRequest{InstanceID: res.InstanceID, OrganizationKey: h.orgKey, RunID: "cleanup-" + op.ID})
	if !errors.Is(err, tfexec.ErrNamespaceContents) || out.Destroyed {
		t.Fatalf("foreign contents: %+v %v", out, err)
	}
	for _, id := range []string{"Pod/stranger", "PersistentVolumeClaim/customer-data", "Secret/token", "Endpoints/custom"} {
		if !strings.Contains(err.Error(), id) {
			t.Errorf("foreign object %s not named in %v", id, err)
		}
	}
	if strings.Contains(err.Error(), "Secret/pw") {
		t.Fatal("an own object was reported foreign")
	}
	// Nothing was destroyed: no apply, instance and state intact, IDs reported.
	if h.runner.applies != appliesBefore || h.status(res.InstanceID) != tfresource.InstanceReady {
		t.Fatalf("a refused namespace destroy changed things: applies %d status %s", h.runner.applies, h.status(res.InstanceID))
	}
	if len(out.Remaining) == 0 || h.count(`SELECT count(*) FROM terraform_states WHERE resource_instance_id=$1::uuid`, res.InstanceID) != 1 {
		t.Fatalf("remaining %v", out.Remaining)
	}
	attempts := h.attempts(res.InstanceID)
	if last := attempts[len(attempts)-1]; last.Status != tfresource.AttemptFailed || last.Action != tfresource.ActionDestroy {
		t.Fatalf("attempt %+v", last)
	}
	if len(h.inspector.calls) != 1 || h.inspector.calls[0] != "orc-ns" {
		t.Fatalf("inspector calls %v", h.inspector.calls)
	}
}

func TestNamespaceDestroyProceedsOnlyForOwnAndProvenDefaults(t *testing.T) {
	h := newHarness(t)
	res := h.createInstance("run-1", defaultDescriptor, defaultScope(), nil)
	asNamespace(h)
	h.inspector.objects = []terraformexec.NamespaceObject{
		{Kind: "ConfigMap", Name: "kube-root-ca.crt", UID: "u1", DataKeys: []string{"ca.crt"}, Managers: []string{"kube-controller-manager"}},
		{Kind: "ServiceAccount", Name: "default", UID: "u2", Managers: []string{"kube-controller-manager"}},
		{Kind: "Secret", Name: "pw", UID: "uid-pw"},
	}
	// FAKE provider: every object of the state is gone after the destroy.
	h.runner.onPlan = func(plan terraformexec.Plan, mode terraformexec.PlanMode) (terraformexec.Plan, error) {
		if mode == terraformexec.PlanRefreshOnly {
			plan.Drift = []string{"terraform_data.db", "terraform_data.pw"}
		}
		return plan, nil
	}
	ctx, op := h.claim("proc-clean")
	out, err := h.svc.Destroy(ctx, tfexec.DestroyRequest{InstanceID: res.InstanceID, OrganizationKey: h.orgKey, RunID: "cleanup-" + op.ID})
	if err != nil || !out.Destroyed {
		t.Fatalf("valid contents: %+v %v", out, err)
	}
}

func TestNamespaceDestroyFailsClosedWithoutAnInspectorOrOnInspectionError(t *testing.T) {
	h := newHarness(t)
	res := h.createInstance("run-1", defaultDescriptor, defaultScope(), nil)
	asNamespace(h)
	h.inspector.err = errors.New("api down")
	ctx, op := h.claim("proc-clean")
	if _, err := h.svc.Destroy(ctx, tfexec.DestroyRequest{InstanceID: res.InstanceID, OrganizationKey: h.orgKey, RunID: "cleanup-" + op.ID}); !errors.Is(err, tfexec.ErrNamespaceContents) {
		t.Fatalf("unreadable contents: %v", err)
	}
	// A look-alike default is foreign: provenance, not name, is the proof.
	h.inspector.err = nil
	h.inspector.objects = []terraformexec.NamespaceObject{{Kind: "ConfigMap", Name: "kube-root-ca.crt", UID: "u1", DataKeys: []string{"ca.crt"}, Managers: []string{"kubectl-create"}}}
	if _, err := h.svc.Destroy(ctx, tfexec.DestroyRequest{InstanceID: res.InstanceID, OrganizationKey: h.orgKey, RunID: "cleanup-" + op.ID}); !errors.Is(err, tfexec.ErrNamespaceContents) {
		t.Fatalf("look-alike default: %v", err)
	}
	// A namespace replaced behind the same name (another UID) is refused.
	h.inspector.objects = nil
	h.inspector.uid = "uid-replaced"
	if _, err := h.svc.Destroy(ctx, tfexec.DestroyRequest{InstanceID: res.InstanceID, OrganizationKey: h.orgKey, RunID: "cleanup-" + op.ID}); !errors.Is(err, tfexec.ErrNamespaceContents) {
		t.Fatalf("replaced namespace: %v", err)
	}
	h.inspector.uid = ""
	if h.status(res.InstanceID) != tfresource.InstanceReady {
		t.Fatal("a refused destroy changed the instance status")
	}
	// No inspector configured at all refuses too.
	noInspector, err := tfexec.New(tfexec.Config{BackendBase: h.base, InvocationTimeout: 2 * 60 * 1e9, ReadinessTimeout: 60 * 1e9, RetryBackoff: 1e6},
		tfexec.Deps{State: h.stateRepo, Tokens: h.backend, Modules: &fixtureModules{h: h, bundles: map[string]module{h.mod.bundle.ID: h.mod}}, Runner: h.runner, Binder: h.binder, Readiness: h.ready, Recovery: h.recover, Verification: h.recover, Secrets: h.secrets})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := noInspector.Destroy(ctx, tfexec.DestroyRequest{InstanceID: res.InstanceID, OrganizationKey: h.orgKey, RunID: "cleanup-" + op.ID}); !errors.Is(err, tfexec.ErrNamespaceContents) {
		t.Fatalf("no inspector: %v", err)
	}
}

// ---- run cleanup vs another run's partial UPDATE ----

func TestRunCleanupRejectsAnotherRunsFailedPartialUpdate(t *testing.T) {
	h := newHarness(t)
	a := h.createInstance("run-1", defaultDescriptor, defaultScope(), nil)
	// Another run's UPDATE reaches APPLYING and fails: state or data may have changed.
	// The update changed the object and the backend acknowledged the state
	// before the CLI exited non-zero: a known, retained failure.
	h.runner.onApply = func(_ int, real terraformexec.Session) error {
		_ = real.Apply(context.Background())
		return &terraformexec.StepError{Step: "apply", Class: terraformexec.ClassFailed, Outcome: terraformexec.OutcomeConfirmed, Stopped: true}
	}
	ctx, op := h.claim("proc-upd")
	if _, err := h.svc.Execute(ctx, h.request("run-2", h.mod, inputs("db-changed", "k"))); err == nil {
		t.Fatal("the update was expected to fail")
	}
	h.release(op, environment.OpFailed)
	h.runner.onApply = nil
	cctx, cop, err := h.admit(tfresource.CleanupRunCleanup, a.InstanceID, []string{a.InstanceID}, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	opens := h.runner.opens
	if _, err := h.svc.RunCleanup(cctx, h.orgKey, h.store, nil); !errors.Is(err, tfexec.ErrCleanupRejected) || h.runner.opens != opens {
		t.Fatalf("partial update by another run: %v", err)
	}
	h.release(cop, environment.OpFailed)
	if h.status(a.InstanceID) == tfresource.InstanceDestroyed {
		t.Fatal("the instance was destroyed")
	}
}

// ---- exact canonical cleanup conditions ----

func TestUserCleanupRootMustBeUnreferencedAndSelectionOneGeneration(t *testing.T) {
	h := newHarness(t)
	topo := h.chain()
	// FAILED and PENDING roots are not admissible roots of a user deletion.
	for _, status := range []tfresource.InstanceStatus{tfresource.InstanceFailed, tfresource.InstancePending} {
		ctx, op := h.claim("proc-mark")
		inst, _ := h.state.GetInstance(ctx, topo.db.InstanceID)
		if _, err := h.state.SetInstanceStatus(ctx, topo.db.InstanceID, inst.Version, status); err != nil {
			t.Fatal(err)
		}
		h.release(op, environment.OpSucceeded)
		cctx, cop, err := h.admit(tfresource.CleanupUserConfirmed, topo.db.InstanceID, []string{topo.db.InstanceID}, "")
		if err != nil {
			t.Fatal(err)
		}
		opens := h.runner.opens
		if _, err := h.svc.RunCleanup(cctx, h.orgKey, h.store, nil); !errors.Is(err, tfexec.ErrCleanupRejected) || h.runner.opens != opens {
			t.Fatalf("%s root: %v", status, err)
		}
		h.release(cop, environment.OpFailed)
	}
	// Mixed target generations are rejected with no effect.
	h.markUnreferenced(topo.db.InstanceID)
	h.exec(`UPDATE environments SET generation_high=1 WHERE id=$1::uuid`, h.envID)
	ctx, op := h.claim("proc-gen")
	req := h.request("run-1", h.mod, inputs("db-g1", "k"))
	req.Generation = 1
	req.Descriptor, req.Scope = "postgres.default#modules.api.externals.g1", scopeOf("g1")
	req.Identity = h.identity(req.Descriptor, req.Scope)
	req.Identity.TargetGeneration = 1
	other, err := h.svc.Execute(ctx, req)
	if err != nil {
		t.Fatalf("generation 1 create: %v", err)
	}
	h.release(op, environment.OpSucceeded)
	cctx, cop, err := h.admit(tfresource.CleanupUserConfirmed, topo.db.InstanceID, []string{topo.db.InstanceID, other.InstanceID}, "")
	if err != nil {
		t.Fatal(err)
	}
	opens := h.runner.opens
	if _, err := h.svc.RunCleanup(cctx, h.orgKey, h.store, nil); !errors.Is(err, tfexec.ErrCleanupRejected) || h.runner.opens != opens {
		t.Fatalf("mixed generations: %v", err)
	}
	h.release(cop, environment.OpFailed)
}

// ---- remaining IDs ----

func TestRemainingIDsCoverFailedRetainedAndUnattemptedBranches(t *testing.T) {
	h := newHarness(t)
	topo := h.chain()
	h.markUnreferenced(topo.db.InstanceID)
	consumers := []string{topo.db.InstanceID, topo.cache.InstanceID}
	sortStrings(consumers)
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
	if err != nil || res.Terminal != tfexec.CleanupFailed {
		t.Fatalf("cleanup %+v %v", res, err)
	}
	by := map[string]tfexec.InstanceResult{}
	for _, r := range res.Instances {
		by[r.InstanceID] = r
	}
	// Failed consumer: its pre-destroy objects. Retained provider: its last
	// acknowledged observations. Independent success: nothing remains.
	if f := by[topo.db.InstanceID]; f.Destroyed || len(f.Remaining) == 0 || !strings.HasPrefix(f.Remaining[0], "terraform_data.") {
		t.Fatalf("failed consumer %+v", f)
	}
	if r := by[topo.ns.InstanceID]; r.Destroyed || len(r.Remaining) == 0 || r.RemainingUnknown {
		t.Fatalf("retained provider %+v", r)
	}
	if ok := by[topo.cache.InstanceID]; !ok.Destroyed || len(ok.Remaining) != 0 {
		t.Fatalf("independent success %+v", ok)
	}
	if got := res.RemainingIDs(); len(got) != len(by[topo.db.InstanceID].Remaining)+len(by[topo.ns.InstanceID].Remaining) {
		t.Fatalf("operation remaining IDs %v", got)
	}
	// The pre-destroy IDs were audited BEFORE the failed apply.
	attempts := h.attempts(topo.db.InstanceID)
	if last := attempts[len(attempts)-1]; last.Action != tfresource.ActionDestroy || len(last.ObservedPhysicalIDs) == 0 {
		t.Fatalf("destroy attempt did not audit the pre-destroy IDs: %+v", last)
	}
}

func TestBindingFailureStillReportsLastAcknowledgedIDs(t *testing.T) {
	h := newHarness(t)
	res := h.createInstance("run-1", defaultDescriptor, defaultScope(), nil)
	h.binder.err = errors.New("credential unavailable")
	ctx, op := h.claim("proc-clean")
	out, err := h.svc.Destroy(ctx, tfexec.DestroyRequest{InstanceID: res.InstanceID, OrganizationKey: h.orgKey, RunID: "cleanup-" + op.ID})
	if !errors.Is(err, tfexec.ErrBinding) || out.Destroyed || len(out.Remaining) == 0 || out.RemainingUnknown {
		t.Fatalf("binding failure: %+v %v", out, err)
	}
}

func TestEmptyRemainingIsNeverReportedAsAbsence(t *testing.T) {
	h := newHarness(t)
	res := h.createInstance("run-1", defaultDescriptor, defaultScope(), nil)
	h.exec(`UPDATE terraform_resource_attempts SET observed_physical_ids='[]'::jsonb WHERE resource_instance_id=$1::uuid`, res.InstanceID)
	h.binder.err = errors.New("credential unavailable")
	ctx, op := h.claim("proc-clean")
	out, _ := h.svc.Destroy(ctx, tfexec.DestroyRequest{InstanceID: res.InstanceID, OrganizationKey: h.orgKey, RunID: "cleanup-" + op.ID})
	if out.Destroyed || len(out.Remaining) != 0 || !out.RemainingUnknown {
		t.Fatalf("unknown remaining must be flagged: %+v", out)
	}
}

// ---- storage failures never lose the hold ----

// faultyRepo wraps the real repository and fails selected writes.
type faultyRepo struct {
	terraformstate.Repository
	failAttempt func(p tfresource.AttemptProgress) bool
	failStatus  func(s tfresource.InstanceStatus) bool
}

func (f *faultyRepo) UpdateAttempt(ctx context.Context, id string, p tfresource.AttemptProgress) (tfresource.Attempt, error) {
	if f.failAttempt != nil && f.failAttempt(p) {
		return tfresource.Attempt{}, errors.New("storage unavailable")
	}
	return f.Repository.UpdateAttempt(ctx, id, p)
}

func (f *faultyRepo) SetInstanceStatus(ctx context.Context, id string, v int64, s tfresource.InstanceStatus) (tfresource.Instance, error) {
	if f.failStatus != nil && f.failStatus(s) {
		return tfresource.Instance{}, errors.New("storage unavailable")
	}
	return f.Repository.SetInstanceStatus(ctx, id, v, s)
}

func TestTerminalRecordFailuresHoldTheEnvironment(t *testing.T) {
	type scenario struct {
		name   string
		faulty *faultyRepo
		// prepare runs before the faulty execution.
		prepare func(h *harness)
		input   map[string]any
	}
	failedWrite := func(p tfresource.AttemptProgress) bool { return p.Status == tfresource.AttemptFailed }
	for _, sc := range []scenario{
		{name: "success record", faulty: &faultyRepo{failAttempt: func(p tfresource.AttemptProgress) bool { return p.Status == tfresource.AttemptSucceeded }}, input: inputs("db1", "k")},
		{name: "READY status", faulty: &faultyRepo{failStatus: func(s tfresource.InstanceStatus) bool { return s == tfresource.InstanceReady }}, input: inputs("db1", "k")},
		{name: "failed record after a rejected plan", faulty: &faultyRepo{failAttempt: failedWrite}, input: inputs("db1", "changed-trigger"),
			prepare: func(h *harness) {
				ctx, op := h.claim("proc-a")
				mustExecute(t, h, ctx, h.request("run-1", h.mod, inputs("db1", "k")))
				h.release(op, environment.OpSucceeded)
			}},
		{name: "readiness failure record", faulty: &faultyRepo{failAttempt: failedWrite}, input: inputs("db1", "k"),
			prepare: func(h *harness) { h.ready.err = terraformexec.ErrNotReady }},
	} {
		h := newHarness(t)
		if sc.prepare != nil {
			sc.prepare(h)
		}
		sc.faulty.Repository = h.state
		h.stateRepo = sc.faulty
		svc := h.newService(nil)
		ctx, op := h.claimHeld("proc-b")
		_, err := svc.Execute(ctx, h.request("run-2", h.mod, sc.input))
		if !errors.Is(err, tfexec.ErrNeedsAttention) || !persistence.Held(ctx) {
			t.Fatalf("%s: %v held=%v", sc.name, err, persistence.Held(ctx))
		}
		// With healthy storage the next execution cannot start: the attempt
		// or the instance is not in a state that allows another CREATE/UPDATE.
		inst := mustFind(t, h)
		attemptsBefore := len(h.attempts(inst.ID))
		h.release(op, environment.OpFailed)
		h.stateRepo = h.state
		ctx2, _ := h.claim("proc-c")
		opens := h.runner.opens
		if sc.name == "success record" || sc.name == "READY status" {
			// Apply succeeded and state is acknowledged but the terminal record
			// is missing: the attempt is still RUNNING (or the status stale).
			if _, err := h.newService(nil).Execute(ctx2, h.request("run-3", h.mod, inputs("db1", "k"))); !errors.Is(err, tfexec.ErrInstanceHeld) {
				t.Fatalf("%s: next execution: %v", sc.name, err)
			}
			if h.runner.opens != opens || len(h.attempts(inst.ID)) != attemptsBefore {
				t.Fatalf("%s: a next execution started", sc.name)
			}
		}
	}
}

func mustFind(t *testing.T, h *harness) tfresource.Instance {
	t.Helper()
	inst, ok := h.instance(context.Background(), defaultDescriptor, defaultScope())
	if !ok {
		t.Fatal("no instance")
	}
	return inst
}

func TestDestroyedRecordFailureHoldsTheEnvironment(t *testing.T) {
	h := newHarness(t)
	res := h.createInstance("run-1", defaultDescriptor, defaultScope(), nil)
	h.stateRepo = &faultyRepo{Repository: h.state, failStatus: func(s tfresource.InstanceStatus) bool { return s == tfresource.InstanceDestroyed }}
	svc := h.newService(nil)
	ctx, op := h.claimHeld("proc-clean")
	_, err := svc.Destroy(ctx, tfexec.DestroyRequest{InstanceID: res.InstanceID, OrganizationKey: h.orgKey, RunID: "cleanup-" + op.ID})
	if !errors.Is(err, tfexec.ErrNeedsAttention) || !persistence.Held(ctx) {
		t.Fatalf("unconfirmed destroyed record: %v held=%v", err, persistence.Held(ctx))
	}
}

func sortStrings(in []string) {
	for i := 1; i < len(in); i++ {
		for j := i; j > 0 && in[j] < in[j-1]; j-- {
			in[j], in[j-1] = in[j-1], in[j]
		}
	}
}

// ---- retained workspaces get unique, resolvable, opaque handles ----

func TestRetainedWorkspacesHaveUniqueOpaqueHandlesWithPrivateMapping(t *testing.T) {
	h := newHarness(t)
	h.recover.fail = true // the encrypted recovery store is unavailable
	type held struct {
		inst   tfresource.Instance
		handle string
	}
	var results []held
	for i, name := range []string{"a", "b"} {
		descriptor := "postgres.default#modules.api.externals." + name
		scope := scopeOf(name)
		h.runner.onApply = func(_ int, real terraformexec.Session) error {
			// Terraform kept the state locally because the backend refused it.
			art := h.runner.lastArtifact
			if err := os.WriteFile(filepath.Join(art.ModuleDir, "errored.tfstate"), []byte(`{"version":4,"serial":`+strconv.Itoa(i+7)+`,"lineage":"l","resources":[]}`), 0o600); err != nil {
				return err
			}
			return &terraformexec.StepError{Step: "apply", Class: terraformexec.ClassBackend, Outcome: terraformexec.OutcomeConfirmed, Stopped: true}
		}
		ctx, op := h.claim("proc-" + name)
		req := h.request("run-"+name, h.mod, inputs("db-"+name, "k"))
		req.Descriptor, req.Scope, req.Identity = descriptor, scope, h.identity(descriptor, scope)
		if _, err := h.svc.Execute(ctx, req); !errors.Is(err, tfexec.ErrNeedsAttention) {
			t.Fatalf("%s: %v", name, err)
		}
		inst, _ := h.instance(ctx, descriptor, scope)
		attempts := h.attempts(inst.ID)
		results = append(results, held{inst: inst, handle: attempts[0].RecoveryHandle})
		h.release(op, environment.OpFailed)
	}
	if results[0].handle == results[1].handle || !strings.HasPrefix(results[0].handle, "retained-") || !strings.HasPrefix(results[1].handle, "retained-") {
		t.Fatalf("handles %q %q", results[0].handle, results[1].handle)
	}
	for i, r := range results {
		if strings.ContainsAny(r.handle, "/\\") || strings.Contains(r.handle, os.TempDir()) {
			t.Fatalf("a path entered the audit handle: %q", r.handle)
		}
		// The private mapping resolves the handle; the sole state copy is there.
		mapping, err := os.ReadFile(filepath.Join(filepath.Dir(rootOf(t, h, r.handle)), ".retained-"+r.handle))
		if err != nil {
			t.Fatalf("no private mapping for %s: %v", r.handle, err)
		}
		state, err := os.ReadFile(filepath.Join(string(mapping), "sources", "test@rev1", "root", "errored.tfstate"))
		if err != nil || !strings.Contains(string(state), `"serial":`+strconv.Itoa(i+7)) {
			t.Fatalf("retained state for %s: %v", r.handle, err)
		}
		// No public metadata carries a path or the state.
		if h.count(`SELECT count(*) FROM terraform_resource_attempts WHERE resource_instance_id=$1::uuid AND (position('/' in coalesce(recovery_handle,''))>0 OR position('errored' in module_snapshot::text||target_snapshot::text)>0)`, r.inst.ID) != 0 {
			t.Fatal("a path or state reached public attempt metadata")
		}
	}
	if len(h.binder.retained) != 2 || h.binder.retained[0] == h.binder.retained[1] {
		t.Fatalf("provider credential directories were not retained under unique handles: %v", h.binder.retained)
	}
	if h.binder.closed != 0 {
		t.Fatalf("provider credentials were released while the state is retained: %d", h.binder.closed)
	}
}

// rootOf finds the retained root through the harness' mapping directory.
func rootOf(t *testing.T, h *harness, handle string) string {
	t.Helper()
	if h.runner.lastArtifact == nil {
		t.Fatal("no artifact")
	}
	// The mapping sits beside the artifact root of the SAME harness (all
	// fixture roots share one parent), so any root's parent locates it.
	return h.runner.lastArtifact.Root
}

// An unconfirmed process keeps its workspace and provider credentials under
// the same kind of unique handle, and releases nothing.
func TestUnconfirmedProcessRetainsWorkspaceAndCredentials(t *testing.T) {
	h := newHarness(t)
	h.runner.onApply = func(int, terraformexec.Session) error {
		return &terraformexec.StepError{Step: "apply", Class: terraformexec.ClassFailed, Outcome: terraformexec.OutcomeUnknown, Stopped: false}
	}
	ctx, _ := h.claimHeld("proc-a")
	if _, err := h.svc.Execute(ctx, h.request("run-1", h.mod, inputs("db1", "k"))); !errors.Is(err, tfexec.ErrNeedsAttention) {
		t.Fatalf("unconfirmed stop: %v", err)
	}
	inst, _ := h.instance(ctx, defaultDescriptor, defaultScope())
	attempts := h.attempts(inst.ID)
	handle := attempts[0].RecoveryHandle
	if handle != "retained-"+attempts[0].ID || len(h.binder.retained) != 1 || h.binder.retained[0] != handle || h.binder.closed != 0 {
		t.Fatalf("handle %q retained %v closed %d", handle, h.binder.retained, h.binder.closed)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(h.runner.lastArtifact.Root), ".retained-"+handle)); err != nil {
		t.Fatalf("no private mapping: %v", err)
	}
	if _, err := os.Stat(h.runner.lastArtifact.Root); err != nil {
		t.Fatalf("the workspace was removed: %v", err)
	}
}
