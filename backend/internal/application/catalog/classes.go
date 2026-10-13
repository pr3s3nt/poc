package catalog

import (
	"context"
	"errors"
	"sort"

	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/ports/moduleregistry"
	"orchestrator/internal/ports/persistence"
)

// ResourceTypeView is the read projection of GET /resource-types: the
// registered contract plus the computed, non-persisted `classes` (UC-16 API
// mapping, I04-07). Classes are never a Type registration field.
type ResourceTypeView struct {
	resource.Type
	Classes []string `json:"classes"`
}

// ListResourceTypeViews returns the Organization's Resource Types with the
// provisioning classes the scoped catalog can serve. Types, Definitions and
// verified registry metadata are read once in one snapshot; the query never
// fetches or inspects a module, calls a provider or CLI, touches a secret
// store or writes. A class is offered only when an authored Definition of the
// Type declares it in a criterion and, for a Terraform Definition, the pinned
// module is a verified registry bundle that serves the Type. Nothing is
// guessed from names, targets or defaults: a Type with no such class has `[]`.
func (s *Service) ListResourceTypeViews(ctx context.Context, organizationKey string) ([]ResourceTypeView, error) {
	var types []resource.Type
	var definitions []resource.Definition
	err := s.store.ReadSnapshot(ctx, func(ctx context.Context, view persistence.Store) error {
		var err error
		if types, err = view.ListResourceTypes(ctx, organizationKey); err != nil {
			return err
		}
		definitions, err = view.ListResourceDefinitions(ctx, organizationKey)
		return err
	})
	if err != nil {
		return nil, err
	}
	classes := map[string]map[string]bool{}
	for _, def := range definitions {
		declared := declaredClasses(def)
		if len(declared) == 0 {
			continue
		}
		serves, err := s.servesType(ctx, def)
		if err != nil {
			return nil, &internalError{err: err}
		}
		if !serves {
			continue
		}
		if classes[def.ResourceTypeKey] == nil {
			classes[def.ResourceTypeKey] = map[string]bool{}
		}
		for _, class := range declared {
			classes[def.ResourceTypeKey][class] = true
		}
	}
	out := make([]ResourceTypeView, 0, len(types))
	for _, typ := range types {
		view := ResourceTypeView{Type: typ, Classes: []string{}}
		for class := range classes[typ.Key] {
			view.Classes = append(view.Classes, class)
		}
		sort.Strings(view.Classes)
		out = append(out, view)
	}
	return out, nil
}

func declaredClasses(def resource.Definition) []string {
	seen := map[string]bool{}
	var out []string
	for _, criterion := range def.Criteria {
		if criterion.Class != "" && !seen[criterion.Class] {
			seen[criterion.Class] = true
			out = append(out, criterion.Class)
		}
	}
	return out
}

// servesType reports whether a Definition can provide its Type. Only
// Terraform Definitions depend on registry metadata: their pinned bundle must
// be verified and register the Type. Other drivers have no registry bundle.
func (s *Service) servesType(ctx context.Context, def resource.Definition) (bool, error) {
	if def.DriverType != resource.DriverTerraform {
		return true, nil
	}
	if s.modules == nil {
		return false, errors.New("catalog: the Terraform module registry is unavailable")
	}
	inputs, err := tfmodule.ParseDriverInputs(def.DriverValues())
	if err != nil {
		return false, nil // not a v2 Terraform pin: nothing verified to offer
	}
	bundle, err := s.modules.Bundle(ctx, inputs.Source)
	switch {
	case errors.Is(err, moduleregistry.ErrNotVerified), errors.Is(err, moduleregistry.ErrReleaseMismatch):
		return false, nil
	case err != nil:
		return false, err
	}
	return bundle.ServesType(def.ResourceTypeKey), nil
}
