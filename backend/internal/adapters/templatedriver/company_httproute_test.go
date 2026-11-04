package templatedriver

import (
	"context"
	"path/filepath"
	"testing"

	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/templatesource"
	"orchestrator/internal/renderengine"
)

// Exercise the publishable Git bundle through the real renderer and admission,
// without changing the immutable embedded release or contacting a cluster.
func TestCompanyHTTPRouteFollowsWorkloadService(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "..", "projects", "platform-resources", "templates", "standard")
	files := readBundle(t, dir)
	bundle, err := renderengine.Validate(files)
	if err != nil {
		t.Fatal(err)
	}
	renderer, selections := gitRenderer(t, map[string]map[string][]byte{"company": files})
	for _, tc := range []struct {
		name                     string
		enabled                  bool
		workload, listener, port string
		conflict                 bool
		want                     int
		wantError                bool
	}{
		{"frontend", true, "frontend", "https", "http", false, 1, false},
		{"optional-listener", true, "frontend", "", "http", false, 1, false},
		{"disabled", false, "frontend", "https", "http", false, 0, false},
		{"other-workload", true, "backend", "https", "http", false, 0, false},
		{"missing-port", true, "frontend", "https", "missing", false, 0, true},
		{"platform-route-conflict", true, "frontend", "https", "http", true, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := gitRequest(selections["company"])
			req.WorkloadID = tc.workload
			if tc.workload == "frontend" {
				req.Module.Annotations = map[string]string{"routing.orchestrator.dev/hostname": "web.company.example", "routing.orchestrator.dev/path": "/", "routing.orchestrator.dev/service-port": tc.port}
			}
			req.Module.Spec.Service = &environment.Service{Ports: map[string]environment.Port{"http": {Port: 80, TargetPort: 8080}}}
			if tc.conflict {
				req.Module.Spec.Service.PublicRoutes = []environment.PublicPath{{Path: "/", Port: "http"}}
			}
			inputs := map[string]any{"httpRouteEnabled": tc.enabled, "httpRouteGatewayName": "company-gateway", "httpRouteGatewayNamespace": "networking", "httpRouteGatewaySectionName": tc.listener}
			req.Selection.Inputs, err = templatesource.Effective(bundle.Contract.InputsSchema, inputs)
			if err != nil {
				t.Fatal(err)
			}
			req.Selection.InputsHash, _ = templatesource.InputsHash(req.Selection.Inputs)
			manifests, err := renderer.Render(context.Background(), req)
			if tc.wantError {
				if err == nil {
					t.Fatal("invalid route accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			count := 0
			for _, manifest := range manifests {
				if manifest.Kind == "Gateway" || manifest.Kind == "GatewayClass" {
					t.Fatal("template must not own Gateway infrastructure")
				}
				if manifest.Kind != "HTTPRoute" {
					continue
				}
				count++
				if manifest.Auxiliary {
					t.Fatal("HTTPRoute must be in workload inventory, not platform auxiliary cleanup")
				}
				meta := object(manifest.Object["metadata"])
				spec := object(manifest.Object["spec"])
				if meta["name"] != tc.workload || meta["namespace"] != req.Namespace || object(meta["labels"])["orchestrator.io/workload"] != tc.workload {
					t.Fatalf("route missing workload ownership: %v", meta)
				}
				parent := object(list(spec["parentRefs"])[0])
				if parent["name"] != "company-gateway" || parent["namespace"] != "networking" {
					t.Fatal(parent)
				}
				if tc.listener == "" {
					if _, present := parent["sectionName"]; present {
						t.Fatal("empty listener must be omitted")
					}
				} else if parent["sectionName"] != tc.listener {
					t.Fatal(parent)
				}
				rule := object(list(spec["rules"])[0])
				backend := object(list(rule["backendRefs"])[0])
				if backend["name"] != tc.workload || backend["port"] != float64(80) || list(spec["hostnames"])[0] != "web.company.example" {
					t.Fatalf("route does not follow Service: %v", spec)
				}
			}
			if count != tc.want {
				t.Fatalf("got %d routes, want %d", count, tc.want)
			}
		})
	}
}
