package pending_test

import (
	"context"
	"errors"
	"orchestrator/internal/adapters/postgres"
	"orchestrator/internal/adapters/postgres/postgrestest"
	"orchestrator/internal/domain/configuration"
	domain "orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
	"testing"
	"time"

	"orchestrator/internal/application/pending"
	"orchestrator/internal/application/workloadconfig"
	"orchestrator/internal/bootstrap"
	"orchestrator/internal/bootstrap/bootstraptest"
	"orchestrator/internal/planning"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/seed/seedtest"
)

type releaseTestDeployer struct {
	calls        int
	failRollback bool
	removed      []string
}

func (d *releaseTestDeployer) Apply(context.Context, execution.Target, []execution.Manifest) error {
	d.calls++
	if d.calls == 4 || (d.failRollback && d.calls == 6) {
		return errors.New("confirmed test failure")
	}
	return nil
}
func (*releaseTestDeployer) WaitReady(context.Context, execution.Target, []execution.WorkloadRef) error {
	return nil
}
func (*releaseTestDeployer) Remove(context.Context, execution.Target, string) error { return nil }

func TestSuccessfulEnvironmentReleaseSurvivesPartialDeploy(t *testing.T) {
	testEnvironmentRelease(t, "", false)
}

func TestEnvironmentReleaseSurvivesPartialDeployAndPostgresReopen(t *testing.T) {
	testEnvironmentRelease(t, postgrestest.Fresh(t).Console, false)
}

