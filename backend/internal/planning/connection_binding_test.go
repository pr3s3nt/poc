package planning

import (
	"testing"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
)

func labCluster(connection string) resource.Definition {
	return resource.Definition{
		Key: "cluster-lab", Name: "cluster-lab", Version: 1, ResourceTypeKey: "k8s-cluster", DriverType: resource.DriverExistingCluster,
		DriverAccount: connection,
		DriverInputs:  map[string]any{"values": map[string]any{"variables": map[string]any{"name": "${context.connection.cluster}", "kubeContext": "${context.connection.context}"}}},
		Criteria:      []resource.Criterion{{Class: "internal", ResourceID: "connections.lab"}},
	}
}

// UC-03 V02-02: the Kubernetes Definition carries no account; its legacy
// Environment binding is not an account selector or a matching filter.
func TestPlan_KubernetesDefinitionNeedsNoDriverAccount(t *testing.T) {
	req := testRequest(t, application.ProfileInternalK8s, "backend")
	for _, def := range req.Catalog.Definitions {
		if def.DriverType == resource.DriverKubernetes && def.DriverAccount != "" {
			t.Fatalf("seeded Kubernetes definition %s names an account", def.Key)
		}
	}
	if _, err := NewService(testBundles()).Plan(req); err != nil {
		t.Fatalf("plan: %v", err)
	}
}

func mutateDefinition(req *Request, key string, change func(*resource.Definition)) {
	for i := range req.Catalog.Definitions {
		if req.Catalog.Definitions[i].Key == key {
			change(&req.Catalog.Definitions[i])
		}
	}
}

func awsPins(t *testing.T, plan *Plan) map[string]tfmodule.ProviderPin {
	t.Helper()
	out := map[string]tfmodule.ProviderPin{}
	for _, contract := range plan.Terraform {
		for _, pin := range contract.ProviderBindings {
			if pin.Kind == tfmodule.BindingAWS {
				out[contract.DefinitionKey] = pin
			}
		}
	}
	return out
}

func TestPlan_AWSRegionMustBeALiteralAwsRegion(t *testing.T) {
	for name, region := range map[string]any{"missing": nil, "placeholder": "${context.env.region}", "bad shape": "mars-1", "number": 1} {
		req := testRequest(t, application.ProfileAWSEKS, "backend")
		mutateDefinition(&req, "vpc-aws", func(d *resource.Definition) {
			variables := d.DriverInputs["values"].(map[string]any)["variables"].(map[string]any)
			if region == nil {
				delete(variables, "region")
			} else {
				variables["region"] = region
			}
		})
		if _, err := NewService(testBundles()).Plan(req); err == nil {
			t.Fatalf("%s region was planned", name)
		}
	}
}
