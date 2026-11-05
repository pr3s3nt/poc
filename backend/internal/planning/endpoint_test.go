package planning

import (
	"strings"
	"testing"

	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning/workloadspec"
)

func endpointPolicy() map[string]any {
	return map[string]any{"rootDomain": "apps.example.com", "scheme": "https", "gatewayName": "public", "gatewayNamespace": "gateway-system", "reservedNames": []any{"www"}}
}

// endpointRequest adds the built-in endpoint Type/Definition and declares one
// endpoint on the acceptance backend workload.
func endpointRequest(t *testing.T, params map[string]any) Request {
	t.Helper()
	req := testRequest(t, "internal-k8s", "backend")
	req.Catalog.Types["endpoint"] = resource.Type{Key: "endpoint", Inputs: []resource.InputField{{Name: "name", Type: "string", Required: true}, {Name: "port", Type: "string", Required: true}, {Name: "path", Type: "string"}}, Outputs: []resource.OutputField{{Name: "hostname", Type: "string"}, {Name: "url", Type: "string"}}}
	req.Catalog.Definitions = append(req.Catalog.Definitions, resource.Definition{Key: "public-endpoint", Name: "Public endpoint", ResourceTypeKey: "endpoint", DriverType: resource.DriverEndpoint, DriverInputs: map[string]any{"values": map[string]any{"variables": endpointPolicy()}}, Criteria: []resource.Criterion{{}}, Version: 1})
	if req.After.Resources == nil {
		req.After.Resources = map[string]workloadspec.ResourceSpec{}
	}
	req.After.Resources["public"] = workloadspec.ResourceSpec{Type: "endpoint", Params: params}
	return req
}

func TestPlanResolvesWorkloadEndpoint(t *testing.T) {
	plan, err := NewService(testBundles()).Plan(endpointRequest(t, map[string]any{"name": "shop", "port": "http", "path": "/api"}))
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	got, ok := plan.Endpoints["backend"]
	if !ok {
		t.Fatalf("endpoint missing: %v", plan.Endpoints)
	}
	if got.Hostname != "shop.apps.example.com" || got.URL != "https://shop.apps.example.com/api" || got.Port != "http" || got.Path != "/api" {
		t.Fatalf("unexpected endpoint %+v", got)
	}
	if got.Policy.GatewayName != "public" || got.Policy.GatewayNamespace != "gateway-system" {
		t.Fatalf("policy not pinned: %+v", got.Policy)
	}
}

func TestPlanRejectsInvalidEndpoints(t *testing.T) {
	for name, tc := range map[string]struct {
		params map[string]any
		mutate func(*Request)
		want   string
	}{
		"missing port":     {params: map[string]any{"name": "shop", "port": "grpc"}, want: "named Service port"},
		"reserved name":    {params: map[string]any{"name": "www", "port": "http"}, want: "reserved"},
		"policy override":  {params: map[string]any{"name": "shop", "port": "http", "gatewayName": "other"}, want: "does not declare input"},
		"multi-label name": {params: map[string]any{"name": "a.b", "port": "http"}, want: "DNS label"},
		"relative path":    {params: map[string]any{"name": "shop", "port": "http", "path": "api"}, want: "absolute"},
		"dot segment path": {params: map[string]any{"name": "shop", "port": "http", "path": "/a/../b"}, want: "absolute"},
		"no Service":       {params: map[string]any{"name": "shop", "port": "http"}, mutate: func(r *Request) { r.After.Service = nil }, want: "needs a Service"},
		"wrong driver": {params: map[string]any{"name": "shop", "port": "http"}, mutate: func(r *Request) {
			for i := range r.Catalog.Definitions {
				if r.Catalog.Definitions[i].Key == "public-endpoint" {
					r.Catalog.Definitions[i].DriverType = resource.DriverEcho
				}
			}
		}, want: "built-in endpoint driver"},
		"invalid policy": {params: map[string]any{"name": "shop", "port": "http"}, mutate: func(r *Request) {
			for i := range r.Catalog.Definitions {
				if r.Catalog.Definitions[i].Key == "public-endpoint" {
					policy := endpointPolicy()
					policy["scheme"] = "ftp"
					r.Catalog.Definitions[i].DriverInputs = map[string]any{"values": map[string]any{"variables": policy}}
				}
			}
		}, want: "scheme"},
	} {
		t.Run(name, func(t *testing.T) {
			req := endpointRequest(t, tc.params)
			if tc.mutate != nil {
				tc.mutate(&req)
			}
			_, err := NewService(testBundles()).Plan(req)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
		})
	}
}

func TestPlanRejectsSecondEndpointOnOneWorkload(t *testing.T) {
	req := endpointRequest(t, map[string]any{"name": "shop", "port": "http"})
	req.After.Resources["admin"] = workloadspec.ResourceSpec{Type: "endpoint", Params: map[string]any{"name": "admin", "port": "http"}}
	if _, err := NewService(testBundles()).Plan(req); err == nil || !strings.Contains(err.Error(), "one endpoint") {
		t.Fatalf("second endpoint accepted: %v", err)
	}
}
