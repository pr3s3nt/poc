package endpoint

import (
	"fmt"
	"orchestrator/internal/domain/resource"
)

const GatewayType = "k8s-gateway"
const GatewayOutput = "gateway"

// GatewayMetadataDriver identifies the supported representations of an external
// Gateway. It does not classify arbitrary Echo resources as preexisting.
func GatewayMetadataDriver(typ string, driver resource.DriverType) bool {
	return typ == GatewayType && (driver == resource.DriverEcho || driver == resource.DriverExistingGateway)
}

func ValidateGatewayType(typ resource.Type, driver resource.DriverType) error {
	field, found := typ.Output(GatewayOutput)
	if !GatewayMetadataDriver(typ.Key, driver) || len(typ.Inputs) != 0 || !found || field.Secret || field.Type != "any" {
		return fmt.Errorf("Gateway requires a supported metadata driver, no developer inputs and a nonsecret gateway output")
	}
	if driver == resource.DriverEcho && (len(typ.Outputs) != 1 || !field.Required) {
		return fmt.Errorf("Echo Gateway requires exactly one required gateway output")
	}
	return nil
}

// GatewayDefinitionValues reads the authored metadata contract independently of
// the legacy flat or new Echo envelope. No runtime observation is performed.
func GatewayDefinitionValues(def resource.Definition) (map[string]any, error) {
	if !GatewayMetadataDriver(def.ResourceTypeKey, def.DriverType) || def.DriverAccount != "" {
		return nil, fmt.Errorf("Gateway needs a supported metadata driver without a Driver Account")
	}
	values := def.Variables()
	if def.DriverType == resource.DriverEcho {
		gateway, ok := values[GatewayOutput].(map[string]any)
		if len(values) != 1 || !ok || gateway == nil {
			return nil, fmt.Errorf("Echo Gateway variables must contain exactly the gateway output object")
		}
		values = gateway
	}
	if _, err := GatewayPolicy(values); err != nil {
		return nil, err
	}
	text, _ := values[resource.VariableCluster].(string)
	_, output, whole := resource.WorkloadReference(text)
	if !whole || output != resource.ClusterTargetOutput {
		return nil, fmt.Errorf("Gateway needs a whole cluster target reference")
	}
	return values, nil
}

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
