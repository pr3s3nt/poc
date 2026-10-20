package provisioning_test

import (
	"context"
	"errors"
	"orchestrator/internal/adapters/terraform/registry"
	"orchestrator/internal/seed/seedtest"
	"strings"
	"testing"

	"orchestrator/internal/adapters/fake"
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

// deployBackend deploys the backend on the Internal seed (eks=false) or on the Definition-placed EKS chain of V05 (eks=true).
func deployBackend(t *testing.T, eks bool) (*bootstrap.App, seed.Options, string, string) {
	t.Helper()
	var (
		app  *bootstrap.App
		opts seed.Options
	)
	score := func(o seed.Options) map[string]any { return seed.AcceptanceScores(o)["backend"] }
	environment := ""
	if eks {
		app, opts = eksHarness(t, v2Registry{v2: &v2Executor{fake: fake.NewResourceExecutor()}, fake: fake.NewResourceExecutor()}, nil)
		score = func(o seed.Options) map[string]any { return eksScore(o, "backend") }
		environment = eksEnv
	} else {
		opts = seedtest.Defaults()
		opts.Region, opts.AccountID = "us-east-1", "000000000000"
		var err error
		if app, err = bootstraptest.Build(context.Background(), bootstrap.Options{Seed: opts, Adapters: bootstrap.AdapterFake}); err != nil {
			t.Fatalf("bootstrap: %v", err)
		}
		environment = opts.EnvironmentKey
	}
	result, err := app.Deployments.DeployWorkload(context.Background(), appsvc.DeployCommand{
		OrganizationKey: opts.OrganizationKey,
		ApplicationKey:  opts.ApplicationKey,
		EnvironmentKey:  environment,
		WorkloadID:      "backend",
		ScoreAfter:      score(opts),
		Actor:           "test",
		RunID:           "run-test",
	})
	if err != nil {
		t.Fatalf("deploy backend (eks=%v): %v", eks, err)
	}
	return app, opts, environment, result.DeploymentID
}

func TestProvision_PostgresOutputContractEquivalentAcrossPlacements(t *testing.T) {
	ctx := context.Background()
	want := []string{"database", "host", "password", "port", "username"}

	for _, eks := range []bool{false, true} {
		app, opts, environment, deploymentID := deployBackend(t, eks)
		view, err := app.Queries.GetDeployment(ctx, appsvc.GetDeploymentQuery{OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey, EnvironmentKey: environment, DeploymentID: deploymentID})
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
					t.Fatalf("eks=%v postgres outputs are missing %q: %v", eks, key, res.Outputs)
				}
			}
		}
		if !found {
			t.Fatalf("eks=%v deployment has no postgres resource", eks)
		}
	}
}

