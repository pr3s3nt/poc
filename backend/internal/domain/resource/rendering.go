package resource

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"orchestrator/internal/domain/templatesource"
	"orchestrator/internal/planning/placeholder"
)

// TemplateSource is the immutable pin of a rendering implementation. An
// installed bundle is process configuration; a Git bundle (ADR-021) adds its
// source pin and engine contract. Planning never executes the renderer.
type TemplateSource struct {
	ID           string `json:"id"`
	Version      string `json:"version"`
	BinaryDigest string `json:"binaryDigest"`
	Digest       string `json:"digest"`
	// Source is the Git origin/directory/commit/content pin; nil for an
	// installed bundle, whose plans keep their original encoding.
	Source          *templatesource.Source `json:"source,omitempty"`
	EngineContract  string                 `json:"engineContract,omitempty"`
	InputContract   string                 `json:"inputContract,omitempty"`
	RendererVersion string                 `json:"rendererVersion,omitempty"`
}

// Git reports whether the pin is a Git render bundle.
func (b TemplateSource) Git() bool { return b.Source != nil }

// GitRenderPin is the plan pin of one verified Git bundle record.
func GitRenderPin(b templatesource.Bundle) TemplateSource {
	source := b.Source
	return TemplateSource{ID: b.ID, Version: b.ShortRevision(), BinaryDigest: b.RendererBinary, Digest: b.Source.ContentSHA256,
		Source: &source, EngineContract: b.EngineContract, InputContract: b.Contract.InputContract, RendererVersion: b.RendererVersion}
}

// RenderingSelection pins a Definition and its rendering implementation in the plan.
// The zero value means the explicit built-in native renderer.
type RenderingSelection struct {
	References     map[string]string `json:"references,omitempty"`
	DefinitionKey  string            `json:"definitionKey"`
	DefinitionHash string            `json:"definitionHash"`
	// Class is the explicit-or-default workload class the Definition matched.
	Class      string         `json:"class,omitempty"`
	DriverType DriverType     `json:"driverType"`
	Bundle     TemplateSource `json:"bundle"`
	// Inputs are the effective typed render inputs of a Git bundle and
	// InputsHash their canonical hash; both are empty for installed bundles.
	Inputs     map[string]any `json:"inputs,omitempty"`
	InputsHash string         `json:"inputsHash,omitempty"`
}

func CopyTemplateSources(in map[string]TemplateSource) map[string]TemplateSource {
	out := map[string]TemplateSource{}
	for k, v := range in {
		out[k] = v
	}
	return out
}
func CopyRendering(in map[string]RenderingSelection) map[string]RenderingSelection {
	out := map[string]RenderingSelection{}
	for k, v := range in {
		if v.References != nil {
			refs := map[string]string{}
			for name, value := range v.References {
				refs[name] = value
			}
			v.References = refs
		}
		if v.Inputs != nil {
			v.Inputs, _ = templatesource.DeepCopy(v.Inputs).(map[string]any)
		}
		out[k] = v
	}
	return out
}

// TemplateSourceAvailable is the single availability predicate shared by
// registration and installed-bundle discovery.
func TemplateSourceAvailable(bundles map[string]TemplateSource, id string) bool {
	bundle, installed := bundles[id]
	return installed && id != "" && bundle.ID == id && bundle.Digest != "" && bundle.BinaryDigest != "" && bundle.Version != ""
}

// Workload driver variables (ADR-016). template_source is a literal installed
// bundle ID; namespace and cluster are whole typed resource-output references.
const (
	VariableTemplateSource = "template_source"
	VariableNamespace      = "namespace"
	VariableCluster        = "cluster"
	// VariableTemplateInputs is the optional literal typed input object of a
	// Git bundle (ADR-021); installed bundles accept none.
	VariableTemplateInputs = "template_inputs"
)

