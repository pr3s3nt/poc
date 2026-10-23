package store

import (
	"context"
	"fmt"
	"sort"

	"orchestrator/internal/domain/effectiveness"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
)

// ListEffectivenessBaselines returns the versions of one metric and scope,
// highest version first.
func (s *Store) ListEffectivenessBaselines(ctx context.Context, organizationKey, metric, applicationKey string) ([]effectiveness.Baseline, error) {
	defer s.rlock(ctx)()
	out := []effectiveness.Baseline{}
	for _, b := range s.state.Baselines {
		if b.OrganizationKey == organizationKey && b.Metric == metric && b.ApplicationKey == applicationKey {
			out = append(out, b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version > out[j].Version })
	return out, nil
}

// AppendEffectivenessBaseline inserts version expectedVersion+1 of the scope.
func (s *Store) AppendEffectivenessBaseline(ctx context.Context, b effectiveness.Baseline, expectedVersion int64) (effectiveness.Baseline, error) {
	defer s.lock(ctx)()
	if _, ok := s.state.Organizations[b.OrganizationKey]; !ok {
		return effectiveness.Baseline{}, fmt.Errorf("%w: organization", persistence.ErrNotFound)
	}
	if b.ApplicationKey != "" {
		app, ok := s.state.Applications[b.ApplicationKey]
		if !ok || app.OrganizationKey != b.OrganizationKey {
			return effectiveness.Baseline{}, fmt.Errorf("%w: application", persistence.ErrNotFound)
		}
	}
	if author, ok := s.state.UserAccounts[b.CreatedBy]; !ok || author.OrganizationKey != b.OrganizationKey {
		return effectiveness.Baseline{}, fmt.Errorf("%w: author", persistence.ErrNotFound)
	}
	var current int64
	for _, v := range s.state.Baselines {
		if v.OrganizationKey == b.OrganizationKey && v.Metric == b.Metric && v.ApplicationKey == b.ApplicationKey && v.Version > current {
			current = v.Version
		}
	}
	if current != expectedVersion {
		return effectiveness.Baseline{}, persistence.ErrVersionConflict
	}
	b.ID, b.Version = ids.New(), expectedVersion+1
	s.state.Baselines[b.ID] = b
	return b, nil
}
