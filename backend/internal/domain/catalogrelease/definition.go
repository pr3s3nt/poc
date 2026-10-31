package catalogrelease

import (
	"bytes"
	"encoding/json"
	"gopkg.in/yaml.v3"
	"io"
	"orchestrator/internal/domain/resource"
)

// DefinitionDocument references verified components and platform-owned bindings.
type DefinitionDocument struct {
	Component  string              `json:"component,omitempty"`
	Binding    string              `json:"binding,omitempty"`
	Definition resource.Definition `json:"definition"`
}

func ParseDefinition(raw []byte) (DefinitionDocument, error) {
	var result DefinitionDocument
	if len(raw) == 0 || len(raw) > MaxManifestBytes {
		return result, ErrInvalid
	}
	var node yaml.Node
	if yaml.Unmarshal(raw, &node) != nil || !plainYAML(&node) {
		return result, ErrInvalid
	}
	var object map[string]any
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	if dec.Decode(&object) != nil || object == nil {
		return result, ErrInvalid
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		return result, ErrInvalid
	}
	data, err := json.Marshal(object)
	if err != nil {
		return result, ErrInvalid
	}
	jd := json.NewDecoder(bytes.NewReader(data))
	jd.DisallowUnknownFields()
	if jd.Decode(&result) != nil {
		return DefinitionDocument{}, ErrInvalid
	}
	d := result.Definition
	if d.Validate() != nil || !ValidID(d.Key) || d.DriverAccount != "" || d.SourceFingerpr != "" {
		return DefinitionDocument{}, ErrInvalid
	}
	switch d.DriverType {
	case resource.DriverTerraform:
		if !ValidID(result.Component) || !ValidID(result.Binding) || d.Source()["source_id"] != nil {
			return DefinitionDocument{}, ErrInvalid
		}
		if _, exists := d.DriverValues()["source"]; exists {
			return DefinitionDocument{}, ErrInvalid
		}
	case resource.DriverScoreK8s:
		if !ValidID(result.Component) || result.Binding != "" {
			return DefinitionDocument{}, ErrInvalid
		}
		if _, exists := d.Variables()[resource.VariableRenderBundle]; exists {
			return DefinitionDocument{}, ErrInvalid
		}
	case resource.DriverExistingCluster:
		if result.Component != "" || !ValidID(result.Binding) {
			return DefinitionDocument{}, ErrInvalid
		}
	default:
		return DefinitionDocument{}, ErrInvalid
	}
	return result, nil
}
