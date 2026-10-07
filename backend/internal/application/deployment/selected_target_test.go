package deployment_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"orchestrator/internal/adapters/fake"
	"orchestrator/internal/application/application"
	appsvc "orchestrator/internal/application/deployment"
	"orchestrator/internal/bootstrap"
	domain "orchestrator/internal/domain/application"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning"
	"orchestrator/internal/ports/execution"
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
	key := "second"
	created, err := application.NewService(app.Store).Create(ctx, application.CreateCommand{OrganizationKey: opts.OrganizationKey, Name: "Second App", Subdomain: "second-app", ConnectionKey: &key})
	if err != nil {
		t.Fatal(err)
	}
	return app, opts, executor, deployer, created.Application.Key
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

// Without a matching Definition the seed internal-cluster Definition would
// silently retarget the Application; planning must refuse before any executor.
func TestSelectedTarget_MissingMatchingDefinitionFailsBeforeExecutorOrApply(t *testing.T) {
	app, opts, executor, deployer, key := selectedTargetApp(t, "")
	err := deployBackendTo(app, opts, key, "staging")
	if err == nil || !errors.Is(err, planning.ErrConnectionMismatch) {
		t.Fatalf("deploy = %v", err)
	}
	if len(executor.requests) != 0 || len(deployer.targets) != 0 || len(app.FakeDeploy.Applied) != 0 {
		t.Fatalf("side effects before planning rejection: %d executor calls, %d applies", len(executor.requests), len(app.FakeDeploy.Applied))
	}
}

func TestSelectedTarget_DefinitionForAnotherConnectionIsRejected(t *testing.T) {
	app, opts, executor, deployer, key := selectedTargetApp(t, "internal-cluster")
	err := deployBackendTo(app, opts, key, "staging")
	if err == nil || !errors.Is(err, planning.ErrConnectionMismatch) || strings.Contains(err.Error(), "memory://") {
		t.Fatalf("deploy = %v", err)
	}
	if len(executor.requests) != 0 || len(deployer.targets) != 0 {
		t.Fatalf("executor ran %d times, applies %d", len(executor.requests), len(deployer.targets))
	}
}
