package deployment_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"orchestrator/internal/adapters/fake"
	appsvc "orchestrator/internal/application/deployment"
	"orchestrator/internal/bootstrap"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/seed"
)

// credentialExecutor behaves like the existing-cluster adapter for a
// KUBECONFIG Connection: it returns only the opaque Organization/Connection
// identity. Every other node uses the fake executor. It records the targets
// handed to dependent nodes.
type credentialExecutor struct {
	mu       sync.Mutex
	fake     *fake.ResourceExecutor
	requests []execution.ProvisionRequest
}

func (e *credentialExecutor) Resolve(resource.DriverType) (execution.ResourceExecutor, error) {
	return e, nil
}

func (e *credentialExecutor) Provision(ctx context.Context, req execution.ProvisionRequest) (execution.ProvisionResult, error) {
	e.mu.Lock()
	e.requests = append(e.requests, req)
	e.mu.Unlock()
	if req.ResourceType == "k8s-cluster" && req.Connection.CredentialBacked() {
		conn := req.Connection
		return execution.ProvisionResult{
			Outputs: map[string]any{"name": conn.ConfigString("cluster"), "endpoint": conn.ConfigString("endpoint"), "kubeContext": conn.ConfigString("kubeContext")},
			State:   map[string]any{"driver": "existing-cluster", "organization": req.OrganizationKey, "connection": conn.Key},
			Target:  &execution.Target{Kind: "kubernetes", Context: conn.ConfigString("kubeContext"), ClusterName: conn.ConfigString("cluster"), Organization: req.OrganizationKey, Connection: conn.Key},
		}, nil
	}
	return e.fake.Provision(ctx, req)
}

type targetDeployer struct {
	*fake.WorkloadDeployer
	mu      sync.Mutex
	targets []execution.Target
}

func (d *targetDeployer) record(target execution.Target) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.targets = append(d.targets, target)
}

func (d *targetDeployer) Apply(ctx context.Context, target execution.Target, manifests []execution.Manifest) error {
	d.record(target)
	return d.WorkloadDeployer.Apply(ctx, target, manifests)
}

func (d *targetDeployer) Remove(ctx context.Context, target execution.Target, workloadID string) error {
	d.record(target)
	return d.WorkloadDeployer.Remove(ctx, target, workloadID)
}

type targetRoutes struct{ targets []execution.Target }

func (r *targetRoutes) Reconcile(_ context.Context, target execution.Target, _ execution.PublicRoute) error {
	r.targets = append(r.targets, target)
	return nil
}

// credentialBackedApp registers a READY KUBECONFIG Connection and an
// existing-cluster Definition matching only the acceptance Application
// (UC-03 consumption). The seed Definition and Organization default stay as
// they are.
func credentialBackedApp(t *testing.T, status application.ConnectionStatus) (*bootstrap.App, seed.Options, *credentialExecutor, *targetDeployer) {
	t.Helper()
	executor := &credentialExecutor{fake: fake.NewResourceExecutor()}
	deployer := &targetDeployer{WorkloadDeployer: fake.NewWorkloadDeployer()}
	app, opts := newApp(t, func(o *bootstrap.Options) {
		o.RegistryOverride = executor
		o.DeployerOverride = deployer
	})
	ctx := context.Background()
	if err := app.Store.SaveConnection(ctx, application.Connection{
		Key: "lab", Name: "Lab", OrganizationKey: opts.OrganizationKey, Kind: application.ConnectionKubernetes,
		AuthenticationType: application.AuthKubeconfig, Status: application.ConnectionReady,
		Config:    map[string]any{"cluster": "lab-cluster", "kubeContext": "lab", "endpoint": "https://lab.example"},
		SecretRef: "memory://local/orchestrator/connections/acme/lab/credentials/00000000-0000-4000-8000-000000000000",
	}); err != nil {
		t.Fatal(err)
	}
	// A UC-01 Application whose staging Environment selected the Connection;
	// the seeded acceptance binding is immutable and stays untouched.
	opts.ApplicationKey, opts.EnvironmentKey = newBoundApplication(t, app.Store, opts.OrganizationKey, "Lab App", "lab-app", "lab", ""), "staging"
	if status != application.ConnectionReady {
		// The Connection degrades after the Environment selected it.
		stored, err := app.Store.GetConnection(ctx, opts.OrganizationKey, "lab")
		if err != nil {
			t.Fatal(err)
		}
		stored.Status = status
		if err := app.Store.SaveConnection(ctx, stored); err != nil {
			t.Fatal(err)
		}
	}
	if err := app.Store.SaveResourceDefinition(ctx, opts.OrganizationKey, resource.Definition{
		Key: "cluster-lab", ResourceTypeKey: "k8s-cluster", DriverType: resource.DriverExistingCluster,
		ExecutionProfile: "internal-k8s", ConnectionKey: "lab",
		DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{"name": "${context.connection.cluster}", "kubeContext": "${context.connection.context}"}}},
		Criteria:     []resource.Criterion{{Class: "internal", ApplicationID: opts.ApplicationKey}},
	}); err != nil {
		t.Fatal(err)
	}
	return app, opts, executor, deployer
}

