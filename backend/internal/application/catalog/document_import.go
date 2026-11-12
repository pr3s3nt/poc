package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"

	"orchestrator/internal/domain/endpoint"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/templatesource"
	"orchestrator/internal/domain/tfmodule"
)

// DocumentImport reports only field paths on conflict, never configuration values.
type DocumentImport struct {
	Status string   `json:"status"`
	Fields []string `json:"fields,omitempty"`
}

func importComparison(a, b any) DocumentImport {
	left, right := documentMap(a), documentMap(b)
	fields := []string{}
	diffDocument("", left, right, &fields)
	sort.Strings(fields)
	if len(fields) == 0 {
		return DocumentImport{Status: "REUSED"}
	}
	return DocumentImport{Status: "CONFLICT", Fields: fields}
}
func documentMap(value any) map[string]any {
	data, _ := json.Marshal(value)
	var doc map[string]any
	_ = json.Unmarshal(data, &doc)
	for _, key := range []string{"version", "revisionId", "sourceFingerprint", "renderPin"} {
		delete(doc, key)
	}
	// Empty schema arrays have the same contract as absent arrays.
	if _, ok := doc["resourceType"]; !ok {
		for _, key := range []string{"inputs", "outputs"} {
			if doc[key] == nil {
				doc[key] = []any{}
			}
		}
	}
	if doc["driverType"] == "endpoint" {
		if d, ok := value.(resource.Definition); ok {
			if p, err := endpoint.ParsePolicy(d.Variables()); err == nil {
				data, _ := json.Marshal(p)
				var policy map[string]any
				_ = json.Unmarshal(data, &policy)
				if inputs, ok := doc["driverInputs"].(map[string]any); ok {
					if values, ok := inputs["values"].(map[string]any); ok {
						values["variables"] = policy
					}
				}
			}
		}
	}
	// Schema field order and OR-criterion order do not change the contract.
	for _, key := range []string{"inputs", "outputs", "criteria"} {
		if list, ok := doc[key].([]any); ok {
			if key == "inputs" || key == "outputs" {
				for _, field := range list {
					if m, ok := field.(map[string]any); ok && m["type"] == "" {
						m["type"] = "any"
					}
				}
			}
			sort.Slice(list, func(i, j int) bool {
				a, _ := json.Marshal(list[i])
				b, _ := json.Marshal(list[j])
				return string(a) < string(b)
			})
		}
	}
	return doc
}
func diffDocument(path string, a, b any, fields *[]string) {
	am, aok := a.(map[string]any)
	bm, bok := b.(map[string]any)
	if aok && bok {
		keys := map[string]bool{}
		for k := range am {
			keys[k] = true
		}
		for k := range bm {
			keys[k] = true
		}
		for k := range keys {
			next := k
			if path != "" {
				next = path + "." + k
			}
			diffDocument(next, am[k], bm[k], fields)
		}
		return
	}
	if !reflect.DeepEqual(a, b) {
		*fields = append(*fields, path)
	}
}

// ImportResourceType retains the insert-only registration boundary and re-reads
// the winner of an insert race. An uncertain commit is never retried.
func (s *Service) ImportResourceType(ctx context.Context, org string, typ resource.Type) (DocumentImport, error) {
	_, err := s.RegisterResourceType(ctx, org, typ)
	if err == nil {
		return DocumentImport{Status: "CREATED"}, nil
	}
	if !errors.Is(err, ErrDuplicate) {
		return DocumentImport{}, err
	}
	types, err := s.store.ListResourceTypes(ctx, org)
	if err != nil {
		return DocumentImport{}, err
	}
	for _, existing := range types {
		if existing.Key == typ.Key {
			return importComparison(existing, typ), nil
		}
	}
	return DocumentImport{}, ErrDuplicate
}
func (s *Service) ImportResourceDefinition(ctx context.Context, org string, def resource.Definition) (DocumentImport, error) {
	_, err := s.RegisterResourceDefinition(ctx, org, def)
	if err == nil {
		return DocumentImport{Status: "CREATED"}, nil
	}
	if !errors.Is(err, ErrDuplicate) {
		return DocumentImport{}, err
	}
	def.Version = 1
	prepared, bundle, err := s.prepareDefinition(ctx, org, def)
	if err != nil {
		return DocumentImport{}, err
	}
	existing, err := s.loadDefinition(ctx, org, def.Key)
	if err != nil {
		return DocumentImport{}, err
	}
	left, err := s.normalizedDefinitionDocument(ctx, org, existing, nil)
	if err != nil {
		return DocumentImport{}, err
	}
	right, err := s.normalizedDefinitionDocument(ctx, org, prepared, bundle)
	if err != nil {
		return DocumentImport{}, err
	}
	return importComparison(left, right), nil
}

// Normalize only contract-owned defaults. Never drop arbitrary empty/null values
// in authored variables, whose presence can change fallback behavior.
func (s *Service) normalizedDefinitionDocument(ctx context.Context, org string, def resource.Definition, bundle *tfmodule.Bundle) (map[string]any, error) {
	if def.DriverType == resource.DriverTerraform && bundle == nil {
		_, b, err := s.prepareDefinition(ctx, org, def)
		if err != nil {
			return nil, err
		}
		bundle = b
	}
	doc := documentMap(def)
	inputs, _ := doc["driverInputs"].(map[string]any)
	values, _ := inputs["values"].(map[string]any)
	if values == nil {
		return doc, nil
	}
	variables, _ := values["variables"].(map[string]any)
	if variables == nil {
		variables = map[string]any{}
		values["variables"] = variables
	}
	if bundle != nil {
		for name, input := range bundle.Contract.Inputs {
			if input.HasDefault {
				if _, present := variables[name]; !present {
					variables[name] = input.Default
				}
			}
		}
	}
	if def.DriverType == resource.DriverTemplate {
		git, err := s.gitTemplateSources(ctx, org, def)
		if err != nil {
			return nil, err
		}
		id, _ := variables[resource.VariableTemplateSource].(string)
		if record, ok := git[id]; ok {
			supplied, _ := variables[resource.VariableTemplateInputs].(map[string]any)
			if supplied == nil {
				supplied = map[string]any{}
			}
			effective, err := templatesource.Effective(record.Contract.InputsSchema, supplied)
			if err != nil {
				return nil, err
			}
			variables[resource.VariableTemplateInputs] = effective
		}
	}
	for _, key := range []string{"input_mapping", "output_mapping", "provider_bindings"} {
		if m, ok := values[key].(map[string]any); ok && len(m) == 0 {
			delete(values, key)
		}
	}
	return doc, nil
}
