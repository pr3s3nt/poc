package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PruneDriftHistory is an explicit owner-only maintenance task. Runtime roles
// keep their existing no-DELETE/no-DDL privileges and never receive this URL.
// Windows are fixed by the contract, not caller-supplied cutoffs.
func PruneDriftHistory(ctx context.Context, ownerURL string) (scans, plans int64, err error) {
	pool, err := pgxpool.New(ctx, ownerURL)
	if err != nil {
		return 0, 0, fmt.Errorf("postgres: open retention owner")
	}
	defer pool.Close()
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("postgres: begin retention")
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, baselineLockKey); err != nil {
		return 0, 0, err
	}
	var schema string
	var valid bool
	err = tx.QueryRow(ctx, `SELECT n.nspname,session_user=current_user AND NOT r.rolsuper AND (n.nspowner=r.oid OR (n.nspowner='pg_database_owner'::regrole AND EXISTS(SELECT 1 FROM pg_database d WHERE d.datname=current_database() AND d.datdba=r.oid))) FROM pg_namespace n JOIN pg_roles r ON r.rolname=current_user WHERE n.nspname=current_schema()`).Scan(&schema, &valid)
	if err != nil || !valid {
		return 0, 0, setupRefused("retention requires the ordinary schema owner connected as itself")
	}
	if err = useSchema(ctx, tx, schema); err != nil {
		return 0, 0, err
	}
	marker, err := verifyMarker(ctx, tx, schema)
	if err != nil {
		return 0, 0, err
	}
	catalog, err := catalogDump(ctx, tx, schema)
	if err != nil {
		return 0, 0, err
	}
	if digest(strings.Join(catalog, "\n")) != marker.catalog {
		return 0, 0, resetRequired("retention catalog does not match")
	}
	// Environment row locks serialize with admission. Busy/held work is never
	// interrupted to meet retention, and every batch is bounded.
	if _, err = tx.Exec(ctx, `CREATE TEMP TABLE drift_retention_environments ON COMMIT DROP AS SELECT id FROM environments WHERE false`); err != nil {
		return 0, 0, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO drift_retention_environments SELECT e.id FROM environments e WHERE e.active_operation_id IS NULL AND (EXISTS(SELECT 1 FROM reconciliation_private_artifacts p JOIN terraform_resource_instances i ON i.id=p.owner_id WHERE i.environment_id=e.id AND p.purpose='TERRAFORM_REVIEW' AND drift_repair_artifact_expired(p.id)) OR EXISTS(SELECT 1 FROM drift_scans s WHERE s.environment_id=e.id AND s.status IN ('SUCCEEDED','FAILED','SKIPPED') AND s.finished_at<now()-interval '30 days' AND NOT EXISTS(SELECT 1 FROM drift_repair_reviews r WHERE r.scan_id=s.id) AND (s.operation_id IS NULL OR EXISTS(SELECT 1 FROM environment_operations o WHERE o.id=s.operation_id AND o.status IN ('SUCCEEDED','FAILED','RECOVERED'))))) ORDER BY e.id LIMIT 1000 FOR UPDATE OF e SKIP LOCKED`); err != nil {
		return 0, 0, err
	}
	tag, err := tx.Exec(ctx, `DELETE FROM reconciliation_private_artifacts WHERE id IN (SELECT p.id FROM reconciliation_private_artifacts p JOIN terraform_resource_instances i ON i.id=p.owner_id JOIN drift_retention_environments e ON e.id=i.environment_id WHERE p.purpose='TERRAFORM_REVIEW' AND drift_repair_artifact_expired(p.id) ORDER BY p.created_at,p.id LIMIT 1000 FOR UPDATE OF p SKIP LOCKED)`)
	if err != nil {
		return 0, 0, fmt.Errorf("postgres: prune expired plans (%s)", sqlState(err))
	}
	plans = tag.RowsAffected()
	tag, err = tx.Exec(ctx, `DELETE FROM drift_scans WHERE id IN (SELECT s.id FROM drift_scans s JOIN drift_retention_environments e ON e.id=s.environment_id WHERE s.status IN ('SUCCEEDED','FAILED','SKIPPED') AND s.finished_at<now()-interval '30 days' AND NOT EXISTS(SELECT 1 FROM drift_repair_reviews r WHERE r.scan_id=s.id) AND (s.operation_id IS NULL OR EXISTS(SELECT 1 FROM environment_operations o WHERE o.id=s.operation_id AND o.status IN ('SUCCEEDED','FAILED','RECOVERED'))) ORDER BY s.created_at,s.id LIMIT 1000 FOR UPDATE OF s SKIP LOCKED)`)
	if err != nil {
		return 0, 0, fmt.Errorf("postgres: prune old scans (%s)", sqlState(err))
	}
	scans = tag.RowsAffected()
	if err = tx.Commit(ctx); err != nil {
		return 0, 0, fmt.Errorf("postgres: commit retention")
	}
	return scans, plans, nil
}
