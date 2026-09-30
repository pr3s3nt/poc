package pending_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	tf "orchestrator/internal/adapters/terraform"
	appsvc "orchestrator/internal/application/deployment"
	"orchestrator/internal/application/pending"
	"orchestrator/internal/application/workloadconfig"
	"orchestrator/internal/bootstrap"
	"orchestrator/internal/planning"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/seed"
)

type routeRecorder struct {
	calls    []execution.PublicRoute
	failOnce bool
}

func (r *routeRecorder) Reconcile(_ context.Context, _ execution.Target, route execution.PublicRoute) error {
	r.calls = append(r.calls, route)
	if r.failOnce {
		r.failOnce = false
		return errors.New("route unavailable")
	}
	return nil
}

func scoreFor(name, image string, public bool) map[string]any {
	service := map[string]any{"ports": map[string]any{"http": map[string]any{"port": 8080}}}
	if public {
		service["publicPort"] = "http"
	}
	return map[string]any{"apiVersion": "score.dev/v1b1", "metadata": map[string]any{"name": name}, "containers": map[string]any{"main": map[string]any{"image": image}}, "service": service}
}

func fixture(t *testing.T) (*bootstrap.App, *workloadconfig.Service, *pending.Service, seed.Options) {
	t.Helper()
	opts := seed.Defaults()
	app, err := bootstrap.Build(context.Background(), bootstrap.Options{Seed: opts, Adapters: bootstrap.AdapterFake})
	if err != nil {
		t.Fatal(err)
	}
	workloads := workloadconfig.NewService(app.Store)
	preview := pending.NewService(app.Store, planning.NewService(), workloads, tf.NewInspector())
	preview.SetDeployer(app.Deployments)
	return app, workloads, preview, opts
}

func deploy(t *testing.T, app *bootstrap.App, opts seed.Options, name string, raw map[string]any) {
	t.Helper()
	_, err := app.Deployments.DeployWorkload(context.Background(), appsvc.DeployCommand{OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey, WorkloadID: name, ScoreAfter: raw, Actor: "test"})
	if err != nil {
		t.Fatal(err)
	}
}

func TestNoopDraftDoesNotPreviewOrDeploy(t *testing.T) {
	ctx := context.Background()
	app, workloads, svc, opts := fixture(t)
	initial := scoreFor("backend", "example.invalid/backend:v1", false)
	deploy(t, app, opts, "backend", initial)
	changed := scoreFor("backend", "example.invalid/backend:v1", true)
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

func TestMovePublicPathBetweenWorkloadsInOneDeploy(t *testing.T) {
	ctx := context.Background()
	app, workloads, svc, opts := fixture(t)
	routes := &routeRecorder{}
	app.Deployments.SetPublicRouteManager(routes, "example.com")
	backend := scoreFor("backend", "example.invalid/backend:v1", true)
	frontend := scoreFor("frontend", "example.invalid/frontend:v1", false)
	deploy(t, app, opts, "backend", backend)
	deploy(t, app, opts, "frontend", frontend)
	backendPrivate := scoreFor("backend", "example.invalid/backend:v1", false)
	frontendPublic := scoreFor("frontend", "example.invalid/frontend:v1", true)
	if _, err := workloads.Save(ctx, opts.ApplicationKey, opts.EnvironmentKey, "backend", backendPrivate, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := workloads.Save(ctx, opts.ApplicationKey, opts.EnvironmentKey, "frontend", frontendPublic, 1); err != nil {
		t.Fatal(err)
	}
	preview, err := svc.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Changes) != 2 {
		t.Fatalf("expected route transfer, got %+v", preview.Changes)
	}
	before := len(routes.calls)
	report, err := svc.Deploy(ctx, opts.ApplicationKey, opts.EnvironmentKey, "test", preview.Token)
	if err != nil || report.Status != "SUCCEEDED" {
		t.Fatalf("deploy: %+v %v", report, err)
	}
	if len(routes.calls) != before+1 || len(routes.calls[before].Paths) != 1 || routes.calls[before].Paths[0].WorkloadID != "frontend" {
		t.Fatalf("routes reconciled out of order: %+v", routes.calls)
	}
}

func TestFailedRouteCanRetryWithoutRestartingWorkload(t *testing.T) {
	ctx := context.Background()
	app, workloads, svc, opts := fixture(t)
	private := scoreFor("backend", "example.invalid/backend:v1", false)
	deploy(t, app, opts, "backend", private)
	routes := &routeRecorder{failOnce: true}
	app.Deployments.SetPublicRouteManager(routes, "example.com")
	if _, err := workloads.Save(ctx, opts.ApplicationKey, opts.EnvironmentKey, "backend", scoreFor("backend", "example.invalid/backend:v1", true), 0); err != nil {
		t.Fatal(err)
	}
	preview, err := svc.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Deploy(ctx, opts.ApplicationKey, opts.EnvironmentKey, "test", preview.Token); !errors.Is(err, pending.ErrRouteReconcile) || strings.Contains(err.Error(), "route unavailable") {
		t.Fatalf("route failure not reported safely: %v", err)
	}
	previousApplies := len(app.FakeDeploy.Applied)
	retry, err := svc.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}
	if !retry.RoutePending || len(retry.Changes) != 0 {
		t.Fatalf("route-only retry not offered: %+v", retry)
	}
	if _, err := svc.Deploy(ctx, opts.ApplicationKey, opts.EnvironmentKey, "test", retry.Token); err != nil {
		t.Fatal(err)
	}
	if len(app.FakeDeploy.Applied) != previousApplies {
		t.Fatal("route retry restarted workload")
	}
	resolved, err := svc.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil || resolved.RoutePending {
		t.Fatalf("route retry status not cleared: %+v %v", resolved, err)
	}
}
