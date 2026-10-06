package deployment_test

import (
	"context"
	"orchestrator/internal/adapters/kubernetes"
	"orchestrator/internal/adapters/scorek8s"
	"orchestrator/internal/application/catalog"
	appsvc "orchestrator/internal/application/deployment"
	"orchestrator/internal/bootstrap"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/seed"
	"os"
	"path/filepath"
	"testing"
)

func TestRenderingPreflightAndGenerationFailureDoNotCommit(t *testing.T) {
	for _, mode := range []string{"binary-changed", "generation-failed"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			binary := filepath.Join(t.TempDir(), "score-k8s")
			script := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo 'score-k8s 0.15.0 (test)'; exit 0; fi\nexit 1\n"
			if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			app, opts := newApp(t, func(o *bootstrap.Options) { o.ScoreK8sPath = binary })
			renderer, err := scorek8s.New(ctx, binary, kubernetes.NewRenderer())
			if err != nil {
				t.Fatal(err)
			}
			svc := catalog.NewService(app.Store)
			svc.SetRenderBundles(renderer.Bundles())
			def := resource.Definition{Key: "render", ResourceTypeKey: "workload", ExecutionProfile: "internal-k8s", DriverType: resource.DriverScoreK8s, Criteria: []resource.Criterion{{}}, DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{"render_bundle": scorek8s.BundleID}}}}
			if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, def); err != nil {
				t.Fatal(err)
			}
			before, _ := app.Store.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)
			if mode == "binary-changed" {
				if err := os.WriteFile(binary, []byte(script+"# changed\n"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			_, err = app.Deployments.DeployWorkload(ctx, appsvc.DeployCommand{OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey, WorkloadID: "backend", ScoreAfter: seed.AcceptanceScores(opts)["backend"]})
			if err == nil {
				t.Fatal("renderer failure accepted")
			}
			if len(app.FakeDeploy.Applied) != 0 {
				t.Fatal("generation failure applied workload")
			}
			if mode == "binary-changed" && len(app.FakeExec.Calls) != 0 {
				t.Fatal("preflight failure provisioned infrastructure")
			}
			after, _ := app.Store.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)
			if after.CurrentDeploymentSetID != before.CurrentDeploymentSetID || after.Version != before.Version {
				t.Fatal("render failure committed candidate")
			}
		})
	}
}
