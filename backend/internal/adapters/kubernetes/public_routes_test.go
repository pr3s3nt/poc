package kubernetes

import (
	"testing"

	"orchestrator/internal/ports/execution"
)

func TestPublicIngressTargetsSelectedServicePort(t *testing.T) {
	route := execution.PublicRoute{ApplicationID: "app-1", EnvironmentID: "staging", Host: "staging.shop.example.com", Paths: []execution.PublicPath{{Path: "/", WorkloadID: "frontend", PortName: "http"}, {Path: "/api", WorkloadID: "backend", PortName: "http"}}}
	object, err := publicIngress("app-1-staging", route, "")
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
	api := rule["http"].(map[string]any)["paths"].([]any)[1].(map[string]any)
	if api["path"] != "/api" || api["backend"].(map[string]any)["service"].(map[string]any)["name"] != "backend" {
		t.Fatal("wrong API route")
	}
	if _, err := publicIngress("app-1-staging", execution.PublicRoute{Host: "invalid host", Paths: route.Paths}, ""); err == nil {
		t.Fatal("invalid host accepted")
	}
}

// ADR-024: the Gateway API route attaches to the configured Gateway with one
// PathPrefix rule per public path and the numeric Service port.
func TestPublicHTTPRouteAttachesToGateway(t *testing.T) {
	gateway, err := ParseGatewayRef("gateway-system/public/https")
	if err != nil {
		t.Fatal(err)
	}
	route := execution.PublicRoute{ApplicationID: "shop", EnvironmentID: "staging", Host: "staging.shop.example.com", Paths: []execution.PublicPath{{Path: "/api", WorkloadID: "backend", PortName: "http", Port: 8080}, {Path: "/", WorkloadID: "frontend", PortName: "web", Port: 80}}}
	object, err := PublicHTTPRoute("shop-staging", route, gateway)
	if err != nil {
		t.Fatal(err)
	}
	spec := object["spec"].(map[string]any)
	parent := spec["parentRefs"].([]any)[0].(map[string]any)
	if parent["namespace"] != "gateway-system" || parent["name"] != "public" || parent["sectionName"] != "https" || object["kind"] != "HTTPRoute" {
		t.Fatalf("parent: %v", parent)
	}
	if spec["hostnames"].([]any)[0] != route.Host {
		t.Fatal("wrong hostname")
	}
	first := spec["rules"].([]any)[0].(map[string]any)
	match := first["matches"].([]any)[0].(map[string]any)["path"].(map[string]any)
	backend := first["backendRefs"].([]any)[0].(map[string]any)
	if match["value"] != "/" || match["type"] != "PathPrefix" || backend["name"] != "frontend" || backend["port"] != 80 {
		t.Fatalf("rule: %v", first)
	}
	labels := object["metadata"].(map[string]any)["labels"].(map[string]any)
	if labels["orchestrator.io/environment"] != "staging" || labels["app.kubernetes.io/managed-by"] != "orchestrator" {
		t.Fatal("route must carry Environment ownership")
	}
	route.Paths[0].Port = 0
	if _, err := PublicHTTPRoute("shop-staging", route, gateway); err == nil {
		t.Fatal("a path without a numeric port must fail")
	}
	for _, bad := range []string{"public", "a/b/c/d", "Bad/x"} {
		if _, err := ParseGatewayRef(bad); err == nil {
			t.Fatalf("%q accepted", bad)
		}
	}
}
