package planning

import (
	"fmt"
	"strings"

	"orchestrator/internal/domain/endpoint"
	"orchestrator/internal/domain/resource"
)

func endpointErr(reason string, err error) error {
	return &StageError{Stage: StageRoutes, Reason: reason, Err: err}
}

// EndpointTaken classifies a hostname reservation conflict for public output.
func EndpointTaken(hostname string) error {
	return endpointErr(ReasonEndpointTaken, fmt.Errorf("endpoint: hostname %q is already reserved", hostname))
}

// planEndpoints resolves every workload endpoint against its matched built-in
// endpoint Definition policy (EP-01..03). Nothing is reserved here.
func planEndpoints(plan *Plan, catalog Catalog) (map[string]endpoint.Desired, error) {
	out := map[string]endpoint.Desired{}
	for _, node := range plan.Graph.Nodes {
		if node.ResourceType != "endpoint" {
			continue
		}
		if node.Scope.Type != resource.ScopeWorkload {
			return nil, endpointErr(ReasonEndpointInvalid, fmt.Errorf("endpoint: shared endpoints are not supported"))
		}
		parsed, err := resource.ParseDescriptor(node.Descriptor)
		if err != nil {
			return nil, err
		}
		parts := strings.Split(parsed.ID, ".")
		if len(parts) < 4 || parts[0] != "modules" || parts[2] != "externals" {
			return nil, endpointErr(ReasonEndpointInvalid, fmt.Errorf("endpoint: endpoint must belong to one workload"))
		}
		workload := parts[1]
		if _, exists := out[workload]; exists {
			return nil, endpointErr(ReasonEndpointInvalid, fmt.Errorf("endpoint: only one endpoint is supported per workload"))
		}
		match := plan.Matches[node.Descriptor]
		var def resource.Definition
		for _, candidate := range catalog.Definitions {
			if candidate.Key == match.DefinitionKey {
				def = candidate
				break
			}
		}
		if def.DriverType != resource.DriverEndpoint {
			return nil, endpointErr(ReasonEndpointPolicy, fmt.Errorf("endpoint: endpoint requires the built-in endpoint driver"))
		}
		policy, err := endpoint.ParsePolicy(def.Variables())
		if err != nil {
			return nil, endpointErr(ReasonEndpointPolicy, err)
		}
		desired, err := endpoint.Resolve(policy, node.Params)
		if err != nil {
			return nil, endpointErr(ReasonEndpointInvalid, err)
		}
		module, ok := plan.CandidateSet.Modules[workload]
		if !ok || module.Spec.Service == nil {
			return nil, endpointErr(ReasonEndpointInvalid, fmt.Errorf("endpoint: workload needs a Service"))
		}
		service := module.Spec.Service
		port, ok := service.Ports[desired.Port]
		if !ok || port.Port < 1 || port.Port > 65535 {
			return nil, endpointErr(ReasonEndpointInvalid, fmt.Errorf("endpoint: named Service port does not exist"))
		}
		if len(service.PublicRoutes) > 0 {
			return nil, endpointErr(ReasonEndpointInvalid, fmt.Errorf("endpoint: cannot combine endpoint with legacy publicRoutes"))
		}
		for _, existing := range out {
			if existing.Hostname == desired.Hostname {
				return nil, endpointErr(ReasonEndpointInvalid, fmt.Errorf("endpoint: duplicate hostname in Candidate Set"))
			}
		}
		out[workload] = desired
	}
	return out, nil
}
