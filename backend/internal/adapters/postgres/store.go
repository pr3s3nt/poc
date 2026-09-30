// Package postgres implements the persistence ports with normalized PostgreSQL tables.
package postgres

import (
	"context"
	_ "embed"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"orchestrator/internal/ports/persistence"
)

//go:embed migration.sql
var migration string

const migration2 = `
ALTER TABLE user_accounts ADD COLUMN new_id uuid;
UPDATE user_accounts SET new_id = CASE
  WHEN id ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$' THEN id::uuid
  ELSE gen_random_uuid()
END;
ALTER TABLE sessions ADD COLUMN new_user_account_id uuid;
UPDATE sessions s SET new_user_account_id=u.new_id FROM user_accounts u WHERE u.id=s.user_account_id;
ALTER TABLE sessions DROP CONSTRAINT sessions_user_account_id_fkey;
ALTER TABLE user_accounts DROP CONSTRAINT user_accounts_pkey;
ALTER TABLE sessions DROP COLUMN user_account_id;
ALTER TABLE sessions RENAME COLUMN new_user_account_id TO user_account_id;
ALTER TABLE user_accounts DROP COLUMN id;
ALTER TABLE user_accounts RENAME COLUMN new_id TO id;
ALTER TABLE user_accounts ALTER COLUMN id SET NOT NULL;
ALTER TABLE user_accounts ADD PRIMARY KEY(id);
ALTER TABLE sessions ALTER COLUMN id TYPE uuid USING id::uuid;
ALTER TABLE sessions ALTER COLUMN user_account_id SET NOT NULL;
ALTER TABLE sessions ADD FOREIGN KEY(user_account_id) REFERENCES user_accounts(id);
`

// migration3 makes Application Name unique per Organization regardless of
// case. It fails with a clear message when existing rows already collide.
const migration3 = `
DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM applications GROUP BY organization_id, lower(name) HAVING count(*) > 1) THEN
    RAISE EXCEPTION 'applications contain names that differ only by case within one organization; rename them before upgrading';
  END IF;
END $$;
CREATE UNIQUE INDEX IF NOT EXISTS applications_organization_lower_name_key ON applications(organization_id, lower(name));
`

// migration4 adds immutable-per-run workload history. Earlier installations
// never recorded that history, and it is not reconstructed: the backfill is
// latest-only. Each current workload_instances row becomes the snapshot of its
// last_deployment_id; every older Deployment keeps no workload rows instead of
// showing state from a later run.
const migration4 = `
CREATE TABLE IF NOT EXISTS deployment_workloads (
  deployment_id uuid NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
  workload_id text NOT NULL,
  status text NOT NULL,
  target_ref jsonb NOT NULL,
  manifest_digest text NOT NULL,
  applied_config_revision_id uuid REFERENCES configuration_revisions(id),
  observed_at timestamptz NOT NULL,
  PRIMARY KEY(deployment_id, workload_id)
);
INSERT INTO deployment_workloads(deployment_id,workload_id,status,target_ref,manifest_digest,applied_config_revision_id,observed_at)
SELECT last_deployment_id,workload_id,status,target_ref,manifest_digest,applied_config_revision_id,observed_at
FROM workload_instances
ON CONFLICT(deployment_id,workload_id) DO NOTHING;
`

type querier interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type txKey struct{}

// Store is a normalized PostgreSQL persistence adapter.
type Store struct{ pool *pgxpool.Pool }

func Open(ctx context.Context, databaseURL string) (*Store, error) {
	if databaseURL == "" {
		return nil, fmt.Errorf("postgres: empty database URL")
	}
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("postgres: parse database URL: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("postgres: open pool: %w", err)
	}
	if err = pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}
	if err = migrate(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{pool: pool}, nil
}

func migrate(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("postgres: begin migration: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(1862274311)`); err != nil {
		return fmt.Errorf("postgres: lock migrations: %w", err)
	}
	if _, err = tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations(version bigint PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return fmt.Errorf("postgres: create migration ledger: %w", err)
	}
	for _, item := range []struct {
		version int
		sql     string
	}{{1, migration}, {2, migration2}, {3, migration3}, {4, migration4}} {
		var applied bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, item.version).Scan(&applied); err != nil {
			return err
		}
		if applied {
			continue
		}
		if _, err = tx.Exec(ctx, item.sql); err != nil {
			return fmt.Errorf("postgres: apply migration %d: %w", item.version, err)
		}
		if _, err = tx.Exec(ctx, `INSERT INTO schema_migrations(version) VALUES($1)`, item.version); err != nil {
			return err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("postgres: commit migrations: %w", err)
	}
	return nil
}

func (s *Store) Close() { s.pool.Close() }

func (s *Store) q(ctx context.Context) querier {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return tx
	}
	return s.pool
}

func (s *Store) Transact(ctx context.Context, fn func(context.Context) error) error {
	if _, nested := ctx.Value(txKey{}).(pgx.Tx); nested {
		return fn(ctx)
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("postgres: begin: %w", err)
	}
	defer tx.Rollback(ctx) // no-op after commit
	if err = fn(context.WithValue(ctx, txKey{}, tx)); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return translate(err)
	}
	return nil
}

// ReadSnapshot runs fn in one REPEATABLE READ READ ONLY transaction, so every
// query sees the same committed snapshot and no GET can persist a change.
// Inside Transact, fn reads the enclosing transaction.
func (s *Store) ReadSnapshot(ctx context.Context, fn func(context.Context, persistence.Store) error) error {
	if _, nested := ctx.Value(txKey{}).(pgx.Tx); nested {
		return fn(ctx, s)
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return fmt.Errorf("postgres: begin read snapshot: %w", err)
	}
	// Nothing may be kept from a read, so the transaction always rolls back.
	defer tx.Rollback(ctx)
	return fn(context.WithValue(ctx, txKey{}, tx), s)
}

func translate(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return persistence.ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return fmt.Errorf("%w: %s", persistence.ErrImmutable, pgErr.ConstraintName)
		case "23503":
			return fmt.Errorf("%w: %s", persistence.ErrNotFound, pgErr.ConstraintName)
		case "40001":
			return persistence.ErrVersionConflict
		}
	}
	return err
}

func jsonBytes(v any) ([]byte, error) { return marshalJSON(v) }

var _ persistence.Store = (*Store)(nil)
