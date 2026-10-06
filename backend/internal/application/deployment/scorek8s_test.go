package deployment_test

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"path/filepath"
	"testing"

	"orchestrator/internal/adapters/kubernetes"
	"orchestrator/internal/adapters/scorek8s"
	"orchestrator/internal/adapters/store"
	"orchestrator/internal/application/catalog"
	appsvc "orchestrator/internal/application/deployment"
	"orchestrator/internal/application/preview"
	"orchestrator/internal/bootstrap"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/seed"
)

func TestScoreK8sDefinitionPreviewDeployAndSnapshot(t *testing.T) {
	ctx := context.Background()
	path, err := exec.LookPath("score-k8s")
	if err != nil {
		t.Skip("local score-k8s unavailable")
	}
	state := filepath.Join(t.TempDir(), "state.json")
	app, opts := newApp(t, func(o *bootstrap.Options) { o.ScoreK8sPath = path; o.StatePath = state })
	renderer, err := scorek8s.New(ctx, path, kubernetes.NewRenderer())
	if err != nil {
		t.Fatal(err)
	}
	catalogSvc := catalog.NewService(app.Store)
	catalogSvc.SetRenderBundles(renderer.Bundles())
	def := resource.Definition{Key: "score-workloads", ResourceTypeKey: "workload", ExecutionProfile: "internal-k8s", DriverType: resource.DriverScoreK8s, DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{"render_bundle": scorek8s.BundleID}}}, Criteria: []resource.Criterion{{ApplicationID: opts.ApplicationKey}}}
	registered, err := catalogSvc.RegisterResourceDefinition(ctx, opts.OrganizationKey, def)
	if err != nil {
		t.Fatal(err)
	}
	if registered.SourceFingerpr == "" {
		t.Fatal("missing bundle pin")
	}
	scores := seed.AcceptanceScores(opts)
	for _, id := range seed.AcceptanceOrder() {
		p, err := app.Previews.PreviewDeployment(ctx, preview.PreviewDeploymentQuery{OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey, WorkloadID: id, Action: "deploy", RunID: "render-test", ScoreAfter: scores[id]})
		if err != nil {
			t.Fatal(err)
		}
		if p.Plan.Rendering[id].DefinitionKey != def.Key {
			t.Fatalf("missing renderer selection for %s", id)
		}
		calls := len(app.FakeExec.Calls)
		applies := len(app.FakeDeploy.Applied)
		again, err := app.Previews.PreviewDeployment(ctx, preview.PreviewDeploymentQuery{OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey, WorkloadID: id, Action: "deploy", RunID: "render-test", ScoreAfter: scores[id]})
		if err != nil || again.Plan.PlanHash != p.Plan.PlanHash {
			t.Fatal("preview is not deterministic", err)
		}
		if len(app.FakeExec.Calls) != calls || len(app.FakeDeploy.Applied) != applies {
			t.Fatal("preview executed side effects")
		}
		result, err := app.Deployments.DeployWorkload(ctx, appsvc.DeployCommand{OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey, WorkloadID: id, ScoreAfter: scores[id], RunID: "render-test", ExpectedPlanHash: p.Plan.PlanHash})
		if err != nil {
			t.Fatal(err)
		}
		reopened, err := store.NewWithSnapshot(state)
		if err != nil {
			t.Fatal(err)
		}
		snapshot, err := reopened.GetPlan(ctx, result.DeploymentID)
		if err != nil {
			t.Fatal(err)
		}
		selections, _ := snapshot["rendering"].(map[string]any)
		if selections[id] == nil {
			t.Fatal("renderer pin lost on reopen")
		}
	}
	active, err := app.Store.ListActiveResources(ctx, opts.OrganizationKey)
	if err != nil {
		t.Fatal(err)
	}
	dbs := 0
	for _, a := range active {
		if a.Descriptor.Type == "postgres" {
			dbs++
		}
		if a.Descriptor.Type == "workload" {
			t.Fatal("workload entered provisioning")
		}
	}
	if dbs != 1 {
		t.Fatalf("shared database count %d", dbs)
	}
	// A newly registered, more-specific rendering Definition invalidates a previously calculated plan.
	id := "backend"
	query := preview.PreviewDeploymentQuery{OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey, WorkloadID: id, Action: "update", RunID: "render-test", ScoreBefore: scores[id], ScoreAfter: scores[id]}
	p, err := app.Previews.PreviewDeployment(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	def.Key = "score-specific"
	def.Criteria = []resource.Criterion{{ApplicationID: opts.ApplicationKey, EnvironmentID: opts.EnvironmentKey}}
	if _, err := catalogSvc.RegisterResourceDefinition(ctx, opts.OrganizationKey, def); err != nil {
		t.Fatal(err)
	}
	calls := len(app.FakeExec.Calls)
	applies := len(app.FakeDeploy.Applied)
	_, err = app.Deployments.DeployWorkload(ctx, appsvc.DeployCommand{OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey, WorkloadID: id, ScoreBefore: scores[id], ScoreAfter: scores[id], RunID: "render-test", ExpectedPlanHash: p.Plan.PlanHash})
	if !errors.Is(err, appsvc.ErrStalePlan) {
		t.Fatalf("expected stale plan: %v", err)
	}
	if len(app.FakeExec.Calls) != calls || len(app.FakeDeploy.Applied) != applies {
		t.Fatal("stale plan caused effects")
	}
	for _, applied := range app.FakeDeploy.Applied {
		for _, m := range applied.Manifests {
			if m.Kind == "StatefulSet" || m.Kind == "PersistentVolumeClaim" || m.Kind == "Namespace" {
				t.Fatal("renderer generated infrastructure")
			}
			if m.Kind == "Deployment" {
				raw, _ := json.Marshal(m.Object)
				if len(raw) == 0 {
					t.Fatal("empty workload")
				}
			}
		}
	}
}
