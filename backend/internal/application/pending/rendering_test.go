package pending_test

import (
	"context"
	"errors"
	"orchestrator/internal/adapters/kubernetes"
	"orchestrator/internal/adapters/scorek8s"
	tf "orchestrator/internal/adapters/terraform"
	"orchestrator/internal/application/catalog"
	"orchestrator/internal/application/pending"
	"orchestrator/internal/application/workloadconfig"
	"orchestrator/internal/bootstrap"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning"
	"orchestrator/internal/seed"
	"os/exec"
	"testing"
)

func TestRendererDefinitionChangeTriggersPendingUpdateAndStaleToken(t *testing.T) {
	ctx := context.Background()
	path, err := exec.LookPath("score-k8s")
	if err != nil {
		t.Skip("local score-k8s unavailable")
	}
	opts := seed.Defaults()
	app, err := bootstrap.Build(ctx, bootstrap.Options{Seed: opts, ScoreK8sPath: path})
	if err != nil {
		t.Fatal(err)
	}
	render, err := scorek8s.New(ctx, path, kubernetes.NewRenderer())
	if err != nil {
		t.Fatal(err)
	}
	workloads := workloadconfig.NewService(app.Store)
	svc := pending.NewService(app.Store, planning.NewService(render.Bundles()), workloads, tf.NewInspector())
	svc.SetDeployer(app.Deployments)
	deploy(t, app, opts, "backend", scoreFor("backend", "backend:v1", false))
	old, err := svc.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil || len(old.Changes) != 0 {
		t.Fatal("initial no-op", err)
	}
	catalogSvc := catalog.NewService(app.Store)
	catalogSvc.SetRenderBundles(render.Bundles())
	def := resource.Definition{Key: "render", ResourceTypeKey: "workload", ExecutionProfile: "internal-k8s", DriverType: resource.DriverScoreK8s, Criteria: []resource.Criterion{{ApplicationID: opts.ApplicationKey}}, DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{"render_bundle": scorek8s.BundleID}}}}
	if _, err := catalogSvc.RegisterResourceDefinition(ctx, opts.OrganizationKey, def); err != nil {
		t.Fatal(err)
	}
	next, err := svc.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Changes) != 1 || next.Changes[0].Rendering.DefinitionKey != "render" || old.Token == next.Token {
		t.Fatalf("rendering update not surfaced: %+v", next)
	}
	calls := len(app.FakeExec.Calls)
	if _, err := svc.Deploy(ctx, opts.ApplicationKey, opts.EnvironmentKey, "test", old.Token); !errors.Is(err, pending.ErrStalePreview) {
		t.Fatal("old token accepted", err)
	}
	if len(app.FakeExec.Calls) != calls {
		t.Fatal("stale preview provisioned resources")
	}
	report, err := svc.Deploy(ctx, opts.ApplicationKey, opts.EnvironmentKey, "test", next.Token)
	if err != nil || report.Status != "SUCCEEDED" {
		t.Fatal(report, err)
	}
	done, err := svc.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil || len(done.Changes) != 0 {
		t.Fatalf("rendering update not recorded: %+v %v", done, err)
	}
}
