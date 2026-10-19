package bootstraptest

import (
	"context"
	"testing"

	"orchestrator/internal/adapters/terraform/registry"
	"orchestrator/internal/bootstrap"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/seed"
)

// EKSCatalog saves the Definition-driven catalog of V05 into a fake-adapter application: VPC and EKS with the Definitions' own AWS
// account and literal region, the namespace and PostgreSQL bound to the cluster RESOURCE, and a workload template that references
// the cluster the same plan creates. There is no Environment profile, Connection or binding anywhere.
func EKSCatalog(t testing.TB, app *bootstrap.App, opts seed.Options) {
	t.Helper()
	ctx := context.Background()
	refs, err := registry.PackagedRefs()
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Store.SaveConnection(ctx, application.Connection{ID: "id-aws-a", Key: "aws-a", Name: "aws-a", OrganizationKey: opts.OrganizationKey, Kind: application.ConnectionAWS,
		AuthenticationType: application.AuthAWSAccessKey, Status: application.ConnectionReady, Version: 5, SecretRef: "secret://x", Config: map[string]any{"accountId": "000000000000"}}); err != nil {
		t.Fatal(err)
	}
	types := []resource.Type{
		{Key: "workload"},
		{Key: "vpc", Outputs: []resource.OutputField{{Name: "id", Type: "string", Required: true}, {Name: "subnetIds", Type: "any"}}},
		{Key: "k8s-cluster", Outputs: []resource.OutputField{{Name: "name", Type: "string"}, {Name: "cluster", Type: "any", Required: true}}},
		{Key: "k8s-namespace", Outputs: []resource.OutputField{{Name: "name", Type: "string", Required: true}}},
		{Key: "postgres", Inputs: []resource.InputField{{Name: "database", Type: "string", Required: true}, {Name: "username", Type: "string", Required: true}},
			Outputs: []resource.OutputField{{Name: "host", Type: "string", Required: true}, {Name: "port", Type: "number", Required: true}, {Name: "database", Type: "string", Required: true}, {Name: "username", Type: "string", Required: true}, {Name: "password", Type: "string", Required: true, Secret: true}}},
	}
	for _, typ := range types {
		if err := app.Store.SaveResourceType(ctx, opts.OrganizationKey, typ); err != nil {
			t.Fatal(err)
		}
	}
	awsDef := func(key, typ, module, class string, variables map[string]any, outputs map[string]any) resource.Definition {
		variables["region"] = "us-east-1"
		return resource.Definition{Key: key, Name: key, Version: 1, ResourceTypeKey: typ, DriverType: resource.DriverTerraform, DriverAccount: "aws-a", Criteria: []resource.Criterion{{Class: class}},
			DriverInputs: map[string]any{"values": map[string]any{"source": refs[module].Map(), "variables": variables, "input_mapping": map[string]any{}, "output_mapping": outputs,
				"provider_bindings": map[string]any{"aws": map[string]any{"target": tfmodule.TargetAccount}}}}}
	}
	k8sDef := func(key, typ, module, class string, variables map[string]any, inputs, outputs map[string]any) resource.Definition {
		return resource.Definition{Key: key, Name: key, Version: 1, ResourceTypeKey: typ, DriverType: resource.DriverTerraform, Criteria: []resource.Criterion{{Class: class}},
			DriverInputs: map[string]any{"values": map[string]any{"source": refs[module].Map(), "variables": variables, "input_mapping": inputs, "output_mapping": outputs,
				"provider_bindings": map[string]any{"kubernetes": map[string]any{"target": tfmodule.TargetResource, "resource_ref": "k8s-cluster.eks#environments.@app.@env"}}}}}
	}
	nsOut := "${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}"
	defs := []resource.Definition{
		awsDef("vpc-aws", "vpc", "vpc", "default", map[string]any{"name": "${context.infra.resourceName}", "cidr": "10.42.0.0/16"}, map[string]any{"id": "id", "subnetIds": "subnetIds"}),
		awsDef("cluster-eks", "k8s-cluster", "eks", "eks", map[string]any{"name": "${context.infra.resourceName}", "subnet_ids": "${resources['vpc.default#environments.@app.@env'].outputs.subnetIds}",
			"kubernetes_version": "1.35", "node_instance_type": "t4g.medium", "node_capacity_type": "SPOT", "node_ami_type": "AL2023_ARM_64_STANDARD", "node_disk_size": float64(20), "ebs_csi_driver": true, "public_access_cidrs": []any{"203.0.113.7/32"}}, map[string]any{"name": "name"}),
		k8sDef("namespace-eks", "k8s-namespace", "k8s-namespace", "default", map[string]any{"name": "${context.env.namespace}"}, map[string]any{}, map[string]any{"name": "name"}),
		k8sDef("pg-eks", "postgres", "postgres-k8s", "in-cluster", map[string]any{"namespace": nsOut}, map[string]any{"database": map[string]any{"param": "database"}, "username": map[string]any{"param": "username"}},
			map[string]any{"host": "host", "port": "port", "database": "database", "username": "username", "password": "password"}),
		{Key: "render-eks", Name: "render", Version: 1, ResourceTypeKey: "workload", DriverType: resource.DriverScoreK8s, Criteria: []resource.Criterion{{}},
			DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{"render_bundle": bootstrap.FakeRenderBundleID, "namespace": nsOut, "cluster": "${resources['k8s-cluster.eks#environments.@app.@env'].outputs.cluster}"}}}},
	}
	for _, def := range defs {
		if err := app.Store.SaveResourceDefinition(ctx, opts.OrganizationKey, def); err != nil {
			t.Fatalf("%s: %v", def.Key, err)
		}
	}
}
