package resource

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"orchestrator/internal/domain/renderbundle"
	"orchestrator/internal/planning/placeholder"
)

// RenderBundle is the immutable pin of a rendering implementation. An
// installed bundle is process configuration; a Git bundle (ADR-021) adds its
// source pin and engine contract. Planning never executes the renderer.
type RenderBundle struct {
	ID           string `json:"id"`
	Version      string `json:"version"`
	BinaryDigest string `json:"binaryDigest"`
	Digest       string `json:"digest"`
	// Source is the Git origin/directory/commit/content pin; nil for an
	// installed bundle, whose plans keep their original encoding.
	Source          *renderbundle.Source `json:"source,omitempty"`
	EngineContract  string               `json:"engineContract,omitempty"`
	InputContract   string               `json:"inputContract,omitempty"`
	RendererVersion string               `json:"rendererVersion,omitempty"`
}

// Git reports whether the pin is a Git render bundle.
func (b RenderBundle) Git() bool { return b.Source != nil }

// GitRenderPin is the plan pin of one verified Git bundle record.
func GitRenderPin(b renderbundle.Bundle) RenderBundle {
	source := b.Source
	return RenderBundle{ID: b.ID, Version: b.ShortRevision(), BinaryDigest: b.RendererBinary, Digest: b.Source.ContentSHA256,
		Source: &source, EngineContract: b.EngineContract, InputContract: b.Contract.InputContract, RendererVersion: b.RendererVersion}
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
	// Inputs are the effective typed render inputs of a Git bundle and
	// InputsHash their canonical hash; both are empty for installed bundles.
	Inputs     map[string]any `json:"inputs,omitempty"`
	InputsHash string         `json:"inputsHash,omitempty"`
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
		if v.Inputs != nil {
			v.Inputs, _ = renderbundle.DeepCopy(v.Inputs).(map[string]any)
		}
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
	// VariableRenderInputs is the optional literal typed input object of a
	// Git bundle (ADR-021); installed bundles accept none.
	VariableRenderInputs = "render_inputs"
)

// Render selection failures that delivery classifies with stable codes.
var (
	// ErrRenderBundleUnavailable: the selected bundle is not an available
	// installed bundle or a verified Git bundle of the Organization.
	ErrRenderBundleUnavailable = errors.New("resource: render bundle is unavailable")
	// ErrRenderInputsInvalid: render_inputs are not admitted by the bundle.
	ErrRenderInputsInvalid = errors.New("resource: render inputs are not valid")
)

// RenderSelectionError wraps a classified render selection failure.
type RenderSelectionError struct {
	Kind error
	Err  error
}

func (e *RenderSelectionError) Error() string   { return e.Err.Error() }
func (e *RenderSelectionError) Unwrap() []error { return []error{e.Kind, e.Err} }

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
// workload Resource Type and the installed bundles; see ResolveRenderDefinition.
func ValidateRenderDefinition(def Definition, typ Type, bundles map[string]RenderBundle, placed bool) error {
	_, _, err := ResolveRenderDefinition(def, typ, bundles, nil, placed)
	return err
}

