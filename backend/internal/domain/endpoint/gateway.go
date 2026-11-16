package endpoint

import (
	"fmt"
	"orchestrator/internal/domain/resource"
)

const GatewayType = "k8s-gateway"
const GatewayOutput = "gateway"

// GatewaySelection references a separately matched Gateway resource. Flat
// policies remain readable as the explicit legacy endpoint representation.
func GatewaySelection(values map[string]any) (string, bool, error) {
	_, selected := values["gateway"]
	if !selected {
		return "", false, nil
	}
	text, _ := values["gateway"].(string)
	ref, output, whole := resource.WorkloadReference(text)
	if len(values) != 1 || !whole || output != GatewayOutput {
		return "", true, fmt.Errorf("endpoint: gateway must be a whole Gateway resource output reference")
	}
	return ref, true, nil
}

// GatewayPolicy validates metadata of an existing Gateway. The cluster
// reference is validated separately; no Gateway object is authored here.
func GatewayPolicy(values map[string]any) (Policy, error) {
	policy := map[string]any{}
	for key, value := range values {
		if key != "cluster" {
			policy[key] = value
		}
	}
	return ParsePolicy(policy)
}
