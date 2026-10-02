package catalog

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning"
	"orchestrator/internal/planning/placeholder"
)

// publicID is the identifier policy for new public Resource Type and
// Definition registrations (UC-02 BR-05, UC-03 BR-10). It is enforced only on
// public registration; legacy catalog reads, seeds and the planner keep their
// own rules.
var publicID = regexp.MustCompile(`^[a-z0-9-]+$`)

// reservedTypeKeys are the virtual Score Resource Types (UC-02 BR-06).
var reservedTypeKeys = map[string]bool{"environment": true, "service": true}

// validatePublicID checks a new public ID without trimming or normalizing it.
// The submitted ID is not echoed because it failed the policy.
func validatePublicID(kind, id string) error {
	if !publicID.MatchString(id) {
		return fmt.Errorf("%s ID must be non-empty and contain only lowercase letters, digits and hyphens", kind)
	}
	return nil
}

// valueType is the subset of Terraform/contract value types a Driver Input
// variable can declare.
type valueType struct {
	kind string // string, number, bool, any, list, map
	elem *valueType
}

func (t valueType) String() string {
	if t.elem != nil {
		return t.kind + "(" + t.elem.String() + ")"
	}
	return t.kind
}

var stringType = valueType{kind: "string"}

// parseTerraformType reads the type expression an embedded module declares.
func parseTerraformType(expr string) (valueType, error) {
	expr = strings.TrimSpace(expr)
	switch expr {
	case "string", "number", "bool", "any":
		return valueType{kind: expr}, nil
	}
	for _, collection := range []string{"list", "set", "map"} {
		inner, ok := strings.CutPrefix(expr, collection+"(")
		if !ok || !strings.HasSuffix(inner, ")") {
			continue
		}
		elem, err := parseTerraformType(strings.TrimSuffix(inner, ")"))
		if err != nil {
			return valueType{}, err
		}
		kind := collection
		if kind == "set" {
			kind = "list"
		}
		return valueType{kind: kind, elem: &elem}, nil
	}
	return valueType{}, fmt.Errorf("catalog: unsupported Terraform variable type %q in embedded module", expr)
}

// driverVariableSchemas are the static variable contracts of the
// non-Terraform executors, keyed by driver and Resource Type (UC-03 BR-12).
var driverVariableSchemas = map[resource.DriverType]map[string]map[string]valueType{
	resource.DriverKubernetes: {
		"k8s-namespace": {"name": stringType},
		"postgres": {
			"database": stringType, "username": stringType, "image": stringType,
			"storage": stringType, "namespace": stringType,
		},
	},
	resource.DriverExistingCluster: {
		"k8s-cluster": {"name": stringType, "kubeContext": stringType},
	},
}

// credentialVariables are never accepted as Driver Inputs (UC-03 BR-14). The
// Terraform executor generates master_password itself.
var credentialVariables = map[string]bool{"master_password": true, "password": true}

const variablesPath = "driverInputs.values.variables"

// validateDriverInputShape enforces the strict nested Driver Inputs structure
// (UC-03 BR-11) and returns the variables object.
func validateDriverInputShape(def resource.Definition) (map[string]any, error) {
	if def.DriverInputs == nil {
		return nil, errors.New("driverInputs.values must be an object")
	}
	for _, key := range sortedKeys(def.DriverInputs) {
		switch key {
		case "values":
		case "secret_refs", "secrets":
			return nil, fmt.Errorf("driverInputs.%s is not supported: secret Driver Inputs are not accepted", key)
		default:
			return nil, fmt.Errorf("driverInputs.%s is not a supported field", key)
		}
	}
	values, ok := def.DriverInputs["values"].(map[string]any)
	if !ok || values == nil {
		return nil, errors.New("driverInputs.values must be an object")
	}
	for _, key := range sortedKeys(values) {
		switch {
		case key == "variables":
		case key == "source" && def.DriverType == resource.DriverTerraform:
		case key == "source":
			return nil, fmt.Errorf("driverInputs.values.source is only supported for the Terraform driver")
		default:
			return nil, fmt.Errorf("driverInputs.values.%s is not a supported field", key)
		}
	}
	variables, ok := values["variables"].(map[string]any)
	if !ok || variables == nil {
		return nil, errors.New("driverInputs.values.variables must be an object")
	}
	if def.DriverType == resource.DriverTerraform {
		source, ok := values["source"].(map[string]any)
		if !ok || source == nil {
			return nil, errors.New("driverInputs.values.source must be an object")
		}
		for _, key := range sortedKeys(source) {
			if key != "module" {
				return nil, fmt.Errorf("driverInputs.values.source.%s is not supported; only module is supported", key)
			}
		}
		if module, ok := source["module"].(string); !ok || module == "" {
			return nil, errors.New("driverInputs.values.source.module must be a non-empty string")
		}
	}
	return variables, nil
}

