package store

import (
	"context"
	"sort"

	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/ports/persistence"
)

// InsertModuleBundle implements persistence.ModuleBundleRepository with the
// same semantics as PostgreSQL: one bundle per revision path, never updated.
func (s *Store) InsertModuleBundle(ctx context.Context, b tfmodule.Bundle) (tfmodule.Bundle, error) {
	if err := b.Validate(); err != nil {
		return tfmodule.Bundle{}, err
	}
	defer s.lock(ctx)()
	if existing, ok := s.state.ModuleBundles[b.Ref.Pin()]; ok {
		return existing.Clone(), nil
	}
	for _, existing := range s.state.ModuleBundles {
		if existing.ID == b.ID {
			return tfmodule.Bundle{}, persistence.ErrDuplicate
		}
	}
	s.state.ModuleBundles[b.Ref.Pin()] = b.Clone()
	return b.Clone(), nil
}

// GetModuleBundle implements persistence.ModuleBundleRepository.
func (s *Store) GetModuleBundle(ctx context.Context, ref tfmodule.SourceRef) (tfmodule.Bundle, error) {
	defer s.rlock(ctx)()
	b, ok := s.state.ModuleBundles[ref.Pin()]
	if !ok {
		return tfmodule.Bundle{}, persistence.ErrNotFound
	}
	return b.Clone(), nil
}

// ListModuleBundles implements persistence.ModuleBundleRepository.
func (s *Store) ListModuleBundles(ctx context.Context) ([]tfmodule.Bundle, error) {
	defer s.rlock(ctx)()
	out := make([]tfmodule.Bundle, 0, len(s.state.ModuleBundles))
	for _, b := range s.state.ModuleBundles {
		out = append(out, b.Clone())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ref.Key() < out[j].Ref.Key() })
	return out, nil
}
