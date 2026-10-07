package provisioning_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"orchestrator/internal/adapters/secrets"
	appsvc "orchestrator/internal/application/deployment"
	"orchestrator/internal/application/provisioning"
	"orchestrator/internal/bootstrap"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning"
	"orchestrator/internal/platform/clock"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/seed"
)

func deployBackend(t *testing.T, profile application.ExecutionProfile) (*bootstrap.App, seed.Options, string, string) {
	t.Helper()
	seedOptions := seed.Defaults()
	seedOptions.Region = "us-east-1"
	seedOptions.AccountID = "000000000000"
	applicationKey := seedOptions.ApplicationKey
	if profile == application.ProfileAWSEKS {
		applicationKey = seedOptions.CloudApplicationKey
	}
	app, err := bootstrap.Build(context.Background(), bootstrap.Options{Seed: seedOptions, Adapters: bootstrap.AdapterFake})
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	scores := seed.AcceptanceScores(seedOptions)
	result, err := app.Deployments.DeployWorkload(context.Background(), appsvc.DeployCommand{
		OrganizationKey: seedOptions.OrganizationKey,
		ApplicationKey:  applicationKey,
		EnvironmentKey:  seedOptions.EnvironmentKey,
		WorkloadID:      "backend",
		ScoreAfter:      scores["backend"],
		Actor:           "test",
		RunID:           "run-test",
	})
	if err != nil {
		t.Fatalf("deploy backend on %s: %v", profile, err)
	}
	return app, seedOptions, applicationKey, result.DeploymentID
}

func TestProvision_PostgresOutputContractEquivalentAcrossProfiles(t *testing.T) {
	ctx := context.Background()
	want := []string{"database", "host", "password", "port", "username"}

	for _, profile := range []application.ExecutionProfile{application.ProfileInternalK8s, application.ProfileAWSEKS} {
		app, seedOptions, applicationKey, deploymentID := deployBackend(t, profile)
		view, err := app.Queries.GetDeployment(ctx, appsvc.GetDeploymentQuery{OrganizationKey: seedOptions.OrganizationKey, ApplicationKey: applicationKey, EnvironmentKey: seedOptions.EnvironmentKey, DeploymentID: deploymentID})
		if err != nil {
			t.Fatalf("get deployment: %v", err)
		}
		found := false
		for _, res := range view.Resources {
			if res.ResourceType != "postgres" {
				continue
			}
			found = true
			for _, key := range want {
				if _, ok := res.Outputs[key]; !ok {
					t.Fatalf("%s postgres outputs are missing %q: %v", profile, key, res.Outputs)
				}
			}
		}
		if !found {
			t.Fatalf("%s deployment has no postgres resource", profile)
		}
	}
}

func TestProvision_ReusesApplicationScopedVPCAndEKS(t *testing.T) {
	ctx := context.Background()
	app, seedOptions, applicationKey, _ := deployBackend(t, application.ProfileAWSEKS)

	scores := seed.AcceptanceScores(seedOptions)
	if _, err := app.Deployments.DeployWorkload(ctx, appsvc.DeployCommand{
		OrganizationKey: seedOptions.OrganizationKey,
		ApplicationKey:  applicationKey,
		EnvironmentKey:  seedOptions.EnvironmentKey,
		WorkloadID:      "worker",
		ScoreAfter:      scores["worker"],
		Actor:           "test",
		RunID:           "run-test",
	}); err != nil {
		t.Fatalf("deploy worker: %v", err)
	}

	active, err := app.Store.ListActiveResources(ctx, seedOptions.OrganizationKey)
	if err != nil {
		t.Fatalf("list active resources: %v", err)
	}
	counts := map[string]int{}
	for _, a := range active {
		counts[a.Descriptor.Type]++
	}
	if counts["vpc"] != 1 || counts["k8s-cluster"] != 1 {
		t.Fatalf("an Application must keep one VPC and one EKS: %v", counts)
	}
	for _, a := range active {
		if a.Descriptor.Type == "vpc" && a.Version < 2 {
			t.Fatalf("the second deployment must reconcile the same VPC row, version %d", a.Version)
		}
	}
}

// countingRegistry records every executor call so tests prove none happened.
type countingRegistry struct{ calls int }

func (r *countingRegistry) Resolve(resource.DriverType) (execution.ResourceExecutor, error) {
	r.calls++
	return nil, errors.New("executor must not be resolved")
}

