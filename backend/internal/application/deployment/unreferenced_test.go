package deployment_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	appsvc "orchestrator/internal/application/deployment"
	"orchestrator/internal/bootstrap"
	domain "orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/seed"
)

// hookDeployer applies successfully and lets a test fail or interleave with
// Remove, the runtime action of UC-07 VAR-02.
type hookDeployer struct {
	removeErr      error
	onRemove       func()
	calls, removes int
}

func (d *hookDeployer) Apply(context.Context, execution.Target, []execution.Manifest) error {
	d.calls++
	return nil
}
func (*hookDeployer) WaitReady(context.Context, execution.Target, []execution.WorkloadRef) error {
	return nil
}
func (d *hookDeployer) Remove(context.Context, execution.Target, string) error {
	d.calls++
	d.removes++
	if d.onRemove != nil {
		d.onRemove()
	}
	return d.removeErr
}

func uc07App(t *testing.T) (*bootstrap.App, seed.Options, *hookDeployer) {
	t.Helper()
	deployer := &hookDeployer{}
	app, opts := newApp(t, func(o *bootstrap.Options) { o.DeployerOverride = deployer })
	return app, opts, deployer
}

func uc07Deploy(t *testing.T, app *bootstrap.App, opts seed.Options, id string, before, after map[string]any) (*appsvc.DeployResult, error) {
	t.Helper()
	return app.Deployments.DeployWorkload(context.Background(), appsvc.DeployCommand{
		OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey,
		WorkloadID: id, ScoreBefore: before, ScoreAfter: after, Actor: "test",
	})
}

// sharedDB returns the Active Resource of the acceptance shared database.
func sharedDB(t *testing.T, st persistence.Store, opts seed.Options) resource.ActiveResource {
	t.Helper()
	active, err := st.ListActiveResources(context.Background(), opts.OrganizationKey)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range active {
		if a.Descriptor.Type == "postgres" && a.Scope.Type == resource.ScopeShared {
			return a
		}
	}
	t.Fatalf("no shared postgres Active Resource in %+v", active)
	return resource.ActiveResource{}
}

func TestRemoveWorkload_PreservesSharedDatabaseAndMarksLastReference(t *testing.T) {
	ctx := context.Background()
	app, opts, _ := uc07App(t)
	scores := seed.AcceptanceScores(opts)
	for _, id := range []string{"backend", "worker"} {
		if _, err := uc07Deploy(t, app, opts, id, nil, scores[id]); err != nil {
			t.Fatal(err)
		}
	}
	db := sharedDB(t, app.Store, opts)
	provisions := len(app.FakeExec.Calls)

	// Removing backend keeps the database: worker still references it.
	if _, err := uc07Deploy(t, app, opts, "backend", scores["backend"], nil); err != nil {
		t.Fatal(err)
	}
	// UC-08 may reconcile a still-desired resource (version bump), but it
	// stays the same READY resource.
	if got := sharedDB(t, app.Store, opts); got.Status != resource.StatusReady || got.ID != db.ID {
		t.Fatalf("still-shared database changed: %s %s", got.Status, got.ID)
	}
	db = sharedDB(t, app.Store, opts)
	provisions = len(app.FakeExec.Calls)
	env, _ := app.Store.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	set, _ := app.Store.GetDeploymentSet(ctx, env.CurrentDeploymentSetID)
	if _, ok := set.Document.Modules["backend"]; ok {
		t.Fatal("backend still in the current set")
	}
	if _, ok := set.Document.Modules["worker"]; !ok {
		t.Fatal("worker was not preserved")
	}

	// Removing the last consumer marks it UNREFERENCED without destroying it.
	result, err := uc07Deploy(t, app, opts, "worker", scores["worker"], nil)
	if err != nil {
		t.Fatal(err)
	}
	got := sharedDB(t, app.Store, opts)
	if got.Status != resource.StatusUnreferenced || got.LastDeploymentID != result.DeploymentID || got.Version != db.Version+1 {
		t.Fatalf("last reference not marked: %s last=%s v%d", got.Status, got.LastDeploymentID, got.Version)
	}
	if got.ID != db.ID || got.InputFingerprint != db.InputFingerprint || !reflect.DeepEqual(got.Outputs, db.Outputs) || !reflect.DeepEqual(got.ExecutorState, db.ExecutorState) {
		t.Fatal("marking changed identity, outputs, executor state or fingerprint")
	}
	for _, call := range app.FakeExec.Calls[provisions:] {
		if call.Descriptor == db.Descriptor.String() {
			t.Fatal("removal called the executor for the unreferenced database")
		}
	}

	// Referencing it again reuses the same resource and returns it to READY.
	if _, err := uc07Deploy(t, app, opts, "worker", nil, scores["worker"]); err != nil {
		t.Fatal(err)
	}
	if again := sharedDB(t, app.Store, opts); again.Status != resource.StatusReady || again.ID != db.ID {
		t.Fatalf("re-referenced database: %s %s", again.Status, again.ID)
	}
}

