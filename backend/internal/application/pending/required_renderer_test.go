package pending_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"orchestrator/internal/adapters/scorek8s"
	"orchestrator/internal/application/catalog"
	"orchestrator/internal/application/pending"
	"orchestrator/internal/bootstrap"
	"orchestrator/internal/bootstrap/bootstraptest"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/seed"
	"orchestrator/internal/seed/seedtest"
)

// requiredApp is the product renderer policy: score-k8s is mandatory and the
// native renderer is unreachable. The CLI is a recording wrapper around the
// real pinned binary so a test can prove Preview never runs it.
func requiredApp(t *testing.T, withBinary bool) (*bootstrap.App, seed.Options, string) {
	t.Helper()
	app, opts, log, _ := requiredAppBundles(t, withBinary)
	return app, opts, log
}

func requiredAppBundles(t *testing.T, withBinary bool) (*bootstrap.App, seed.Options, string, map[string]resource.RenderBundle) {
	t.Helper()
	real, err := exec.LookPath("score-k8s")
	if err != nil && withBinary {
		t.Skip("local score-k8s 0.15.0 unavailable")
	}
	opts := seedtest.Defaults()
	options := bootstrap.Options{Seed: opts, Adapters: bootstrap.AdapterFake, RequireScoreRenderer: true}
	log := filepath.Join(t.TempDir(), "calls.log")
	bundles := map[string]resource.RenderBundle{}
	if withBinary {
		wrapper := filepath.Join(t.TempDir(), "score-k8s")
		script := "#!/bin/sh\nif [ \"$1\" != \"--version\" ]; then echo \"$@\" >> '" + log + "'; fi\nexec '" + real + "' \"$@\"\n"
		if err := os.WriteFile(wrapper, []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
		options.ScoreK8sPath = wrapper
		renderer, err := scorek8s.New(context.Background(), wrapper, nil)
		if err != nil {
			t.Fatal(err)
		}
		bundles = renderer.Bundles()
	}
	app, err := bootstraptest.Build(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	return app, opts, log, bundles
}

func registerRenderer(t *testing.T, app *bootstrap.App, bundles map[string]resource.RenderBundle, opts seed.Options, key, profile string, criteria ...resource.Criterion) {
	t.Helper()
	svc := catalog.NewService(app.Store)
	svc.SetRenderBundles(bundles)
	if len(criteria) == 0 {
		criteria = []resource.Criterion{{}}
	}
	def := resource.Definition{Key: key, ResourceTypeKey: "workload", ExecutionProfile: profile, DriverType: resource.DriverScoreK8s, Criteria: criteria,
		DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{"render_bundle": scorek8s.BundleID}}}}
	if _, err := svc.RegisterResourceDefinition(context.Background(), opts.OrganizationKey, def); err != nil {
		t.Fatal(err)
	}
}

func withDatabase(name string) map[string]any {
	raw := scoreFor(name, name+":v1", false)
	raw["resources"] = map[string]any{"db": map[string]any{"type": "postgres", "class": "default", "params": map[string]any{"database": "appdb", "username": "appuser"}}}
	return raw
}

// UC-05 BR-09 / UC-06 BR-17: without exactly one compatible score-k8s
// Definition and an installed bundle, Preview and Deploy fail before any
// executor or deployer call; there is no native fallback.
func TestRequiredRendererMissingTieAndUnavailableFailBeforeResources(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name       string
		withBinary bool
		prepare    func(*testing.T, *bootstrap.App, map[string]resource.RenderBundle, seed.Options)
		want       string
	}{
		{"no-definition", true, func(*testing.T, *bootstrap.App, map[string]resource.RenderBundle, seed.Options) {}, "rendering template"},
		{"tie", true, func(t *testing.T, app *bootstrap.App, b map[string]resource.RenderBundle, o seed.Options) {
			registerRenderer(t, app, b, o, "render-a", "internal-k8s")
			registerRenderer(t, app, b, o, "render-b", "internal-k8s")
		}, "equally"},
		{"wrong-profile", true, func(t *testing.T, app *bootstrap.App, b map[string]resource.RenderBundle, o seed.Options) {
			registerRenderer(t, app, b, o, "render-cloud", "aws-eks")
		}, "rendering template"},
		{"no-binary", false, func(*testing.T, *bootstrap.App, map[string]resource.RenderBundle, seed.Options) {}, "rendering template"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, opts, _, bundles := requiredAppBundles(t, tc.withBinary)
			tc.prepare(t, app, bundles, opts)
			if _, err := app.Workloads.Save(ctx, opts.ApplicationKey, opts.EnvironmentKey, "backend", withDatabase("backend"), 0); err != nil {
				t.Fatal(err)
			}
			_, err := app.Pending.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
			if !errors.Is(err, pending.ErrInvalid) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Preview must be refused: %v", err)
			}
			if _, err := app.Pending.Deploy(ctx, opts.ApplicationKey, opts.EnvironmentKey, "test", "forged-token"); err == nil {
				t.Fatal("Deploy must be refused")
			}
			if len(app.FakeExec.Calls) != 0 || len(app.FakeDeploy.Applied) != 0 {
				t.Fatalf("side effects before the renderer was proven: %d executor calls, %d applies", len(app.FakeExec.Calls), len(app.FakeDeploy.Applied))
			}
		})
	}
}

