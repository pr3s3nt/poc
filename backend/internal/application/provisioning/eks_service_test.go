package provisioning_test

import (
	"context"
	"strings"
	"testing"

	"orchestrator/internal/adapters/fake"
	appsvc "orchestrator/internal/application/deployment"
	"orchestrator/internal/bootstrap"
	"orchestrator/internal/bootstrap/bootstraptest"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/seed"
	"orchestrator/internal/seed/seedtest"
)

func eksBackendScore(opts seed.Options) map[string]any {
	score := seed.AcceptanceScores(opts)["backend"]
	score["resources"].(map[string]any)["db"].(map[string]any)["class"] = "in-cluster"
	return score
}

// ADR-017 V05 end to end at service level: EKS is created by the authored graph in THE SAME execution, publishes a typed cluster output
// of its ORIGINAL account and region, the namespace and PostgreSQL run on that very cluster, the workload resolves its target from its own
// cluster reference and persists it, and nothing depends on an Environment profile, Connection or account.
func TestProvision_EKSCreatedInTheSameExecutionDrivesNamespaceDatabaseAndWorkload(t *testing.T) {
	opts := seedtest.Defaults()
	opts.Catalog = seed.CatalogNone
	reg := v2Registry{v2: &v2Executor{fake: fake.NewResourceExecutor()}, fake: fake.NewResourceExecutor()}
	proof := &scriptedReadiness{}
	app, err := bootstraptest.Build(context.Background(), bootstrap.Options{Seed: opts, Adapters: bootstrap.AdapterFake, RegistryOverride: reg, ClusterReadinessOverride: proof})
	if err != nil {
		t.Fatal(err)
	}
	bootstraptest.EKSCatalog(t, app, opts)
	result, err := app.Deployments.DeployWorkload(context.Background(), appsvc.DeployCommand{
		OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey, EnvironmentKey: "staging",
		WorkloadID: "backend", ScoreAfter: eksBackendScore(opts), Actor: "test", RunID: "run-a",
	})
	if err != nil {
		t.Fatalf("deploy: %v", err)
	}
	order := []string{}
	byType := map[string]execution.ProvisionRequest{}
	for _, call := range reg.v2.calls {
		order = append(order, call.ResourceType)
		byType[call.ResourceType] = call
	}
	index := func(typ string) int {
		for i, got := range order {
			if got == typ {
				return i
			}
		}
		return -1
	}
	if !(index("vpc") < index("k8s-cluster") && index("k8s-cluster") < index("k8s-namespace") && index("k8s-namespace") < index("postgres")) {
		t.Fatalf("provider-first order violated: %v", order)
	}
	// The module returns its name before the platform completes the typed target.
	// Its intermediate contract must accept that real module output, while the
	// registered full Type still requires the platform-produced cluster output.
	if err := byType["k8s-cluster"].Type.ValidateOutputs(map[string]any{"name": "created-eks"}); err != nil {
		t.Fatalf("EKS module outputs before platform completion: %v", err)
	}
	registeredTypes, err := app.Store.ListResourceTypes(context.Background(), opts.OrganizationKey)
	if err != nil {
		t.Fatal(err)
	}
	var registeredType resource.Type
	for _, typ := range registeredTypes {
		if typ.Key == resource.ClusterTargetType {
			registeredType = typ
		}
	}
	if field, ok := registeredType.Output(resource.ClusterTargetOutput); !ok || !field.Required {
		t.Fatal("platform completion changed the registered required cluster contract")
	}
	// The cluster and VPC ran with the Definition's own account; the proof received the completed opaque target.
	if byType["k8s-cluster"].Connection.Key != "aws-a" || byType["vpc"].Connection.Key != "aws-a" {
		t.Fatalf("AWS nodes must run with the Definition's Driver Account: %+v", byType["k8s-cluster"].Connection)
	}
	if len(proof.calls) != 1 || proof.calls[0].Provider != "eks" || proof.calls[0].Region != "us-east-1" || proof.calls[0].Connection != "aws-a" {
		t.Fatalf("the usability proof gets the completed EKS target: %+v", proof.calls)
	}
	// The consumers got the EKS provider target and an EKS pin: no Kubernetes Connection exists for them.
	for _, typ := range []string{"k8s-namespace", "postgres"} {
		call := byType[typ]
		if len(call.ProviderPins) != 1 || call.ProviderPins[0].ClusterProvider != planning.ClusterEKS || call.ProviderPins[0].ConnectionKey != "" || call.ProviderPins[0].ClusterAccount != "aws-a" {
			t.Fatalf("%s pin = %+v", typ, call.ProviderPins)
		}
		target, ok := call.ProviderTargets[call.ProviderPins[0].Provider]
		if !ok || target.Provider != "eks" || target.Region != "us-east-1" || target.Connection != "aws-a" || target.Kubeconfig != "" {
			t.Fatalf("%s provider target = %+v", typ, target)
		}
		if call.Target.Provider != "eks" || call.Target.Connection != "aws-a" {
			t.Fatalf("%s runs on the EKS target: %+v", typ, call.Target)
		}
	}
	// The cluster's published output is the typed target of its original identity, persisted as a safe output.
	active, err := app.Store.ListActiveResources(context.Background(), opts.OrganizationKey)
	if err != nil {
		t.Fatal(err)
	}
	var clusterOutput map[string]any
	for _, a := range active {
		if a.Descriptor.Type == "k8s-cluster" {
			clusterOutput, _ = a.Outputs["cluster"].(map[string]any)
		}
	}
	if err := resource.ValidateK8sTarget(clusterOutput); err != nil || clusterOutput["provider"] != "eks" || clusterOutput["account"] != "aws-a" || clusterOutput["region"] != "us-east-1" || clusterOutput["accountVersion"] != float64(5) {
		t.Fatalf("cluster output = %v (%v)", clusterOutput, err)
	}
	// The workload was applied to that same cluster and its pinned target is persisted without credential material.
	if len(app.FakeDeploy.Applied) == 0 {
		t.Fatal("the workload was not applied")
	}
	applied := app.FakeDeploy.Applied[len(app.FakeDeploy.Applied)-1].Target
	if applied.Provider != "eks" || applied.Region != "us-east-1" || applied.Connection != "aws-a" || applied.Organization != opts.OrganizationKey || applied.Namespace == "" || applied.Kubeconfig != "" {
		t.Fatalf("applied target = %+v", applied)
	}
	env, err := app.Store.GetEnvironment(context.Background(), opts.ApplicationKey, "staging")
	if err != nil || env.ConnectionKey != "" || env.Profile != "" {
		t.Fatalf("the Environment must stay unbound: %+v %v", env, err)
	}
	_ = result
	instances, err := app.Store.ListWorkloadInstancesFor(context.Background(), opts.ApplicationKey+"/"+"staging", env.TargetGeneration)
	if err != nil || len(instances) != 1 {
		t.Fatalf("instances = %+v %v", instances, err)
	}
	ref := instances[0].TargetRef
	placement, _ := ref["placement"].(map[string]any)
	if ref["provider"] != "eks" || ref["region"] != "us-east-1" || ref["connection"] != "aws-a" || placement["provider"] != "eks" || placement["region"] != "us-east-1" || placement["account"] != "aws-a" {
		t.Fatalf("persisted target = %v", ref)
	}
	for key := range ref {
		if strings.Contains(strings.ToLower(key), "token") || strings.Contains(strings.ToLower(key), "kubeconfig") || strings.Contains(strings.ToLower(key), "endpoint") {
			t.Fatalf("a credential-bearing key was persisted: %s", key)
		}
	}
}

