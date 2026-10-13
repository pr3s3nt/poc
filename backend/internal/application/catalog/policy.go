package catalog

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
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
type valueType = tfmodule.ValueType

var stringType = valueType{Kind: "string"}

// supportedPair is one Resource Type/driver pair a static (non-Terraform)
// executor supports. Terraform pairs come from the verified module registry:
// the Admin/release policy pairs each module with its Resource Types
// (UC-03 BR-08/BR-23).
type supportedPair struct {
	resourceType string
	driver       resource.DriverType
}

var supportedPairs = []supportedPair{
	{"k8s-namespace", resource.DriverKubernetes},
	{"postgres", resource.DriverKubernetes},
}

func driverSupportsType(driver resource.DriverType, resourceType string) bool {
	for _, pair := range supportedPairs {
		if pair.driver == driver && pair.resourceType == resourceType {
			return true
		}
	}
	return false
}

// staticVariable is the contract of one non-Terraform executor variable. The
// defaults mirror the executor (adapters/kubernetes stringInput fallbacks) and
// preset is the opt-in placeholder offered by authoring guidance (BR-25).
type staticVariable struct {
	typ        valueType
	required   bool
	hasDefault bool
	defaultVal any
	preset     string
}

// driverVariableSchemas are the static variable contracts of the
// non-Terraform executors, keyed by driver and Resource Type (UC-03 BR-12).
var driverVariableSchemas = map[resource.DriverType]map[string]map[string]staticVariable{
	resource.DriverKubernetes: {
		"k8s-namespace": {"name": {typ: stringType, required: true, preset: "${context.env.namespace}"}},
		"postgres": {
			"database":  {typ: stringType, hasDefault: true, defaultVal: "app"},
			"username":  {typ: stringType, hasDefault: true, defaultVal: "app"},
			"image":     {typ: stringType, hasDefault: true, defaultVal: "postgres:16-alpine"},
			"storage":   {typ: stringType, hasDefault: true, defaultVal: "1Gi"},
			"namespace": {typ: stringType},
		},
	},
}

// terraformPresets are the opt-in placeholders per module variable, taken from
// the current seed policy: the infrastructure resource name and the shared
// VPC outputs through the @infra token (BR-25).
var terraformPresets = map[string]map[string]string{
	"vpc": {"name": "${context.infra.resourceName}"},
	"eks": {"name": "${context.infra.resourceName}", "subnet_ids": "${resources['vpc.default#@infra'].outputs.subnetIds}"},
	// In-cluster Kubernetes modules (Redis/PostgreSQL/namespace): the target
	// namespace is the namespace resource output; the namespace name itself is
	// the stable Environment namespace.
	"k8s-namespace": {"name": "${context.env.namespace}"},
	"redis-k8s":     {"namespace": "${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}"},
	"postgres-k8s":  {"namespace": "${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}"},
	"elasticache":   {"vpc_id": "${resources['vpc.default#@infra'].outputs.id}", "subnet_ids": "${resources['vpc.default#@infra'].outputs.subnetIds}", "private_subnet_ids": "${resources['vpc.default#@infra'].outputs.privateSubnetIds}"},
	"aurora":        {"name": "${context.infra.resourceName}", "vpc_id": "${resources['vpc.default#@infra'].outputs.id}", "subnet_ids": "${resources['vpc.default#@infra'].outputs.subnetIds}", "private_subnet_ids": "${resources['vpc.default#@infra'].outputs.privateSubnetIds}"},
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
	if def.DriverType == resource.DriverTerraform {
		// The strict Terraform shape (pin, mappings, bindings) is parsed by
		// tfmodule.ParseDriverInputs.
		parsed, err := tfmodule.ParseDriverInputs(values)
		if err != nil {
			return nil, err
		}
		return parsed.Variables, nil
	}
	for _, key := range sortedKeys(values) {
		switch {
		case key == "variables":
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
	return variables, nil
}

// validateStaticVariables checks Kubernetes variables
// against the executor contract (UC-03 BR-12).
func validateStaticVariables(def resource.Definition, typ resource.Type, variables map[string]any) error {
	schema := driverVariableSchemas[def.DriverType][def.ResourceTypeKey]
	for _, name := range sortedKeys(variables) {
		if credentialVariables[name] {
			return fmt.Errorf("%s.%s is a credential and cannot be a Driver Input", variablesPath, name)
		}
		variable, ok := schema[name]
		if !ok {
			return fmt.Errorf("%s.%s is not supported by the %s driver for %s", variablesPath, name, def.DriverType, def.ResourceTypeKey)
		}
		if err := checkVariableValue(variablesPath+"."+name, variable.typ, variables[name]); err != nil {
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
		if full || want.Kind == "string" || want.Kind == "any" {
			return nil
		}
		return fmt.Errorf("%s must be %s", path, want)
	}
	switch want.Kind {
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
			if err := checkVariableValue(fmt.Sprintf("%s[%d]", path, i), *want.Elem, item); err != nil {
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
			if err := checkVariableValue(path+"."+key, *want.Elem, entries[key]); err != nil {
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