func TestProvision_ReusesEnvironmentScopedVPCAndEKS(t *testing.T) {
	ctx := context.Background()
	app, opts, environment, _ := deployBackend(t, true)

	if _, err := app.Deployments.DeployWorkload(ctx, appsvc.DeployCommand{
		OrganizationKey: opts.OrganizationKey,
		ApplicationKey:  opts.ApplicationKey,
		EnvironmentKey:  environment,
		WorkloadID:      "worker",
		ScoreAfter:      eksScore(opts, "worker"),
		Actor:           "test",
		RunID:           "run-test",
	}); err != nil {
		t.Fatalf("deploy worker: %v", err)
	}

	active, err := app.Store.ListActiveResources(ctx, opts.OrganizationKey)
	if err != nil {
		t.Fatalf("list active resources: %v", err)
	}
	counts := map[string]int{}
	for _, a := range active {
		counts[a.Descriptor.Type]++
	}
	if counts["vpc"] != 1 || counts["k8s-cluster"] != 1 {
		t.Fatalf("an Environment must keep one VPC and one EKS: %v", counts)
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

// A Kubernetes or existing-cluster Definition inherits its cluster access; one
// that names a Driver Account is refused before any executor call.
func TestProvision_NonTerraformDefinitionWithAnAccountFailsBeforeExecutor(t *testing.T) {
	ctx := context.Background()
	seedOptions := seedtest.Defaults()
	app, err := bootstraptest.Build(ctx, bootstrap.Options{Seed: seedOptions, Adapters: bootstrap.AdapterFake})
	if err != nil {
		t.Fatal(err)
	}
	descriptor := "k8s-cluster.internal#connections.lab"
	for _, driver := range []resource.DriverType{resource.DriverExistingCluster, resource.DriverKubernetes} {
		registry := &countingRegistry{}
		service := provisioning.NewService(app.Store, registry, secrets.NewMemory(), clock.System{})
		def := resource.Definition{Key: "cluster-other", Name: "cluster-other", Version: 1, ResourceTypeKey: "k8s-cluster", DriverType: driver, DriverAccount: "other", Criteria: []resource.Criterion{{}}}
		_, err := service.Provision(ctx, provisioning.Request{
			DeploymentID: "dep-1", RunID: "run-1",
			Context: planning.Context{OrganizationKey: seedOptions.OrganizationKey, App: application.Application{Key: "a", OrganizationKey: seedOptions.OrganizationKey}, Env: environment.Environment{Key: "staging"}},
			Plan: &planning.Plan{
				Graph:   planning.Graph{Nodes: []planning.Node{{Descriptor: descriptor, Kind: planning.NodeResource, ResourceType: "k8s-cluster", Class: "internal", Scope: resource.Scope{Type: resource.ScopeApplication, ID: "a"}}}},
				Matches: map[string]planning.Match{descriptor: {Descriptor: descriptor, DefinitionKey: def.Key, DriverType: driver}},
				Batches: [][]string{{descriptor}},
			},
			Types:       map[string]resource.Type{"k8s-cluster": {Key: "k8s-cluster"}},
			Definitions: map[string]resource.Definition{def.Key: def},
		})
		want := "does not use"
		if driver == resource.DriverExistingCluster {
			// ADR-016: an existing cluster runs with its own pinned account; a stale or
			// foreign account is an unavailable Connection, still before any executor call.
			want = "is unavailable"
		}
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%s account error = %v", driver, err)
		}
		if registry.calls != 0 {
			t.Fatalf("%s: executor resolved %d times before the account was rejected", driver, registry.calls)
		}
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
		if err := app.Store.SaveConnection(context.Background(), application.Connection{Name: key, ID: key, Key: key, OrganizationKey: org, Kind: application.ConnectionAWS, Status: application.ConnectionReady, AuthenticationType: application.AuthAWSAccessKey, Config: map[string]any{"region": "us-east-1"}}); err != nil {
			t.Fatal(err)
		}
	}
}

// provisionAWS runs one Terraform node of an aws-eks Environment pinned to
// envConnection with a Definition whose key is definitionKey.
func provisionAWS(t *testing.T, app *bootstrap.App, org, resourceType, definitionKey, _ string, registry execution.ExecutorRegistry) error {
	t.Helper()
	descriptor := resourceType + ".default#applications.a"
	service := provisioning.NewService(app.Store, registry, secrets.NewMemory(), clock.System{})
	def := resource.Definition{Key: "def-" + resourceType, Name: "def-" + resourceType, Version: 1, ResourceTypeKey: resourceType, DriverType: resource.DriverTerraform, DriverInputs: terraformInputs(), DriverAccount: definitionKey, Criteria: []resource.Criterion{{}}}
	if err := app.Store.SaveResourceDefinition(context.Background(), org, def); err != nil {
		t.Fatal(err)
	}
	_, err := service.Provision(context.Background(), provisioning.Request{
		DeploymentID: "dep-1", RunID: "run-1",
		Context: planning.Context{OrganizationKey: org, App: application.Application{Key: "a", OrganizationKey: org}, Env: environment.Environment{Key: "staging"}},
		Plan: &planning.Plan{
			Graph:   planning.Graph{Nodes: []planning.Node{{Descriptor: descriptor, Kind: planning.NodeResource, ResourceType: resourceType, Class: "default", Scope: resource.Scope{Type: resource.ScopeApplication, ID: "a"}}}},
			Matches: map[string]planning.Match{descriptor: {Descriptor: descriptor, DefinitionKey: def.Key, DriverType: def.DriverType, DriverAccount: definitionKey}},
			Batches: [][]string{{descriptor}},
		},
		Types:       map[string]resource.Type{resourceType: {Key: resourceType}},
		Definitions: map[string]resource.Definition{def.Key: def},
	})
	return err
}

// V02: every AWS node runs under the Driver Account of its Definition, whatever
// account the Environment is bound to; the executor request and the persisted
// ActiveResource agree. A node without an account never inherits the
// Environment's one.
func TestProvision_AWSNodesRunUnderTheirOwnDriverAccount(t *testing.T) {
	ctx := context.Background()
	seedOptions := seedtest.Defaults()
	for _, resourceType := range []string{"vpc", "k8s-cluster", "postgres"} {
		app, err := bootstraptest.Build(ctx, bootstrap.Options{Seed: seedOptions, Adapters: bootstrap.AdapterFake})
		if err != nil {
			t.Fatal(err)
		}
		saveAWSConnections(t, app, seedOptions.OrganizationKey, "aws-a", "aws-b")
		registry := &recordingRegistry{}
		if err := provisionAWS(t, app, seedOptions.OrganizationKey, resourceType, "aws-b", "aws-a", registry); err != nil {
			t.Fatalf("%s: %v", resourceType, err)
		}
		if len(registry.requests) != 1 || registry.requests[0].Connection.Key != "aws-b" {
			t.Fatalf("%s: executor requests = %+v", resourceType, registry.requests)
		}
		active, _ := app.Store.ListActiveResources(ctx, seedOptions.OrganizationKey)
		found := false
		for _, a := range active {
			if a.Descriptor.Type == resourceType {
				found = true
				if a.ConnectionKey != "aws-b" {
					t.Fatalf("%s: ActiveResource connection = %q", resourceType, a.ConnectionKey)
				}
			}
		}
		if !found {
			t.Fatalf("%s: no ActiveResource persisted", resourceType)
		}
		// No Driver Account: fail closed; the Environment account is not a fallback.
		none := &recordingRegistry{}
		if err := provisionAWS(t, app, seedOptions.OrganizationKey, resourceType, "", "aws-a", none); err == nil || len(none.requests) != 0 {
			t.Fatalf("%s without an account: err=%v calls=%d", resourceType, err, len(none.requests))
		}
	}
}

// UC-08 BR-07 / IMP-020: persisted resource state is reused only under the same
// effective connection; another account fails before the executor and keeps the
// stored identity.
func TestProvision_ReuseNeverCrossesAccounts(t *testing.T) {
	ctx := context.Background()
	seedOptions := seedtest.Defaults()
	for _, resourceType := range []string{"vpc", "k8s-cluster", "postgres"} {
		app, err := bootstraptest.Build(ctx, bootstrap.Options{Seed: seedOptions, Adapters: bootstrap.AdapterFake})
		if err != nil {
			t.Fatal(err)
		}
		saveAWSConnections(t, app, seedOptions.OrganizationKey, "aws-a", "aws-b")
		registry := &recordingRegistry{}
		org := seedOptions.OrganizationKey
		if err := provisionAWS(t, app, org, resourceType, "aws-a", "aws-a", registry); err != nil {
			t.Fatalf("%s first run: %v", resourceType, err)
		}
		// Same account reuses the stored executor state.
		if err := provisionAWS(t, app, org, resourceType, "aws-a", "aws-a", registry); err != nil || registry.requests[1].PriorState["account"] != "aws-a" {
			t.Fatalf("%s same-account reuse: err=%v requests=%+v", resourceType, err, registry.requests)
		}
		calls := len(registry.requests)
		err = provisionAWS(t, app, org, resourceType, "aws-b", "aws-a", registry)
		if err == nil || !strings.Contains(err.Error(), "belongs to another connection") || len(registry.requests) != calls {
			t.Fatalf("%s cross-account reuse: err=%v executor calls %d -> %d", resourceType, err, calls, len(registry.requests))
		}
		active, _ := app.Store.ListActiveResources(ctx, org)
		for _, a := range active {
			if a.Descriptor.Type == resourceType && a.ConnectionKey != "aws-a" {
				t.Fatalf("%s: stored identity overwritten with %q", resourceType, a.ConnectionKey)
			}
		}
	}
}

// The Definition's Driver Account must exist, be READY, AWS and belong to the
// Organization; otherwise the node fails before any executor call.
func TestProvision_EffectiveAWSConnectionUnavailableFailsBeforeExecutor(t *testing.T) {
	ctx := context.Background()
	seedOptions := seedtest.Defaults()
	app, err := bootstraptest.Build(ctx, bootstrap.Options{Seed: seedOptions, Adapters: bootstrap.AdapterFake})
	if err != nil {
		t.Fatal(err)
	}
	org := seedOptions.OrganizationKey
	saveAWSConnections(t, app, org, "aws-ok")
	if err := app.Store.SaveConnection(ctx, application.Connection{Name: "pending", ID: "pending", Key: "pending", OrganizationKey: org, Kind: application.ConnectionAWS, Status: application.ConnectionVerifying, AuthenticationType: application.AuthAWSAccessKey}); err != nil {
		t.Fatal(err)
	}
	if err := app.Store.SaveConnection(ctx, application.Connection{Name: "kube", ID: "kube", Key: "kube", OrganizationKey: org, Kind: application.ConnectionKubernetes, Status: application.ConnectionReady, AuthenticationType: application.AuthKubeconfig}); err != nil {
		t.Fatal(err)
	}
	if err := app.Store.SaveOrganization(ctx, application.Organization{Key: "globex", Name: "Globex"}); err != nil {
		t.Fatal(err)
	}
	if err := app.Store.SaveConnection(ctx, application.Connection{Name: "foreign", ID: "foreign", Key: "foreign", OrganizationKey: "globex", Kind: application.ConnectionAWS, Status: application.ConnectionReady, AuthenticationType: application.AuthAWSAccessKey}); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"pending", "kube", "foreign", "missing"} {
		for _, resourceType := range []string{"vpc", "k8s-cluster", "postgres"} {
			registry := &recordingRegistry{}
			err := provisionAWS(t, app, org, resourceType, bad, "aws-ok", registry)
			if err == nil || !strings.Contains(err.Error(), "kind does not match") && !strings.Contains(err.Error(), "must exist and be READY") || len(registry.requests) != 0 {
				t.Fatalf("%s with Driver Account %q: err=%v calls=%d", resourceType, bad, err, len(registry.requests))
			}
		}
	}
}

// terraformInputs is a v2 Terraform Driver Inputs document (packaged VPC
// pin, Driver Account AWS binding). The fake executor runs no module code, so
// no artifact verification happens in these connection tests.
func terraformInputs() map[string]any {
	refs, err := registry.PackagedRefs()
	if err != nil {
		panic(err)
	}
	return map[string]any{"values": map[string]any{
		"source":            refs["vpc"].Map(),
		"variables":         map[string]any{},
		"provider_bindings": map[string]any{"aws": map[string]any{"target": "account"}},
	}}
}
