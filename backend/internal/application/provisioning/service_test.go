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
	"orchestrator/internal/bootstrap/bootstraptest"
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
	app, err := bootstraptest.Build(context.Background(), bootstrap.Options{Seed: seedOptions, Adapters: bootstrap.AdapterFake})
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
	app, err := bootstraptest.Build(ctx, bootstrap.Options{Seed: seedOptions, Adapters: bootstrap.AdapterFake})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"lab", "other"} {
		if err := app.Store.SaveConnection(ctx, application.Connection{ID: key, Key: key, OrganizationKey: seedOptions.OrganizationKey, Kind: application.ConnectionKubernetes, Status: application.ConnectionReady, AuthenticationType: application.AuthKubeconfig}); err != nil {
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
	app, err := bootstraptest.Build(ctx, bootstrap.Options{Seed: seedOptions, Adapters: bootstrap.AdapterFake})
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

// recordingRegistry captures every executor request and succeeds with empty outputs.
type recordingRegistry struct{ requests []execution.ProvisionRequest }

func (r *recordingRegistry) Resolve(resource.DriverType) (execution.ResourceExecutor, error) {
	return r, nil
}

func (r *recordingRegistry) Provision(_ context.Context, req execution.ProvisionRequest) (execution.ProvisionResult, error) {
	r.requests = append(r.requests, req)
	return execution.ProvisionResult{Outputs: map[string]any{}, State: map[string]any{"account": req.Connection.Key}}, nil
}

func saveAWSConnections(t *testing.T, app *bootstrap.App, org string, keys ...string) {
	t.Helper()
	for _, key := range keys {
		if err := app.Store.SaveConnection(context.Background(), application.Connection{ID: key, Key: key, OrganizationKey: org, Kind: application.ConnectionAWS, Status: application.ConnectionReady, AuthenticationType: application.AuthAWSAccessKey, Config: map[string]any{"region": "us-east-1"}}); err != nil {
			t.Fatal(err)
		}
	}
}

// provisionAWS runs one Terraform node of an aws-eks Environment pinned to
// envConnection with a Definition whose key is definitionKey.
func provisionAWS(t *testing.T, app *bootstrap.App, org, resourceType, definitionKey, envConnection string, registry execution.ExecutorRegistry) error {
	t.Helper()
	descriptor := resourceType + ".default#applications.a"
	service := provisioning.NewService(app.Store, registry, secrets.NewMemory(), clock.System{})
	def := resource.Definition{Key: "def-" + resourceType, ResourceTypeKey: resourceType, DriverType: resource.DriverTerraform, ConnectionKey: definitionKey, ExecutionProfile: "aws-eks", Criteria: []resource.Criterion{{}}}
	if err := app.Store.SaveResourceDefinition(context.Background(), org, def); err != nil {
		t.Fatal(err)
	}
	_, err := service.Provision(context.Background(), provisioning.Request{
		DeploymentID: "dep-1", RunID: "run-1",
		Context: planning.Context{OrganizationKey: org, App: application.Application{Key: "a", OrganizationKey: org}, Env: environment.Environment{Key: "staging", Profile: application.ProfileAWSEKS, ConnectionKey: envConnection, Region: "us-east-1"}},
		Plan: &planning.Plan{
			Graph:   planning.Graph{Nodes: []planning.Node{{Descriptor: descriptor, Kind: planning.NodeResource, ResourceType: resourceType, Class: "default", Scope: resource.Scope{Type: resource.ScopeApplication, ID: "a"}}}},
			Matches: map[string]planning.Match{descriptor: {Descriptor: descriptor, DefinitionKey: def.Key, DriverType: def.DriverType, ConnectionKey: definitionKey}},
			Batches: [][]string{{descriptor}},
		},
		Types:       map[string]resource.Type{resourceType: {Key: resourceType}},
		Definitions: map[string]resource.Definition{def.Key: def},
	})
	return err
}

// UC-08 BR-07 (T16): a VPC/EKS Definition without a key inherits whichever AWS
// Environment connection is pinned; executor request and ActiveResource agree.
func TestProvision_AWSVPCAndEKSInheritEnvironmentConnection(t *testing.T) {
	ctx := context.Background()
	seedOptions := seed.Defaults()
	for _, resourceType := range []string{"vpc", "k8s-cluster"} {
		for _, target := range []string{"aws-a", "aws-b"} {
			app, err := bootstraptest.Build(ctx, bootstrap.Options{Seed: seedOptions, Adapters: bootstrap.AdapterFake})
			if err != nil {
				t.Fatal(err)
			}
			saveAWSConnections(t, app, seedOptions.OrganizationKey, "aws-a", "aws-b")
			registry := &recordingRegistry{}
			if err := provisionAWS(t, app, seedOptions.OrganizationKey, resourceType, "", target, registry); err != nil {
				t.Fatalf("%s on %s: %v", resourceType, target, err)
			}
			if len(registry.requests) != 1 || registry.requests[0].Connection.Key != target {
				t.Fatalf("%s on %s: executor requests = %+v", resourceType, target, registry.requests)
			}
			active, _ := app.Store.ListActiveResources(ctx, seedOptions.OrganizationKey)
			found := false
			for _, a := range active {
				if a.Descriptor.Type == resourceType {
					found = true
					if a.ConnectionKey != target {
						t.Fatalf("%s on %s: ActiveResource connection = %q", resourceType, target, a.ConnectionKey)
					}
				}
			}
			if !found {
				t.Fatalf("%s on %s: no ActiveResource persisted", resourceType, target)
			}
		}
	}
}

// Aurora keeps its explicit account, never swaps it for the Environment account
// and fails closed instead of inheriting when the key is absent.
func TestProvision_AuroraKeepsExplicitAccountAndNeverInherits(t *testing.T) {
	ctx := context.Background()
	seedOptions := seed.Defaults()
	app, err := bootstraptest.Build(ctx, bootstrap.Options{Seed: seedOptions, Adapters: bootstrap.AdapterFake})
	if err != nil {
		t.Fatal(err)
	}
	saveAWSConnections(t, app, seedOptions.OrganizationKey, "aws-a", "aws-b")
	registry := &recordingRegistry{}
	if err := provisionAWS(t, app, seedOptions.OrganizationKey, "postgres", "aws-b", "aws-a", registry); err != nil {
		t.Fatal(err)
	}
	if len(registry.requests) != 1 || registry.requests[0].Connection.Key != "aws-b" {
		t.Fatalf("Aurora executor requests = %+v", registry.requests)
	}
	active, _ := app.Store.ListActiveResources(ctx, seedOptions.OrganizationKey)
	for _, a := range active {
		if a.Descriptor.Type == "postgres" && a.ConnectionKey != "aws-b" {
			t.Fatalf("Aurora ActiveResource connection = %q", a.ConnectionKey)
		}
	}

	missing := &recordingRegistry{}
	err = provisionAWS(t, app, seedOptions.OrganizationKey, "postgres", "", "aws-a", missing)
	if err == nil || !strings.Contains(err.Error(), "must exist and be READY") || len(missing.requests) != 0 {
		t.Fatalf("Aurora without account: err=%v executor calls=%d", err, len(missing.requests))
	}
}

// A malformed VPC/EKS key different from the pinned target is rejected before
// any executor call, and an empty key is not a mismatch.
func TestProvision_MalformedVPCKeyIsRejectedWithoutFallback(t *testing.T) {
	ctx := context.Background()
	seedOptions := seed.Defaults()
	app, err := bootstraptest.Build(ctx, bootstrap.Options{Seed: seedOptions, Adapters: bootstrap.AdapterFake})
	if err != nil {
		t.Fatal(err)
	}
	saveAWSConnections(t, app, seedOptions.OrganizationKey, "aws-a", "aws-b")
	for _, resourceType := range []string{"vpc", "k8s-cluster"} {
		registry := &recordingRegistry{}
		err := provisionAWS(t, app, seedOptions.OrganizationKey, resourceType, "aws-b", "aws-a", registry)
		if err == nil || !strings.Contains(err.Error(), "differs from the Environment connection") || len(registry.requests) != 0 {
			t.Fatalf("%s: err=%v executor calls=%d", resourceType, err, len(registry.requests))
		}
	}
}

// UC-08 BR-07 / IMP-020: persisted resource state is reused only under the same
// effective connection; another account fails before the executor and keeps the
// stored identity.
func TestProvision_ReuseNeverCrossesAccounts(t *testing.T) {
	ctx := context.Background()
	seedOptions := seed.Defaults()
	cases := []struct{ name, resourceType, first, firstEnv, second, secondEnv string }{
		{"vpc", "vpc", "", "aws-a", "", "aws-b"},
		{"eks", "k8s-cluster", "", "aws-a", "", "aws-b"},
		{"aurora", "postgres", "aws-a", "aws-a", "aws-b", "aws-a"},
	}
	for _, c := range cases {
		app, err := bootstraptest.Build(ctx, bootstrap.Options{Seed: seedOptions, Adapters: bootstrap.AdapterFake})
		if err != nil {
			t.Fatal(err)
		}
		saveAWSConnections(t, app, seedOptions.OrganizationKey, "aws-a", "aws-b")
		registry := &recordingRegistry{}
		org := seedOptions.OrganizationKey
		if err := provisionAWS(t, app, org, c.resourceType, c.first, c.firstEnv, registry); err != nil {
			t.Fatalf("%s first run: %v", c.name, err)
		}
		// Same account reuses the stored executor state.
		if err := provisionAWS(t, app, org, c.resourceType, c.first, c.firstEnv, registry); err != nil || registry.requests[1].PriorState["account"] != "aws-a" {
			t.Fatalf("%s same-account reuse: err=%v requests=%+v", c.name, err, registry.requests)
		}
		calls := len(registry.requests)
		err = provisionAWS(t, app, org, c.resourceType, c.second, c.secondEnv, registry)
		if err == nil || !strings.Contains(err.Error(), "belongs to another connection") || len(registry.requests) != calls {
			t.Fatalf("%s cross-account reuse: err=%v executor calls %d -> %d", c.name, err, calls, len(registry.requests))
		}
		active, _ := app.Store.ListActiveResources(ctx, org)
		for _, a := range active {
			if a.Descriptor.Type == c.resourceType && a.ConnectionKey != "aws-a" {
				t.Fatalf("%s: stored identity overwritten with %q", c.name, a.ConnectionKey)
			}
		}
	}
}

// The effective connection must exist, be READY, AWS and belong to the
// Organization: VPC/EKS check the Environment connection, Aurora its own.
func TestProvision_EffectiveAWSConnectionUnavailableFailsBeforeExecutor(t *testing.T) {
	ctx := context.Background()
	seedOptions := seed.Defaults()
	app, err := bootstraptest.Build(ctx, bootstrap.Options{Seed: seedOptions, Adapters: bootstrap.AdapterFake})
	if err != nil {
		t.Fatal(err)
	}
	org := seedOptions.OrganizationKey
	saveAWSConnections(t, app, org, "aws-ok")
	if err := app.Store.SaveConnection(ctx, application.Connection{ID: "pending", Key: "pending", OrganizationKey: org, Kind: application.ConnectionAWS, Status: application.ConnectionVerifying, AuthenticationType: application.AuthAWSAccessKey}); err != nil {
		t.Fatal(err)
	}
	if err := app.Store.SaveConnection(ctx, application.Connection{ID: "kube", Key: "kube", OrganizationKey: org, Kind: application.ConnectionKubernetes, Status: application.ConnectionReady, AuthenticationType: application.AuthKubeconfig}); err != nil {
		t.Fatal(err)
	}
	if err := app.Store.SaveOrganization(ctx, application.Organization{Key: "globex", Name: "Globex"}); err != nil {
		t.Fatal(err)
	}
	if err := app.Store.SaveConnection(ctx, application.Connection{ID: "foreign", Key: "foreign", OrganizationKey: "globex", Kind: application.ConnectionAWS, Status: application.ConnectionReady, AuthenticationType: application.AuthAWSAccessKey}); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"pending", "kube", "foreign", "missing"} {
		for _, resourceType := range []string{"vpc", "k8s-cluster"} {
			registry := &recordingRegistry{}
			err := provisionAWS(t, app, org, resourceType, "", bad, registry)
			if err == nil || !strings.Contains(err.Error(), "must exist and be READY") || len(registry.requests) != 0 {
				t.Fatalf("%s with Environment connection %q: err=%v calls=%d", resourceType, bad, err, len(registry.requests))
			}
		}
		registry := &recordingRegistry{}
		err := provisionAWS(t, app, org, "postgres", bad, "aws-ok", registry)
		if err == nil || !strings.Contains(err.Error(), "must exist and be READY") || len(registry.requests) != 0 {
			t.Fatalf("Aurora with explicit account %q: err=%v calls=%d", bad, err, len(registry.requests))
		}
	}
}
