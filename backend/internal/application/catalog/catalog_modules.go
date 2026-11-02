package catalog

import (
	"context"
	"encoding/json"
	"strings"

	domain "orchestrator/internal/domain/catalogrelease"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/ports/persistence"
)

// CatalogModule is a component choice backed by an exact verified execution pin.
type CatalogModule struct {
	ComponentID string     `json:"componentId"`
	Capability  Capability `json:"capability"`
}

type moduleSelection struct {
	SourceID    string `json:"sourceId"`
	Tag         string `json:"tag"`
	ComponentID string `json:"componentId"`
}

// CatalogModules reads scoped immutable metadata only; it never fetches Git.
func (s *Service) CatalogModules(ctx context.Context, org, source, tag, resourceType string) ([]CatalogModule, error) {
	release, err := s.store.GetCatalogRelease(ctx, org, source, tag)
	if err != nil {
		return nil, err
	}
	types, err := s.store.ListResourceTypes(ctx, org)
	if err != nil {
		return nil, err
	}
	var typ resource.Type
	for _, t := range types {
		if t.Key == resourceType {
			typ = t
			break
		}
	}
	if typ.Key == "" {
		return nil, persistence.ErrNotFound
	}
	out := []CatalogModule{}
	for _, c := range release.Components {
		if c.Kind != domain.TerraformModule || c.ResourceType != resourceType {
			continue
		}
		bundle, err := s.store.GetModuleBundleByID(ctx, c.BundleID)
		if err != nil {
			return nil, err
		}
		if s.modules == nil {
			return nil, domain.ErrUnavailable
		}
		verified, err := s.modules.Bundle(ctx, bundle.Ref)
		if err != nil {
			return nil, err
		}
		if verified.ID != c.BundleID || verified.Ref.ContentSHA256 != c.SHA256 || !verified.ServesType(resourceType) || !bundle.SameVerifiedContent(verified) {
			return nil, domain.ErrIncomplete
		}
		capability, err := terraformCapability(verified, typ, resourceType)
		if err != nil {
			return nil, err
		}
		out = append(out, CatalogModule{ComponentID: c.ID, Capability: capability})
	}
	return out, nil
}

func (s *Service) validateCatalogModule(ctx context.Context, org string, def resource.Definition) error {
	raw, present := def.DriverInputs["catalog"]
	if !present {
		return nil
	}
	bad := func() error { return invalid("definition.driverInputs.catalog") }
	if def.DriverType != resource.DriverTerraform || raw == nil {
		return bad()
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return bad()
	}
	var selection moduleSelection
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if dec.Decode(&selection) != nil || !domain.ValidID(selection.SourceID) || selection.Tag == "" || len(selection.Tag) > 253 || !domain.ValidID(selection.ComponentID) {
		return bad()
	}
	modules, err := s.CatalogModules(ctx, org, selection.SourceID, selection.Tag, def.ResourceTypeKey)
	if err != nil {
		return bad()
	}
	pin, err := tfmodule.ParseSourceRef(def.DriverValues()["source"])
	if err != nil {
		return bad()
	}
	for _, module := range modules {
		if module.ComponentID != selection.ComponentID {
			continue
		}
		source := module.Capability.Source
		if source != nil && source.SourceID == pin.SourceID && source.ModuleID == pin.ModuleID && source.Revision == pin.Revision && source.Subdirectory == pin.Subdirectory && source.ContentSHA256 == pin.ContentSHA256 {
			return nil
		}
	}
	return bad()
}
