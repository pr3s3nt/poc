package pending_test

import (
	"context"
	"errors"
	"orchestrator/internal/seed/seedtest"
	"testing"

	deliveryapp "orchestrator/internal/application/delivery"
	appsvc "orchestrator/internal/application/deployment"
	"orchestrator/internal/application/pending"
	"orchestrator/internal/application/workloadconfig"
	"orchestrator/internal/bootstrap"
	"orchestrator/internal/bootstrap/bootstraptest"
	"orchestrator/internal/planning"
	"orchestrator/internal/seed"
)

func scoreFor(name, image string, _ bool) map[string]any {
	service := map[string]any{"ports": map[string]any{"http": map[string]any{"port": 8080}}}
	return map[string]any{"apiVersion": "orchestrator.dev/workload/v1", "metadata": map[string]any{"name": name}, "containers": map[string]any{"main": map[string]any{"image": image}}, "service": service}
}

func fixture(t *testing.T) (*bootstrap.App, *workloadconfig.Service, *pending.Service, seed.Options) {
	t.Helper()
	opts := seedtest.Defaults()
	app, err := bootstraptest.Build(context.Background(), bootstrap.Options{Seed: opts, Adapters: bootstrap.AdapterFake})
	if err != nil {
		t.Fatal(err)
	}
	workloads := workloadconfig.NewService(app.Store)
	preview := pending.NewSimulationService(app.Store, planning.NewService(bootstrap.FakeTemplateSources()), workloads, planning.RegistryInspector{Metadata: app.Modules})
	preview.SetDeployer(app.Deployments)
	return app, workloads, preview, opts
}

func deploy(t *testing.T, app *bootstrap.App, opts seed.Options, name string, raw map[string]any) {
	t.Helper()
	_, err := app.Deployments.DeployWorkload(context.Background(), appsvc.DeployCommand{OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey, WorkloadID: name, WorkloadAfter: raw, Actor: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if err := seedtest.ApplyCatalogFixture(context.Background(), app.Store, opts); err != nil {
		t.Fatal(err)
	}
}

func TestNoopDraftDoesNotPreviewOrDeploy(t *testing.T) {
	ctx := context.Background()
	app, workloads, svc, opts := fixture(t)
	initial := scoreFor("backend", "example.invalid/backend:v1", false)
	deploy(t, app, opts, "backend", initial)
	changed := scoreFor("backend", "example.invalid/backend:changed", false)
	if _, err := workloads.Save(ctx, opts.ApplicationKey, opts.EnvironmentKey, "backend", changed, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := workloads.Save(ctx, opts.ApplicationKey, opts.EnvironmentKey, "backend", initial, 1); err != nil {
		t.Fatal(err)
	}
	view, err := workloads.List(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil || len(view.Workloads) != 1 || view.Workloads[0].State != "" {
		t.Fatalf("no-op draft shown as pending: %+v %v", view, err)
	}
	result, err := svc.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Changes) != 0 {
		t.Fatalf("no-op draft produced changes: %+v", result.Changes)
	}
	withExplicitTarget := scoreFor("backend", "example.invalid/backend:v1", false)
	withExplicitTarget["service"].(map[string]any)["ports"].(map[string]any)["http"].(map[string]any)["targetPort"] = 8080
	if _, err := workloads.Save(ctx, opts.ApplicationKey, opts.EnvironmentKey, "backend", withExplicitTarget, 2); err != nil {
		t.Fatal(err)
	}
	result, err = svc.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil || len(result.Changes) != 0 {
		t.Fatalf("default targetPort triggered redeploy: %+v %v", result.Changes, err)
	}
	// Frontend disables Deploy for an empty Changes list. Server accepts the
	// token but must not create a workload revision either.
	before := len(app.FakeDeploy.Applied)
	if _, err := svc.Deploy(ctx, opts.ApplicationKey, opts.EnvironmentKey, "test", result.Token); err != nil {
		t.Fatal(err)
	}
	if len(app.FakeDeploy.Applied) != before {
		t.Fatal("no-op caused workload apply")
	}
	updated := scoreFor("backend", "example.invalid/backend:v2", false)
	if _, err := workloads.Save(ctx, opts.ApplicationKey, opts.EnvironmentKey, "backend", updated, 3); err != nil {
		t.Fatal(err)
	}
	result, err = svc.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Changes) != 1 || result.Changes[0].WorkloadID != "backend" {
		t.Fatalf("image update missing: %+v", result.Changes)
	}
}

func TestRealPreviewRequiresCDWithoutDiscardingDrafts(t *testing.T) {
	ctx := context.Background()
	app, workloads, _, opts := fixture(t)
	if _, err := workloads.Save(ctx, opts.ApplicationKey, opts.EnvironmentKey, "web", scoreFor("web", "web:v1", false), 0); err != nil {
		t.Fatal(err)
	}
	svc := pending.NewService(app.Store, planning.NewService(bootstrap.FakeTemplateSources()), workloads, planning.RegistryInspector{Metadata: app.Modules})
	if _, err := svc.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey); !errors.Is(err, deliveryapp.ErrCDRequired) {
		t.Fatalf("preview bypassed required CD binding: %v", err)
	}
	// The simulation fixture can still read/plan the same saved draft; the real
	// preflight did not clear it or silently choose a provider.
	simulation := pending.NewSimulationService(app.Store, planning.NewService(bootstrap.FakeTemplateSources()), workloads, planning.RegistryInspector{Metadata: app.Modules})
	out, err := simulation.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil || len(out.Changes) != 1 {
		t.Fatalf("required-CD refusal discarded the draft: %v", err)
	}
}
