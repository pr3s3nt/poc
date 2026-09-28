package kubernetes

import (
	"testing"

	"orchestrator/internal/ports/execution"
)

func TestPublicIngressTargetsSelectedServicePort(t *testing.T) {
	route := execution.PublicRoute{ApplicationID: "app-1", EnvironmentID: "staging", WorkloadID: "frontend", Host: "staging.shop.example.com", PortName: "http"}
	object, err := publicIngress("app-1-staging", route)
	if err != nil {
		t.Fatal(err)
	}
	spec := object["spec"].(map[string]any)
	if spec["ingressClassName"] != "traefik" {
		t.Fatal("wrong Ingress class")
	}
	rule := spec["rules"].([]any)[0].(map[string]any)
	if rule["host"] != route.Host {
		t.Fatal("wrong host")
	}
	path := rule["http"].(map[string]any)["paths"].([]any)[0].(map[string]any)
	service := path["backend"].(map[string]any)["service"].(map[string]any)
	if service["name"] != "frontend" || service["port"].(map[string]any)["name"] != "http" {
		t.Fatal("wrong backend")
	}
	if _, err := publicIngress("app-1-staging", execution.PublicRoute{Host: "invalid host", PortName: "http"}); err == nil {
		t.Fatal("invalid host accepted")
	}
}
