package deployment_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"orchestrator/internal/adapters/fake"
	appsvc "orchestrator/internal/application/deployment"
	"orchestrator/internal/bootstrap"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/seed"
)

// ADR-013: a JSON-reopened store keeps the admitted system Definition and
// Active cluster identity; update (retry-shaped redeploy) and remove still work.
func TestBuiltinCluster_SurvivesJSONReopenForRetryAndRemove(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.json")
	withState := func(o *bootstrap.Options) { o.StatePath = path }
	app, opts := newApp(t, withState)
	scores := seed.AcceptanceScores(opts)
	cmd := func(a *bootstrap.App, before, after map[string]any) error {
		_, err := a.Deployments.DeployWorkload(ctx, appsvc.DeployCommand{OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey, WorkloadID: "backend", ScoreBefore: before, ScoreAfter: after, Actor: "test", RunID: "run-1"})
		return err
	}
	if err := cmd(app, nil, scores["backend"]); err != nil {
		t.Fatal(err)
	}

	reopened, _ := newApp(t, withState)
	defs, err := reopened.Store.ListResourceDefinitions(ctx, opts.OrganizationKey)
	if err != nil {
		t.Fatal(err)
	}
	stored := 0
	for _, d := range defs {
		if d.Key == planning.BuiltinClusterKey && planning.IsBuiltinClusterDefinition(d) {
			stored++
		}
	}
	if stored != 1 {
		t.Fatalf("reopened store holds %d system definitions", stored)
	}
	// Retry-shaped redeploy reuses the same Active cluster identity.
	if err := cmd(reopened, scores["backend"], scores["backend"]); err != nil {
		t.Fatalf("redeploy after reopen: %v", err)
	}
	active, _ := reopened.Store.ListActiveResources(ctx, opts.OrganizationKey)
	clusters := 0
	for _, a := range active {
		if a.Descriptor.Type == "k8s-cluster" {
			clusters++
			if a.DefinitionKey != planning.BuiltinClusterKey {
				t.Fatalf("cluster definition = %q", a.DefinitionKey)
			}
		}
	}
	if clusters != 1 {
		t.Fatalf("cluster Active Resources = %d", clusters)
	}
	if err := cmd(reopened, scores["backend"], nil); err != nil {
		t.Fatalf("remove after reopen: %v", err)
	}
}

// flakyClusterRegistry fails the first k8s-cluster provisioning, then delegates.
type flakyClusterRegistry struct {
	inner  *fake.ResourceExecutor
	failed bool
}

func (r *flakyClusterRegistry) Resolve(resource.DriverType) (execution.ResourceExecutor, error) {
	return r, nil
}

func (r *flakyClusterRegistry) Provision(ctx context.Context, req execution.ProvisionRequest) (execution.ProvisionResult, error) {
	if req.ResourceType == "k8s-cluster" && !r.failed {
		r.failed = true
		return execution.ProvisionResult{}, errors.New("cluster unreachable")
	}
	return r.inner.Provision(ctx, req)
}

// Failed-deploy retry: the first run fails at the builtin cluster after the
// system Definition was admitted; the retry succeeds on the same identity.
func TestBuiltinCluster_FailedDeployRetrySucceeds(t *testing.T) {
	ctx := context.Background()
	registry := &flakyClusterRegistry{inner: fake.NewResourceExecutor()}
	app, opts := newApp(t, func(o *bootstrap.Options) { o.RegistryOverride = registry })
	cmd := appsvc.DeployCommand{OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey, WorkloadID: "backend", ScoreAfter: seed.AcceptanceScores(opts)["backend"], Actor: "test", RunID: "run-1"}
	if _, err := app.Deployments.DeployWorkload(ctx, cmd); err == nil {
		t.Fatal("first deploy must fail at the cluster")
	}
	defs, _ := app.Store.ListResourceDefinitions(ctx, opts.OrganizationKey)
	admitted := false
	for _, d := range defs {
		admitted = admitted || d.Key == planning.BuiltinClusterKey
	}
	if !admitted {
		t.Fatal("system definition not admitted before the failed progress write")
	}
	if _, err := app.Deployments.DeployWorkload(ctx, cmd); err != nil {
		t.Fatalf("retry: %v", err)
	}
	active, _ := app.Store.ListActiveResources(ctx, opts.OrganizationKey)
	for _, a := range active {
		if a.Descriptor.Type == "k8s-cluster" && a.DefinitionKey != planning.BuiltinClusterKey {
			t.Fatalf("cluster definition = %q", a.DefinitionKey)
		}
	}
}
