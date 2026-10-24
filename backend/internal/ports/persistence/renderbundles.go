package persistence

import (
	"context"

	"orchestrator/internal/domain/renderbundle"
)

// RenderBundleRepository owns the verified Git render bundle registry
// (render_bundles, ADR-021). Records are Organization scoped, insert-only and
// immutable; one (organization, source URL, directory, commit, engine) pin has
// at most one record. It is optional on a Store: a store without it has no Git
// render bundles.
type RenderBundleRepository interface {
	// InsertRenderBundle inserts b unless the Organization already holds a
	// record for the same pin, and returns the stored record: b itself or the
	// existing one, which callers compare to detect drift. ErrNotFound for an
	// unknown Organization. It never updates a stored record.
	InsertRenderBundle(ctx context.Context, b renderbundle.Bundle) (renderbundle.Bundle, error)
	// GetRenderBundle returns one record of the Organization, or ErrNotFound
	// for a missing or foreign ID.
	GetRenderBundle(ctx context.Context, organizationKey, id string) (renderbundle.Bundle, error)
	// ListRenderBundles returns every record of the Organization.
	ListRenderBundles(ctx context.Context, organizationKey string) ([]renderbundle.Bundle, error)
}

// RenderBundlesOf returns the Organization's verified Git render bundles by
// ID, or an empty map when the store keeps no registry.
func RenderBundlesOf(ctx context.Context, st Store, organizationKey string) (map[string]renderbundle.Bundle, error) {
	out := map[string]renderbundle.Bundle{}
	repo, ok := st.(RenderBundleRepository)
	if !ok {
		return out, nil
	}
	list, err := repo.ListRenderBundles(ctx, organizationKey)
	if err != nil {
		return nil, err
	}
	for _, b := range list {
		out[b.ID] = b
	}
	return out, nil
}
