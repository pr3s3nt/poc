package pending_test

import (
	"context"
	"testing"

	"orchestrator/internal/adapters/postgres/postgrestest"
	"orchestrator/internal/application/pending"
	"orchestrator/internal/application/workloadconfig"
	"orchestrator/internal/bootstrap"
	"orchestrator/internal/bootstrap/bootstraptest"
	"orchestrator/internal/planning"
	"orchestrator/internal/seed/seedtest"
)

// DEVEX-ENV E02/E10 on a disposable PostgreSQL database: an accepted ordinary value
// deployed by UC-16 stays readable after the application is closed and rebuilt on
// the same database, and a direct-API literal never becomes readable.
func TestAcceptedLiteralsSurviveRestartOnPostgres(t *testing.T) {
	ctx := context.Background()
	url := postgrestest.Fresh(t).Console
	opts := seedtest.Defaults()
	build := func() (*bootstrap.App, *workloadconfig.Service, *pending.Service) {
		app, err := bootstraptest.Build(ctx, bootstrap.Options{Seed: opts, Adapters: bootstrap.AdapterFake, DatabaseURL: url})
		if err != nil {
			t.Fatal(err)
		}
		workloads := workloadconfig.NewService(app.Store)
		svc := pending.NewService(app.Store, planning.NewService(bootstrap.FakeRenderBundles()), workloads, planning.RegistryInspector{Metadata: app.Modules})
		svc.SetDeployer(app.Deployments)
		return app, workloads, svc
	}

	app, workloads, svc := build()
	if _, err := workloads.Save(ctx, opts.ApplicationKey, opts.EnvironmentKey, "api", literalScore("api", acceptedLiterals), 0); err != nil {
		t.Fatal(err)
	}
	preview, err := svc.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}
	if report, err := svc.Deploy(ctx, opts.ApplicationKey, opts.EnvironmentKey, "test", preview.Token); err != nil || report.Status != "SUCCEEDED" {
		t.Fatalf("deploy: %+v %v", report, err)
	}
	deploy(t, app, opts, "legacy", literalScore("legacy", map[string]string{"TOKEN": "unproven"}))
	app.Close()

	reopened, reopenedWorkloads, _ := build()
	defer reopened.Close()
	got := variablesOf(liveScore(t, reopenedWorkloads, opts.ApplicationKey, opts.EnvironmentKey, "api"))
	for key, want := range acceptedLiterals {
		if value, ok := got[key]; !ok || value != want {
			t.Fatalf("after restart %s = %q (present %v), want %q", key, value, ok, want)
		}
	}
	if score := liveScore(t, reopenedWorkloads, opts.ApplicationKey, opts.EnvironmentKey, "legacy"); score != nil {
		t.Fatalf("an unproven literal became readable after restart: %v", score)
	}
}