func TestUnreferencedMarking_StaysInOrganizationAndEnvironmentScope(t *testing.T) {
	ctx := context.Background()
	app, opts, _ := uc07App(t)
	backend := seed.AcceptanceScores(opts)["backend"]
	if _, err := uc07Deploy(t, app, opts, "backend", nil, backend); err != nil {
		t.Fatal(err)
	}
	db := sharedDB(t, app.Store, opts)
	others := []resource.ActiveResource{db, db, db}
	others[0].Scope = resource.Scope{Type: resource.ScopeShared, ID: opts.ApplicationKey + ".production"}
	others[1].Scope = resource.Scope{Type: resource.ScopeApplication, ID: opts.ApplicationKey}
	others[2].OrganizationKey = "globex"
	for i := range others {
		others[i].ID = ""
		saved, err := app.Store.UpsertActiveResource(ctx, others[i])
		if err != nil {
			t.Fatal(err)
		}
		others[i] = saved
	}
	if _, err := uc07Deploy(t, app, opts, "backend", backend, nil); err != nil {
		t.Fatal(err)
	}
	if got := sharedDB(t, app.Store, opts); got.Status != resource.StatusUnreferenced {
		t.Fatalf("own resource not marked: %s", got.Status)
	}
	for _, other := range others {
		got, err := app.Store.FindByLogicalIdentity(ctx, other.OrganizationKey, other.Descriptor, other.Scope)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != resource.StatusReady || got.Version != other.Version {
			t.Fatalf("resource outside the Environment scope changed: %+v %s", other.Scope, got.Status)
		}
	}
}

func TestUnreferencedMarking_RollsBackWithFailedRuntimeOrCommit(t *testing.T) {
	ctx := context.Background()
	for name, arrange := range map[string]func(*hookDeployer, *bootstrap.App, seed.Options){
		"runtime remove fails": func(d *hookDeployer, _ *bootstrap.App, _ seed.Options) {
			d.removeErr = errors.New("remove failed")
		},
		"final version conflict": func(d *hookDeployer, app *bootstrap.App, opts seed.Options) {
			d.onRemove = func() {
				env, _ := app.Store.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)
				env.Version++
				_ = app.Store.SaveEnvironment(ctx, env)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			app, opts, deployer := uc07App(t)
			backend := seed.AcceptanceScores(opts)["backend"]
			if _, err := uc07Deploy(t, app, opts, "backend", nil, backend); err != nil {
				t.Fatal(err)
			}
			db := sharedDB(t, app.Store, opts)
			env, _ := app.Store.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)
			arrange(deployer, app, opts)
			if _, err := uc07Deploy(t, app, opts, "backend", backend, nil); err == nil {
				t.Fatal("remove unexpectedly succeeded")
			}
			if got := sharedDB(t, app.Store, opts); got.Status != resource.StatusReady || got.Version != db.Version {
				t.Fatalf("marker committed without a successful remove: %s v%d", got.Status, got.Version)
			}
			after, _ := app.Store.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)
			if after.CurrentDeploymentSetID != env.CurrentDeploymentSetID {
				t.Fatal("current set moved")
			}
			list, _ := app.Store.ListDeployments(ctx, opts.ApplicationKey, opts.EnvironmentKey)
			if list[0].Status != domain.StatusFailed {
				t.Fatalf("latest Deployment = %s, want FAILED", list[0].Status)
			}
		})
	}
}