// validateStaticVariables checks Kubernetes and existing-cluster variables
// against the executor contract (UC-03 BR-12).
func validateStaticVariables(def resource.Definition, typ resource.Type, variables map[string]any) error {
	schema := driverVariableSchemas[def.DriverType][def.ResourceTypeKey]
	for _, name := range sortedKeys(variables) {
		if credentialVariables[name] {
			return fmt.Errorf("%s.%s is a credential and cannot be a Driver Input", variablesPath, name)
		}
		want, ok := schema[name]
		if !ok {
			return fmt.Errorf("%s.%s is not supported by the %s driver for %s", variablesPath, name, def.DriverType, def.ResourceTypeKey)
		}
		if err := checkVariableValue(variablesPath+"."+name, want, variables[name]); err != nil {
			return err
		}
	}
	if def.DriverType == resource.DriverKubernetes && def.ResourceTypeKey == "k8s-namespace" {
		if _, ok := variables["name"]; !ok {
			if input, ok := typ.Input("name"); !ok || !input.Required || input.Type != "string" {
				return fmt.Errorf("%s.name is required unless the Resource Type declares a required string name input", variablesPath)
			}
		}
	}
	return nil
}

// validateTerraformVariables checks variable names and literal types against
// the embedded module contract (UC-03 BR-12/BR-14).
func validateTerraformVariables(contract planning.ModuleContract, variables map[string]any) error {
	for _, name := range sortedKeys(variables) {
		if credentialVariables[name] {
			return fmt.Errorf("%s.%s is executor-owned or a credential and cannot be a Driver Input", variablesPath, name)
		}
		variable, ok := contract.Variables[name]
		if !ok {
			return fmt.Errorf("%s.%s is not declared by module %q", variablesPath, name, contract.Module)
		}
		want, err := parseTerraformType(variable.Type)
		if err != nil {
			return &internalError{err: err}
		}
		if err := checkVariableValue(variablesPath+"."+name, want, variables[name]); err != nil {
			return err
		}
	}
	return nil
}

// checkVariableValue type-checks one Driver Input value (UC-03 BR-13). A
// complete placeholder defers its type to planning/execution; interpolation
// is only valid where a string is expected; literal collection elements are
// still checked. Errors carry the path, never the value.
func checkVariableValue(path string, want valueType, value any) error {
	if value == nil {
		return fmt.Errorf("%s must not be null", path)
	}
	if text, ok := value.(string); ok {
		full, err := classifyPlaceholders(text)
		if err != nil {
			return fmt.Errorf("%s contains a malformed placeholder", path)
		}
		if full || want.kind == "string" || want.kind == "any" {
			return nil
		}
		return fmt.Errorf("%s must be %s", path, want)
	}
	switch want.kind {
	case "string":
		return fmt.Errorf("%s must be string", path)
	case "number":
		switch value.(type) {
		case float64, int, int64:
			return nil
		}
		return fmt.Errorf("%s must be number", path)
	case "bool":
		if _, ok := value.(bool); ok {
			return nil
		}
		return fmt.Errorf("%s must be bool", path)
	case "list":
		items, ok := value.([]any)
		if !ok {
			return fmt.Errorf("%s must be %s", path, want)
		}
		for i, item := range items {
			if err := checkVariableValue(fmt.Sprintf("%s[%d]", path, i), *want.elem, item); err != nil {
				return err
			}
		}
		return nil
	case "map":
		entries, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("%s must be %s", path, want)
		}
		for _, key := range sortedKeys(entries) {
			if err := checkVariableValue(path+"."+key, *want.elem, entries[key]); err != nil {
				return err
			}
		}
		return nil
	default: // any
		return checkAnyValue(path, value)
	}
}

// checkAnyValue rejects nulls and malformed placeholders inside an untyped tree.
func checkAnyValue(path string, value any) error {
	switch t := value.(type) {
	case nil:
		return fmt.Errorf("%s must not be null", path)
	case string:
		if _, err := classifyPlaceholders(t); err != nil {
			return fmt.Errorf("%s contains a malformed placeholder", path)
		}
	case []any:
		for i, item := range t {
			if err := checkAnyValue(fmt.Sprintf("%s[%d]", path, i), item); err != nil {
				return err
			}
		}
	case map[string]any:
		for _, key := range sortedKeys(t) {
			if err := checkAnyValue(path+"."+key, t[key]); err != nil {
				return err
			}
		}
	}
	return nil
}

// classifyPlaceholders reports whether text is exactly one non-escaped
// placeholder. Escaped `$${...}` stays literal. A `${` left after removing
// every well-formed placeholder, a nested `{` inside a non-escaped body, or
// an unparsable body, is malformed.
func classifyPlaceholders(text string) (full bool, err error) {
	matches := placeholder.Pattern.FindAllStringSubmatchIndex(text, -1)
	var rest strings.Builder
	last := 0
	for _, m := range matches {
		rest.WriteString(text[last:m[0]])
		last = m[1]
		if strings.HasPrefix(text[m[0]:m[1]], "$$") {
			continue
		}
		// Pattern stops at the first closing brace, so a nested `${...}` or
		// `$${...}` leaves its opening brace inside the body.
		body := text[m[2]:m[3]]
		if strings.Contains(body, "{") {
			return false, errors.New("nested placeholder")
		}
		ref, err := placeholder.Parse(body)
		if err != nil {
			return false, err
		}
		if ref.Kind == placeholder.KindContext && ref.Context == "" {
			return false, errors.New("empty context path")
		}
	}
	rest.WriteString(text[last:])
	if strings.Contains(rest.String(), "${") {
		return false, errors.New("unterminated placeholder")
	}
	full = len(matches) == 1 && matches[0][0] == 0 && matches[0][1] == len(text) && !strings.HasPrefix(text, "$$")
	return full, nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
