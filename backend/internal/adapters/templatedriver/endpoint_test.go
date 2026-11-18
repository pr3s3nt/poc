package templatedriver

import (
	"context"
	"strings"
	"testing"

	"orchestrator/internal/domain/endpoint"
	"orchestrator/internal/ports/execution"
)

func TestEndpointTemplateRendersRouteToActualService(t *testing.T) {
	r, selections := gitRenderer(t, map[string]map[string][]byte{"standard": readBundle(t, standardDir)})
	req := gitRequest(selections["standard"])
	req.Endpoint = (&endpoint.Desired{TemplateOwned: true, Hostname: "shop.apps.example.com", Port: "http", Path: "/api", Policy: endpoint.Policy{GatewayName: "public", GatewayNamespace: "gateway-system", GatewaySectionName: "web"}}).Intent(endpoint.Owner{Organization: "org", Application: "shop", Environment: "staging", Workload: req.WorkloadID})
	manifests, err := r.Render(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range manifests {
		if m.Kind != "HTTPRoute" {
			continue
		}
		if m.Name != req.Endpoint.Name || m.Namespace != req.Namespace {
			t.Fatal("route identity differs from intent")
		}
		spec := object(m.Object["spec"])
		backend := object(list(object(list(spec["rules"])[0])["backendRefs"])[0])
		if backend["name"] != req.WorkloadID || toNumber(backend["port"]) != 8080 {
			t.Fatalf("unexpected Service %+v", backend)
		}
		return
	}
	t.Fatal("Git template did not render HTTPRoute")
}

func TestEndpointTemplateRejectsMissingOrChangedRoute(t *testing.T) {
	for _, mode := range []string{"missing", "hostname", "gateway", "backend", "duplicate", "extra-rule"} {
		t.Run(mode, func(t *testing.T) {
			files := readBundle(t, standardDir)
			text := string(files["resources/httproute.tpl"])
			switch mode {
			case "missing":
				text = "[]"
			case "hostname":
				text = strings.ReplaceAll(text, "$endpoint.hostname", `"other.example.com"`)
			case "gateway":
				text = strings.ReplaceAll(text, `"name" $endpoint.gatewayName`, `"name" "other"`)
			case "backend":
				text = strings.ReplaceAll(text, "$port.port", "9999")
			case "duplicate":
				files["resources/duplicate-route.tpl"] = []byte(text)
				files["bundle.yaml"] = []byte(strings.Replace(string(files["bundle.yaml"]), "  - resources/httproute.tpl", "  - resources/httproute.tpl\n  - resources/duplicate-route.tpl", 1))
			case "extra-rule":
				text = strings.ReplaceAll(text, `"rules" (list $rule)`, `"rules" (list $rule $rule)`)
			}
			files["resources/httproute.tpl"] = []byte(text)
			r, selections := gitRenderer(t, map[string]map[string][]byte{"changed": files})
			req := gitRequest(selections["changed"])
			req.Endpoint = &endpoint.Intent{Name: "endpoint-test", Hostname: "shop.apps.example.com", Port: "http", Path: "/", GatewayName: "public", GatewayNamespace: "gateway-system", GatewaySectionName: "web"}
			if _, err := r.Render(context.Background(), req); err == nil {
				t.Fatal("invalid route was admitted")
			}
		})
	}
}

var _ execution.WorkloadRenderer = (*Renderer)(nil)
