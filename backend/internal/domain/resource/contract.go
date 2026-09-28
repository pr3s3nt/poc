package resource

import (
	"fmt"
	"sort"
)

// OutputField is one entry of a Resource Type output contract.
type OutputField struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Secret   bool   `json:"secret,omitempty"`
	Required bool   `json:"required,omitempty"`
}

// InputField is one entry of a Resource Type input contract.
type InputField struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required,omitempty"`
}

// Type is a Resource Type: the implementation-independent contract (UC-02).
type Type struct {
	Key     string        `json:"key"`
	Inputs  []InputField  `json:"inputs"`
	Outputs []OutputField `json:"outputs"`
}

// Validate checks the supported MVP schema before a type enters the catalog.
func (t Type) Validate() error {
	if t.Key == "" {
		return fmt.Errorf("resource: type key is empty")
	}
	inputs := make(map[string]bool, len(t.Inputs))
	for _, field := range t.Inputs {
		if field.Name == "" || inputs[field.Name] {
			return fmt.Errorf("resource: type %q has an empty or duplicate input %q", t.Key, field.Name)
		}
		if !validContractType(field.Type) {
			return fmt.Errorf("resource: type %q input %q has unsupported type %q", t.Key, field.Name, field.Type)
		}
		inputs[field.Name] = true
	}
	outputs := make(map[string]bool, len(t.Outputs))
	for _, field := range t.Outputs {
		if field.Name == "" || outputs[field.Name] {
			return fmt.Errorf("resource: type %q has an empty or duplicate output %q", t.Key, field.Name)
		}
		if !validContractType(field.Type) {
			return fmt.Errorf("resource: type %q output %q has unsupported type %q", t.Key, field.Name, field.Type)
		}
		outputs[field.Name] = true
	}
	return nil
}

func validContractType(value string) bool {
	switch value {
	case "string", "number", "bool", "any", "":
		return true
	default:
		return false
	}
}

// Input returns the named input contract entry.
func (t Type) Input(name string) (InputField, bool) {
	for _, i := range t.Inputs {
		if i.Name == name {
			return i, true
		}
	}
	return InputField{}, false
}

// ValidateParams enforces the Resource Type input contract on Score params
// (UC-06 BR-07).
func (t Type) ValidateParams(params map[string]any) error {
	names := make([]string, 0, len(params))
	for name := range params {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		field, ok := t.Input(name)
		if !ok {
			return fmt.Errorf("resource: type %q does not declare input %q", t.Key, name)
		}
		if err := checkType(field.Type, params[name]); err != nil {
			return fmt.Errorf("resource: type %q input %q: %w", t.Key, name, err)
		}
	}
	for _, field := range t.Inputs {
		if !field.Required {
			continue
		}
		if _, ok := params[field.Name]; !ok {
			return fmt.Errorf("resource: type %q requires input %q", t.Key, field.Name)
		}
	}
	return nil
}

// Output returns the named output contract entry.
func (t Type) Output(name string) (OutputField, bool) {
	for _, o := range t.Outputs {
		if o.Name == name {
			return o, true
		}
	}
	return OutputField{}, false
}

// SecretOutputs lists the output names classified as secret.
func (t Type) SecretOutputs() []string {
	var names []string
	for _, o := range t.Outputs {
		if o.Secret {
			names = append(names, o.Name)
		}
	}
	sort.Strings(names)
	return names
}

// ValidateOutputs checks executor outputs against the Resource Type contract (UC-08 BR-03).
func (t Type) ValidateOutputs(outputs map[string]any) error {
	for _, o := range t.Outputs {
		value, ok := outputs[o.Name]
		if !ok {
			if o.Required {
				return fmt.Errorf("resource: type %q requires output %q", t.Key, o.Name)
			}
			continue
		}
		if err := checkType(o.Type, value); err != nil {
			return fmt.Errorf("resource: type %q output %q: %w", t.Key, o.Name, err)
		}
	}
	for name := range outputs {
		if _, ok := t.Output(name); !ok {
			return fmt.Errorf("resource: type %q does not declare output %q", t.Key, name)
		}
	}
	return nil
}

func checkType(want string, value any) error {
	switch want {
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("expected string, got %T", value)
		}
	case "number":
		switch value.(type) {
		case float64, int, int64:
		default:
			return fmt.Errorf("expected number, got %T", value)
		}
	case "bool":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("expected bool, got %T", value)
		}
	case "any", "":
	default:
		return fmt.Errorf("unknown contract type %q", want)
	}
	return nil
}