// TestProvision_InternalDefinitionConnectionMismatchFailsBeforeExecutor covers
// UC-08 BR-07 defense in depth: a plan that bypassed planning cannot retarget
// the Environment's saved Connection, and external accounts keep their
// explicit Driver Account semantics.
func TestProvision_InternalDefinitionConnectionMismatchFailsBeforeExecutor(t *testing.T) {
	ctx := context.Background()
	seedOptions := seed.Defaults()
	app, err := bootstrap.Build(ctx, bootstrap.Options{Seed: seedOptions, Adapters: bootstrap.AdapterFake})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"lab", "other"} {
		if err := app.Store.SaveConnection(ctx, application.Connection{ID: key, Key: key, OrganizationKey: seedOptions.OrganizationKey, Kind: application.ConnectionKubernetes, Status: application.ConnectionReady, AuthenticationType: application.AuthHostContext}); err != nil {
			t.Fatal(err)
		}
	}
	descriptor := "k8s-cluster.internal#connections.lab"
	for _, driver := range []resource.DriverType{resource.DriverExistingCluster, resource.DriverKubernetes} {
		registry := &countingRegistry{}
		service := provisioning.NewService(app.Store, registry, secrets.NewMemory(), clock.System{})
		def := resource.Definition{Key: "cluster-other", ResourceTypeKey: "k8s-cluster", DriverType: driver, ConnectionKey: "other", ExecutionProfile: "internal-k8s", Criteria: []resource.Criterion{{}}}
		_, err := service.Provision(ctx, provisioning.Request{
			DeploymentID: "dep-1", RunID: "run-1",
			Context: planning.Context{OrganizationKey: seedOptions.OrganizationKey, App: application.Application{Key: "a", OrganizationKey: seedOptions.OrganizationKey}, Env: environment.Environment{Key: "staging", Profile: application.ProfileInternalK8s, ConnectionKey: "lab"}},
			Plan: &planning.Plan{
				Graph:   planning.Graph{Nodes: []planning.Node{{Descriptor: descriptor, Kind: planning.NodeResource, ResourceType: "k8s-cluster", Class: "internal", Scope: resource.Scope{Type: resource.ScopeApplication, ID: "a"}}}},
				Matches: map[string]planning.Match{descriptor: {Descriptor: descriptor, DefinitionKey: def.Key, DriverType: driver, ConnectionKey: "other"}},
				Batches: [][]string{{descriptor}},
			},
			Types:       map[string]resource.Type{"k8s-cluster": {Key: "k8s-cluster"}},
			Definitions: map[string]resource.Definition{def.Key: def},
		})
		if err == nil || !strings.Contains(err.Error(), "differs from the Environment connection") {
			t.Fatalf("%s mismatch error = %v", driver, err)
		}
		if registry.calls != 0 {
			t.Fatalf("%s: executor resolved %d times before the mismatch was rejected", driver, registry.calls)
		}
	}
}

// aws-eks: Terraform VPC/EKS Definitions must use the Environment account
// before any executor call; an external database Definition is not checked.
func TestProvision_AWSTargetDefinitionConnectionMismatchFailsBeforeExecutor(t *testing.T) {
	ctx := context.Background()
	seedOptions := seed.Defaults()
	app, err := bootstrap.Build(ctx, bootstrap.Options{Seed: seedOptions, Adapters: bootstrap.AdapterFake})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"aws-a", "aws-b"} {
		if err := app.Store.SaveConnection(ctx, application.Connection{ID: key, Key: key, OrganizationKey: seedOptions.OrganizationKey, Kind: application.ConnectionAWS, Status: application.ConnectionReady, AuthenticationType: application.AuthAWSAccessKey, Config: map[string]any{"region": "us-east-1"}}); err != nil {
			t.Fatal(err)
		}
	}
	run := func(resourceType, connection string) (error, int) {
		descriptor := resourceType + ".default#applications.a"
		registry := &countingRegistry{}
		service := provisioning.NewService(app.Store, registry, secrets.NewMemory(), clock.System{})
		def := resource.Definition{Key: "def-" + resourceType, ResourceTypeKey: resourceType, DriverType: resource.DriverTerraform, ConnectionKey: connection, ExecutionProfile: "aws-eks", Criteria: []resource.Criterion{{}}}
		_, err := service.Provision(ctx, provisioning.Request{
			DeploymentID: "dep-1", RunID: "run-1",
			Context: planning.Context{OrganizationKey: seedOptions.OrganizationKey, App: application.Application{Key: "a", OrganizationKey: seedOptions.OrganizationKey}, Env: environment.Environment{Key: "staging", Profile: application.ProfileAWSEKS, ConnectionKey: "aws-a", Region: "us-east-1"}},
			Plan: &planning.Plan{
				Graph:   planning.Graph{Nodes: []planning.Node{{Descriptor: descriptor, Kind: planning.NodeResource, ResourceType: resourceType, Class: "default", Scope: resource.Scope{Type: resource.ScopeApplication, ID: "a"}}}},
				Matches: map[string]planning.Match{descriptor: {Descriptor: descriptor, DefinitionKey: def.Key, DriverType: def.DriverType, ConnectionKey: connection}},
				Batches: [][]string{{descriptor}},
			},
			Types:       map[string]resource.Type{resourceType: {Key: resourceType}},
			Definitions: map[string]resource.Definition{def.Key: def},
		})
		return err, registry.calls
	}
	for _, resourceType := range []string{"vpc", "k8s-cluster"} {
		err, calls := run(resourceType, "aws-b")
		if err == nil || !strings.Contains(err.Error(), "differs from the Environment connection") || calls != 0 {
			t.Fatalf("%s mismatch: err=%v executor calls=%d", resourceType, err, calls)
		}
		// Same account passes the guard and reaches the executor.
		if err, calls := run(resourceType, "aws-a"); calls != 1 {
			t.Fatalf("%s matching account must reach the executor: err=%v calls=%d", resourceType, err, calls)
		}
	}
	// External provider resources are not retargeted by the guard.
	if err, calls := run("postgres", "aws-b"); calls != 1 {
		t.Fatalf("external database must keep its account: err=%v calls=%d", err, calls)
	}
}
