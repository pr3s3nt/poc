package deployment

import (
	"strings"
	"testing"

	"orchestrator/internal/domain/endpoint"
	"orchestrator/internal/ports/execution"
)

func endpointDesired() endpoint.Desired {
	return endpoint.Desired{Hostname: "shop.apps.example.com", URL: "https://shop.apps.example.com/", Port: "http", Path: "/",
		Policy: endpoint.Policy{RootDomain: "apps.example.com", Scheme: "https", GatewayName: "public", GatewayNamespace: "gateway-system", GatewaySectionName: "websecure"}}
}

func renderedService(name, namespace string, ports ...map[string]any) execution.Manifest {
	list := []any{}
	for _, p := range ports {
		list = append(list, p)
	}
	return execution.Manifest{APIVersion: "v1", Kind: "Service", Name: name, Namespace: namespace, Object: map[string]any{
		"apiVersion": "v1", "kind": "Service", "metadata": map[string]any{"name": name, "namespace": namespace}, "spec": map[string]any{"ports": list}}}
}

func TestEndpointManifestTargetsRenderedService(t *testing.T) {
	owner := endpoint.Owner{Organization: "org", Application: "app", Environment: "staging", Generation: 2, Workload: "frontend"}
	manifests := []execution.Manifest{
		{Kind: "Deployment", Name: "frontend", Namespace: "app-ns"},
		renderedService("frontend-web", "app-ns", map[string]any{"name": "metrics", "port": 9090}, map[string]any{"name": "http", "port": 8080, "protocol": "TCP"}),
	}
	route, err := endpointManifest(endpointDesired(), owner, "app-ns", "dep-1", manifests)
	if err != nil {
		t.Fatal(err)
	}
	if route.Kind != "HTTPRoute" || route.APIVersion != "gateway.networking.k8s.io/v1" || route.Namespace != "app-ns" || !strings.HasPrefix(route.Name, "endpoint-") {
		t.Fatalf("unexpected route identity %+v", route)
	}
	again, _ := endpointManifest(endpointDesired(), owner, "app-ns", "dep-2", manifests)
	if again.Name != route.Name {
		t.Fatal("route name must be stable across deployments of one owner")
	}
	other := owner
	other.Environment = "production"
	if moved, _ := endpointManifest(endpointDesired(), other, "app-ns", "dep-1", manifests); moved.Name == route.Name {
		t.Fatal("route name must differ per owner")
	}
	labels := route.Object["metadata"].(map[string]any)["labels"].(map[string]any)
	if labels["orchestrator.io/workload"] != "frontend" || labels["app.kubernetes.io/managed-by"] != "orchestrator" || labels["orchestrator.io/deployment-id"] != "dep-1" {
		t.Fatalf("route is not in the workload inventory: %v", labels)
	}
	spec := route.Object["spec"].(map[string]any)
	parent := spec["parentRefs"].([]any)[0].(map[string]any)
	if parent["name"] != "public" || parent["namespace"] != "gateway-system" || parent["sectionName"] != "websecure" {
		t.Fatalf("parentRef %v", parent)
	}
	if hosts := spec["hostnames"].([]any); len(hosts) != 1 || hosts[0] != "shop.apps.example.com" {
		t.Fatalf("hostnames %v", hosts)
	}
	rule := spec["rules"].([]any)[0].(map[string]any)
	backend := rule["backendRefs"].([]any)[0].(map[string]any)
	if backend["name"] != "frontend-web" || backend["port"] != 8080 {
		t.Fatalf("backendRef %v", backend)
	}
	match := rule["matches"].([]any)[0].(map[string]any)["path"].(map[string]any)
	if match["type"] != "PathPrefix" || match["value"] != "/" {
		t.Fatalf("match %v", match)
	}
}

func TestEndpointManifestRejectsAmbiguousOrForeignRouting(t *testing.T) {
	owner := endpoint.Owner{Organization: "org", Application: "app", Environment: "staging", Workload: "frontend"}
	http := map[string]any{"name": "http", "port": 8080}
	for name, tc := range map[string]struct {
		manifests []execution.Manifest
		want      string
	}{
		"missing Service":      {manifests: []execution.Manifest{{Kind: "Deployment"}}, want: "absent"},
		"missing named port":   {manifests: []execution.Manifest{renderedService("web", "app-ns", map[string]any{"name": "grpc", "port": 9000})}, want: "absent"},
		"two Services":         {manifests: []execution.Manifest{renderedService("a", "app-ns", http), renderedService("b", "app-ns", http)}, want: "multiple"},
		"template route":       {manifests: []execution.Manifest{renderedService("web", "app-ns", http), {Kind: "HTTPRoute", Name: "own"}}, want: "must not also generate"},
		"template ingress":     {manifests: []execution.Manifest{renderedService("web", "app-ns", http), {Kind: "Ingress", Name: "own"}}, want: "must not also generate"},
		"UDP port":             {manifests: []execution.Manifest{renderedService("web", "app-ns", map[string]any{"name": "http", "port": 8080, "protocol": "UDP"})}, want: "TCP"},
		"foreign namespace":    {manifests: []execution.Manifest{renderedService("web", "other-ns", http)}, want: "absent"},
		"ExternalName Service": {manifests: []execution.Manifest{{Kind: "Service", Name: "ext", Object: map[string]any{"metadata": map[string]any{"name": "ext"}, "spec": map[string]any{"type": "ExternalName", "externalName": "example.com", "ports": []any{http}}}}}, want: "external"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := endpointManifest(endpointDesired(), owner, "app-ns", "dep", tc.manifests); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
		})
	}
}
