package store

import (
	"context"
	"fmt"
	"sort"

	"orchestrator/internal/domain/templatesource"
	"orchestrator/internal/ports/persistence"
)

func templateSourcePin(b templatesource.Bundle) string {
	return b.OrganizationKey + "\x00" + b.Source.URL + "\x00" + b.Source.Path + "\x00" + b.Source.Revision + "\x00" + b.EngineContract + "\x00" + b.RendererBinary
}

// InsertTemplateSource implements persistence.TemplateSourceRepository with the
// same semantics as PostgreSQL: one record per Organization pin, never updated.
func (s *Store) InsertTemplateSource(ctx context.Context, b templatesource.Bundle) (templatesource.Bundle, error) {
	if err := b.Validate(); err != nil {
		return templatesource.Bundle{}, err
	}
	defer s.lock(ctx)()
	if _, ok := s.state.Organizations[b.OrganizationKey]; !ok {
		return templatesource.Bundle{}, fmt.Errorf("%w: organization", persistence.ErrNotFound)
	}
	for _, existing := range s.state.TemplateSources {
		if templateSourcePin(existing) == templateSourcePin(b) {
			return existing.Clone(), nil
		}
		if existing.ID == b.ID {
			return templatesource.Bundle{}, persistence.ErrDuplicate
		}
	}
	s.state.TemplateSources[b.ID] = b.Clone()
	return b.Clone(), nil
}

// GetTemplateSource implements persistence.TemplateSourceRepository.
func (s *Store) GetTemplateSource(ctx context.Context, organizationKey, id string) (templatesource.Bundle, error) {
	defer s.rlock(ctx)()
	b, ok := s.state.TemplateSources[id]
	if !ok || b.OrganizationKey != organizationKey {
		return templatesource.Bundle{}, persistence.ErrNotFound
	}
	return b.Clone(), nil
}

// ListTemplateSources implements persistence.TemplateSourceRepository.
func (s *Store) ListTemplateSources(ctx context.Context, organizationKey string) ([]templatesource.Bundle, error) {
	defer s.rlock(ctx)()
	out := []templatesource.Bundle{}
	for _, b := range s.state.TemplateSources {
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

var _ persistence.TemplateSourceRepository = (*Store)(nil)