// TestDeploymentHistoryHidesUnsafeFailureReasons covers new failures (safe
// summary persisted) and a legacy raw reason (masked on read, not rewritten).
func TestDeploymentHistoryHidesUnsafeFailureReasons(t *testing.T) {
	ctx := context.Background()
	const sentinel = "sentinel-legacy-terraform-password"
	app, opts, deployer := uc07App(t)
	backend := seed.AcceptanceScores(opts)["backend"]
	if _, err := uc07Deploy(t, app, opts, "backend", nil, backend); err != nil {
		t.Fatal(err)
	}
	deployer.removeErr = errors.New("kubectl delete: " + sentinel)
	if _, err := uc07Deploy(t, app, opts, "backend", backend, nil); err == nil || !strings.Contains(err.Error(), sentinel) {
		t.Fatalf("typed cause must still reach the caller: %v", err)
	}
	list, _ := app.Store.ListDeployments(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	failed := list[0]
	if failed.FailureReason != appsvc.FailureRuntime {
		t.Fatalf("persisted reason = %q", failed.FailureReason)
	}
	legacy := failed
	legacy.ID = "00000000-0000-4000-8000-00000000c007"
	legacy.FailureReason = "terraform apply: " + sentinel
	legacy.DeltaSnapshotID, legacy.CandidateDeploymentSet = "", ""
	if err := app.Store.SaveDeployment(ctx, legacy); err != nil {
		t.Fatal(err)
	}
	scope := appsvc.ListDeploymentsQuery{OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey}
	history, err := app.Queries.ListDeployments(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	reasons := map[string]string{}
	for _, d := range history {
		reasons[d.ID] = d.FailureReason
	}
	if reasons[failed.ID] != appsvc.FailureRuntime || reasons[legacy.ID] != appsvc.FailureLegacy {
		t.Fatalf("history reasons = %v", reasons)
	}
	for _, id := range []string{failed.ID, legacy.ID} {
		view, err := app.Queries.GetDeployment(ctx, appsvc.GetDeploymentQuery{OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey, DeploymentID: id})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(view.Deployment.FailureReason, sentinel) || view.Deployment.FailureReason == "" {
			t.Fatalf("detail %s reason = %q", id, view.Deployment.FailureReason)
		}
	}
	if stored, _ := app.Store.GetDeployment(ctx, legacy.ID); !strings.Contains(stored.FailureReason, sentinel) {
		t.Fatal("legacy record was rewritten")
	}
}

func TestUpdateWorkload_PreservesOtherModules(t *testing.T) {
	ctx := context.Background()
	app, opts, _ := uc07App(t)
	scores := seed.AcceptanceScores(opts)
	for _, id := range seed.AcceptanceOrder() {
		if _, err := uc07Deploy(t, app, opts, id, nil, scores[id]); err != nil {
			t.Fatal(err)
		}
	}
	env, _ := app.Store.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	before, _ := app.Store.GetDeploymentSet(ctx, env.CurrentDeploymentSetID)
	db := sharedDB(t, app.Store, opts)
	updated := seed.AcceptanceScores(opts)["frontend"]
	updated["containers"].(map[string]any)["main"].(map[string]any)["image"] = "example.invalid/frontend:v2"
	result, err := uc07Deploy(t, app, opts, "frontend", scores["frontend"], updated)
	if err != nil {
		t.Fatal(err)
	}
	env, _ = app.Store.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	after, _ := app.Store.GetDeploymentSet(ctx, env.CurrentDeploymentSetID)
	for id, module := range before.Document.Modules {
		if id != "frontend" && !reflect.DeepEqual(module, after.Document.Modules[id]) {
			t.Fatalf("module %s changed by the frontend update", id)
		}
	}
	if !reflect.DeepEqual(before.Document.Shared, after.Document.Shared) || sharedDB(t, app.Store, opts).Status != db.Status {
		t.Fatal("shared contribution or database changed")
	}
	record, _ := app.Store.GetDeployment(ctx, result.DeploymentID)
	snapshot, _ := app.Store.GetDeltaSnapshot(ctx, record.DeltaSnapshotID)
	if m := snapshot.Document.Modules; m == nil || len(m.Add) != 0 || len(m.Remove) != 0 || len(m.Update) != 1 || m.Update["frontend"] == nil || len(snapshot.Document.Shared) != 0 {
		t.Fatalf("Delta is not scoped to frontend: %+v", snapshot.Document)
	}
}

func TestUpdateRemove_ConflictsFailBeforeSideEffects(t *testing.T) {
	ctx := context.Background()
	app, opts, deployer := uc07App(t)
	scores := seed.AcceptanceScores(opts)
	if _, err := uc07Deploy(t, app, opts, "backend", nil, scores["backend"]); err != nil {
		t.Fatal(err)
	}
	env, _ := app.Store.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	provisions, applied := len(app.FakeExec.Calls), deployer.calls
	stale := seed.AcceptanceScores(opts)["backend"]
	stale["containers"].(map[string]any)["main"].(map[string]any)["image"] = "example.invalid/other:v0"
	conflict := seed.AcceptanceScores(opts)["worker"]
	conflict["resources"].(map[string]any)["db"].(map[string]any)["params"].(map[string]any)["database"] = "other"
	for name, cmd := range map[string][2]map[string]any{
		"stale before remove": {stale, nil},
		"stale before update": {stale, scores["backend"]},
	} {
		if _, err := uc07Deploy(t, app, opts, "backend", cmd[0], cmd[1]); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
	if _, err := uc07Deploy(t, app, opts, "worker", nil, conflict); err == nil {
		t.Fatal("shared conflict accepted")
	}
	after, _ := app.Store.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if len(app.FakeExec.Calls) != provisions || deployer.calls != applied || after.CurrentDeploymentSetID != env.CurrentDeploymentSetID || after.Version != env.Version {
		t.Fatal("rejected update/remove caused a side effect")
	}
	list, _ := app.Store.ListDeployments(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	for _, d := range list[:3] {
		if d.Status != domain.StatusFailed || d.DeltaSnapshotID != "" {
			t.Fatalf("rejected plan left %s with snapshot %q", d.Status, d.DeltaSnapshotID)
		}
	}
}