// ResolveRenderDefinition validates a workload score-k8s Definition and
// returns its render pin and effective inputs. render_bundle is either an
// available installed bundle ID (legacy contract: no render_inputs) or the
// UUID of one verified Git bundle of the same Organization (git holds only
// the Organization's records), whose schema admits render_inputs. The driver
// otherwise accepts exactly namespace and cluster: nothing at workload
// execution consumes any other variable. placed is true for a
// Definition-placed Environment (ADR-016): namespace and cluster are then both
// required. A render-only Definition (neither reference) is the explicit AWS
// legacy compatibility form until V05; a half pair is never valid.
func ResolveRenderDefinition(def Definition, typ Type, bundles map[string]RenderBundle, git map[string]renderbundle.Bundle, placed bool) (RenderBundle, map[string]any, error) {
	if def.ResourceTypeKey != "workload" || def.DriverType != DriverScoreK8s {
		return RenderBundle{}, nil, fmt.Errorf("score-k8s requires the workload resource type")
	}
	if def.DriverAccount != "" {
		return RenderBundle{}, nil, fmt.Errorf("workload rendering cannot declare a driver account")
	}
	values, ok := def.DriverInputs["values"].(map[string]any)
	if !ok || len(def.DriverInputs) != 1 || len(values) != 1 {
		return RenderBundle{}, nil, fmt.Errorf("workload driver accepts only values.variables")
	}
	variables, ok := values["variables"].(map[string]any)
	id, literal := variables[VariableRenderBundle].(string)
	if !ok || !literal || id == "" {
		return RenderBundle{}, nil, fmt.Errorf("render_bundle must be one installed bundle ID or verified Git bundle ID")
	}
	var pin RenderBundle
	var inputs map[string]any
	rawInputs, hasInputs := variables[VariableRenderInputs]
	if record, verified := git[id]; verified && renderbundle.ValidID(id) {
		supplied := map[string]any{}
		if hasInputs {
			object, isObject := rawInputs.(map[string]any)
			if !isObject {
				return RenderBundle{}, nil, &RenderSelectionError{Kind: ErrRenderInputsInvalid, Err: fmt.Errorf("render_inputs must be a JSON object")}
			}
			supplied = object
		}
		effective, err := renderbundle.Effective(record.Contract.InputsSchema, supplied)
		if err != nil {
			return RenderBundle{}, nil, &RenderSelectionError{Kind: ErrRenderInputsInvalid, Err: err}
		}
		pin, inputs = GitRenderPin(record), effective
	} else {
		if !RenderBundleAvailable(bundles, id) {
			return RenderBundle{}, nil, &RenderSelectionError{Kind: ErrRenderBundleUnavailable, Err: fmt.Errorf("render bundle is unavailable")}
		}
		if hasInputs {
			// Installed legacy bundles keep their exact-three-variable contract.
			return RenderBundle{}, nil, &RenderSelectionError{Kind: ErrRenderInputsInvalid, Err: fmt.Errorf("installed render bundles accept no render_inputs")}
		}
		pin = bundles[id]
	}
	// Lifecycle metadata and readiness live in the existing Deployment/Workload status views. A workload
	// Type output would be a binding nothing emits or validates, so V03 accepts none.
	if len(typ.Outputs) > 0 {
		return RenderBundle{}, nil, fmt.Errorf("the workload resource type declares output %q, which the score-k8s workload driver never emits; workload Types declare no outputs in V03", typ.Outputs[0].Name)
	}
	for _, input := range typ.Inputs {
		if input.Required {
			return RenderBundle{}, nil, fmt.Errorf("the workload resource type requires input %q, which the score-k8s workload driver cannot supply", input.Name)
		}
	}
	_, hasNamespace := variables[VariableNamespace]
	_, hasCluster := variables[VariableCluster]
	if hasNamespace != hasCluster {
		return RenderBundle{}, nil, fmt.Errorf("namespace and cluster references must be declared together")
	}
	if placed && !hasNamespace {
		return RenderBundle{}, nil, fmt.Errorf("a Definition-placed workload needs both the namespace and cluster references")
	}
	for name, value := range variables {
		switch name {
		case VariableRenderBundle, VariableRenderInputs:
		case VariableNamespace, VariableCluster:
			text, _ := value.(string)
			descriptor, output, whole := WorkloadReference(text)
			wantType, wantOutput := "k8s-namespace", "name"
			if name == VariableCluster {
				wantType, wantOutput = "k8s-cluster", "cluster"
			}
			if !whole || output != wantOutput || !strings.HasPrefix(descriptor, wantType) {
				return RenderBundle{}, nil, fmt.Errorf("%s must be one whole %s output reference of a %s resource", name, wantOutput, wantType)
			}
		default:
			return RenderBundle{}, nil, fmt.Errorf("workload variable %q is not supported: the workload driver accepts only %s, %s, %s and %s", name, VariableRenderBundle, VariableRenderInputs, VariableNamespace, VariableCluster)
		}
	}
	// Definition.SourceFingerpr is registration-time audit provenance. It stays in
	// the Definition hash but is not compared with the installed bundle: plans pin
	// the installed snapshot, so an upgrade only makes older plans stale.
	return pin, inputs, nil
}
