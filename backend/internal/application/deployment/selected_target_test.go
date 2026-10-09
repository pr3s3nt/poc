package deployment_test

import (
	"context"
	"errors"
	"testing"

	"orchestrator/internal/adapters/fake"
	"orchestrator/internal/application/application"
	appsvc "orchestrator/internal/application/deployment"
	"orchestrator/internal/bootstrap"
	domain "orchestrator/internal/domain/application"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/seed"
)

// hostContextExecutor behaves like the existing-cluster adapter for a
// HOST_CONTEXT Connection: the cluster node reports the Connection's own
// cluster and kube context as the Target of every dependent node.
type hostContextExecutor struct{ credentialExecutor }

func (e *hostContextExecutor) Resolve(resource.DriverType) (execution.ResourceExecutor, error) {
	return e, nil
}

func (e *hostContextExecutor) Provision(ctx context.Context, req execution.ProvisionRequest) (execution.ProvisionResult, error) {
	if req.ResourceType == "k8s-cluster" && req.Connection.AuthenticationType == domain.AuthHostContext {
		e.mu.Lock()
		e.requests = append(e.requests, req)
		e.mu.Unlock()
		conn := req.Connection
		return execution.ProvisionResult{
			Outputs: map[string]any{"name": conn.ConfigString("cluster"), "endpoint": "https://" + conn.Key + ".invalid", "kubeContext": conn.ConfigString("kubeContext")},
			State:   map[string]any{"driver": "existing-cluster", "kubeContext": conn.ConfigString("kubeContext")},
			Target:  &execution.Target{Kind: "kubernetes", Context: conn.ConfigString("kubeContext"), ClusterName: conn.ConfigString("cluster")},
		}, nil
	}
	return e.credentialExecutor.Provision(ctx, req)
}

