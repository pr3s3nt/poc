package catalog

import (
	"context"

	catalogapp "orchestrator/internal/application/catalogrelease"
	"orchestrator/internal/application/definitionversions"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/ports/persistence"
)

func (s *Service) ChangeRevisionStatus(ctx context.Context, org, key, id string, expected int64, archive bool) error {
	h, err := s.store.GetDefinitionHistory(ctx, org, key)
	if err != nil {
		return err
	}
	if h.Generation != expected {
		return persistence.ErrVersionConflict
	}
	found := false
	for _, v := range h.Versions {
		if v.ID == id {
			found = true
			if !archive {
				if _, _, err := s.prepareDefinition(ctx, org, v.Definition); err != nil {
					return err
				}
			}
			break
		}
	}
	if !found {
		return persistence.ErrNotFound
	}
	return s.store.Transact(ctx, func(ctx context.Context) error {
		if !archive {
			if err := definitionversions.FreezeOrganization(ctx, s.store, org, catalogapp.ResolveDefinitions); err != nil {
				return err
			}
		}
		return definitionversions.SetStatus(ctx, s.store, org, key, id, expected, archive)
	})
}

func (s *Service) recordRevision(ctx context.Context, org string, def resource.Definition, active bool) (resource.Definition, error) {
	if def.DriverType == resource.DriverTemplate {
		types, err := s.store.ListResourceTypes(ctx, org)
		if err != nil {
			return def, err
		}
		var typ resource.Type
		for _, t := range types {
			if t.Key == def.ResourceTypeKey {
				typ = t
				break
			}
		}
		git, err := s.gitTemplateSources(ctx, org, def)
		if err != nil {
			return def, err
		}
		pin, _, err := resource.ResolveRenderDefinition(def, typ, s.bundles, git, true)
		if err != nil {
			return def, err
		}
		def.RenderPin = &pin
	}
	return definitionversions.Record(ctx, s.store, org, def, active)
}
