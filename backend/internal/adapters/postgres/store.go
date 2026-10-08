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

// migration5 adds the Connection display name and authentication type
// (UC-04). Legacy rows read their key as name; legacy Kubernetes rows are the
// host-context variant and legacy AWS rows are process-configured
// AWS_ACCESS_KEY metadata. This does not claim seeded AWS rows hold durable
// credentials. The check keeps authentication type compatible with kind.
const migration5 = `
ALTER TABLE connections ADD COLUMN IF NOT EXISTS name text;
ALTER TABLE connections ADD COLUMN IF NOT EXISTS authentication_type text;
UPDATE connections SET name=connection_key WHERE name IS NULL OR name='';
UPDATE connections SET authentication_type=CASE kind WHEN 'KUBERNETES' THEN 'HOST_CONTEXT' ELSE 'AWS_ACCESS_KEY' END WHERE authentication_type IS NULL OR authentication_type='';
ALTER TABLE connections ALTER COLUMN name SET NOT NULL;
ALTER TABLE connections ALTER COLUMN authentication_type SET NOT NULL;
ALTER TABLE connections ADD CONSTRAINT connections_authentication_type_check CHECK (
  (kind='KUBERNETES' AND authentication_type IN ('HOST_CONTEXT','KUBECONFIG')) OR
  (kind='AWS' AND authentication_type='AWS_ACCESS_KEY'));
`

// migration6 moves the execution binding from Application to Environment
// (ADR-011). Order matters: additive columns, nullable Application binding,
// one-time backfill of old bound Applications, then consistency CHECK and the
// immutability trigger. New Applications persist connection_id NULL, so the
// backfill predicate never matches them and a rerun leaves them UNCONFIGURED.
const migration6 = `
ALTER TABLE environments ADD COLUMN IF NOT EXISTS connection_id uuid REFERENCES connections(id);
ALTER TABLE environments ADD COLUMN IF NOT EXISTS execution_profile text NOT NULL DEFAULT '';
ALTER TABLE environments ADD COLUMN IF NOT EXISTS region text NOT NULL DEFAULT '';
ALTER TABLE environments ADD COLUMN IF NOT EXISTS runtime_status text NOT NULL DEFAULT 'UNCONFIGURED';
ALTER TABLE environments ADD COLUMN IF NOT EXISTS infrastructure_scope text NOT NULL DEFAULT 'ENVIRONMENT';
ALTER TABLE applications ALTER COLUMN connection_id DROP NOT NULL;
UPDATE environments e SET connection_id=a.connection_id, execution_profile=a.execution_profile, region=a.region,
  runtime_status=a.runtime_status, infrastructure_scope='LEGACY_APPLICATION'
FROM applications a WHERE a.id=e.application_id AND e.connection_id IS NULL AND a.connection_id IS NOT NULL;
DO $$ BEGIN
  ALTER TABLE environments ADD CONSTRAINT environments_binding_consistent CHECK (
    (connection_id IS NULL AND runtime_status='UNCONFIGURED' AND execution_profile='' AND region='') OR
    (connection_id IS NOT NULL AND execution_profile IN ('aws-eks','internal-k8s') AND runtime_status IN ('PENDING','READY')
      AND (execution_profile<>'aws-eks' OR region<>'')));
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN
  ALTER TABLE environments ADD CONSTRAINT environments_infrastructure_scope_check CHECK (infrastructure_scope IN ('ENVIRONMENT','LEGACY_APPLICATION'));
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
CREATE OR REPLACE FUNCTION environments_binding_immutable() RETURNS trigger AS $fn$
BEGIN
  IF OLD.connection_id IS NOT NULL AND (
       NEW.connection_id IS DISTINCT FROM OLD.connection_id OR NEW.execution_profile IS DISTINCT FROM OLD.execution_profile
    OR NEW.region IS DISTINCT FROM OLD.region OR NEW.infrastructure_scope IS DISTINCT FROM OLD.infrastructure_scope) THEN
    RAISE EXCEPTION 'environment execution binding is immutable' USING ERRCODE='23514';
  END IF;
  RETURN NEW;
END $fn$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS environments_binding_immutable ON environments;
CREATE TRIGGER environments_binding_immutable BEFORE UPDATE ON environments
  FOR EACH ROW EXECUTE FUNCTION environments_binding_immutable();
`