// selectedTargetApp creates an Application through UC-01 with a non-default
// Connection and returns it, with fake adapters that record every request.
func selectedTargetApp(t *testing.T, definitionConnection string) (*bootstrap.App, seed.Options, *hostContextExecutor, *targetDeployer, string) {
	t.Helper()
	executor := &hostContextExecutor{credentialExecutor: credentialExecutor{fake: fake.NewResourceExecutor()}}
	deployer := &targetDeployer{WorkloadDeployer: fake.NewWorkloadDeployer()}
	app, opts := newApp(t, func(o *bootstrap.Options) { o.RegistryOverride = executor; o.DeployerOverride = deployer })
	ctx := context.Background()
	if err := app.Store.SaveConnection(ctx, domain.Connection{ID: "conn-second", Key: "second", Name: "Second", OrganizationKey: opts.OrganizationKey, Kind: domain.ConnectionKubernetes, AuthenticationType: domain.AuthHostContext, Status: domain.ConnectionReady, Config: map[string]any{"cluster": "second-cluster", "kubeContext": "second"}, SecretRef: "host-kube-context://second"}); err != nil {
		t.Fatal(err)
	}
	if definitionConnection != "" {
		if err := app.Store.SaveResourceDefinition(ctx, opts.OrganizationKey, resource.Definition{
			Key: "cluster-second", ResourceTypeKey: "k8s-cluster", DriverType: resource.DriverExistingCluster,
			ExecutionProfile: "internal-k8s", ConnectionKey: definitionConnection,
			DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{"name": "${context.connection.cluster}", "kubeContext": "${context.connection.context}"}}},
			Criteria:     []resource.Criterion{{Class: "internal", ResourceID: "connections.second"}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	return app, opts, executor, deployer, newBoundApplication(t, app.Store, opts.OrganizationKey, "Second App", "second-app", "second", "second")
}

// newBoundApplication creates an Application through UC-01 and sets the given
// Connection on each Environment (staging, production) through the product
// set-once operation. Pass an empty key to leave that Environment unset.
func newBoundApplication(t *testing.T, store persistence.Store, organizationKey, name, subdomain, stagingKey, productionKey string) string {
	t.Helper()
	ctx := context.Background()
	svc := application.NewService(store)
	created, err := svc.Create(ctx, application.CreateCommand{OrganizationKey: organizationKey, Name: name, Subdomain: subdomain})
	if err != nil {
		t.Fatal(err)
	}
	for env, key := range map[string]string{"staging": stagingKey, "production": productionKey} {
		if key == "" {
			continue
		}
		if _, err := svc.SetConnection(ctx, application.SetConnectionCommand{OrganizationKey: organizationKey, ApplicationKey: created.Application.Key, EnvironmentKey: env, ConnectionKey: key, ExpectedVersion: 1}); err != nil {
			t.Fatal(err)
		}
	}
	return created.Application.Key
}

func deployBackendTo(app *bootstrap.App, opts seed.Options, applicationKey, environment string) error {
	scores := seed.AcceptanceScores(opts)
	_, err := app.Deployments.DeployWorkload(context.Background(), appsvc.DeployCommand{OrganizationKey: opts.OrganizationKey, ApplicationKey: applicationKey, EnvironmentKey: environment, WorkloadID: "backend", ScoreAfter: scores["backend"], Actor: "test", RunID: "run-1"})
	return err
}

// UC-06 BR-20: a Platform Engineer registered a matching cluster Definition,
// so both Environments deploy to the selected Connection.
func TestSelectedTarget_MatchingDefinitionDeploysToSelectedConnection(t *testing.T) {
	app, opts, executor, deployer, key := selectedTargetApp(t, "second")
	for _, env := range []string{"staging", "production"} {
		if err := deployBackendTo(app, opts, key, env); err != nil {
			t.Fatalf("deploy %s: %v", env, err)
		}
	}
	// Both Environments applied their workload to the selected cluster.
	if len(deployer.targets) != 2 {
		t.Fatalf("apply targets = %#v", deployer.targets)
	}
	for _, target := range deployer.targets {
		if target.Kind != "kubernetes" || target.Context != "second" || target.ClusterName != "second-cluster" {
			t.Fatalf("workload applied to %#v, want the selected second cluster", target)
		}
	}
	sawCluster := 0
	for _, req := range executor.requests {
		if req.ResourceType == "k8s-cluster" {
			sawCluster++
			if req.Connection.Key != "second" {
				t.Fatalf("cluster executed on %q", req.Connection.Key)
			}
		}
		if req.Connection.Key == "internal-cluster" {
			t.Fatalf("resource %s fell back to the default cluster", req.Descriptor)
		}
	}
	if sawCluster == 0 {
		t.Fatal("cluster node never executed")
	}
}

// ADR-013: no cluster Definition is needed; the Environment Connection backs the
// implicit builtin cluster, which is admitted before progress is written.
func TestSelectedTarget_NoClusterDefinitionNeeded(t *testing.T) {
	app, opts, executor, deployer, key := selectedTargetApp(t, "")
	if err := deployBackendTo(app, opts, key, "staging"); err != nil {
		t.Fatalf("deploy = %v", err)
	}
	assertBuiltinCluster(t, app, opts, executor, deployer)
}

// An authored cluster Definition for another Connection neither retargets nor
// rejects the implicit cluster.
func TestSelectedTarget_AuthoredDefinitionForAnotherConnectionIsIgnored(t *testing.T) {
	app, opts, executor, deployer, key := selectedTargetApp(t, "internal-cluster")
	if err := deployBackendTo(app, opts, key, "staging"); err != nil {
		t.Fatalf("deploy = %v", err)
	}
	assertBuiltinCluster(t, app, opts, executor, deployer)
}

func assertBuiltinCluster(t *testing.T, app *bootstrap.App, opts seed.Options, executor *hostContextExecutor, deployer *targetDeployer) {
	t.Helper()
	if len(executor.requests) == 0 || len(deployer.targets) == 0 {
		t.Fatalf("executor calls %d, applies %d", len(executor.requests), len(deployer.targets))
	}
	for _, req := range executor.requests {
		if req.ResourceType != "k8s-cluster" {
			continue
		}
		if req.Connection.Key != "second" || req.DefinitionKey != planning.BuiltinClusterKey {
			t.Fatalf("cluster ran with connection %q definition %q", req.Connection.Key, req.DefinitionKey)
		}
	}
	for _, target := range deployer.targets {
		if target.Context != "second" {
			t.Fatalf("workload applied to %#v", target)
		}
	}
	defs, err := app.Store.ListResourceDefinitions(context.Background(), opts.OrganizationKey)
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, d := range defs {
		if d.Key == planning.BuiltinClusterKey {
			found++
			if !planning.IsBuiltinClusterDefinition(d) {
				t.Fatalf("admitted definition differs from the trusted one: %+v", d)
			}
		}
	}
	if found != 1 {
		t.Fatalf("builtin definition admitted %d times", found)
	}
	active, err := app.Store.ListActiveResources(context.Background(), opts.OrganizationKey)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range active {
		if a.Descriptor.Type == "k8s-cluster" && a.DefinitionKey != planning.BuiltinClusterKey {
			t.Fatalf("active cluster definition = %q", a.DefinitionKey)
		}
	}
}

// registerSecondCluster saves a READY Kubernetes Connection with its matching
// cluster Definition so an Environment can select it.
func registerCluster(t *testing.T, app *bootstrap.App, organizationKey, key string) {
	t.Helper()
	ctx := context.Background()
	if err := app.Store.SaveConnection(ctx, domain.Connection{ID: "conn-" + key, Key: key, Name: key, OrganizationKey: organizationKey, Kind: domain.ConnectionKubernetes, AuthenticationType: domain.AuthHostContext, Status: domain.ConnectionReady, Config: map[string]any{"cluster": key + "-cluster", "kubeContext": key}, SecretRef: "host-kube-context://" + key}); err != nil {
		t.Fatal(err)
	}
	if err := app.Store.SaveResourceDefinition(ctx, organizationKey, resource.Definition{
		Key: "cluster-" + key, ResourceTypeKey: "k8s-cluster", DriverType: resource.DriverExistingCluster,
		ExecutionProfile: "internal-k8s", ConnectionKey: key,
		DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{"name": "${context.connection.cluster}", "kubeContext": "${context.connection.context}"}}},
		Criteria:     []resource.Criterion{{Class: "internal", ResourceID: "connections." + key}},
	}); err != nil {
		t.Fatal(err)
	}
}

