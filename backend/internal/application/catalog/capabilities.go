package catalog

import (
	"context"
	"errors"
	"sort"

	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning"
)

// CapabilityVariable is the public, secret-free view of one editable Driver
// Input variable (UC-03 BR-23, OC-04d).
type CapabilityVariable struct {
	Name          string `json:"name"`
	Type          string `json:"type"`
	Required      bool   `json:"required"`
	HasDefault    bool   `json:"hasDefault"`
	Default       any    `json:"default,omitempty"`
	ResourceParam bool   `json:"resourceParam"`
	Preset        string `json:"preset,omitempty"`
}

// Capability describes one registrable Resource Type/driver/module pair.
type Capability struct {
	ResourceType  string               `json:"resourceType"`
	DriverType    string               `json:"driverType"`
	Module        string               `json:"module"`
	Variables     []CapabilityVariable `json:"variables"`
	ExecutorOwned []string             `json:"executorOwned"`
}

// ListDefinitionCapabilities returns the supported registration pairs with
// their editable variable metadata. It only reads the Organization's Resource
// Types and inspects embedded module contracts: no write, admission, execution
// or provider call. Module variables never define a Resource Type; resourceParam
// only reflects the inputs the scoped Resource Type really declares.
func (s *Service) ListDefinitionCapabilities(ctx context.Context, organizationKey string) ([]Capability, error) {
	types, err := s.store.ListResourceTypes(ctx, organizationKey)
	if err != nil {
		return nil, err
	}
	byKey := map[string]resource.Type{}
	for _, typ := range types {
		byKey[typ.Key] = typ
	}
	out := make([]Capability, 0, len(supportedPairs))
	for _, pair := range supportedPairs {
		typ := byKey[pair.resourceType]
		capability := Capability{ResourceType: pair.resourceType, DriverType: string(pair.driver), Module: pair.module, Variables: []CapabilityVariable{}, ExecutorOwned: []string{}}
		if pair.driver == resource.DriverTerraform {
			if s.inspector == nil {
				return nil, &internalError{err: errors.New("catalog: Terraform contract inspector is unavailable")}
			}
			contract, err := s.inspector.Inspect(pair.module)
			if err != nil {
				return nil, &internalError{err: err}
			}
			capability.Variables, capability.ExecutorOwned, err = terraformCapabilityVariables(contract, s.inspector.ExecutorVariables(), typ)
			if err != nil {
				return nil, &internalError{err: err}
			}
		} else {
			for _, name := range sortedKeys(driverVariableSchemas[pair.driver][pair.resourceType]) {
				variable := driverVariableSchemas[pair.driver][pair.resourceType][name]
				input, param := typ.Input(name)
				if pair.driver == resource.DriverKubernetes && pair.resourceType == "k8s-namespace" && name == "name" {
					// UC-03 BR-12: registration only accepts the Resource Type as the namespace
					// name source when its input is a required string.
					param = param && input.Required && input.Type == "string"
				}
				entry := CapabilityVariable{Name: name, Type: variable.typ.String(), Required: variable.required, HasDefault: variable.hasDefault, ResourceParam: param, Preset: variable.preset}
				if variable.hasDefault {
					entry.Default = variable.defaultVal
				}
				capability.Variables = append(capability.Variables, entry)
			}
		}
		out = append(out, capability)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.ResourceType != b.ResourceType {
			return a.ResourceType < b.ResourceType
		}
		if a.DriverType != b.DriverType {
			return a.DriverType < b.DriverType
		}
		return a.Module < b.Module
	})
	return out, nil
}

func terraformCapabilityVariables(contract planning.ModuleContract, executorVariables []string, typ resource.Type) ([]CapabilityVariable, []string, error) {
	owned := map[string]bool{}
	for _, name := range executorVariables {
		owned[name] = true
	}
	variables := []CapabilityVariable{}
	executorOwned := []string{}
	for _, name := range sortedKeys(contract.Variables) {
		if owned[name] {
			executorOwned = append(executorOwned, name)
			continue
		}
		declared := contract.Variables[name]
		parsed, err := parseTerraformType(declared.Type)
		if err != nil {
			return nil, nil, err
		}
		_, param := typ.Input(name)
		entry := CapabilityVariable{Name: name, Type: parsed.String(), Required: !declared.HasDefault, HasDefault: declared.HasDefault, ResourceParam: param, Preset: terraformPresets[contract.Module][name]}
		if declared.HasDefault && declared.Default != nil {
			entry.Default = declared.Default
		}
		variables = append(variables, entry)
	}
	return variables, executorOwned, nil
}
