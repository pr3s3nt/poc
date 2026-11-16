package templatedriver

import (
	"orchestrator/internal/adapters/kubernetes"
	"orchestrator/internal/ports/execution"
)

// guardEndpoint validates intent; it never constructs or repairs a manifest.
func guardEndpoint(req execution.RenderRequest, delivery kubernetes.Delivery, objects []execution.Manifest) error {
	if req.Endpoint == nil {
		return nil
	}
	want := req.Endpoint
	var route *execution.Manifest
	serviceName := ""
	servicePort := float64(0)
	for i := range objects {
		m := &objects[i]
		if m.Kind == "Gateway" || m.Kind == "Ingress" {
			return violated("endpoint templates cannot publish Gateway or Ingress")
		}
		if m.Kind == "HTTPRoute" {
			if route != nil || m.APIVersion != "gateway.networking.k8s.io/v1" || m.Name != want.Name || m.Namespace != req.Namespace {
				return violated("endpoint must render exactly one authorized HTTPRoute")
			}
			route = m
		}
		if m.APIVersion != "v1" || m.Kind != "Service" {
			continue
		}
		spec := object(m.Object["spec"])
		for _, raw := range list(spec["ports"]) {
			port := object(raw)
			if port["name"] != want.Port {
				continue
			}
			if serviceName != "" || spec["type"] == "ExternalName" || spec["externalName"] != nil || port["protocol"] != nil && port["protocol"] != "TCP" {
				return violated("endpoint Service is ambiguous or external")
			}
			serviceName, servicePort = m.Name, toNumber(port["port"])
		}
	}
	if route == nil || serviceName == "" || servicePort < 1 || servicePort > 65535 {
		return violated("endpoint route or named Service port is missing")
	}
	labels := object(object(route.Object["metadata"])["labels"])
	for name, value := range delivery.Labels {
		if labels[name] != value {
			return violated("endpoint route must carry workload ownership")
		}
	}
	spec := object(route.Object["spec"])
	if len(spec) != 3 {
		return violated("endpoint route contains unsupported policy")
	}
	hosts, parents, rules := list(spec["hostnames"]), list(spec["parentRefs"]), list(spec["rules"])
	if len(hosts) != 1 || hosts[0] != want.Hostname || len(parents) != 1 || len(rules) != 1 {
		return violated("endpoint hostname or routing scope differs from intent")
	}
	parent := object(parents[0])
	expectedParentFields := 4
	if want.GatewaySectionName != "" {
		expectedParentFields++
	}
	if len(parent) != expectedParentFields || parent["group"] != "gateway.networking.k8s.io" || parent["kind"] != "Gateway" || parent["name"] != want.GatewayName || parent["namespace"] != want.GatewayNamespace || want.GatewaySectionName != "" && parent["sectionName"] != want.GatewaySectionName {
		return violated("endpoint Gateway differs from intent")
	}
	rule := object(rules[0])
	matches, backends := list(rule["matches"]), list(rule["backendRefs"])
	if len(rule) != 2 || len(matches) != 1 || len(backends) != 1 {
		return violated("endpoint rule differs from intent")
	}
	match, backend := object(matches[0]), object(backends[0])
	path := object(match["path"])
	if len(match) != 1 || len(path) != 2 || path["type"] != "PathPrefix" || path["value"] != want.Path {
		return violated("endpoint path differs from intent")
	}
	if len(backend) != 4 || backend["group"] != "" || backend["kind"] != "Service" || backend["name"] != serviceName || toNumber(backend["port"]) != servicePort {
		return violated("endpoint backend differs from rendered Service")
	}
	return nil
}
