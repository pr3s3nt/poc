package planning_test

import (
	"strings"
	"testing"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/adapters/terraform/registry"
	"orchestrator/internal/adapters/terraform/registry/registrytest"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/planning"
	"orchestrator/internal/planning/score"
	"orchestrator/internal/seed"
	"orchestrator/internal/seed/seedtest"
)

// awsSharedRequest plans one backend with TWO shared Aurora descriptors and TWO
// shared ElastiCache descriptors in one AWS Environment, over the REAL verified
// registry bundles (vpc, eks, aurora, elasticache).
func awsSharedRequest(t *testing.T) planning.Request {
	t.Helper()
	refs, err := registry.PackagedRefs()
	if err != nil {
		t.Fatal(err)
	}
	o := seedtest.Defaults()
	o.RunID, o.Region = "run-test", "us-east-1"
	infra := func(output string) string {
		return "${resources['vpc.default#@infra'].outputs." + output + "}"
	}
	catalog := planning.Catalog{Types: map[string]resource.Type{}}
	for _, typ := range seed.ResourceTypes() {
		catalog.Types[typ.Key] = typ
	}
	catalog.Types["redis"] = resource.Type{Key: "redis", Outputs: []resource.OutputField{
		{Name: "host", Type: "string", Required: true}, {Name: "port", Type: "number", Required: true}, {Name: "username", Type: "string", Required: true},
		{Name: "password", Type: "string", Required: true, Secret: true}, {Name: "tls", Type: "bool", Required: true}}}
	catalog.Definitions = seed.ResourceDefinitions(o)
	elasticache := k8sTerraformDefinition("redis-aws-elasticache", "redis", refs["elasticache"], map[string]any{
		"vpc_id": infra("id"), "subnet_ids": infra("subnetIds"), "private_subnet_ids": infra("privateSubnetIds"),
		"engine_version": "7.1", "node_type": "cache.t4g.micro",
	}, nil, []string{"host", "port", "username", "password", "tls"})
	elasticache.DriverAccount = o.CloudConnectionKey
	elasticache.DriverInputs["values"].(map[string]any)["variables"].(map[string]any)["region"] = o.Region
	elasticache.DriverInputs["values"].(map[string]any)["provider_bindings"] = map[string]any{"aws": map[string]any{"target": tfmodule.TargetAccount}}
	catalog.Accounts = map[string]application.Connection{o.CloudConnectionKey: {Key: o.CloudConnectionKey, Name: "AWS", OrganizationKey: o.OrganizationKey, Kind: application.ConnectionAWS, Status: application.ConnectionReady, Version: 1}}
	catalog.Definitions = append(catalog.Definitions, elasticache)

	scoreMap := seed.AcceptanceScores(o)["backend"]
	resources := scoreMap["resources"].(map[string]any)
	for name, r := range resources {
		if m, ok := r.(map[string]any); ok && m["type"] == "postgres" {
			delete(resources, name)
		}
	}
	resources["db"] = map[string]any{"type": "postgres", "id": "db-a", "params": map[string]any{"database": "a", "username": "app"}}
	resources["db2"] = map[string]any{"type": "postgres", "id": "db-b", "params": map[string]any{"database": "b", "username": "app"}}
	resources["cache-a"] = map[string]any{"type": "redis", "class": "default", "id": "cache-a"}
	resources["cache-b"] = map[string]any{"type": "redis", "class": "default", "id": "cache-b"}
	doc, err := score.FromMap(scoreMap)
	if err != nil {
		t.Fatal(err)
	}
	appKey, ns, conn := o.CloudApplicationKey, o.CloudNamespaceIdentity, o.CloudConnectionKey
	return planning.Request{
		OrganizationKey: o.OrganizationKey,
		App:             application.Application{Key: appKey, OrganizationKey: o.OrganizationKey, Name: appKey, Version: 1},
		Env: environment.Environment{Key: o.EnvironmentKey, ApplicationKey: appKey, Name: o.EnvironmentName, Type: o.EnvironmentType, NamespaceIdentity: ns, Version: 1,
			ConnectionKey: conn, Profile: application.ProfileAWSEKS, RuntimeStatus: application.RuntimeReady, InfrastructureScope: environment.ScopeEnvironment, Region: o.Region},
		Connection: application.Connection{Key: conn, Kind: application.ConnectionAWS, OrganizationKey: o.OrganizationKey, Status: application.ConnectionReady,
			Config: map[string]any{"cluster": o.ClusterName, "kubeContext": o.KubeContext}},
		BaseSet: environment.NewDocument(), After: doc, WorkloadID: "backend", RunID: o.RunID, Catalog: catalog,
		Terraform: registrytest.Inspector(t, store.New()),
	}
}

// Real registry/planner (no mock): two distinct Aurora and two distinct
// ElastiCache descriptors share ONE VPC provider node. Each consumer has its own
// node/descriptor and an edge to that single VPC; the private subnet IDs come
// from the VPC output reference, never from a literal or a per-consumer network.
func TestAWSTwoDatabasesAndTwoCachesShareOneVPCNode(t *testing.T) {
	plan, err := planning.NewService(placedBundles()).Plan(awsSharedRequest(t))
	if err != nil {
		t.Fatal(err)
	}
	const vpc = "vpc.default#"
	counts := map[string]int{}
	var vpcNode string
	for _, n := range plan.Graph.Nodes {
		counts[strings.SplitN(n.Descriptor, ".", 2)[0]]++
		if strings.HasPrefix(n.Descriptor, vpc) {
			vpcNode = n.Descriptor
		}
	}
	if counts["vpc"] != 1 || counts["postgres"] != 2 || counts["redis"] != 2 || counts["k8s-cluster"] != 1 {
		t.Fatalf("node counts %v", counts)
	}
	for _, consumer := range []string{"postgres.default#shared.db-a", "postgres.default#shared.db-b", "redis.default#shared.cache-a", "redis.default#shared.cache-b"} {
		if !hasEdge(plan, consumer, vpcNode, "resource-reference") {
			t.Errorf("%s has no reference edge to the single VPC node %s", consumer, vpcNode)
		}
		if batchOf(plan, consumer) <= batchOf(plan, vpcNode) {
			t.Errorf("%s is not planned after the VPC", consumer)
		}
	}
	// Each consumer plans its own Terraform instance of its module.
	modules := map[string]int{}
	for _, c := range plan.Terraform {
		modules[c.Module]++
	}
	if modules["vpc"] != 1 || modules["aurora"] != 2 || modules["elasticache"] != 2 {
		t.Fatalf("terraform instances %v", modules)
	}
}
