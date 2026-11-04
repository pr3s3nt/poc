package pending_test

import (
	"context"
	"errors"
	"orchestrator/internal/application/catalog"
	"orchestrator/internal/application/pending"
	"orchestrator/internal/application/workloadconfig"
	"orchestrator/internal/bootstrap"
	"orchestrator/internal/bootstrap/bootstraptest"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning"
	"orchestrator/internal/seed/seedtest"
	standardtemplates "orchestrator/template-sources"
	"testing"
)

func TestRendererDefinitionChangeTriggersPendingUpdateAndStaleToken(t *testing.T) {
	ctx := context.Background()
	var err error
	opts := seedtest.Defaults()
	app, err := bootstraptest.Build(ctx, bootstrap.Options{Seed: opts})
	if err != nil {
		t.Fatal(err)
	}
	workloads := workloadconfig.NewService(app.Store)
	svc := pending.NewSimulationService(app.Store, planning.NewService(map[string]resource.TemplateSource{}), workloads, planning.RegistryInspector{Metadata: app.Modules})
	svc.SetDeployer(app.Deployments)
	deploy(t, app, opts, "backend", scoreFor("backend", "backend:v1", false))
	old, err := svc.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil || len(old.Changes) != 0 {
		t.Fatal("initial no-op", err)
	}
	catalogSvc := catalog.NewService(app.Store)
	catalogSvc.SetRenderRegistry(app.RenderRegistry)
	disableSeededRenderers(t, app.Store, opts.OrganizationKey)
	def := resource.Definition{Key: "render", Name: "render", Version: 1, ResourceTypeKey: "workload", DriverType: resource.DriverTemplate, Criteria: []resource.Criterion{{ApplicationID: opts.ApplicationKey}}, DriverInputs: map[string]any{"values": map[string]any{"variables": placedVariables(map[string]any{"template_source": standardtemplates.ID(opts.OrganizationKey)})}}}
	if _, err := catalogSvc.RegisterResourceDefinition(ctx, opts.OrganizationKey, def); err != nil {
		t.Fatal(err)
	}
	if err := seedtest.PublishCatalog(ctx, app.Store, opts); err != nil {
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

// copyCLI installs the real template under a new path. extra makes a different
// binary digest (trailing bytes after the executable image are ignored by the loader).
