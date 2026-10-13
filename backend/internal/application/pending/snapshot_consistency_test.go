package pending_test

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"orchestrator/internal/adapters/postgres/postgrestest"
	"orchestrator/internal/application/pending"
	"orchestrator/internal/application/workloadconfig"
	"orchestrator/internal/bootstrap"
	appdomain "orchestrator/internal/domain/application"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/seed"
)

// snapshotOnly is a Store whose only usable entry point is ReadSnapshot: any read
// issued on it directly (outside the snapshot view) dereferences the nil embedded
// Store and panics, which the test reports as a read outside the snapshot.
type snapshotOnly struct {
	persistence.Store
	real       persistence.Store
	afterFirst func()
	planReads  *int
}

func (s snapshotOnly) ReadSnapshot(ctx context.Context, fn func(context.Context, persistence.Store) error) error {
	return s.real.ReadSnapshot(ctx, func(ctx context.Context, view persistence.Store) error {
		return fn(ctx, &countingView{Store: view, afterFirst: s.afterFirst, planReads: s.planReads})
	})
}

type countingView struct {
	persistence.Store
	afterFirst func()
	planReads  *int
}

func (v *countingView) GetApplication(ctx context.Context, key string) (a appdomain.Application, err error) {
	a, err = v.Store.GetApplication(ctx, key)
	if v.afterFirst != nil {
		fire := v.afterFirst
		v.afterFirst = nil
		fire() // concurrent committed writers, after the snapshot is fixed
	}
	return a, err
}

func (v *countingView) GetPlan(ctx context.Context, id string) (map[string]any, error) {
	*v.planReads++
	return v.Store.GetPlan(ctx, id)
}

// UC-05 BR-01 / OC-06 for the pending Preview: every planning input and every
// baseline plan is read through ONE snapshot view, nothing from the outer store,
// and writers that commit after the snapshot cannot mix into the result or the
// Preview token.
func assertPendingPreviewIsOneSnapshot(t *testing.T, app *bootstrap.App, opts seed.Options, bundles map[string]resource.RenderBundle) {
	t.Helper()
	ctx := context.Background()
	registerRenderer(t, app, bundles, opts, "render", "internal-k8s")
	if _, err := app.Workloads.Save(ctx, opts.ApplicationKey, opts.EnvironmentKey, "api", withDatabase("api"), 0); err != nil {
		t.Fatal(err)
	}
	first, err := app.Pending.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}
	if report, err := app.Pending.Deploy(ctx, opts.ApplicationKey, opts.EnvironmentKey, "test", first.Token); err != nil || report.Status != "SUCCEEDED" {
		t.Fatalf("deploy: %+v %v", report, err)
	}
	draftVersion := func() int64 {
		env, err := app.Store.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)
		if err != nil {
			t.Fatal(err)
		}
		return env.DraftVersion
	}
	changed := withDatabase("api")
	changed["containers"].(map[string]any)["main"].(map[string]any)["image"] = "api:v2"
	if _, err := app.Workloads.Save(ctx, opts.ApplicationKey, opts.EnvironmentKey, "api", changed, draftVersion()); err != nil {
		t.Fatal(err)
	}
	reference, err := app.Pending.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil || len(reference.Changes) != 1 || len(reference.Changes[0].ResourceIntents) == 0 {
		t.Fatalf("reference preview: %+v %v", reference, err)
	}

	planReads := 0
	store := snapshotOnly{real: app.Store, planReads: &planReads}
	store.afterFirst = func() {
		// Committed concurrently: a newer draft, overwritten baseline plans and Active
		// Resources pointed at a deployment without a plan.
		other := withDatabase("api")
		other["containers"].(map[string]any)["main"].(map[string]any)["image"] = "api:v3"
		if _, err := app.Workloads.Save(ctx, opts.ApplicationKey, opts.EnvironmentKey, "api", other, draftVersion()); err != nil {
			t.Error(err)
		}
		active, _ := app.Store.ListActiveResources(ctx, opts.OrganizationKey)
		for _, a := range active {
			if a.LastDeploymentID != "" {
				_ = app.Store.SavePlan(ctx, a.LastDeploymentID, map[string]any{"graph": "garbage"})
			}
			moved := a
			moved.LastDeploymentID = "concurrent-deployment"
			moved.Version++
			_, _ = app.Store.UpsertActiveResource(ctx, moved)
		}
	}
	service := pending.NewService(store, planning.NewRequiredRendererService(bundles), workloadconfig.NewService(app.Store), nil)
	var got pending.Preview
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				t.Fatalf("a read was issued outside the snapshot view: %v", recovered)
			}
		}()
		got, err = service.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	}()
	if err != nil {
		t.Fatal(err)
	}
	if planReads == 0 {
		t.Fatal("the baseline plans must be read inside the snapshot")
	}
	if got.Token != reference.Token || got.DraftVersion != reference.DraftVersion {
		t.Fatalf("the Preview mixed snapshots: token %s draft %d, want %s draft %d", got.Token, got.DraftVersion, reference.Token, reference.DraftVersion)
	}
	if !reflect.DeepEqual(got.Changes[0].ResourceIntents, reference.Changes[0].ResourceIntents) {
		t.Fatalf("intents changed with concurrent writes:\n%v\n%v", got.Changes[0].ResourceIntents, reference.Changes[0].ResourceIntents)
	}
	// A later Preview sees the newer committed state: the pinned versions moved on.
	later, err := app.Pending.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}
	if later.DraftVersion == reference.DraftVersion || later.Token == reference.Token {
		t.Fatalf("the concurrent write must be visible to the next Preview (draft %d vs %d)", later.DraftVersion, reference.DraftVersion)
	}
	_ = fmt.Sprint
}

func TestPendingPreviewReadsOneSnapshot(t *testing.T) {
	app, opts, _, bundles := requiredAppBundles(t, true)
	assertPendingPreviewIsOneSnapshot(t, app, opts, bundles)
}

// The same, over real PostgreSQL REPEATABLE READ snapshots.
func TestPendingPreviewReadsOneSnapshotOnPostgres(t *testing.T) {
	app, opts, _, bundles := requiredAppBundlesOn(t, true, postgrestest.Fresh(t).Console)
	assertPendingPreviewIsOneSnapshot(t, app, opts, bundles)
}
