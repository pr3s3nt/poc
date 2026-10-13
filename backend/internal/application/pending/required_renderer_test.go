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

// The same rule holds on aws-eks: a template of the other profile never
// applies, the matching aws-eks template is pinned, the Preview shows the
// AWS infrastructure intent with the Environment's AWS binding, and Deploy
// renders with the real CLI only after VPC/EKS/namespace outputs exist,
// leaving the protected namespace identity and generating no infrastructure.
func TestRequiredRendererAWSProfileRendersAfterInfrastructure(t *testing.T) {
	ctx := context.Background()
	real, err := exec.LookPath("score-k8s")
	if err != nil {
		t.Skip("local score-k8s 0.15.0 unavailable")
	}
	opts := seedtest.Defaults()
	opts.Region, opts.AccountID = "us-east-1", "000000000000"
	log := filepath.Join(t.TempDir(), "calls.log")
	wrapper := filepath.Join(t.TempDir(), "score-k8s")
	script := "#!/bin/sh\nif [ \"$1\" != \"--version\" ]; then echo \"$@\" >> '" + log + "'; fi\nexec '" + real + "' \"$@\"\n"
	if err := os.WriteFile(wrapper, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	renderer, err := scorek8s.New(ctx, wrapper, nil)
	if err != nil {
		t.Fatal(err)
	}
	app, err := bootstraptest.Build(ctx, bootstrap.Options{Seed: opts, Adapters: bootstrap.AdapterFake, ScoreK8sPath: wrapper, RequireScoreRenderer: true})
	if err != nil {
		t.Fatal(err)
	}
	opts.ApplicationKey = opts.CloudApplicationKey
	registerRenderer(t, app, renderer.Bundles(), opts, "render-internal", "internal-k8s")
	if _, err := app.Workloads.Save(ctx, opts.ApplicationKey, opts.EnvironmentKey, "backend", scoreFor("backend", "backend:v1", false), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Pending.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey); !errors.Is(err, pending.ErrInvalid) || !strings.Contains(err.Error(), "rendering template") {
		t.Fatalf("a template of the other profile must not apply: %v", err)
	}
	registerRenderer(t, app, renderer.Bundles(), opts, "render-cloud", "aws-eks")
	preview, err := app.Pending.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil || len(preview.Changes) != 1 {
		t.Fatalf("preview: %+v %v", preview, err)
	}
	if _, statErr := os.Stat(log); statErr == nil {
		t.Fatal("Preview ran the score-k8s CLI")
	}
	if preview.Binding.Profile != "aws-eks" || preview.Binding.Region != "us-east-1" || preview.AccountID == "" {
		t.Fatalf("target not pinned: %+v account=%q", preview.Binding, preview.AccountID)
	}
	change := preview.Changes[0]
	if change.Rendering.DefinitionKey != "render-cloud" {
		t.Fatalf("wrong template: %+v", change.Rendering)
	}
	byType := map[string]planningIntent{}
	for _, intent := range change.ResourceIntents {
		byType[intent.ResourceType] = planningIntent{intent.Intent, intent.DefinitionKey, intent.DriverType}
		if intent.DriverType == "terraform" && (intent.Module == nil || intent.Module.Revision == "" || len(intent.Providers) == 0) {
			t.Fatalf("Terraform intent lacks its pin or provider targets: %+v", intent)
		}
	}
	for _, want := range []string{"vpc", "k8s-cluster", "k8s-namespace"} {
		if byType[want].intent != "CREATE" {
			t.Fatalf("%s intent: %+v", want, byType)
		}
	}
	report, err := app.Pending.Deploy(ctx, opts.ApplicationKey, opts.EnvironmentKey, "test", preview.Token)
	if err != nil || report.Status != "SUCCEEDED" {
		t.Fatalf("deploy: %+v %v", report, err)
	}
	if len(app.FakeExec.Calls) == 0 {
		t.Fatal("infrastructure was not executed before rendering")
	}
	called, err := os.ReadFile(log)
	if err != nil || !strings.Contains(string(called), "generate") {
		t.Fatalf("score-k8s did not render after the outputs: %q %v", called, err)
	}
	env, err := app.Store.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}
	deployments := 0
	for _, applied := range app.FakeDeploy.Applied {
		for _, manifest := range applied.Manifests {
			switch manifest.Kind {
			case "Namespace", "PersistentVolumeClaim", "StatefulSet", "Secret":
				if manifest.Kind != "Secret" {
					t.Fatalf("renderer generated infrastructure: %s", manifest.Kind)
				}
			case "Deployment":
				deployments++
				if manifest.Namespace != env.Namespace() {
					t.Fatalf("protected namespace identity changed: %q != %q", manifest.Namespace, env.Namespace())
				}
			}
		}
	}
	if deployments != 1 {
		t.Fatalf("workload Deployments: %d", deployments)
	}
}

