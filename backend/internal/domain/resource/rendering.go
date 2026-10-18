package resource

import (
	"fmt"
	"regexp"
	"strings"

	"orchestrator/internal/planning/placeholder"
)

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
	DefinitionKey  string `json:"definitionKey"`
	DefinitionHash string `json:"definitionHash"`
	// Class is the explicit-or-default workload class the Definition matched.
	Class      string       `json:"class,omitempty"`
	DriverType DriverType   `json:"driverType"`
	Bundle     RenderBundle `json:"bundle"`
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

// RenderBundleAvailable is the single availability predicate shared by
// registration and installed-bundle discovery.
func RenderBundleAvailable(bundles map[string]RenderBundle, id string) bool {
	bundle, installed := bundles[id]
	return installed && id != "" && bundle.ID == id && bundle.Digest != "" && bundle.BinaryDigest != "" && bundle.Version != ""
}

// Workload driver variables (ADR-016). render_bundle is a literal installed
// bundle ID; namespace and cluster are whole typed resource-output references.
const (
	VariableRenderBundle = "render_bundle"
	VariableNamespace    = "namespace"
	VariableCluster      = "cluster"
)

var (
	wholeReference = regexp.MustCompile(`^\$\{(resources(?:\[[^\]]+\]|\.[^.}]+)\.outputs\.([A-Za-z0-9_]+))\}$`)
)

// WorkloadReference reports whether text is one whole resource-output
// reference and returns its descriptor text and output name.
func WorkloadReference(text string) (descriptor, output string, ok bool) {
	m := wholeReference.FindStringSubmatch(text)
	if m == nil || strings.Count(text, "${") != 1 {
		return "", "", false
	}
	ref, err := placeholder.Parse(m[1])
	if err != nil || ref.Kind != placeholder.KindResource {
		return "", "", false
	}
	return ref.Resource, ref.OutputKey, true
}

// ValidateRenderDefinition checks a workload score-k8s Definition against the
// workload Resource Type and the installed bundles. The workload driver accepts
// exactly render_bundle, namespace and cluster: nothing at workload execution
// resolves or consumes any other variable, so none is accepted (a typed input
// would be a silent no-op). placed is true for a Definition-placed Environment
// (ADR-016): namespace and cluster are then both required. A render-only
// Definition (neither reference) is the explicit AWS legacy compatibility form
// until V05; a half pair is never valid.
func ValidateRenderDefinition(def Definition, typ Type, bundles map[string]RenderBundle, placed bool) error {
	if def.ResourceTypeKey != "workload" || def.DriverType != DriverScoreK8s {
		return fmt.Errorf("score-k8s requires the workload resource type")
	}
	if def.DriverAccount != "" {
		return fmt.Errorf("workload rendering cannot declare a driver account")
	}
	values, ok := def.DriverInputs["values"].(map[string]any)
	if !ok || len(def.DriverInputs) != 1 || len(values) != 1 {
		return fmt.Errorf("workload driver accepts only values.variables")
	}
	variables, ok := values["variables"].(map[string]any)
	id, literal := variables[VariableRenderBundle].(string)
	if !ok || !literal || id == "" {
		return fmt.Errorf("render_bundle must be one installed literal bundle ID")
	}
	if !RenderBundleAvailable(bundles, id) {
		return fmt.Errorf("render bundle is unavailable")
	}
	// Lifecycle metadata and readiness live in the existing Deployment/Workload status views. A workload
	// Type output would be a binding nothing emits or validates, so V03 accepts none.
	if len(typ.Outputs) > 0 {
		return fmt.Errorf("the workload resource type declares output %q, which the score-k8s workload driver never emits; workload Types declare no outputs in V03", typ.Outputs[0].Name)
	}
	for _, input := range typ.Inputs {
		if input.Required {
			return fmt.Errorf("the workload resource type requires input %q, which the score-k8s workload driver cannot supply", input.Name)
		}
	}
	_, hasNamespace := variables[VariableNamespace]
	_, hasCluster := variables[VariableCluster]
	if hasNamespace != hasCluster {
		return fmt.Errorf("namespace and cluster references must be declared together")
	}
	if placed && !hasNamespace {
		return fmt.Errorf("a Definition-placed workload needs both the namespace and cluster references")
	}
	for name, value := range variables {
		switch name {
		case VariableRenderBundle:
		case VariableNamespace, VariableCluster:
			text, _ := value.(string)
			descriptor, output, whole := WorkloadReference(text)
			wantType, wantOutput := "k8s-namespace", "name"
			if name == VariableCluster {
				wantType, wantOutput = "k8s-cluster", "cluster"
			}
			if !whole || output != wantOutput || !strings.HasPrefix(descriptor, wantType) {
				return fmt.Errorf("%s must be one whole %s output reference of a %s resource", name, wantOutput, wantType)
			}
		default:
			return fmt.Errorf("workload variable %q is not supported: the workload driver accepts only %s, %s and %s", name, VariableRenderBundle, VariableNamespace, VariableCluster)
		}
	}
	// Definition.SourceFingerpr is registration-time audit provenance. It stays in
	// the Definition hash but is not compared with the installed bundle: plans pin
	// the installed snapshot, so an upgrade only makes older plans stale.
	return nil
}
