package store

import (
	"context"
	"orchestrator/internal/ports/persistence"
)

func (s *Store) GetGitSettings(ctx context.Context, org string) (persistence.GitSettings, error) {
	defer s.rlock(ctx)()
	v, ok := s.state.GitSettings[org]
	if !ok {
		return persistence.GitSettings{}, persistence.ErrNotFound
	}
	return v, nil
}
func (s *Store) SaveGitSettings(ctx context.Context, expected int64, v persistence.GitSettings) error {
	defer s.lock(ctx)()
	if _, ok := s.state.Organizations[v.OrganizationKey]; !ok {
		return persistence.ErrNotFound
	}
	if s.state.GitSettings[v.OrganizationKey].Version != expected || v.Version != expected+1 {
		return persistence.ErrVersionConflict
	}
	if s.state.GitSettings == nil {
		s.state.GitSettings = map[string]persistence.GitSettings{}
	}
	s.state.GitSettings[v.OrganizationKey] = v
	return nil
}
