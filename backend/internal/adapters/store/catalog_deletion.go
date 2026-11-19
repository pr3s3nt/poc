package store

import (
	"context"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/ports/persistence"
)

func (s *Store) DeleteCatalogEntry(ctx context.Context, org, key string, definition bool, check func(context.Context) error) error {
	return s.Transact(ctx, func(ctx context.Context) error {
		id := catalogKey(org, key)
		if definition {
			if _, ok := s.state.Definitions[id]; !ok {
				return persistence.ErrNotFound
			}
		} else if _, ok := s.state.ResourceTypes[id]; !ok {
			return persistence.ErrNotFound
		}
		if err := check(ctx); err != nil {
			return err
		}
		if definition {
			delete(s.state.Definitions, id)
		} else {
			delete(s.state.ResourceTypes, id)
		}
		for key, env := range s.state.Environments {
			app := s.state.Applications[env.ApplicationKey]
			if app.OrganizationKey == org && app.Lifecycle() == application.StatusActive {
				env.Version++
				env.DraftVersion++
				s.state.Environments[key] = env
			}
		}
		return nil
	})
}
