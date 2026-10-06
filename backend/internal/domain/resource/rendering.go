package resource

import "fmt"

// RenderBundle is immutable process configuration. Planning never executes the renderer.
type RenderBundle struct {
	ID           string `json:"id"`
	Version      string `json:"version"`
	BinaryDigest string `json:"binaryDigest"`
	Digest       string `json:"digest"`
}

// RenderingSelection pins a Definition and its rendering implementation in the plan.
// The zero value means the explicit built-in native renderer.
type RenderingSelection struct {
	DefinitionKey  string       `json:"definitionKey"`
	DefinitionHash string       `json:"definitionHash"`
	DriverType     DriverType   `json:"driverType"`
	Bundle         RenderBundle `json:"bundle"`
}

func CopyRenderBundles(in map[string]RenderBundle) map[string]RenderBundle {
	out := map[string]RenderBundle{}
	for k, v := range in {
		out[k] = v
	}
	return out
}
func CopyRendering(in map[string]RenderingSelection) map[string]RenderingSelection {
	out := map[string]RenderingSelection{}
	for k, v := range in {
		out[k] = v
	}
	return out
}

func ValidateRenderDefinition(def Definition, typ Type, bundles map[string]RenderBundle) error {
	if def.ResourceTypeKey != "workload" || def.ExecutionProfile != "internal-k8s" || def.DriverType != DriverScoreK8s {
		return fmt.Errorf("score-k8s requires workload and internal-k8s")
	}
	if def.ConnectionKey != "" || len(def.Provision) > 0 || len(typ.Inputs) > 0 || len(typ.Outputs) > 0 {
		return fmt.Errorf("workload rendering cannot declare connection, provision rules or resource inputs/outputs")
	}
	values, ok := def.DriverInputs["values"].(map[string]any)
	if !ok || len(def.DriverInputs) != 1 || len(values) != 1 {
		return fmt.Errorf("workload driver accepts only values.variables.render_bundle")
	}
	variables, ok := values["variables"].(map[string]any)
	id, literal := variables["render_bundle"].(string)
	if !ok || len(variables) != 1 || !literal || id == "" {
		return fmt.Errorf("render_bundle must be one installed literal bundle ID")
	}
	bundle, installed := bundles[id]
	if !installed || bundle.ID != id || bundle.Digest == "" || bundle.BinaryDigest == "" || bundle.Version == "" {
		return fmt.Errorf("render bundle is unavailable")
	}
	if def.SourceFingerpr != "" && def.SourceFingerpr != bundle.Digest {
		return fmt.Errorf("render bundle fingerprint changed; register a new Definition")
	}
	return nil
}
