package persistence

import (
	"context"

	"orchestrator/internal/domain/templatesource"
)

// TemplateSourceRepository owns the verified Git render bundle registry
// (template_sources, ADR-021). Records are Organization scoped, insert-only and
// immutable; one (organization, source URL, directory, commit, engine) pin has
// at most one record. It is optional on a Store: a store without it has no Git
// render bundles.
type TemplateSourceRepository interface {
	// InsertTemplateSource inserts b unless the Organization already holds a
	// record for the same pin, and returns the stored record: b itself or the
	// existing one, which callers compare to detect drift. ErrNotFound for an
	// unknown Organization. It never updates a stored record.
	InsertTemplateSource(ctx context.Context, b templatesource.Bundle) (templatesource.Bundle, error)
	// GetTemplateSource returns one record of the Organization, or ErrNotFound
	// for a missing or foreign ID.
	GetTemplateSource(ctx context.Context, organizationKey, id string) (templatesource.Bundle, error)
	// ListTemplateSources returns every record of the Organization.
	ListTemplateSources(ctx context.Context, organizationKey string) ([]templatesource.Bundle, error)
}

// TemplateSourcesOf returns the Organization's verified Git render bundles by
// ID, or an empty map when the store keeps no registry.
func TemplateSourcesOf(ctx context.Context, st Store, organizationKey string) (map[string]templatesource.Bundle, error) {
	out := map[string]templatesource.Bundle{}
	repo, ok := st.(TemplateSourceRepository)
	if !ok {
		return out, nil
	}
	list, err := repo.ListTemplateSources(ctx, organizationKey)
	if err != nil {
		return nil, err
	}
	for _, b := range list {
		out[b.ID] = b
	}
	return out, nil
}