// Each Environment of one Application deploys to its own selected Connection;
// nothing falls back to the Application, the Organization default or the other
// Environment (ADR-011).
func TestSelectedTarget_EnvironmentsDeployToTheirOwnConnectionIndependently(t *testing.T) {
	executor := &hostContextExecutor{credentialExecutor: credentialExecutor{fake: fake.NewResourceExecutor()}}
	deployer := &targetDeployer{WorkloadDeployer: fake.NewWorkloadDeployer()}
	app, opts := newApp(t, func(o *bootstrap.Options) { o.RegistryOverride = executor; o.DeployerOverride = deployer })
	registerCluster(t, app, opts.OrganizationKey, "stage-cluster")
	registerCluster(t, app, opts.OrganizationKey, "prod-cluster")
	key := newBoundApplication(t, app.Store, opts.OrganizationKey, "Split App", "split-app", "stage-cluster", "prod-cluster")

	for _, env := range []string{"staging", "production"} {
		before := len(deployer.targets)
		if err := deployBackendTo(app, opts, key, env); err != nil {
			t.Fatalf("deploy %s: %v", env, err)
		}
		want := map[string]string{"staging": "stage-cluster", "production": "prod-cluster"}[env]
		if len(deployer.targets) == before {
			t.Fatalf("%s applied nothing", env)
		}
		for _, target := range deployer.targets[before:] {
			if target.Context != want || target.ClusterName != want+"-cluster" {
				t.Fatalf("%s applied to %#v, want %s", env, target, want)
			}
		}
	}
	for _, req := range executor.requests {
		if req.Connection.Key == "internal-cluster" {
			t.Fatalf("%s executed on the Organization default", req.Descriptor)
		}
	}
}

// An UNCONFIGURED Environment fails deploy with the typed error before a
// Deployment, a plan, a provisioning call or any apply exists, while draft and
// configuration-style reads of the Environment keep working.
func TestUnconfiguredEnvironmentFailsDeployBeforeAnySideEffect(t *testing.T) {
	executor := &hostContextExecutor{credentialExecutor: credentialExecutor{fake: fake.NewResourceExecutor()}}
	deployer := &targetDeployer{WorkloadDeployer: fake.NewWorkloadDeployer()}
	app, opts := newApp(t, func(o *bootstrap.Options) { o.RegistryOverride = executor; o.DeployerOverride = deployer })
	key := newBoundApplication(t, app.Store, opts.OrganizationKey, "Unset App", "unset-app", "", "")

	err := deployBackendTo(app, opts, key, "staging")
	if !errors.Is(err, planning.ErrEnvironmentUnconfigured) {
		t.Fatalf("deploy = %v", err)
	}
	if len(executor.requests) != 0 || len(deployer.targets) != 0 || len(app.FakeDeploy.Applied) != 0 {
		t.Fatalf("side effects: %d executor calls, %d applies", len(executor.requests), len(app.FakeDeploy.Applied))
	}
	if deployments, _ := app.Store.ListDeployments(context.Background(), key, "staging"); len(deployments) != 0 {
		t.Fatalf("a Deployment was recorded: %+v", deployments)
	}
	if _, err := app.Store.GetEnvironment(context.Background(), key, "staging"); err != nil {
		t.Fatalf("environment still readable: %v", err)
	}
}

// A binding set after a preview changes the Environment version and the pinned
// target, so a deploy cannot reuse a plan made for another target.
func TestPlanHashDiffersBetweenTargetsOfTheSameWorkload(t *testing.T) {
	app, opts, _, _, _ := selectedTargetApp(t, "second")
	registerCluster(t, app, opts.OrganizationKey, "third")
	scores := seed.AcceptanceScores(opts)
	hashes := map[string]string{}
	for _, tc := range []struct{ name, subdomain, key string }{{"On Second", "on-second", "second"}, {"On Third", "on-third", "third"}} {
		appKey := newBoundApplication(t, app.Store, opts.OrganizationKey, tc.name, tc.subdomain, tc.key, "")
		result, err := app.Deployments.DeployWorkload(context.Background(), appsvc.DeployCommand{OrganizationKey: opts.OrganizationKey, ApplicationKey: appKey, EnvironmentKey: "staging", WorkloadID: "backend", ScoreAfter: scores["backend"], Actor: "test", RunID: "run-1"})
		if err != nil {
			t.Fatal(err)
		}
		hashes[tc.key] = result.PlanHash
	}
	if hashes["second"] == hashes["third"] {
		t.Fatalf("plans for different targets share a hash: %v", hashes)
	}
}
