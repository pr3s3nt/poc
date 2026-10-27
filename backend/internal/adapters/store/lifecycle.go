package store

import (
	"context"
	"fmt"
	"sort"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/configuration"
	"orchestrator/internal/ports/persistence"
)

var _ persistence.ApplicationLifecycleRepository = (*Store)(nil)

// TransitionApplication is the UC-18 lifecycle compare-and-set.
func (s *Store) TransitionApplication(ctx context.Context, key string, expected int64, status application.LifecycleStatus, deletion *application.Deletion) (application.Application, error) {
	switch status {
	case application.StatusActive, application.StatusDeleting, application.StatusDeleted:
	default:
		return application.Application{}, fmt.Errorf("store: invalid application status %q", status)
	}
	defer s.lock(ctx)()
	app, ok := s.state.Applications[key]
	if !ok {
		return application.Application{}, fmt.Errorf("%w: application %q", persistence.ErrNotFound, key)
	}
	if app.Version != expected {
		return application.Application{}, persistence.ErrVersionConflict
	}
	app.Status = status
	app.Deletion = deletion.Clone()
	app.Version++
	s.state.Applications[key] = app
	out := app
	out.Deletion = app.Deletion.Clone()
	return out, nil
}

// ListConfigurationRevisions returns every revision of one Environment.
func (s *Store) ListConfigurationRevisions(ctx context.Context, app, env string) ([]configuration.Revision, error) {
	defer s.rlock(ctx)()
	out := []configuration.Revision{}
	for _, revision := range s.state.ConfigRevisions {
		if revision.ApplicationKey == app && revision.EnvironmentKey == env {
			out = append(out, copyConfigurationRevision(revision))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}