// Render selection failures that delivery classifies with stable codes.
var (
	// ErrTemplateSourceUnavailable: the selected bundle is not an available
	// installed bundle or a verified Git bundle of the Organization.
	ErrTemplateSourceUnavailable = errors.New("resource: render bundle is unavailable")
	// ErrRenderInputsInvalid: template_inputs are not admitted by the bundle.
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

// ValidateRenderDefinition checks a workload template Definition against the
// workload Resource Type and the installed bundles; see ResolveRenderDefinition.
func ValidateRenderDefinition(def Definition, typ Type, bundles map[string]TemplateSource, placed bool) error {
	_, _, err := ResolveRenderDefinition(def, typ, bundles, nil, placed)
	return err
}

// ResolveRenderDefinition validates a workload template Definition and
// returns its render pin and effective inputs. template_source is either an
// available installed bundle ID (legacy contract: no template_inputs) or the
// UUID of one verified Git bundle of the same Organization (git holds only
// the Organization's records), whose schema admits template_inputs. The driver
// otherwise accepts exactly namespace and cluster: nothing at workload
// execution consumes any other variable. placed is true for a
// Definition-placed Environment (ADR-016): namespace and cluster are then both
// required. A render-only Definition (neither reference) is the explicit AWS
// legacy compatibility form until V05; a half pair is never valid.
func ResolveRenderDefinition(def Definition, typ Type, bundles map[string]TemplateSource, git map[string]templatesource.Bundle, placed bool) (TemplateSource, map[string]any, error) {
	if def.ResourceTypeKey != "workload" || def.DriverType != DriverTemplate {
		return TemplateSource{}, nil, fmt.Errorf("template requires the workload resource type")
	}
	if def.DriverAccount != "" {
		return TemplateSource{}, nil, fmt.Errorf("workload rendering cannot declare a driver account")
	}
	values, ok := def.DriverInputs["values"].(map[string]any)
	if !ok || len(def.DriverInputs) != 1 || len(values) != 1 {
		return TemplateSource{}, nil, fmt.Errorf("workload driver accepts only values.variables")
	}
	variables, ok := values["variables"].(map[string]any)
	id, literal := variables[VariableTemplateSource].(string)
	if !ok || !literal || id == "" {
		return TemplateSource{}, nil, fmt.Errorf("template_source must be one installed bundle ID or verified Git bundle ID")
	}
	var pin TemplateSource
	var inputs map[string]any
	rawInputs, hasInputs := variables[VariableTemplateInputs]
	if record, verified := git[id]; verified && templatesource.ValidID(id) {
		supplied := map[string]any{}
		if hasInputs {
			object, isObject := rawInputs.(map[string]any)
			if !isObject {
				return TemplateSource{}, nil, &RenderSelectionError{Kind: ErrRenderInputsInvalid, Err: fmt.Errorf("template_inputs must be a JSON object")}
			}
			supplied = object
		}
		effective, err := templatesource.Effective(record.Contract.InputsSchema, supplied)
		if err != nil {
			return TemplateSource{}, nil, &RenderSelectionError{Kind: ErrRenderInputsInvalid, Err: err}
		}
		pin, inputs = GitRenderPin(record), effective
	} else {
		if !TemplateSourceAvailable(bundles, id) {
			return TemplateSource{}, nil, &RenderSelectionError{Kind: ErrTemplateSourceUnavailable, Err: fmt.Errorf("render bundle is unavailable")}
		}
		if hasInputs {
			// Installed legacy bundles keep their exact-three-variable contract.
			return TemplateSource{}, nil, &RenderSelectionError{Kind: ErrRenderInputsInvalid, Err: fmt.Errorf("installed render bundles accept no template_inputs")}
		}
		pin = bundles[id]
	}
	// Lifecycle metadata and readiness live in the existing Deployment/Workload status views. A workload
	// Type output would be a binding nothing emits or validates, so V03 accepts none.
	if len(typ.Outputs) > 0 {
		return TemplateSource{}, nil, fmt.Errorf("the workload resource type declares output %q, which the template workload driver never emits; workload Types declare no outputs in V03", typ.Outputs[0].Name)
	}
	for _, input := range typ.Inputs {
		if input.Required {
			return TemplateSource{}, nil, fmt.Errorf("the workload resource type requires input %q, which the template workload driver cannot supply", input.Name)
		}
	}
	_, hasNamespace := variables[VariableNamespace]
	_, hasCluster := variables[VariableCluster]
	if hasCluster && !hasNamespace {
		return TemplateSource{}, nil, fmt.Errorf("a cluster reference requires a namespace reference")
	}
	if placed && !hasNamespace {
		return TemplateSource{}, nil, fmt.Errorf("a Definition-placed workload needs a namespace reference")
	}
	for name, value := range variables {
		switch name {
		case "resource_references":
			refs, ok := value.(map[string]any)
			if !ok || len(refs) > 64 {
				return TemplateSource{}, nil, fmt.Errorf("resource_references must be a bounded object")
			}
			for name, raw := range refs {
				text, ok := raw.(string)
				_, _, whole := WorkloadReference(text)
				if !ok || !whole || !regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`).MatchString(name) {
					return TemplateSource{}, nil, fmt.Errorf("resource references need names and whole output references")
				}
			}
		case VariableTemplateSource, VariableTemplateInputs:
		case VariableNamespace, VariableCluster:
			text, _ := value.(string)
			descriptor, output, whole := WorkloadReference(text)
			wantType, wantOutput := "k8s-namespace", "name"
			if name == VariableCluster {
				wantType, wantOutput = "k8s-cluster", "cluster"
			}
			if !whole || output != wantOutput || !strings.HasPrefix(descriptor, wantType) {
				return TemplateSource{}, nil, fmt.Errorf("%s must be one whole %s output reference of a %s resource", name, wantOutput, wantType)
			}
		default:
			return TemplateSource{}, nil, fmt.Errorf("workload variable %q is not supported: the workload driver accepts only %s, %s, %s and %s", name, VariableTemplateSource, VariableTemplateInputs, VariableNamespace, VariableCluster)
		}
	}
	// Definition.SourceFingerpr is registration-time audit provenance. It stays in
	// the Definition hash but is not compared with the installed bundle: plans pin
	// the installed snapshot, so an upgrade only makes older plans stale.
	if def.RenderPin != nil {
		a, _ := json.Marshal(def.RenderPin)
		b, _ := json.Marshal(pin)
		if string(a) != string(b) {
			return TemplateSource{}, nil, &RenderSelectionError{Kind: ErrTemplateSourceUnavailable, Err: fmt.Errorf("pinned renderer contract is unavailable; choose a verified compatible revision")}
		}
	}
	return pin, inputs, nil
}
