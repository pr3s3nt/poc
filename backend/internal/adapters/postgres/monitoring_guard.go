package postgres

import (
	"context"
	"orchestrator/internal/domain/monitoring"
	"orchestrator/internal/ports/persistence"
)

// One short metadata guard orders monitoring mutation before Environment claim
// locks. It is never held while talking to a cluster or credential provider.
const monitoringConfigLock int64 = 0x4f52434d4f4e

func (s *Store) monitoringMutation(ctx context.Context, fn func(context.Context) error) error {
	return s.Transact(ctx, func(ctx context.Context) error {
		if _, e := s.q(ctx).Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, monitoringConfigLock); e != nil {
			return translate(e)
		}
		return fn(ctx)
	})
}
func (s *Store) lockMonitoringEnvironment(ctx context.Context, org, app, env string) error {
	var id string
	e := s.q(ctx).QueryRow(ctx, `SELECT e.id::text FROM environments e JOIN applications a ON a.id=e.application_id JOIN organizations o ON o.id=a.organization_id WHERE o.organization_key=$1 AND a.application_key=$2 AND e.environment_key=$3 FOR UPDATE OF e`, org, app, env).Scan(&id)
	return translate(e)
}
func (s *Store) monitoringReferencesIdle(ctx context.Context, org, id string) error {
	rows, e := s.q(ctx).Query(ctx, `SELECT a.application_key,e.environment_key,COALESCE(e.active_operation_id::text,'') FROM monitoring_bindings b JOIN monitoring_connections c ON c.id=b.connection_id JOIN environments e ON e.id=b.environment_id JOIN applications a ON a.id=e.application_id JOIN organizations o ON o.id=a.organization_id WHERE o.organization_key=$1 AND c.id::text=$2 ORDER BY e.id FOR UPDATE OF e`, org, id)
	if e != nil {
		return translate(e)
	}
	defer rows.Close()
	for rows.Next() {
		var app, env, op string
		if e = rows.Scan(&app, &env, &op); e != nil {
			return translate(e)
		}
		if op != "" && op != persistence.OperationFrom(ctx) {
			return persistence.Busy(app, env)
		}
	}
	return rows.Err()
}
func (s *Store) PutMonitoringConnection(ctx context.Context, c monitoring.Connection, expected int64) error {
	return s.monitoringMutation(ctx, func(ctx context.Context) error {
		if expected > 0 {
			if e := s.monitoringReferencesIdle(ctx, c.Organization, c.ID); e != nil {
				return e
			}
		}
		return s.putMonitoringConnection(ctx, c, expected)
	})
}
func (s *Store) DeleteMonitoringConnection(ctx context.Context, org, id string, expected int64) error {
	return s.monitoringMutation(ctx, func(ctx context.Context) error {
		if e := s.monitoringReferencesIdle(ctx, org, id); e != nil {
			return e
		}
		return s.deleteMonitoringConnection(ctx, org, id, expected)
	})
}
func (s *Store) PutMonitoringBinding(ctx context.Context, b monitoring.Binding, expected int64) error {
	return s.monitoringMutation(ctx, func(ctx context.Context) error {
		if e := s.lockMonitoringEnvironment(ctx, b.Organization, b.Application, b.Environment); e != nil {
			return e
		}
		return s.putMonitoringBinding(ctx, b, expected)
	})
}
func (s *Store) DeleteMonitoringBinding(ctx context.Context, org, app, env string, expected int64) error {
	return s.monitoringMutation(ctx, func(ctx context.Context) error {
		if e := s.lockMonitoringEnvironment(ctx, org, app, env); e != nil {
			return e
		}
		return s.deleteMonitoringBinding(ctx, org, app, env, expected)
	})
}

func (s *Store) MarkMonitoringDeleting(ctx context.Context, org, id string, expected int64) error {
	return s.monitoringMutation(ctx, func(ctx context.Context) error {
		c, e := s.GetMonitoringConnection(ctx, org, id)
		if e != nil {
			return e
		}
		if c.Revision != expected {
			return persistence.ErrVersionConflict
		}
		var referenced bool
		if e = s.q(ctx).QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM monitoring_bindings WHERE connection_id=$1::uuid)`, id).Scan(&referenced); e != nil {
			return translate(e)
		}
		if referenced {
			return monitoring.ErrInUse
		}
		tag, e := s.q(ctx).Exec(ctx, `UPDATE monitoring_connections SET document=jsonb_set(document,'{deleting}','true'::jsonb) WHERE id=$1::uuid AND revision=$2`, id, expected)
		if e != nil {
			return translate(e)
		}
		if tag.RowsAffected() != 1 {
			return persistence.ErrVersionConflict
		}
		return nil
	})
}
func (s *Store) ClearRetiredMonitoringCredentials(ctx context.Context, org, id string, expected int64) error {
	tag, e := s.q(ctx).Exec(ctx, `UPDATE monitoring_connections m SET document=document-'retiredCredentials' FROM organizations o WHERE o.id=m.organization_id AND o.organization_key=$1 AND m.id::text=$2 AND m.revision=$3`, org, id, expected)
	if e != nil {
		return translate(e)
	}
	if tag.RowsAffected() != 1 {
		return persistence.ErrVersionConflict
	}
	return nil
}