// Both profiles' Previews are pure (the CLI is never run), pin the Definition
// and bundle, and show the safe resource projection. A renderer or Definition
// change makes the old token stale before any effect; Deploy renders with the
// real CLI only after the resource outputs.
func TestRequiredRendererPreviewIsPurePinsAndDeployRendersAfterOutputs(t *testing.T) {
	ctx := context.Background()
	app, opts, log, bundles := requiredAppBundles(t, true)
	registerRenderer(t, app, bundles, opts, "render", "internal-k8s")
	if _, err := app.Workloads.Save(ctx, opts.ApplicationKey, opts.EnvironmentKey, "backend", withDatabase("backend"), 0); err != nil {
		t.Fatal(err)
	}
	preview, err := app.Pending.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil || len(preview.Changes) != 1 {
		t.Fatalf("preview: %+v %v", preview, err)
	}
	if _, err := os.Stat(log); err == nil {
		t.Fatal("Preview ran the score-k8s CLI")
	}
	if len(app.FakeExec.Calls) != 0 || len(app.FakeDeploy.Applied) != 0 {
		t.Fatal("Preview had effects")
	}
	change := preview.Changes[0]
	if change.Rendering.DefinitionKey != "render" || change.Rendering.DriverType != resource.DriverScoreK8s || change.Rendering.Bundle.ID != scorek8s.BundleID {
		t.Fatalf("renderer not pinned: %+v", change.Rendering)
	}
	byType := map[string]string{}
	for _, intent := range change.ResourceIntents {
		byType[intent.ResourceType] = intent.Intent + "/" + intent.Class + "/" + intent.DefinitionKey
	}
	if byType["postgres"] != "CREATE/default/postgres-internal-statefulset" {
		t.Fatalf("resource projection: %v", byType)
	}
	// The projection carries no Score params or resolved inputs; the whole
	// Preview carries no credential, kubeconfig, state or secret value.
	intents, _ := json.Marshal(change.ResourceIntents)
	for _, forbidden := range []string{"appuser", "appdb", "params"} {
		if strings.Contains(string(intents), forbidden) {
			t.Fatalf("resource projection exposes %q", forbidden)
		}
	}
	raw, _ := json.Marshal(preview)
	for _, forbidden := range []string{"kubeconfig", "credential", "stateRef", "password\":\"", "secretRef"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("Preview exposes %q", forbidden)
		}
	}
	// A second Definition makes a tie: the earlier token is refused before side effects.
	registerRenderer(t, app, bundles, opts, "render-2", "internal-k8s")
	if _, err := app.Pending.Deploy(ctx, opts.ApplicationKey, opts.EnvironmentKey, "test", preview.Token); err == nil {
		t.Fatal("stale token accepted")
	}
	if len(app.FakeExec.Calls) != 0 || len(app.FakeDeploy.Applied) != 0 {
		t.Fatal("stale Preview had effects")
	}
}

func TestRequiredRendererDeployRendersWithRealCLIAfterOutputs(t *testing.T) {
	ctx := context.Background()
	app, opts, log, bundles := requiredAppBundles(t, true)
	registerRenderer(t, app, bundles, opts, "render", "internal-k8s")
	if _, err := app.Workloads.Save(ctx, opts.ApplicationKey, opts.EnvironmentKey, "backend", withDatabase("backend"), 0); err != nil {
		t.Fatal(err)
	}
	preview, err := app.Pending.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}
	report, err := app.Pending.Deploy(ctx, opts.ApplicationKey, opts.EnvironmentKey, "test", preview.Token)
	if err != nil || report.Status != "SUCCEEDED" {
		t.Fatalf("deploy: %+v %v", report, err)
	}
	called, err := os.ReadFile(log)
	if err != nil || !strings.Contains(string(called), "generate") {
		t.Fatalf("score-k8s did not render: %q %v", called, err)
	}
	if len(app.FakeExec.Calls) == 0 || len(app.FakeDeploy.Applied) == 0 {
		t.Fatalf("resources/workload not applied: %d/%d", len(app.FakeExec.Calls), len(app.FakeDeploy.Applied))
	}
}
