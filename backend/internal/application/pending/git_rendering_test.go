package pending_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"orchestrator/internal/adapters/kubernetes"
	"orchestrator/internal/adapters/scorek8s"
	"orchestrator/internal/adapters/terraform/registry"
	"orchestrator/internal/application/catalog"
	"orchestrator/internal/application/pending"
	"orchestrator/internal/application/workloadconfig"
	"orchestrator/internal/bootstrap"
	"orchestrator/internal/bootstrap/bootstraptest"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning"
	"orchestrator/internal/ports/moduleregistry"
	"orchestrator/internal/ports/renderregistry"
	"orchestrator/internal/renderengine"
	"orchestrator/internal/seed/seedtest"
)

func TestMain(m *testing.M) {
	renderengine.RunWorkerIfRequested()
	os.Exit(m.Run())
}

// gitFixture serves fixed commits like the hardened Git transport and counts
// every fetch so tests can prove Preview/Deploy never touch Git.
type gitFixture struct {
	trees map[string]registry.Tree
	calls int
}

func (g *gitFixture) Resolve(_ context.Context, _ registry.GitSource, ref string) (string, error) {
	g.calls++
	if _, ok := g.trees[ref]; ok {
		return ref, nil
	}
	return "", moduleregistry.ErrUnavailable
}

type fixtureSnapshot struct{ tree registry.Tree }

func (s fixtureSnapshot) Tree(string) (registry.Tree, error) { return s.tree, nil }
func (s fixtureSnapshot) Close()                             {}

func (g *gitFixture) Open(_ context.Context, _ registry.GitSource, commit string) (registry.Snapshot, error) {
	g.calls++
	tree, ok := g.trees[commit]
	if !ok {
		return nil, moduleregistry.ErrUnavailable
	}
	return fixtureSnapshot{tree: tree}, nil
}