type planningIntent struct{ intent, definition, driver string }

// An existing identity is never displayed as unchanged by default: a changed
// authored param is UPDATE, a verifiably identical plan is RETAIN, and an
// identity whose last plan cannot be read is UNVERIFIED (never RETAIN).
func TestPreviewIntentUpdateRetainUnverifiedForExistingIdentity(t *testing.T) {
	ctx := context.Background()
	app, opts, _, bundles := requiredAppBundles(t, true)
	registerRenderer(t, app, bundles, opts, "render", "internal-k8s")
	shared := func(name, database string) map[string]any {
		raw := scoreFor(name, name+":v1", false)
		raw["resources"] = map[string]any{"db": map[string]any{"type": "postgres", "params": map[string]any{"database": database, "username": "appuser"}}}
		return raw
	}
	if _, err := app.Workloads.Save(ctx, opts.ApplicationKey, opts.EnvironmentKey, "api", shared("api", "orders"), 0); err != nil {
		t.Fatal(err)
	}
	first, err := app.Pending.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}
	intentOf := func(p pending.Preview) string {
		for _, intent := range p.Changes[0].ResourceIntents {
			if intent.ResourceType == "postgres" {
				return intent.Intent
			}
		}
		return ""
	}
	if got := intentOf(first); got != "CREATE" {
		t.Fatalf("new identity: %s", got)
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
	// Same identity, same authored inputs, image changed only: the database is verifiably unchanged.
	same := shared("api", "orders")
	same["containers"].(map[string]any)["main"].(map[string]any)["image"] = "api:v2"
	if _, err := app.Workloads.Save(ctx, opts.ApplicationKey, opts.EnvironmentKey, "api", same, draftVersion()); err != nil {
		t.Fatal(err)
	}
	retained, err := app.Pending.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil || len(retained.Changes) != 1 {
		t.Fatalf("preview: %+v %v", retained, err)
	}
	if got := intentOf(retained); got != "RETAIN" {
		t.Fatalf("unchanged identity: %s", got)
	}
	// An authored param change on the same identity is an update, not "unchanged".
	changed := shared("api", "orders_v2")
	if _, err := app.Workloads.Save(ctx, opts.ApplicationKey, opts.EnvironmentKey, "api", changed, draftVersion()); err != nil {
		t.Fatal(err)
	}
	updated, err := app.Pending.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil || len(updated.Changes) != 1 {
		t.Fatalf("preview: %+v %v", updated, err)
	}
	if got := intentOf(updated); got != "UPDATE" {
		t.Fatalf("changed params: %s", got)
	}
	raw, _ := json.Marshal(updated.Changes[0].ResourceIntents)
	for _, leak := range []string{"orders_v2", "appuser", "params", "fingerprint"} {
		if strings.Contains(string(raw), leak) {
			t.Fatalf("projection exposes %q", leak)
		}
	}
	// Without a readable last plan the identity is unverified, never retained.
	active, err := app.Store.ListActiveResources(ctx, opts.OrganizationKey)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range active {
		if a.Descriptor.Type == "postgres" {
			a.LastDeploymentID = ""
			if _, err := app.Store.UpsertActiveResource(ctx, a); err != nil {
				t.Fatal(err)
			}
		}
	}
	unknown, err := app.Pending.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}
	if got := intentOf(unknown); got != "UNVERIFIED" {
		t.Fatalf("no baseline: %s", got)
	}
}

