package store

import (
	"context"
	"orchestrator/internal/domain/monitoring"
	"orchestrator/internal/ports/persistence"
	"sort"
)

func (s *Store) ListMonitoringConnections(ctx context.Context, org string) ([]monitoring.Connection, error) {
	defer s.rlock(ctx)()
	out := []monitoring.Connection{}
	for _, c := range s.state.MonitoringConnections {
		if c.Organization == org {
			out = append(out, monitoring.CloneConnection(c))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}
func (s *Store) GetMonitoringConnection(ctx context.Context, org, id string) (monitoring.Connection, error) {
	defer s.rlock(ctx)()
	c, ok := s.state.MonitoringConnections[id]
	if !ok || c.Organization != org {
		return monitoring.Connection{}, persistence.ErrNotFound
	}
	return monitoring.CloneConnection(c), nil
}
func (s *Store) PutMonitoringConnection(ctx context.Context, c monitoring.Connection, expected int64) error {
	defer s.lock(ctx)()
	if c.ID == "" || c.Key == "" || c.Revision != expected+1 {
		return monitoring.ErrInvalid
	}
	if _, ok := s.state.Organizations[c.Organization]; !ok {
		return persistence.ErrNotFound
	}
	old, ok := s.state.MonitoringConnections[c.ID]
	if (expected == 0 && ok) || (expected > 0 && (!ok || old.Organization != c.Organization || old.Key != c.Key || old.Revision != expected)) {
		return persistence.ErrVersionConflict
	}
	for id, prior := range s.state.MonitoringConnections {
		if id != c.ID && prior.Organization == c.Organization && prior.Key == c.Key {
			return persistence.ErrVersionConflict
		}
	}
	s.state.MonitoringConnections[c.ID] = monitoring.CloneConnection(c)
	return nil
}
func (s *Store) VerifyMonitoringConnection(ctx context.Context, c monitoring.Connection, expected int64) error {
	defer s.lock(ctx)()
	old, ok := s.state.MonitoringConnections[c.ID]
	if !ok || old.Organization != c.Organization {
		return persistence.ErrNotFound
	}
	if old.Revision != expected || c.Revision != expected || c.Verification.Revision != expected {
		return persistence.ErrVersionConflict
	}
	old.Verification = c.Verification
	old.VerifiedAt = c.VerifiedAt
	s.state.MonitoringConnections[c.ID] = monitoring.CloneConnection(old)
	return nil
}
func (s *Store) DeleteMonitoringConnection(ctx context.Context, org, id string, expected int64) error {
	defer s.lock(ctx)()
	old, ok := s.state.MonitoringConnections[id]
	if !ok || old.Organization != org {
		return persistence.ErrNotFound
	}
	if old.Revision != expected {
		return persistence.ErrVersionConflict
	}
	for _, b := range s.state.MonitoringBindings {
		if b.ConnectionID == id {
			return monitoring.ErrInUse
		}
	}
	delete(s.state.MonitoringConnections, id)
	return nil
}
func (s *Store) GetMonitoringBinding(ctx context.Context, org, app, env string) (monitoring.Binding, error) {
	defer s.rlock(ctx)()
	a, ok := s.state.Applications[app]
	if !ok || a.OrganizationKey != org {
		return monitoring.Binding{}, persistence.ErrNotFound
	}
	b, ok := s.state.MonitoringBindings[envKey(app, env)]
	if !ok || b.Organization != org {
		return monitoring.Binding{}, persistence.ErrNotFound
	}
	return monitoring.CloneBinding(b), nil
}
func (s *Store) PutMonitoringBinding(ctx context.Context, b monitoring.Binding, expected int64) error {
	defer s.lock(ctx)()
	a, ok := s.state.Applications[b.Application]
	if !ok || a.OrganizationKey != b.Organization {
		return persistence.ErrNotFound
	}
	env, ok := s.state.Environments[envKey(b.Application, b.Environment)]
	if !ok {
		return persistence.ErrNotFound
	}
	if env.Busy() && env.ActiveOperationID != persistence.OperationFrom(ctx) {
		return persistence.Busy(b.Application, b.Environment)
	}
	if env.TargetGeneration != b.Generation {
		return persistence.ErrVersionConflict
	}
	c, ok := s.state.MonitoringConnections[b.ConnectionID]
	if !ok || c.Organization != b.Organization || c.Revision != b.ConnectionRevision {
		return persistence.ErrVersionConflict
	}
	old, ok := s.state.MonitoringBindings[envKey(b.Application, b.Environment)]
	if b.Revision != expected+1 || (ok && old.Revision != expected) || (!ok && expected != 0) {
		return persistence.ErrVersionConflict
	}
	s.state.MonitoringBindings[envKey(b.Application, b.Environment)] = monitoring.CloneBinding(b)
	return nil
}
func (s *Store) DeleteMonitoringBinding(ctx context.Context, org, app, env string, expected int64) error {
	defer s.lock(ctx)()
	a, ok := s.state.Applications[app]
	if !ok || a.OrganizationKey != org {
		return persistence.ErrNotFound
	}
	e, ok := s.state.Environments[envKey(app, env)]
	if !ok {
		return persistence.ErrNotFound
	}
	if e.Busy() && e.ActiveOperationID != persistence.OperationFrom(ctx) {
		return persistence.Busy(app, env)
	}
	b, ok := s.state.MonitoringBindings[envKey(app, env)]
	if !ok {
		return persistence.ErrNotFound
	}
	if b.Revision != expected {
		return persistence.ErrVersionConflict
	}
	delete(s.state.MonitoringBindings, envKey(app, env))
	return nil
}