func testEnvironmentRelease(t *testing.T, databaseURL string, failRollback bool) {
	t.Helper()
	ctx := context.Background()
	opts := seedtest.Defaults()
	driver := &releaseTestDeployer{failRollback: failRollback}
	app, err := bootstraptest.Build(ctx, bootstrap.Options{Seed: opts, Adapters: bootstrap.AdapterFake, DeployerOverride: driver, DatabaseURL: databaseURL})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	workloads := workloadconfig.NewService(app.Store)
	svc := pending.NewSimulationService(app.Store, planning.NewService(bootstrap.FakeTemplateSources()), workloads, planning.RegistryInspector{Metadata: app.Modules})
	svc.SetDeployer(app.Deployments)
	if _, err := svc.LastSuccessfulRelease(ctx, opts.ApplicationKey, opts.EnvironmentKey); !errors.Is(err, pending.ErrInvalid) {
		t.Fatalf("initial release: %v", err)
	}
	save := func(version string) {
		t.Helper()
		for _, id := range []string{"alpha", "beta"} {
			view, err := workloads.List(ctx, opts.ApplicationKey, opts.EnvironmentKey)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := workloads.Save(ctx, opts.ApplicationKey, opts.EnvironmentKey, id, scoreFor(id, "example.invalid/"+id+":"+version, false), view.DraftVersion); err != nil {
				t.Fatal(err)
			}
		}
	}
	run := func() pending.DeployReport {
		t.Helper()
		preview, err := svc.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
		if err != nil {
			t.Fatal(err)
		}
		report, err := svc.Deploy(ctx, opts.ApplicationKey, opts.EnvironmentKey, "test", preview.Token)
		if err != nil {
			t.Fatal(err)
		}
		return report
	}
	configure := func(value string) {
		t.Helper()
		scope, err := app.Store.GetConfigurationScope(ctx, opts.ApplicationKey, opts.EnvironmentKey)
		if err != nil {
			t.Fatal(err)
		}
		if err := app.Store.CommitConfigurationRevision(ctx, scope.Version, configuration.Revision{ID: ids.New(), ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey, Version: scope.Version + 1, Entries: map[string]configuration.Entry{"APP_NAME": {Kind: configuration.Variable, Value: value}, "TOKEN": {Kind: configuration.Secret, StoreKey: "test-store", ValueRef: "private-reference-" + value}}}); err != nil {
			t.Fatal(err)
		}
	}
	configure("original")
	save("v1")
	if report := run(); report.Status != "SUCCEEDED" {
		t.Fatalf("first deploy: %+v", report)
	}
	baseline, err := svc.LastSuccessfulRelease(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}
	original, err := app.Store.GetDeploymentSet(ctx, baseline.DeploymentSetID)
	if err != nil || len(original.Document.Modules) != 2 {
		t.Fatalf("whole release: %+v %v", original, err)
	}
	configure("changed")
	save("v2")
	if report := run(); report.Status != "PARTIAL" {
		t.Fatalf("second deploy: %+v", report)
	}
	current, err := app.Store.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil || current.CurrentDeploymentSetID == baseline.DeploymentSetID {
		t.Fatalf("no partial side effect: %+v %v", current, err)
	}
	retained, err := svc.LastSuccessfulRelease(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil || retained.OperationID != baseline.OperationID || retained.DeploymentSetID != baseline.DeploymentSetID {
		t.Fatalf("partial state replaced baseline: %+v %v", retained, err)
	}
	if databaseURL != "" {
		reopened, err := postgres.Open(ctx, databaseURL)
		if err != nil {
			t.Fatal(err)
		}
		defer reopened.Close()
		reader := pending.NewService(reopened, nil, nil, nil)
		recovered, err := reader.LastSuccessfulRelease(ctx, opts.ApplicationKey, opts.EnvironmentKey)
		if err != nil || recovered.Digest != baseline.Digest {
			t.Fatalf("release after database reopen: %+v %v", recovered, err)
		}
	}

	rollback, err := svc.PreviewRollback(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil || rollback.TargetOperationID != baseline.OperationID {
		t.Fatalf("rollback preview: %+v %v", rollback, err)
	}
	if _, err = svc.Rollback(ctx, opts.ApplicationKey, opts.EnvironmentKey, "test", rollback.Token, "wrong"); !errors.Is(err, pending.ErrInvalid) {
		t.Fatalf("confirmation accepted: %v", err)
	}
	// A draft edit after review must refuse rollback before any mutation.
	draftView, _ := workloads.List(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if _, err := workloads.Save(ctx, opts.ApplicationKey, opts.EnvironmentKey, "beta", scoreFor("beta", "example.invalid/beta:v3", false), draftView.DraftVersion); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Rollback(ctx, opts.ApplicationKey, opts.EnvironmentKey, "test", rollback.Token, opts.ApplicationKey); !errors.Is(err, pending.ErrStalePreview) {
		t.Fatalf("stale rollback accepted: %v", err)
	}
	rollback, err = svc.PreviewRollback(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}
	report, err := svc.Rollback(ctx, opts.ApplicationKey, opts.EnvironmentKey, "test", rollback.Token, opts.ApplicationKey)
	if failRollback {
		if err == nil || report.Status == "SUCCEEDED" {
			t.Fatalf("rollback failure hidden: %+v %v", report, err)
		}
		held, ok, readErr := app.Store.ActiveOperation(ctx, opts.ApplicationKey, opts.EnvironmentKey)
		if readErr != nil || !ok || held.Status != environment.OpInterrupted {
			t.Fatalf("partial rollback released claim: %+v %v", held, readErr)
		}
		if _, err := svc.PreviewRollback(ctx, opts.ApplicationKey, opts.EnvironmentKey); !errors.Is(err, persistence.ErrEnvironmentBusy) {
			t.Fatalf("held rollback allowed another writer: %v", err)
		}
		return
	}

	if err != nil || report.Status != "SUCCEEDED" {
		t.Fatalf("rollback: %+v %v", report, err)
	}
	restored, err := app.Store.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}
	restoredScope, err := app.Store.GetConfigurationScope(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}
	restoredConfig, err := app.Store.GetConfigurationRevision(ctx, restoredScope.DesiredRevisionID)
	if err != nil || restoredConfig.Entries["APP_NAME"].Value != "original" || restoredConfig.Entries["TOKEN"].ValueRef != "private-reference-original" {
		t.Fatal("rollback failed to restore config/immutable Secret references")
	}
	restoredSet, err := app.Store.GetDeploymentSet(ctx, restored.CurrentDeploymentSetID)
	if err != nil || restoredSet.DocumentHash != original.DocumentHash {
		t.Fatalf("rollback target not restored: hash=%s expected=%s err=%v", restoredSet.DocumentHash, original.DocumentHash, err)
	}

}

type initialEnvironmentStore struct {
	persistence.Store
	persistence.TemplateSourceRepository
	started time.Time
}

func (s initialEnvironmentStore) ListOperations(ctx context.Context, app, env string, limit int) ([]environment.Operation, error) {
	ops, err := s.Store.ListOperations(ctx, app, env, limit)
	if err != nil {
		return nil, err
	}
	filtered := []environment.Operation{}
	// The seed fixture records a successful catalog setup using OpDeploy. It is
	// outside this fresh test Environment's deployment history, not a release.
	for _, op := range ops {
		if !op.StartedAt.Before(s.started) {
			filtered = append(filtered, op)
		}
	}
	return filtered, nil
}
func TestFirstFailedDeploymentCanRollbackToProvenEmptyBaseline(t *testing.T) {
	testInitialEmptyRollback(t, "")
}
func TestFirstFailedDeploymentEmptyRollbackOnPostgres(t *testing.T) {
	testInitialEmptyRollback(t, postgrestest.Fresh(t).Console)
}
func testInitialEmptyRollback(t *testing.T, databaseURL string) {
	t.Helper()
	ctx := context.Background()
	opts := seedtest.Defaults()
	driver := &releaseTestDeployer{calls: 3} // First workload apply fails once.
	app, err := bootstraptest.Build(ctx, bootstrap.Options{Seed: opts, Adapters: bootstrap.AdapterFake, DeployerOverride: driver, DatabaseURL: databaseURL})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	st := initialEnvironmentStore{Store: app.Store, TemplateSourceRepository: app.Store.(persistence.TemplateSourceRepository), started: time.Now()}
	workloads := workloadconfig.NewService(st)
	svc := pending.NewSimulationService(st, planning.NewService(bootstrap.FakeTemplateSources()), workloads, planning.RegistryInspector{Metadata: app.Modules})
	svc.SetDeployer(app.Deployments)
	if _, err := workloads.Save(ctx, opts.ApplicationKey, opts.EnvironmentKey, "alpha", scoreFor("alpha", "example.invalid/alpha:v1", false), 0); err != nil {
		t.Fatal(err)
	}
	preview, err := svc.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}
	report, err := svc.Deploy(ctx, opts.ApplicationKey, opts.EnvironmentKey, "test", preview.Token)
	if err != nil || report.Status != "FAILED" {
		t.Fatalf("first failure: %+v %v", report, err)
	}
	rollback, err := svc.PreviewRollback(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil || !rollback.Empty {
		t.Fatalf("empty target: %+v %v", rollback, err)
	}
	report, err = svc.Rollback(ctx, opts.ApplicationKey, opts.EnvironmentKey, "test", rollback.Token, opts.ApplicationKey)
	if err != nil || report.Status != "SUCCEEDED" {
		t.Fatalf("empty rollback: %+v %v", report, err)
	}
	env, err := app.Store.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}
	set, err := app.Store.GetDeploymentSet(ctx, env.CurrentDeploymentSetID)
	if err != nil || len(set.Document.Modules) != 0 {
		t.Fatalf("nonempty result: %+v %v", set, err)
	}
	if len(driver.removed) == 0 {
		t.Fatal("failed first-deploy inventory was never removed")
	}
	for _, w := range mustInstances(t, app.Store, opts.ApplicationKey, opts.EnvironmentKey) {
		if w.Status != "REMOVED" {
			t.Fatalf("failed receipt retained: %+v", w)
		}
	}
}
func mustInstances(t *testing.T, st persistence.Store, app, env string) []domain.WorkloadInstance {
	t.Helper()
	instances, err := st.ListWorkloadInstances(context.Background(), app+"/"+env)
	if err != nil {
		t.Fatal(err)
	}
	return instances
}

func (s initialEnvironmentStore) ReadSnapshot(ctx context.Context, fn func(context.Context, persistence.Store) error) error {
	return s.Store.ReadSnapshot(ctx, func(ctx context.Context, view persistence.Store) error {
		return fn(ctx, initialEnvironmentStore{Store: view, TemplateSourceRepository: view.(persistence.TemplateSourceRepository), started: s.started})
	})
}

func TestPartialRollbackRemainsInterruptedAndBlocksAnotherWriter(t *testing.T) {
	testEnvironmentRelease(t, postgrestest.Fresh(t).Console, true)
}

func (d *releaseTestDeployer) RemoveObjects(_ context.Context, _ execution.Target, workload string, _ []execution.ObjectRef) error {
	d.removed = append(d.removed, workload)
	return nil
}