func TestCredentialBackedTarget_PropagatesOpaqueIdentityThroughDeployAndRemove(t *testing.T) {
	ctx := context.Background()
	app, opts, executor, deployer := credentialBackedApp(t, application.ConnectionReady)
	routes := &targetRoutes{}
	app.Deployments.SetPublicRouteManager(routes, "example.com")
	scores := seed.AcceptanceScores(opts)
	// The second run updates the workload. The executor runs again for the
	// same Active Resource identities; this proves identity propagation on a
	// rerun, not that execution is skipped.
	for _, before := range []map[string]any{nil, scores["backend"]} {
		if _, err := app.Deployments.DeployWorkload(ctx, appsvc.DeployCommand{OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey, WorkloadID: "backend", ScoreBefore: before, ScoreAfter: scores["backend"], Actor: "test"}); err != nil {
			t.Fatal(err)
		}
	}
	for _, req := range executor.requests {
		if req.ResourceType == "k8s-namespace" || req.ResourceType == "postgres" {
			if req.Target.Organization != opts.OrganizationKey || req.Target.Connection != "lab" || req.Target.Context != "lab" {
				t.Fatalf("%s target lost the connection identity: %#v", req.ResourceType, req.Target)
			}
		}
	}
	if len(deployer.targets) != 2 || deployer.targets[1].Connection != "lab" || deployer.targets[1].Organization != opts.OrganizationKey {
		t.Fatalf("apply targets: %#v", deployer.targets)
	}
	instances, err := app.Store.ListWorkloadInstances(ctx, opts.ApplicationKey+"/"+opts.EnvironmentKey)
	if err != nil || len(instances) != 1 {
		t.Fatalf("instances: %v %d", err, len(instances))
	}
	ref := instances[0].TargetRef
	if ref["connection"] != "lab" || ref["organization"] != opts.OrganizationKey || ref["context"] != "lab" {
		t.Fatalf("TargetRef: %#v", ref)
	}
	active, _ := app.Store.ListActiveResources(ctx, opts.OrganizationKey)
	for _, item := range active {
		if item.Descriptor.Type == "k8s-cluster" && item.ConnectionKey != "lab" {
			t.Fatalf("cluster Active Resource connection: %q", item.ConnectionKey)
		}
	}
	persisted, _ := json.Marshal(map[string]any{"instances": instances, "active": active})
	for _, leak := range []string{"memory://", "orch-kube-", "kubeconfig\":\"/"} {
		if strings.Contains(string(persisted), leak) {
			t.Fatalf("persisted state holds %q: %s", leak, persisted)
		}
	}

	// Removal in the same process rebuilds the target from TargetRef; the
	// restart case is TestCredentialBackedTarget_RemovalAfterRestart.
	if _, err := app.Deployments.DeployWorkload(ctx, appsvc.DeployCommand{OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey, WorkloadID: "backend", ScoreBefore: scores["backend"], Actor: "test"}); err != nil {
		t.Fatal(err)
	}
	removed := deployer.targets[len(deployer.targets)-1]
	if removed.Connection != "lab" || removed.Organization != opts.OrganizationKey || removed.Context != "lab" {
		t.Fatalf("remove target: %#v", removed)
	}
	if len(routes.targets) == 0 {
		t.Fatal("public routes were not reconciled")
	}
	for _, target := range routes.targets {
		if target.Connection != "lab" {
			t.Fatalf("route target lost identity: %#v", target)
		}
	}
}

func TestCredentialBackedTarget_NotReadyConnectionFailsBeforeExecution(t *testing.T) {
	ctx := context.Background()
	app, opts, executor, deployer := credentialBackedApp(t, application.ConnectionVerifying)
	scores := seed.AcceptanceScores(opts)
	_, err := app.Deployments.DeployWorkload(ctx, appsvc.DeployCommand{OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey, WorkloadID: "backend", ScoreAfter: scores["backend"], Actor: "test"})
	if err == nil || !(strings.Contains(err.Error(), "unavailable") || strings.Contains(err.Error(), "not READY")) {
		t.Fatalf("deploy with a non-READY connection: %v", err)
	}
	for _, req := range executor.requests {
		if req.ResourceType == "k8s-cluster" {
			t.Fatalf("executor ran for a non-READY connection: %#v", req.Connection.Key)
		}
	}
	if len(deployer.targets) != 0 {
		t.Fatal("workload applied")
	}
}
