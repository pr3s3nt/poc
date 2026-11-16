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
		policy, cluster, selected, err := endpointPolicyForPlan(plan, node, def, catalog)
		if err != nil {
			return nil, endpointErr(ReasonEndpointPolicy, err)
		}
		desired, err := endpoint.Resolve(policy, node.Params)
		if err != nil {
			return nil, endpointErr(ReasonEndpointInvalid, err)
		}
		desired.Cluster, desired.TemplateOwned = cluster, selected
		if selected && endpointWorkloadCluster(plan, workload) != cluster {
			return nil, endpointErr(ReasonEndpointPolicy, fmt.Errorf("endpoint: Gateway cluster differs from workload namespace cluster"))
		}
		module, ok := plan.CandidateSet.Modules[workload]
		if !ok || module.Spec.EffectiveService() == nil {
			return nil, endpointErr(ReasonEndpointInvalid, fmt.Errorf("endpoint: workload needs a Service"))
		}
		service := module.Spec.EffectiveService()
		port, ok := service.Ports[desired.Port]
		if !ok || port.Port < 1 || port.Port > 65535 {
			return nil, endpointErr(ReasonEndpointInvalid, fmt.Errorf("endpoint: named Service port does not exist"))
		}
		// EP-07: workloads of this Environment share a hostname by distinct paths.
		for _, existing := range out {
			if existing.Claim() == desired.Claim() {
				return nil, endpointErr(ReasonEndpointInvalid, fmt.Errorf("endpoint: duplicate hostname path in Candidate Set"))
			}
		}
		out[workload] = desired
	}
	return out, nil
}

func endpointWorkloadCluster(plan *Plan, workload string) string {
	for _, node := range plan.Graph.Nodes {
		if node.Kind == NodeWorkload && descriptorID(node.Descriptor) == "modules."+workload {
			return plan.Placements[node.Descriptor].Cluster
		}
	}
	return ""
}

func endpointPolicyForPlan(plan *Plan, node Node, def resource.Definition, catalog Catalog) (endpoint.Policy, string, bool, error) {
	ref, selected, err := endpoint.GatewaySelection(def.Variables())
	if err != nil {
		return endpoint.Policy{}, "", selected, err
	}
	if !selected {
		p, err := endpoint.ParsePolicy(def.Variables())
		return p, "", false, err
	}
	descriptor := node.Bindings[ref]
	gateway, exists := plan.Graph.Node(descriptor)
	match, matched := plan.Matches[descriptor]
	if !exists || !matched || gateway.ResourceType != endpoint.GatewayType || match.DriverType != resource.DriverExistingGateway {
		return endpoint.Policy{}, "", true, fmt.Errorf("endpoint: Gateway resource is unresolved or has the wrong driver")
	}
	for _, definition := range catalog.Definitions {
		if definition.Key != match.DefinitionKey {
			continue
		}
		if len(gateway.Params) != 0 {
			return endpoint.Policy{}, "", true, fmt.Errorf("Gateway accepts no developer inputs")
		}
		clusterText, _ := definition.Variables()[resource.VariableCluster].(string)
		clusterReference, output, whole := resource.WorkloadReference(clusterText)
		clusterNode, found := plan.Graph.Node(gateway.Bindings[clusterReference])
		if !whole || output != resource.ClusterTargetOutput || !found || clusterNode.ResourceType != TypeCluster {
			return endpoint.Policy{}, "", true, fmt.Errorf("Gateway must reference a whole Kubernetes cluster target output")
		}
		policy, err := endpoint.GatewayPolicy(definition.Variables())
		if err != nil {
			return endpoint.Policy{}, "", true, err
		}
		placement, ok := plan.Placements[descriptor]
		if !ok || placement.Cluster == "" {
			return endpoint.Policy{}, "", true, fmt.Errorf("Gateway has no pinned cluster")
		}
		return policy, placement.Cluster, true, nil
	}
	return endpoint.Policy{}, "", true, fmt.Errorf("endpoint: Gateway Definition is unavailable")
}
