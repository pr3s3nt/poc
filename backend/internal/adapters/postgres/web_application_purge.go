package postgres

import (
	"context"
	"errors"

	"orchestrator/internal/ports/persistence"
)

var _ persistence.WebApplicationPurgeStore = (*Store)(nil)

func (s *Store) WebApplicationPurgeEnabled(ctx context.Context) (bool, error) {
	var enabled bool
	err := s.q(ctx).QueryRow(ctx, `SELECT COALESCE((SELECT enabled FROM application_purge_policy WHERE singleton),false)`).Scan(&enabled)
	return enabled, err
}

func (s *Store) PreviewWebApplicationPurge(ctx context.Context, organization, application string) (ApplicationPurgeReport, error) {
	report := ApplicationPurgeReport{InstallationID: s.Identity().Installation, Organization: organization, Application: application}
	err := s.Transact(ctx, func(ctx context.Context) error {
		err := s.q(ctx).QueryRow(ctx, `SELECT a.id::text,a.name,a.status,
 COALESCE(a.deletion->>'status'='RUNNING',false) OR EXISTS(
   SELECT 1 FROM environment_operations op JOIN environments e ON e.id=op.environment_id
   WHERE e.application_id=a.id AND op.status IN ('ACTIVE','RECOVERING')
 ),
 EXISTS(
  SELECT 1 FROM environment_operations op JOIN environments e ON e.id=op.environment_id
  WHERE e.application_id=a.id AND (op.status='INTERRUPTED' OR EXISTS(
   SELECT 1 FROM terraform_state_locks l WHERE l.environment_operation_id=op.id))
 )
FROM applications a JOIN organizations o ON o.id=a.organization_id
WHERE o.organization_key=$1 AND a.application_key=$2`, organization, application).
			Scan(&report.ApplicationID, &report.DisplayName, &report.Status, &report.Busy, &report.PriorStopRequired)
		if err != nil {
			return translate(err)
		}
		return readApplicationPurgeReport(ctx, s.q(ctx), &report)
	})
	return report, err
}

func (s *Store) ExecuteWebApplicationPurge(ctx context.Context, organization, application, expectedID, name string, priorStopped bool) (string, error) {
	var outcome string
	err := s.Transact(ctx, func(ctx context.Context) error {
		if _, err := s.q(ctx).Exec(ctx, `SET LOCAL lock_timeout = '5s'`); err != nil {
			return err
		}
		return s.q(ctx).QueryRow(ctx, `SELECT purge_one_application($1,$2,$3::uuid,$4,$5)`,
			organization, application, expectedID, name, priorStopped).Scan(&outcome)
	})
	return outcome, err
}

// SetWebApplicationPurge records the installation owner's explicit opt-in.
func SetWebApplicationPurge(ctx context.Context, ownerURL string, enabled bool) error {
	pool, err := connect(ctx, ownerURL)
	if err != nil {
		return errors.New("postgres: could not open purge policy owner connection")
	}
	defer pool.Close()
	tx, err := pool.Begin(ctx)
	if err != nil {
		return errors.New("postgres: could not start purge policy update")
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	var schema string
	if err = tx.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		return purgeError("inspect purge policy schema", err)
	}
	if err = useSchema(ctx, tx, schema); err != nil {
		return purgeError("select purge policy schema", err)
	}
	if _, err = verifyMarker(ctx, tx, schema); err != nil {
		return errors.New("postgres: purge policy requires the release's supported database schema")
	}
	var valid bool
	if err = tx.QueryRow(ctx, `SELECT c.relowner=(SELECT oid FROM pg_roles WHERE rolname=session_user)
FROM pg_class c WHERE c.oid='application_purge_policy'::regclass`).Scan(&valid); err != nil || !valid {
		return errors.New("postgres: purge policy requires the installation owner")
	}
	tag, err := tx.Exec(ctx, `INSERT INTO application_purge_policy(singleton,enabled) VALUES(true,$1) ON CONFLICT(singleton) DO UPDATE SET enabled=EXCLUDED.enabled`, enabled)
	if err != nil || tag.RowsAffected() != 1 {
		return errors.New("postgres: could not set purge policy")
	}
	if err := tx.Commit(ctx); err != nil {
		return purgeError("commit purge policy", err)
	}
	return nil
}
