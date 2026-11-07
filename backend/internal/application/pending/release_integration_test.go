package pending_test

import (
	"context"
	"errors"
	"orchestrator/internal/adapters/postgres"
	"orchestrator/internal/adapters/postgres/postgrestest"
	"testing"

	"orchestrator/internal/application/pending"
	"orchestrator/internal/application/workloadconfig"
	"orchestrator/internal/bootstrap"
	"orchestrator/internal/bootstrap/bootstraptest"
	"orchestrator/internal/planning"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/seed/seedtest"
)

type releaseTestDeployer struct{ calls int }

func (d *releaseTestDeployer) Apply(context.Context, execution.Target, []execution.Manifest) error {
	d.calls++
	if d.calls == 4 {
		return errors.New("confirmed test failure")
	}
	return nil
}
func (*releaseTestDeployer) WaitReady(context.Context, execution.Target, []execution.WorkloadRef) error {
	return nil
}
func (*releaseTestDeployer) Remove(context.Context, execution.Target, string) error { return nil }

func TestSuccessfulEnvironmentReleaseSurvivesPartialDeploy(t *testing.T) {
	testEnvironmentRelease(t, "")
}

func TestEnvironmentReleaseSurvivesPartialDeployAndPostgresReopen(t *testing.T) {
	testEnvironmentRelease(t, postgrestest.Fresh(t).Console)
}

func testEnvironmentRelease(t *testing.T, databaseURL string) {
	t.Helper()
	ctx := context.Background()
	opts := seedtest.Defaults()
	driver := &releaseTestDeployer{}
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

}
