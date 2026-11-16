package catalog_test

import (
	"context"
	"testing"

	"orchestrator/internal/domain/resource"
)

func TestGatewayRegistrationIsSeparateFromClusterAndEndpoint(t *testing.T) {
	ctx := context.Background()
	st, svc, opts := seededCatalog(t, nil)
	withClusterOutput(t, st, opts)
	if err := st.SaveResourceType(ctx, opts.OrganizationKey, resource.Type{Key: "k8s-gateway", Outputs: []resource.OutputField{{Name: "gateway", Type: "any", Required: true}}}); err != nil {
		t.Fatal(err)
	}
	gateway := resource.Definition{Key: "existing-edge", Name: "Existing edge", ResourceTypeKey: "k8s-gateway", DriverType: resource.DriverExistingGateway, Criteria: []resource.Criterion{{}}, DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{"cluster": "${resources['k8s-cluster.internal#environments.@app.@env'].outputs.cluster}", "rootDomain": "apps.example.com", "scheme": "http", "gatewayName": "public", "gatewayNamespace": "gateway-system", "gatewaySectionName": "web"}}}}
	created, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, gateway)
	if err != nil || created.DriverAccount != "" {
		t.Fatalf("Gateway registration: %+v %v", created, err)
	}
	for _, bad := range []string{"literal cluster", "wrong output", "account", "inline credential"} {
		t.Run(bad, func(t *testing.T) {
			def := gateway
			def.Key = "bad-gateway"
			variables := map[string]any{}
			for key, value := range gateway.Variables() {
				variables[key] = value
			}
			def.DriverInputs = map[string]any{"values": map[string]any{"variables": variables}}
			switch bad {
			case "literal cluster":
				variables["cluster"] = "my-cluster"
			case "wrong output":
				variables["cluster"] = "${resources['k8s-cluster.internal#environments.@app.@env'].outputs.name}"
			case "account":
				def.DriverAccount = "other"
			case "inline credential":
				variables["token"] = "sentinel"
			}
			if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, def); err == nil {
				t.Fatal("invalid Gateway configuration accepted")
			}
		})
	}
}
