package store

import (
	"context"

	"orchestrator/internal/domain/resource"
	"orchestrator/internal/ports/persistence"
)

func (s *Store) GetDefinitionHistory(ctx context.Context, org, key string) (persistence.DefinitionHistory, error) {
	defer s.rlock(ctx)()
	h, ok := s.state.DefinitionHistories[catalogReleaseKey(org, key)]
	if !ok {
		return persistence.DefinitionHistory{DefinitionKey: key, Versions: []resource.DefinitionRevision{}}, nil
	}
	return copyCatalog(h), nil
}
func (s *Store) SaveDefinitionHistory(ctx context.Context, org string, expected int64, h persistence.DefinitionHistory) error {
	defer s.lock(ctx)()
	if _, ok := s.state.Organizations[org]; !ok {
		return persistence.ErrNotFound
	}
	key := catalogReleaseKey(org, h.DefinitionKey)
	old := s.state.DefinitionHistories[key]
	if old.Generation != expected || h.Generation != expected+1 {
		return persistence.ErrVersionConflict
	}
	if err := persistence.ValidateDefinitionHistory(old, h); err != nil {
		return err
	}
	if s.state.DefinitionHistories == nil {
		s.state.DefinitionHistories = map[string]persistence.DefinitionHistory{}
	}
	s.state.DefinitionHistories[key] = copyCatalog(h)
	return nil
}
func (s *Store) GetDefinitionSelection(ctx context.Context, app, env string) (persistence.DefinitionSelection, error) {
	defer s.rlock(ctx)()
	if _, ok := s.state.Environments[envKey(app, env)]; !ok {
		return persistence.DefinitionSelection{}, persistence.ErrNotFound
	}
	v := s.state.DefinitionSelections[envKey(app, env)]
	v.ApplicationKey = app
	v.EnvironmentKey = env
	return copyCatalog(v), nil
}
func (s *Store) FreezeDefinitionSelection(ctx context.Context, v persistence.DefinitionSelection) error {
	defer s.lock(ctx)()
	key := envKey(v.ApplicationKey, v.EnvironmentKey)
	if _, ok := s.state.Environments[key]; !ok {
		return persistence.ErrNotFound
	}
	if _, ok := s.state.DefinitionSelections[key]; ok {
		return nil
	}
	if s.state.DefinitionSelections == nil {
		s.state.DefinitionSelections = map[string]persistence.DefinitionSelection{}
	}
	v.Frozen = true
	if v.Desired == nil {
		v.Desired = []resource.Definition{}
	}
	s.state.DefinitionSelections[key] = copyCatalog(v)
	return nil
}
func (s *Store) SelectDefinitionSelection(ctx context.Context, org string, expectedEnv, expected int64, v persistence.DefinitionSelection) error {
	return s.catalogTransaction(ctx, func(ctx context.Context) error {
		app, ok := s.state.Applications[v.ApplicationKey]
		if !ok || app.OrganizationKey != org || app.Lifecycle() != "ACTIVE" {
			return persistence.ErrNotFound
		}
		key := envKey(v.ApplicationKey, v.EnvironmentKey)
		env, ok := s.state.Environments[key]
		if !ok {
			return persistence.ErrNotFound
		}
		if env.Version != expectedEnv || s.state.DefinitionSelections[key].Revision != expected || v.Revision != expected+1 {
			return persistence.ErrVersionConflict
		}
		if err := s.ownerCheck(ctx, env); err != nil {
			return err
		}
		env.Version++
		s.state.Environments[key] = env
		if s.state.DefinitionSelections == nil {
			s.state.DefinitionSelections = map[string]persistence.DefinitionSelection{}
		}
		v.Frozen = true
		s.state.DefinitionSelections[key] = copyCatalog(v)
		return nil
	})
}
func (s *Store) ApplyDefinitionSelection(ctx context.Context, app, env string, expected int64, used []resource.Definition, pin string) error {
	return s.catalogTransaction(ctx, func(ctx context.Context) error {
		current, ok := s.state.Environments[envKey(app, env)]
		if !ok {
			return persistence.ErrNotFound
		}
		if _, ok := persistence.OwnerFrom(ctx); !ok {
			return persistence.ErrOperationLost
		}
		if err := s.ownerCheck(ctx, current); err != nil {
			return err
		}
		key := envKey(app, env)
		v := s.state.DefinitionSelections[key]
		if v.Revision != expected {
			return persistence.ErrVersionConflict
		}
		if v.Desired == nil {
			v.Desired = used
			v.Baseline = used
		}
		// The caller passes the frozen planning snapshot, never a mutable authoring head.
		v.Applied = copyCatalog(used)
		v.Desired = copyCatalog(used)
		v.AppliedRevision = expected
		v.AppliedVerified = true
		v.CatalogPin = pin
		v.ApplicationKey = app
		v.EnvironmentKey = env
		if s.state.DefinitionSelections == nil {
			s.state.DefinitionSelections = map[string]persistence.DefinitionSelection{}
		}
		v.Frozen = true
		s.state.DefinitionSelections[key] = copyCatalog(v)
		return nil
	})
}