func standardBundle(t *testing.T) registry.Tree {
	t.Helper()
	dir := "../../../render-bundles/standard"
	out := registry.Tree{}
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		content, err := os.ReadFile(p)
		out[filepath.ToSlash(rel)] = content
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func appliedDeployment(t *testing.T, app *bootstrap.App) map[string]any {
	t.Helper()
	applied := app.FakeDeploy.Applied
	for i := len(applied) - 1; i >= 0; i-- {
		for _, m := range applied[i].Manifests {
			if m.Kind == "Deployment" {
				return m.Object
			}
		}
	}
	t.Fatal("no Deployment applied")
	return nil
}

func podOf(d map[string]any) map[string]any {
	return d["spec"].(map[string]any)["template"].(map[string]any)
}

// UC-07 RB-01 / UC-05 RB-01..03: a Definition edit that only selects another
// verified Git pin (or inputs) is a pending workload UPDATE with no Score
// draft; the old Preview is stale; Deploy renders B from the cache without a
// Git fetch; re-selecting A restores A; unchanged intent is a no-op.
func TestGitBundleOnlyChangeIsPendingUpdateWithoutGitFetch(t *testing.T) {
	ctx := context.Background()
	cli, err := exec.LookPath("score-k8s")
	if err != nil {
		t.Skip("local score-k8s unavailable")
	}
	opts := seedtest.Defaults()
	app, err := bootstraptest.Build(ctx, bootstrap.Options{Seed: opts, ScoreK8sPath: cli})
	if err != nil {
		t.Fatal(err)
	}
	if app.RenderRegistry == nil {
		t.Fatal("score-k8s runtime has no Git render registry")
	}
	commitA, commitB := strings.Repeat("a", 40), strings.Repeat("b", 40)
	treeA := standardBundle(t)
	treeB := standardBundle(t)
	treeB["defaults.yaml"] = []byte(strings.NewReplacer("cpu: 10m", "cpu: 30m", "podAnnotations: {}", "podAnnotations:\n  example.com/render-policy: b",
		"podSecurityContext: {}", "podSecurityContext:\n  seccompProfile:\n    type: RuntimeDefault").Replace(string(treeB["defaults.yaml"])))
	git := &gitFixture{trees: map[string]registry.Tree{commitA: treeA, commitB: treeB}}
	app.RenderRegistry.Git = git

	render, err := scorek8s.New(ctx, cli, kubernetes.NewRenderer())
	if err != nil {
		t.Fatal(err)
	}
	catalogSvc := catalog.NewService(app.Store)
	catalogSvc.SetRenderBundles(render.Bundles())
	catalogSvc.SetRenderRegistry(app.RenderRegistry)
	url := "https://github.com/pr3s3nt/poc.git"
	a, err := catalogSvc.InspectRenderBundle(ctx, opts.OrganizationKey, renderregistry.Inspection{URL: url, Path: "backend/render-bundles/standard", Ref: commitA})
	if err != nil {
		t.Fatal(err)
	}
	b, err := catalogSvc.InspectRenderBundle(ctx, opts.OrganizationKey, renderregistry.Inspection{URL: url, Path: "backend/render-bundles/standard", Ref: commitB})
	if err != nil {
		t.Fatal(err)
	}
	disableSeededRenderers(t, app.Store, opts.OrganizationKey)
	variables := func(id string) map[string]any {
		return placedVariables(map[string]any{"render_bundle": id, "render_inputs": map[string]any{"team": "payments"}})
	}
	def := resource.Definition{Key: "render-git", Name: "render-git", ResourceTypeKey: "workload", DriverType: resource.DriverScoreK8s, Criteria: []resource.Criterion{{ApplicationID: opts.ApplicationKey}}, DriverInputs: map[string]any{"values": map[string]any{"variables": variables(a.ID)}}}
	saved, err := catalogSvc.RegisterResourceDefinition(ctx, opts.OrganizationKey, def)
	if err != nil {
		t.Fatal(err)
	}
	svc := pending.NewService(app.Store, planning.NewService(render.Bundles()), workloadconfig.NewService(app.Store), planning.RegistryInspector{Metadata: app.Modules})
	svc.SetDeployer(app.Deployments)
	fetches := git.calls

	// Explicit CPU on one container is kept; the missing memory uses the bundle default.
	score := scoreFor("backend", "backend:v1", false)
	score["containers"].(map[string]any)["main"].(map[string]any)["resources"] = map[string]any{"requests": map[string]any{"cpu": "250m"}}
	score["containers"].(map[string]any)["side"] = map[string]any{"image": "busybox:1"}
	deploy(t, app, opts, "backend", score)
	first := appliedDeployment(t, app)
	labels := first["metadata"].(map[string]any)["labels"].(map[string]any)
	if labels["example.com/team"] != "payments" || labels["app.kubernetes.io/managed-by"] != "orchestrator" {
		t.Fatalf("bundle A policy not applied: %v", labels)
	}
	resources := func(d map[string]any, name string) map[string]any {
		for _, c := range podOf(d)["spec"].(map[string]any)["containers"].([]any) {
			if c.(map[string]any)["name"] == name {
				return c.(map[string]any)["resources"].(map[string]any)["requests"].(map[string]any)
			}
		}
		t.Fatalf("container %s missing", name)
		return nil
	}
	if r := resources(first, "main"); r["cpu"] != "250m" || r["memory"] != "32Mi" {
		t.Fatalf("explicit request/default memory: %v", r)
	}
	if r := resources(first, "side"); r["cpu"] != "10m" {
		t.Fatalf("A fallback: %v", r)
	}

	idle, err := svc.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil || len(idle.Changes) != 0 {
		t.Fatalf("unchanged intent must be a no-op: %+v %v", idle.Changes, err)
	}
	def.DriverInputs = map[string]any{"values": map[string]any{"variables": variables(b.ID)}}
	if _, err := catalogSvc.UpdateResourceDefinition(ctx, opts.OrganizationKey, def.Key, saved.Version, def); err != nil {
		t.Fatal(err)
	}
	toB, err := svc.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(toB.Changes) != 1 || toB.Changes[0].WorkloadID != "backend" || toB.Changes[0].Rendering.Bundle.Source.Revision != commitB {
		t.Fatalf("bundle-only change not pending: %+v", toB.Changes)
	}
	if _, err := svc.Deploy(ctx, opts.ApplicationKey, opts.EnvironmentKey, "test", idle.Token); !errors.Is(err, pending.ErrStalePreview) {
		t.Fatalf("old preview accepted: %v", err)
	}
	if report, err := svc.Deploy(ctx, opts.ApplicationKey, opts.EnvironmentKey, "test", toB.Token); err != nil || report.Status != "SUCCEEDED" {
		t.Fatalf("deploy B: %+v %v", report, err)
	}
	second := appliedDeployment(t, app)
	if r := resources(second, "side"); r["cpu"] != "30m" {
		t.Fatalf("B fallback: %v", r)
	}
	if r := resources(second, "main"); r["cpu"] != "250m" {
		t.Fatalf("explicit request must survive B: %v", r)
	}
	if podOf(second)["metadata"].(map[string]any)["annotations"].(map[string]any)["example.com/render-policy"] != "b" {
		t.Fatal("B annotation missing")
	}
	if podOf(second)["spec"].(map[string]any)["securityContext"] == nil {
		t.Fatal("B securityContext missing")
	}
	if done, err := svc.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey); err != nil || len(done.Changes) != 0 {
		t.Fatalf("after B: %+v %v", done.Changes, err)
	}
	// Explicit re-selection of the older verified pin.
	current, _ := app.Store.ListResourceDefinitions(ctx, opts.OrganizationKey)
	var version int64
	for _, d := range current {
		if d.Key == def.Key {
			version = d.Version
		}
	}
	def.DriverInputs = map[string]any{"values": map[string]any{"variables": variables(a.ID)}}
	if _, err := catalogSvc.UpdateResourceDefinition(ctx, opts.OrganizationKey, def.Key, version, def); err != nil {
		t.Fatal(err)
	}
	back, err := svc.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil || len(back.Changes) != 1 {
		t.Fatalf("B->A: %+v %v", back.Changes, err)
	}
	if report, err := svc.Deploy(ctx, opts.ApplicationKey, opts.EnvironmentKey, "test", back.Token); err != nil || report.Status != "SUCCEEDED" {
		t.Fatalf("deploy A again: %+v %v", report, err)
	}
	third := appliedDeployment(t, app)
	if r := resources(third, "side"); r["cpu"] != "10m" {
		t.Fatalf("A restored: %v", r)
	}
	annotations, _ := podOf(third)["metadata"].(map[string]any)["annotations"].(map[string]any)
	if _, ok := annotations["example.com/render-policy"]; ok {
		t.Fatal("B annotation survived A")
	}
	if git.calls != fetches {
		t.Fatalf("Preview/Deploy fetched Git: %d calls", git.calls-fetches)
	}
}