// A second deployment reuses the same logical cluster: no second cluster instance and the workload target is unchanged.
func TestProvision_EKSIsReusedNotDuplicatedByALaterDeployment(t *testing.T) {
	opts := seedtest.Defaults()
	opts.Catalog = seed.CatalogNone
	reg := v2Registry{v2: &v2Executor{fake: fake.NewResourceExecutor()}, fake: fake.NewResourceExecutor()}
	app, err := bootstraptest.Build(context.Background(), bootstrap.Options{Seed: opts, Adapters: bootstrap.AdapterFake, RegistryOverride: reg, ClusterReadinessOverride: &scriptedReadiness{}})
	if err != nil {
		t.Fatal(err)
	}
	bootstraptest.EKSCatalog(t, app, opts)
	cmd := appsvc.DeployCommand{OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey, EnvironmentKey: "staging", WorkloadID: "backend", ScoreAfter: eksBackendScore(opts), Actor: "test", RunID: "run-a"}
	if _, err := app.Deployments.DeployWorkload(context.Background(), cmd); err != nil {
		t.Fatal(err)
	}
	before, _ := app.Store.ListActiveResources(context.Background(), opts.OrganizationKey)
	env, _ := app.Store.GetEnvironment(context.Background(), opts.ApplicationKey, "staging")
	first, _ := app.Store.ListWorkloadInstancesFor(context.Background(), opts.ApplicationKey+"/"+"staging", env.TargetGeneration)
	second := cmd
	second.Action = "UPDATE"
	second.ScoreBefore = cmd.ScoreAfter
	second.RunID = "run-b"
	if _, err := app.Deployments.DeployWorkload(context.Background(), second); err != nil {
		t.Fatalf("update: %v", err)
	}
	after, _ := app.Store.ListActiveResources(context.Background(), opts.OrganizationKey)
	if len(after) != len(before) {
		t.Fatalf("the update created or dropped resources: %d -> %d", len(before), len(after))
	}
	later, _ := app.Store.ListWorkloadInstancesFor(context.Background(), opts.ApplicationKey+"/"+"staging", env.TargetGeneration)
	if len(later) != 1 || later[0].TargetRef["cluster"] != first[0].TargetRef["cluster"] || later[0].TargetRef["connection"] != first[0].TargetRef["connection"] || later[0].TargetRef["region"] != first[0].TargetRef["region"] {
		t.Fatalf("the applied target moved: %v -> %v", first[0].TargetRef, later[0].TargetRef)
	}
}

const eksEnv = "staging"

// eksHarness builds the fake-adapter application with the V05 Definition catalog. A nil proof is a cluster that is always usable.
func eksHarness(t *testing.T, reg v2Registry, proof *scriptedReadiness) (*bootstrap.App, seed.Options) {
	t.Helper()
	opts := seedtest.Defaults()
	opts.Catalog = seed.CatalogNone
	if proof == nil {
		proof = &scriptedReadiness{}
	}
	app, err := bootstraptest.Build(context.Background(), bootstrap.Options{Seed: opts, Adapters: bootstrap.AdapterFake, RegistryOverride: reg, ClusterReadinessOverride: proof})
	if err != nil {
		t.Fatal(err)
	}
	bootstraptest.EKSCatalog(t, app, opts)
	return app, opts
}

// eksScore is an acceptance Score whose PostgreSQL runs in the cluster the plan creates.
func eksScore(opts seed.Options, workload string) map[string]any {
	score := seed.AcceptanceScores(opts)[workload]
	score["resources"].(map[string]any)["db"].(map[string]any)["class"] = "in-cluster"
	return score
}
