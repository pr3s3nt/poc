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
	"os"
	"os/exec"
	"path/filepath"
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

// copyCLI installs the real score-k8s under a new path. extra makes a different
// binary digest (trailing bytes after the executable image are ignored by the loader).
func copyCLI(t *testing.T, src, name, extra string) string {
	t.Helper()
	raw, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(dst, append(raw, []byte(extra)...), 0700); err != nil {
		t.Fatal(err)
	}
	return dst
}

// restart simulates a backend restart on the same persisted state with another
// installed renderer.
func restart(t *testing.T, state, binary string) (*bootstrap.App, *pending.Service, *scorek8s.Renderer, seed.Options) {
	t.Helper()
	ctx := context.Background()
	opts := seed.Defaults()
	app, err := bootstrap.Build(ctx, bootstrap.Options{Seed: opts, ScoreK8sPath: binary, StatePath: state})
	if err != nil {
		t.Fatal(err)
	}
	render, err := scorek8s.New(ctx, binary, kubernetes.NewRenderer())
	if err != nil {
		t.Fatal(err)
	}
	svc := pending.NewService(app.Store, planning.NewService(render.Bundles()), workloadconfig.NewService(app.Store), tf.NewInspector())
	svc.SetDeployer(app.Deployments)
	return app, svc, render, opts
}

// A Definition registered under bundle A keeps selecting the installed bundle
// after an upgrade to B. Tokens pinned to A are stale; a fresh B preview deploys.
func TestExistingDefinitionSurvivesRendererUpgrade(t *testing.T) {
	for _, withDraft := range []bool{false, true} {
		name := "renderer-only"
		if withDraft {
			name = "with-draft"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			cli, err := exec.LookPath("score-k8s")
			if err != nil {
				t.Skip("local score-k8s unavailable")
			}
			binaryA := copyCLI(t, cli, "score-k8s", "")
			binaryB := copyCLI(t, cli, "score-k8s", "\n# upgraded build B\n")
			state := filepath.Join(t.TempDir(), "state.json")
			appA, svcA, renderA, opts := restart(t, state, binaryA)
			catalogSvc := catalog.NewService(appA.Store)
			catalogSvc.SetRenderBundles(renderA.Bundles())
			def := resource.Definition{Key: "render", ResourceTypeKey: "workload", ExecutionProfile: "internal-k8s", DriverType: resource.DriverScoreK8s, Criteria: []resource.Criterion{{ApplicationID: opts.ApplicationKey}}, DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{"render_bundle": scorek8s.BundleID}}}}
			registered, err := catalogSvc.RegisterResourceDefinition(ctx, opts.OrganizationKey, def)
			if err != nil {
				t.Fatal(err)
			}
			deploy(t, appA, opts, "backend", scoreFor("backend", "backend:v1", false))
			var oldToken string
			if withDraft {
				if _, err := workloadconfig.NewService(appA.Store).Save(ctx, opts.ApplicationKey, opts.EnvironmentKey, "backend", scoreFor("backend", "backend:v2", false), 0); err != nil {
					t.Fatal(err)
				}
				p, err := svcA.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
				if err != nil || len(p.Changes) != 1 || p.Changes[0].Rendering.Bundle != renderA.Bundles()[scorek8s.BundleID] {
					t.Fatalf("preview on A: %+v %v", p, err)
				}
				oldToken = p.Token
			} else {
				p, err := svcA.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
				if err != nil || len(p.Changes) != 0 {
					t.Fatalf("expected no-op on A: %+v %v", p, err)
				}
			}

			// Upgrade A -> B on the same persisted Definition.
			appB, svcB, renderB, _ := restart(t, state, binaryB)
			bundleA, bundleB := renderA.Bundles()[scorek8s.BundleID], renderB.Bundles()[scorek8s.BundleID]
			if bundleA.Digest == bundleB.Digest || bundleA.BinaryDigest == bundleB.BinaryDigest {
				t.Fatal("test binaries do not differ")
			}
			if registered.SourceFingerpr != bundleA.Digest {
				t.Fatal("registration should record bundle A as provenance")
			}
			fresh, err := svcB.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
			if err != nil {
				t.Fatalf("existing Definition rejected after upgrade: %v", err)
			}
			if len(fresh.Changes) != 1 || fresh.Changes[0].Rendering.DefinitionKey != "render" || fresh.Changes[0].Rendering.Bundle != bundleB {
				t.Fatalf("fresh preview must pin installed bundle B: %+v", fresh.Changes)
			}
			if oldToken != "" {
				calls := len(appB.FakeExec.Calls)
				applies := len(appB.FakeDeploy.Applied)
				if _, err := svcB.Deploy(ctx, opts.ApplicationKey, opts.EnvironmentKey, "test", oldToken); !errors.Is(err, pending.ErrStalePreview) {
					t.Fatalf("token pinned to A accepted on B: %v", err)
				}
				if len(appB.FakeExec.Calls) != calls || len(appB.FakeDeploy.Applied) != applies {
					t.Fatal("stale A token had effects")
				}
			}
			report, err := svcB.Deploy(ctx, opts.ApplicationKey, opts.EnvironmentKey, "test", fresh.Token)
			if err != nil || report.Status != "SUCCEEDED" {
				t.Fatal(report, err)
			}
			done, err := svcB.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
			if err != nil || len(done.Changes) != 0 {
				t.Fatalf("B deployment not recorded: %+v %v", done, err)
			}
		})
	}
}
