package kubernetes

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"orchestrator/internal/domain/endpoint"
	"orchestrator/internal/ports/execution"
)

const observedGateway = `{"metadata":{"uid":"gateway-original","generation":2},"spec":{"listeners":[{"name":"web","protocol":"HTTP","hostname":"*.apps.example.com","allowedRoutes":{"namespaces":{"from":"All"}}}]},"status":{"conditions":[{"type":"Accepted","status":"True","observedGeneration":2},{"type":"Programmed","status":"True","observedGeneration":2}],"listeners":[{"name":"web","conditions":[{"type":"Accepted","status":"True"},{"type":"ResolvedRefs","status":"True"}],"supportedKinds":[{"group":"gateway.networking.k8s.io","kind":"HTTPRoute"}]}]}}`

func gatewayAdapter(t *testing.T, observation string) *ExistingGatewayAdapter {
	t.Helper()
	dir := t.TempDir()
	data := filepath.Join(dir, "gateway.json")
	if err := os.WriteFile(data, []byte(observation), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "kubectl")
	script := "#!/bin/sh\ncase \" $* \" in\n*' get gateways.gateway.networking.k8s.io '*) cat '" + data + "';;\n*' get namespace '*) echo '{\"metadata\":{\"labels\":{\"team\":\"app\"}}}';;\n*) exit 99;;\nesac\n"
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	return &ExistingGatewayAdapter{KubectlPath: path, Credentials: &stubCredentials{}}
}

func gatewayDesired() endpoint.Desired {
	return endpoint.Desired{TemplateOwned: true, Hostname: "shop.apps.example.com", Policy: endpoint.Policy{RootDomain: "apps.example.com", Scheme: "http", GatewayName: "public", GatewayNamespace: "gateway-system", GatewaySectionName: "web"}}
}

func TestExistingGatewayReadsInfrastructureWithoutTakingOwnership(t *testing.T) {
	adapter := gatewayAdapter(t, observedGateway)
	target := eksTarget()
	policy := gatewayDesired().Policy
	raw, _ := json.Marshal(policy)
	var inputs map[string]any
	if err := json.Unmarshal(raw, &inputs); err != nil {
		t.Fatal(err)
	}
	inputs["cluster"] = target.ClusterOutput("k8s-cluster.default#environments.shop.staging", 3)
	result, err := adapter.Provision(context.Background(), execution.ProvisionRequest{OrganizationKey: target.Organization, Target: target, Inputs: inputs})
	if err != nil {
		t.Fatal(err)
	}
	if result.State["uid"] != "gateway-original" || result.InstanceID != "" || result.Target != nil {
		t.Fatalf("unexpected lifecycle ownership %+v", result)
	}
	if _, ok := result.Outputs["gateway"]; !ok {
		t.Fatal("typed Gateway output missing")
	}
	target.Namespace = "app-ns"
	if err := adapter.CheckGateway(context.Background(), target, gatewayDesired()); err != nil {
		t.Fatal(err)
	}
}

func TestExistingGatewayRefusesStaleUnreadyAndDisallowedRoutes(t *testing.T) {
	for name, observation := range map[string]string{
		"unready":             strings.Replace(observedGateway, `"Programmed","status":"True"`, `"Programmed","status":"False"`, 1),
		"stale":               strings.ReplaceAll(observedGateway, `"observedGeneration":2`, `"observedGeneration":1`),
		"same namespace only": strings.Replace(observedGateway, `"from":"All"`, `"from":"Same"`, 1),
		"different hostname":  strings.Replace(observedGateway, `*.apps.example.com`, `*.other.example.com`, 1),
		"wrong listener":      strings.ReplaceAll(observedGateway, `"name":"web"`, `"name":"private"`),
		"namespace selector":  strings.Replace(observedGateway, `"from":"All"`, `"from":"Selector","selector":{"matchLabels":{"team":"other"}}`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			target := eksTarget()
			target.Namespace = "app-ns"
			if err := gatewayAdapter(t, observation).CheckGateway(context.Background(), target, gatewayDesired()); err == nil {
				t.Fatal("disallowed endpoint was admitted")
			}
		})
	}
}