// migration7 implements ADR-012: separate Secret Store Connections, editable
// Environment selections with target generations, persisted operation claims
// and transition records. The permanent binding trigger is dropped; binding
// writes are guarded by the dedicated versioned commands and claim owner.
// No Vault network I/O happens here; legacy store backfill runs at startup.
const migration7 = `
CREATE TABLE IF NOT EXISTS secret_store_connections (
  id uuid PRIMARY KEY, organization_id uuid NOT NULL REFERENCES organizations(id),
  store_key text NOT NULL, name text NOT NULL, provider text NOT NULL CHECK (provider IN ('VAULT_KV_V2')),
  backend_address text NOT NULL, workload_address text NOT NULL, kv_mount text NOT NULL, auth_mount text NOT NULL,
  tls_ca_pem text NOT NULL DEFAULT '', credential_ref text NOT NULL DEFAULT '', status text NOT NULL CHECK (status IN ('READY','REJECTED')),
  legacy boolean NOT NULL DEFAULT false, verification jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(organization_id, store_key)
);
ALTER TABLE environments ADD COLUMN IF NOT EXISTS secret_store_id uuid REFERENCES secret_store_connections(id);
ALTER TABLE environments ADD COLUMN IF NOT EXISTS target_generation bigint NOT NULL DEFAULT 0;
ALTER TABLE environments ADD COLUMN IF NOT EXISTS generation_high bigint NOT NULL DEFAULT 0;
ALTER TABLE environments ADD COLUMN IF NOT EXISTS active_operation_id uuid;
CREATE TABLE IF NOT EXISTS environment_operations (
  id uuid PRIMARY KEY, environment_id uuid NOT NULL REFERENCES environments(id),
  kind text NOT NULL, owner_instance text NOT NULL, fence bigint NOT NULL DEFAULT 1, status text NOT NULL, stage text NOT NULL DEFAULT '',
  pins jsonb NOT NULL, detail jsonb NOT NULL DEFAULT '{}'::jsonb, failure text NOT NULL DEFAULT '',
  started_at timestamptz NOT NULL, updated_at timestamptz NOT NULL, heartbeat_at timestamptz NOT NULL,
  deadline timestamptz NOT NULL, finished_at timestamptz
);
CREATE UNIQUE INDEX IF NOT EXISTS environment_operations_one_holder ON environment_operations(environment_id)
  WHERE status IN ('ACTIVE','INTERRUPTED','RECOVERING');
DO $$ BEGIN
  ALTER TABLE environments ADD CONSTRAINT environments_active_operation_fk
    FOREIGN KEY(active_operation_id) REFERENCES environment_operations(id) DEFERRABLE INITIALLY DEFERRED;
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
CREATE TABLE IF NOT EXISTS environment_transitions (
  id uuid PRIMARY KEY, environment_id uuid NOT NULL REFERENCES environments(id), operation_id uuid NOT NULL,
  status text NOT NULL, stage text NOT NULL, document jsonb NOT NULL,
  created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL
);
CREATE INDEX IF NOT EXISTS environment_transitions_env_idx ON environment_transitions(environment_id, created_at DESC);
DROP TRIGGER IF EXISTS environments_binding_immutable ON environments;
DROP FUNCTION IF EXISTS environments_binding_immutable();
ALTER TABLE configuration_revision_entries ALTER COLUMN value_ref DROP NOT NULL;
ALTER TABLE configuration_revision_entries ADD COLUMN IF NOT EXISTS store_key text NOT NULL DEFAULT '';
ALTER TABLE configuration_revision_entries ADD COLUMN IF NOT EXISTS variable_value text;
ALTER TABLE workload_instances ADD COLUMN IF NOT EXISTS generation bigint NOT NULL DEFAULT 0;
ALTER TABLE workload_instances DROP CONSTRAINT IF EXISTS workload_instances_environment_id_workload_id_key;
CREATE UNIQUE INDEX IF NOT EXISTS workload_instances_env_generation_workload_key ON workload_instances(environment_id, generation, workload_id);
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS connection_key text NOT NULL DEFAULT '';
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS target_generation bigint NOT NULL DEFAULT 0;
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
	}{{1, migration}, {2, migration2}, {3, migration3}, {4, migration4}, {5, migration5}, {6, migration6}, {7, migration7}} {
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
