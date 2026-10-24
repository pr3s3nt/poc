package store

import (
	"context"
	"fmt"
	"sort"

	"orchestrator/internal/domain/renderbundle"
	"orchestrator/internal/ports/persistence"
)

func renderBundlePin(b renderbundle.Bundle) string {
	return b.OrganizationKey + "\x00" + b.Source.URL + "\x00" + b.Source.Path + "\x00" + b.Source.Revision + "\x00" + b.EngineContract
}

// InsertRenderBundle implements persistence.RenderBundleRepository with the
// same semantics as PostgreSQL: one record per Organization pin, never updated.
func (s *Store) InsertRenderBundle(ctx context.Context, b renderbundle.Bundle) (renderbundle.Bundle, error) {
	if err := b.Validate(); err != nil {
		return renderbundle.Bundle{}, err
	}
	defer s.lock(ctx)()
	if _, ok := s.state.Organizations[b.OrganizationKey]; !ok {
		return renderbundle.Bundle{}, fmt.Errorf("%w: organization", persistence.ErrNotFound)
	}
	for _, existing := range s.state.RenderBundles {
		if renderBundlePin(existing) == renderBundlePin(b) {
			return existing.Clone(), nil
		}
		if existing.ID == b.ID {
			return renderbundle.Bundle{}, persistence.ErrDuplicate
		}
	}
	s.state.RenderBundles[b.ID] = b.Clone()
	return b.Clone(), nil
}

// GetRenderBundle implements persistence.RenderBundleRepository.
func (s *Store) GetRenderBundle(ctx context.Context, organizationKey, id string) (renderbundle.Bundle, error) {
	defer s.rlock(ctx)()
	b, ok := s.state.RenderBundles[id]
	if !ok || b.OrganizationKey != organizationKey {
		return renderbundle.Bundle{}, persistence.ErrNotFound
	}
	return b.Clone(), nil
}

// ListRenderBundles implements persistence.RenderBundleRepository.
func (s *Store) ListRenderBundles(ctx context.Context, organizationKey string) ([]renderbundle.Bundle, error) {
	defer s.rlock(ctx)()
	out := []renderbundle.Bundle{}
	for _, b := range s.state.RenderBundles {
		if b.OrganizationKey == organizationKey {
			out = append(out, b.Clone())
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

var _ persistence.RenderBundleRepository = (*Store)(nil)