// UC-05 BR-10: editing a fallback variable of the SAME Definition key changes
// the effective module inputs, so Preview shows UPDATE, never RETAIN.
func TestPreviewIntentSameDefinitionKeyChangedFallbackIsUpdate(t *testing.T) {
	ctx := context.Background()
	app, opts, _, bundles := requiredAppBundles(t, true)
	registerRenderer(t, app, bundles, opts, "render", "internal-k8s")
	raw := scoreFor("api", "api:v1", false)
	raw["resources"] = map[string]any{"db": map[string]any{"type": "postgres", "params": map[string]any{"database": "orders", "username": "appuser"}}}
	if _, err := app.Workloads.Save(ctx, opts.ApplicationKey, opts.EnvironmentKey, "api", raw, 0); err != nil {
		t.Fatal(err)
	}
	first, err := app.Pending.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}
	if report, err := app.Pending.Deploy(ctx, opts.ApplicationKey, opts.EnvironmentKey, "test", first.Token); err != nil || report.Status != "SUCCEEDED" {
		t.Fatalf("deploy: %+v %v", report, err)
	}
	var definitionKey string
	for _, intent := range first.Changes[0].ResourceIntents {
		if intent.ResourceType == "postgres" {
			definitionKey = intent.DefinitionKey
		}
	}
	intentOf := func(p pending.Preview) string {
		for _, intent := range p.Changes[0].ResourceIntents {
			if intent.ResourceType == "postgres" {
				if intent.DefinitionKey != definitionKey {
					t.Fatalf("the Definition key changed: %s", intent.DefinitionKey)
				}
				return intent.Intent
			}
		}
		return ""
	}
	// Re-save the draft without changes to the Definition: RETAIN.
	env, _ := app.Store.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	same := scoreFor("api", "api:v2", false)
	same["resources"] = raw["resources"]
	if _, err := app.Workloads.Save(ctx, opts.ApplicationKey, opts.EnvironmentKey, "api", same, env.DraftVersion); err != nil {
		t.Fatal(err)
	}
	retained, err := app.Pending.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil || len(retained.Changes) != 1 || intentOf(retained) != "RETAIN" {
		t.Fatalf("identical Definition: %+v %v", retained, err)
	}
	// Same key, different fallback variable.
	defs, err := app.Store.ListResourceDefinitions(ctx, opts.OrganizationKey)
	if err != nil {
		t.Fatal(err)
	}
	for _, def := range defs {
		if def.Key != definitionKey {
			continue
		}
		values := map[string]any{}
		for k, v := range def.DriverValues() {
			values[k] = v
		}
		variables := map[string]any{}
		for k, v := range def.Variables() {
			variables[k] = v
		}
		variables["storage"] = "9Gi"
		values["variables"] = variables
		def.DriverInputs = map[string]any{"values": values}
		if err := app.Store.SaveResourceDefinition(ctx, opts.OrganizationKey, def); err != nil {
			t.Fatal(err)
		}
	}
	updated, err := app.Pending.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil || len(updated.Changes) != 1 {
		t.Fatalf("preview: %+v %v", updated, err)
	}
	if got := intentOf(updated); got != "UPDATE" {
		t.Fatalf("a changed fallback variable of the same Definition key must be UPDATE, got %s", got)
	}
	raw2, _ := json.Marshal(updated.Changes[0].ResourceIntents)
	if strings.Contains(string(raw2), "9Gi") {
		t.Fatal("the projection must not expose the changed value")
	}
}
