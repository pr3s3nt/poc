package planning_test

import (
	"strings"
	"testing"

	"orchestrator/internal/domain/endpoint"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning"
	"orchestrator/internal/planning/workloadspec"
)

func gatewayRequest(t *testing.T) planning.Request {
	t.Helper()
	req := k8sRequest(t, false)
	req.Catalog.Types[endpoint.GatewayType] = resource.Type{Key: endpoint.GatewayType, Outputs: []resource.OutputField{{Name: "gateway", Type: "any", Required: true}}}
	req.Catalog.Types["endpoint"] = resource.Type{Key: "endpoint", Inputs: []resource.InputField{{Name: "name", Type: "string", Required: true}, {Name: "port", Type: "string", Required: true}, {Name: "path", Type: "string"}}, Outputs: []resource.OutputField{{Name: "hostname", Type: "string"}, {Name: "url", Type: "string"}}}
	req.Catalog.Definitions = append(req.Catalog.Definitions,
		resource.Definition{Key: "existing-public-gateway", Name: "Existing public Gateway", Version: 1, ResourceTypeKey: endpoint.GatewayType, DriverType: resource.DriverExistingGateway,
			DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{"cluster": clusterRef, "rootDomain": "apps.example.com", "scheme": "http", "gatewayName": "public", "gatewayNamespace": "gateway-system", "gatewaySectionName": "web"}}}, Criteria: []resource.Criterion{{}}},
		resource.Definition{Key: "gateway-bound-endpoint", Name: "Endpoint", Version: 1, ResourceTypeKey: "endpoint", DriverType: resource.DriverEndpoint,
			DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{"gateway": "${resources['k8s-gateway.default#environments.@app.@env'].outputs.gateway}"}}}, Criteria: []resource.Criterion{{}}},
	)
	req.After.Resources["web"] = workloadspec.ResourceSpec{Type: "endpoint", Params: map[string]any{"name": "shop", "port": "http", "path": "/api"}}
	return req
}

func TestGatewayIsSeparatePreexistingResourceAndEndpointUsesItsPolicy(t *testing.T) {
	plan, err := planning.NewService(nil).Plan(gatewayRequest(t))
	if err != nil {
		t.Fatal(err)
	}
	desired := plan.Endpoints["backend"]
	if !desired.TemplateOwned || desired.Cluster == "" || desired.Hostname != "shop.apps.example.com" || desired.Policy.GatewayName != "public" {
		t.Fatalf("unexpected intent %+v", desired)
	}
	var gateway string
	for _, node := range plan.Graph.Nodes {
		if node.ResourceType == endpoint.GatewayType {
			gateway = node.Descriptor
		}
	}
	if gateway == "" || plan.Placements[gateway].Cluster != desired.Cluster {
		t.Fatal("Gateway cluster was not pinned")
	}
	for _, intent := range plan.ResourceIntents(nil) {
		if intent.Descriptor == gateway && intent.Ownership != planning.OwnerPreexisting {
			t.Fatal("Gateway must not be owned by the application")
		}
	}
}

func TestGatewayEndpointRejectsWrongResourceOrMissingTemplateCapability(t *testing.T) {
	for _, mode := range []string{"wrong-resource", "wrong-cluster-output", "missing-capability", "missing-policy"} {
		t.Run(mode, func(t *testing.T) {
			req := gatewayRequest(t)
			switch mode {
			case "wrong-resource":
				for i := range req.Catalog.Definitions {
					if req.Catalog.Definitions[i].Key == "gateway-bound-endpoint" {
						req.Catalog.Definitions[i].Variables()["gateway"] = clusterRef
					}
				}
			case "wrong-cluster-output":
				for i := range req.Catalog.Definitions {
					if req.Catalog.Definitions[i].Key == "existing-public-gateway" {
						req.Catalog.Definitions[i].Variables()["cluster"] = strings.Replace(clusterRef, ".outputs.cluster", ".outputs.name", 1)
					}
				}
			case "missing-capability":
				for key, bundle := range req.Catalog.TemplateSources {
					bundle.Contract.Manifest.WorkloadCapabilities = nil
					req.Catalog.TemplateSources[key] = bundle
				}
			case "missing-policy":
				for i := range req.Catalog.Definitions {
					if req.Catalog.Definitions[i].Key == "existing-public-gateway" {
						delete(req.Catalog.Definitions[i].Variables(), "gatewayName")
					}
				}
			}
			if _, err := planning.NewService(nil).Plan(req); err == nil || !strings.Contains(strings.ToLower(err.Error()), "gateway") && !strings.Contains(strings.ToLower(err.Error()), "endpoint") {
				t.Fatalf("expected routing rejection, got %v", err)
			}
		})
	}
}
