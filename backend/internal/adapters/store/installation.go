package store

import (
	"context"
	"orchestrator/internal/domain/application"
	"sort"
)

func (s *Store) ListOrganizations(ctx context.Context) ([]application.Organization, error) {
	defer s.rlock(ctx)()
	out := []application.Organization{}
	for _, o := range s.state.Organizations {
		out = append(out, o)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// Transact already holds the installation's exclusive lock.
func (s *Store) LockInstallationSetup(context.Context) error { return nil }
