package deployment_test

import (
	"context"
	"errors"
	"testing"

	"orchestrator/internal/adapters/fake"
	"orchestrator/internal/bootstrap"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/ports/execution"
)

// failFirstCluster fails the first k8s-cluster provisioning, then delegates to the fake executor.
type failFirstCluster struct {
	inner  *fake.ResourceExecutor
	failed bool
}

func (r *failFirstCluster) Resolve(resource.DriverType) (execution.ResourceExecutor, error) {
	return r, nil
}

func (r *failFirstCluster) Provision(ctx context.Context, req execution.ProvisionRequest) (execution.ProvisionResult, error) {
	if req.ResourceType == "k8s-cluster" && !r.failed {
		r.failed = true
		return execution.ProvisionResult{}, errors.New("cluster unreachable")
	}
	return r.inner.Provision(ctx, req)
}

// A deploy that fails at the existing cluster leaves no pinned target behind; the retry succeeds on
// the same cluster Definition and account (the original builtin-cluster retry guarantee, on the
// Definition-placed path).
func TestPlacedDeploy_FailedClusterDeployRetrySucceedsOnTheSamePlacement(t *testing.T) {
	registry := &failFirstCluster{inner: fake.NewResourceExecutor()}
	f := newPlacedApp(t, func(o *bootstrap.Options) { o.RegistryOverride = registry })
	score := f.frontend("example.invalid/frontend:one")
	if err := f.deploy(nil, score); err == nil {
		t.Fatal("the first deploy must fail at the cluster")
	}
	if len(f.deployer.targets) != 0 {
		t.Fatalf("nothing may be applied after a failed cluster: %+v", f.deployer.targets)
	}
	if err := f.deploy(nil, score); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if len(f.deployer.targets) != 1 || f.deployer.targets[0].Connection != "lab" {
		t.Fatalf("the retry must apply on the cluster Definition's account: %+v", f.deployer.targets)
	}
	active, _ := f.app.Store.ListActiveResources(context.Background(), f.opts.OrganizationKey)
	clusters := 0
	for _, a := range active {
		if a.Descriptor.Type == "k8s-cluster" {
			clusters++
			if a.DefinitionKey != "cluster-lab" || a.ConnectionKey != "lab" {
				t.Fatalf("cluster Active Resource = %q on %q", a.DefinitionKey, a.ConnectionKey)
			}
		}
	}
	if clusters != 1 {
		t.Fatalf("cluster Active Resources = %d", clusters)
	}
}

// A JSON-reopened store keeps the Definitions, the account pins and the Active cluster identity: a
// retry-shaped redeploy and a removal after the reopen still work on the original target.
func TestPlacedDeploy_SurvivesJSONReopenForRetryAndRemove(t *testing.T) {
	f := placedApp(t)
	score := f.frontend("example.invalid/frontend:one")
	if err := f.deploy(nil, score); err != nil {
		t.Fatal(err)
	}
	f.app.Close()
	reopened := newPlacedApp(t, func(o *bootstrap.Options) { o.StatePath = f.state })
	if err := reopened.deploy(score, score); err != nil {
		t.Fatalf("redeploy after reopen: %v", err)
	}
	active, _ := reopened.app.Store.ListActiveResources(context.Background(), reopened.opts.OrganizationKey)
	clusters := 0
	for _, a := range active {
		if a.Descriptor.Type == "k8s-cluster" {
			clusters++
			if a.DefinitionKey != "cluster-lab" {
				t.Fatalf("cluster definition = %q", a.DefinitionKey)
			}
		}
	}
	if clusters != 1 {
		t.Fatalf("cluster Active Resources = %d", clusters)
	}
	if err := reopened.deploy(score, nil); err != nil {
		t.Fatalf("remove after reopen: %v", err)
	}
	last := reopened.deployer.targets[len(reopened.deployer.targets)-1]
	if last.Connection != "lab" {
		t.Fatalf("the removal must use the original target: %+v", last)
	}
}
