package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5/pgconn"
	"orchestrator/internal/domain/monitoring"
	"orchestrator/internal/ports/persistence"
)

func (s *Store) ListMonitoringConnections(ctx context.Context, org string) ([]monitoring.Connection, error) {
	rows, e := s.q(ctx).Query(ctx, `SELECT m.document FROM monitoring_connections m JOIN organizations o ON o.id=m.organization_id WHERE o.organization_key=$1 ORDER BY m.connection_key`, org)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []monitoring.Connection{}
	for rows.Next() {
		var b []byte
		var c monitoring.Connection
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		if json.Unmarshal(b, &c) != nil || c.Organization != org {
			return nil, monitoring.ErrInvalid
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
func (s *Store) GetMonitoringConnection(ctx context.Context, org, id string) (monitoring.Connection, error) {
	var c monitoring.Connection
	var b []byte
	e := s.q(ctx).QueryRow(ctx, `SELECT m.document FROM monitoring_connections m JOIN organizations o ON o.id=m.organization_id WHERE o.organization_key=$1 AND m.id::text=$2`, org, id).Scan(&b)
	if e != nil {
		return c, translate(e)
	}
	if json.Unmarshal(b, &c) != nil || c.Organization != org || c.ID != id {
		return c, monitoring.ErrInvalid
	}
	return c, nil
}
func (s *Store) PutMonitoringConnection(ctx context.Context, c monitoring.Connection, expected int64) error {
	if c.ID == "" || c.Organization == "" || c.Revision != expected+1 {
		return monitoring.ErrInvalid
	}
	b, e := json.Marshal(c)
	if e != nil {
		return monitoring.ErrInvalid
	}
	if expected == 0 {
		tag, e := s.q(ctx).Exec(ctx, `INSERT INTO monitoring_connections(id,organization_id,connection_key,revision,document) SELECT $1::uuid,id,$3,$4,$5 FROM organizations WHERE organization_key=$2`, c.ID, c.Organization, c.Key, c.Revision, b)
		if e != nil {
			return translate(e)
		}
		if tag.RowsAffected() != 1 {
			return persistence.ErrNotFound
		}
		return nil
	}
	tag, e := s.q(ctx).Exec(ctx, `UPDATE monitoring_connections m SET revision=$4,document=$5 FROM organizations o WHERE o.id=m.organization_id AND o.organization_key=$2 AND m.id::text=$1 AND m.revision=$3 AND m.connection_key=$6`, c.ID, c.Organization, expected, c.Revision, b, c.Key)
	if e != nil {
		return translate(e)
	}
	if tag.RowsAffected() != 1 {
		return persistence.ErrVersionConflict
	}
	return nil
}
func (s *Store) DeleteMonitoringConnection(ctx context.Context, org, id string, expected int64) error {
	tag, e := s.q(ctx).Exec(ctx, `DELETE FROM monitoring_connections m USING organizations o WHERE o.id=m.organization_id AND o.organization_key=$1 AND m.id::text=$2 AND m.revision=$3`, org, id, expected)
	if e != nil {
		var pgErr *pgconn.PgError
		if errors.As(e, &pgErr) && pgErr.Code == "23503" {
			return monitoring.ErrInUse
		}
		return translate(e)
	}
	if tag.RowsAffected() != 1 {
		return persistence.ErrVersionConflict
	}
	return nil
}
func (s *Store) GetMonitoringBinding(ctx context.Context, org, app, env string) (monitoring.Binding, error) {
	var b []byte
	var out monitoring.Binding
	e := s.q(ctx).QueryRow(ctx, `SELECT b.document FROM monitoring_bindings b JOIN environments e ON e.id=b.environment_id JOIN applications a ON a.id=e.application_id AND a.organization_id=b.organization_id JOIN organizations o ON o.id=a.organization_id WHERE o.organization_key=$1 AND a.application_key=$2 AND e.environment_key=$3`, org, app, env).Scan(&b)
	if e != nil {
		return out, translate(e)
	}
	if json.Unmarshal(b, &out) != nil || out.Organization != org || out.Application != app || out.Environment != env {
		return out, monitoring.ErrInvalid
	}
	return out, nil
}
func (s *Store) PutMonitoringBinding(ctx context.Context, b monitoring.Binding, expected int64) error {
	if b.Revision != expected+1 {
		return monitoring.ErrInvalid
	}
	return s.fenced(ctx, func(ctx context.Context) error {
		env, e := s.GetEnvironment(ctx, b.Application, b.Environment)
		if e != nil {
			return e
		}
		if env.TargetGeneration != b.Generation {
			return persistence.ErrVersionConflict
		}
		if env.Busy() && env.ActiveOperationID != persistence.OperationFrom(ctx) {
			return persistence.Busy(b.Application, b.Environment)
		}
		raw, e := json.Marshal(b)
		if e != nil {
			return monitoring.ErrInvalid
		}
		tag, e := s.q(ctx).Exec(ctx, `INSERT INTO monitoring_bindings(environment_id,organization_id,connection_id,revision,document) SELECT e.id,o.id,c.id,$5,$6 FROM environments e JOIN applications a ON a.id=e.application_id JOIN organizations o ON o.id=a.organization_id JOIN monitoring_connections c ON c.organization_id=o.id AND c.id::text=$4 AND c.revision=$8 WHERE o.organization_key=$1 AND a.application_key=$2 AND e.environment_key=$3 AND (e.active_operation_id IS NULL OR e.active_operation_id=NULLIF($9,'')::uuid) AND e.target_generation=$10 AND ($7=0 OR EXISTS(SELECT 1 FROM monitoring_bindings b WHERE b.environment_id=e.id AND b.revision=$7)) ON CONFLICT(environment_id) DO UPDATE SET connection_id=EXCLUDED.connection_id,revision=EXCLUDED.revision,document=EXCLUDED.document WHERE monitoring_bindings.revision=$7`, b.Organization, b.Application, b.Environment, b.ConnectionID, b.Revision, raw, expected, b.ConnectionRevision, persistence.OperationFrom(ctx), b.Generation)
		if e != nil {
			return translate(e)
		}
		if tag.RowsAffected() != 1 {
			return persistence.ErrVersionConflict
		}
		return nil
	})
}

// Verification updates operational facts without changing the config CAS revision.
func (s *Store) VerifyMonitoringConnection(ctx context.Context, c monitoring.Connection, expected int64) error {
	if c.Revision != expected || c.Verification.Revision != expected {
		return monitoring.ErrInvalid
	}
	verification, e := json.Marshal(c.Verification)
	if e != nil {
		return monitoring.ErrInvalid
	}
	at, e := json.Marshal(c.VerifiedAt)
	if e != nil {
		return monitoring.ErrInvalid
	}
	tag, e := s.q(ctx).Exec(ctx, `UPDATE monitoring_connections m SET document=jsonb_set(jsonb_set(m.document,'{verification}',$4::jsonb),'{verifiedAt}',$5::jsonb) FROM organizations o WHERE o.id=m.organization_id AND o.organization_key=$1 AND m.id::text=$2 AND m.revision=$3`, c.Organization, c.ID, expected, verification, at)
	if e != nil {
		return translate(e)
	}
	if tag.RowsAffected() != 1 {
		return persistence.ErrVersionConflict
	}
	return nil
}
func (s *Store) DeleteMonitoringBinding(ctx context.Context, org, app, env string, expected int64) error {
	return s.fenced(ctx, func(ctx context.Context) error {
		application, e := s.GetApplication(ctx, app)
		if e != nil || application.OrganizationKey != org {
			return persistence.ErrNotFound
		}
		target, e := s.GetEnvironment(ctx, app, env)
		if e != nil {
			return e
		}
		if target.Busy() && target.ActiveOperationID != persistence.OperationFrom(ctx) {
			return persistence.Busy(app, env)
		}
		tag, e := s.q(ctx).Exec(ctx, `DELETE FROM monitoring_bindings b USING environments e, applications a, organizations o WHERE b.environment_id=e.id AND e.application_id=a.id AND a.organization_id=o.id AND b.organization_id=o.id AND o.organization_key=$1 AND a.application_key=$2 AND e.environment_key=$3 AND b.revision=$4 AND (e.active_operation_id IS NULL OR e.active_operation_id=NULLIF($5,'')::uuid)`, org, app, env, expected, persistence.OperationFrom(ctx))
		if e != nil {
			return translate(e)
		}
		if tag.RowsAffected() != 1 {
			return persistence.ErrVersionConflict
		}
		return nil
	})
}
