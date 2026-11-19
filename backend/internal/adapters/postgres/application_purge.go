package postgres

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed application_purge.sql
var applicationPurgeSQL string

// ApplicationPurge identifies one Application in one installation. An empty
// ConfirmationName selects a read-only preview.
type ApplicationPurge struct {
	Organization     string
	Application      string
	ConfirmationName string
	WritersStopped   bool
}

type ApplicationPurgeReport struct {
	InstallationID string                  `json:"installationId"`
	ApplicationID  string                  `json:"applicationId"`
	Organization   string                  `json:"organization"`
	Application    string                  `json:"application"`
	DisplayName    string                  `json:"displayName"`
	Status         string                  `json:"status"`
	Purged         bool                    `json:"purged"`
	Environments   []ApplicationPurgeEnv   `json:"environments"`
	Resources      []string                `json:"resources"`
	Fleet          []ApplicationPurgeFleet `json:"fleet"`
	GitRepos       []string                `json:"gitRepositories"`
}

type ApplicationPurgeEnv struct {
	Key       string `json:"key"`
	Namespace string `json:"namespace"`
}

type ApplicationPurgeFleet struct {
	GitRepo   string `json:"gitRepo"`
	Release   string `json:"release"`
	Namespace string `json:"namespace"`
}

// PurgeApplication runs as the operator's database owner and never wires the
// application runtime or the external cleanup adapters.
func PurgeApplication(ctx context.Context, ownerURL string, in ApplicationPurge) (ApplicationPurgeReport, error) {
	var result ApplicationPurgeReport
	execute := in.ConfirmationName != ""
	if in.Organization == "" || in.Application == "" || (execute && !in.WritersStopped) {
		return result, errors.New("postgres: purge requires Organization and Application keys, and execution requires confirmation that writers stopped")
	}
	pool, err := pgxpool.New(ctx, ownerURL)
	if err != nil {
		return result, errors.New("postgres: could not open database owner connection")
	}
	defer pool.Close()

	options := pgx.TxOptions{IsoLevel: pgx.RepeatableRead}
	if !execute {
		options.AccessMode = pgx.ReadOnly
	}
	tx, err := pool.BeginTx(ctx, options)
	if err != nil {
		return result, errors.New("postgres: could not start Application purge transaction")
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	if _, err := tx.Exec(ctx, `SET LOCAL lock_timeout = '5s'`); err != nil {
		return result, purgeError("configure transaction", err)
	}
	if execute {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, baselineLockKey); err != nil {
			return result, purgeError("lock installation", err)
		}
	}

	var schema string
	var owner bool
	err = tx.QueryRow(ctx, `
SELECT n.nspname, session_user=current_user AND NOT r.rolsuper AND (
 n.nspowner=r.oid OR (n.nspowner='pg_database_owner'::regrole AND EXISTS(
  SELECT 1 FROM pg_database d WHERE d.datname=current_database() AND d.datdba=r.oid))
)
FROM pg_namespace n JOIN pg_roles r ON r.rolname=current_user WHERE n.nspname=current_schema()`).Scan(&schema, &owner)
	if err != nil || !owner {
		return result, errors.New("postgres: Application purge requires the ordinary schema owner connected as itself")
	}
	if err := useSchema(ctx, tx, schema); err != nil {
		return result, purgeError("select installation schema", err)
	}
	marker, err := verifyMarker(ctx, tx, schema)
	if err != nil {
		return result, errors.New("postgres: Application purge requires a database on the release's supported schema")
	}
	catalog, err := catalogDump(ctx, tx, schema)
	if err != nil {
		return result, purgeError("inspect installation catalog", err)
	}
	if digest(strings.Join(catalog, "\n")) != marker.catalog {
		return result, resetRequired("Application purge catalog does not match")
	}

	result.InstallationID = marker.installation
	result.Organization, result.Application = in.Organization, in.Application
	var organizationID string
	lookup := `SELECT a.id::text,o.id::text,a.name,a.status FROM applications a
JOIN organizations o ON o.id=a.organization_id WHERE o.organization_key=$1 AND a.application_key=$2`
	if execute {
		lookup += ` FOR UPDATE OF a`
	}
	err = tx.QueryRow(ctx, lookup, in.Organization, in.Application).Scan(
		&result.ApplicationID, &organizationID, &result.DisplayName, &result.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, errors.New("postgres: Application key was not found in the specified Organization")
	}
	if err != nil {
		return result, purgeError("lock or read Application", err)
	}
	if execute && result.DisplayName != in.ConfirmationName {
		return result, errors.New("postgres: Application confirmation name does not match")
	}
	if err := readApplicationPurgeReport(ctx, tx, &result); err != nil {
		return result, err
	}
	if !execute {
		return result, nil
	}

	// A stopped installation should have released its writer connections.
	// File-only/debug executions and external processes still require the
	// operator to confirm their stop before this check.
	var writers bool
	err = tx.QueryRow(ctx, `SELECT EXISTS (
 SELECT 1 FROM pg_stat_activity a
 WHERE a.datname=current_database() AND a.pid<>pg_backend_pid()
 AND (has_any_column_privilege(a.usesysid,$1,'INSERT,UPDATE') OR has_any_column_privilege(a.usesysid,$2,'INSERT,UPDATE')))`,
		pgx.Identifier{schema, "applications"}.Sanitize(), pgx.Identifier{schema, "terraform_states"}.Sanitize()).Scan(&writers)
	if err != nil {
		return result, purgeError("check writer connections", err)
	}
	if writers {
		return result, errors.New("postgres: the installation still has database writer connections; stop the writers and retry")
	}
	if _, err := tx.Exec(ctx, `SELECT id FROM environments WHERE application_id=$1::uuid ORDER BY id FOR UPDATE`, result.ApplicationID); err != nil {
		return result, purgeError("lock Application environments", err)
	}
	if _, err := tx.Exec(ctx, `CREATE TEMP TABLE app_purge_target (
 application_id uuid PRIMARY KEY, organization_id uuid NOT NULL, application_key text NOT NULL
) ON COMMIT DROP`); err != nil {
		return result, purgeError("stage exact Application identity", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO pg_temp.app_purge_target VALUES ($1::uuid,$2::uuid,$3)`,
		result.ApplicationID, organizationID, in.Application); err != nil {
		return result, purgeError("stage exact Application identity", err)
	}
	if _, err := tx.Exec(ctx, applicationPurgeSQL); err != nil {
		return result, purgeError("remove Application metadata", err)
	}
	catalog, err = catalogDump(ctx, tx, schema)
	if err != nil {
		return result, purgeError("verify installation catalog", err)
	}
	if digest(strings.Join(catalog, "\n")) != marker.catalog {
		return result, errors.New("postgres: Application purge did not restore the installation's schema guards")
	}
	if err := tx.Commit(ctx); err != nil {
		return result, purgeError("commit Application purge", err)
	}
	result.Purged = true
	return result, nil
}

func readApplicationPurgeReport(ctx context.Context, tx pgx.Tx, report *ApplicationPurgeReport) error {
	var environments, fleet []byte
	err := tx.QueryRow(ctx, `
WITH env AS (SELECT id,environment_key,namespace_identity FROM environments WHERE application_id=$1::uuid),
 fleet AS (
   SELECT DISTINCT i.receipt->'Handle'->>'gitrepo' AS "gitRepo", i.receipt->'Handle'->>'release' AS release,
          i.receipt->'Handle'->>'namespace' AS namespace
   FROM delivery_intents i JOIN delivery_units u ON u.id=i.unit_id JOIN env e ON e.id=u.environment_id
   WHERE i.receipt->'Handle'->>'gitrepo' ~ '^[a-z0-9][a-z0-9.-]{0,252}$'
     AND i.receipt->'Handle'->>'release' ~ '^[a-z0-9][a-z0-9.-]{0,252}$'
     AND i.receipt->'Handle'->>'namespace' ~ '^[a-z0-9][a-z0-9-]{0,62}$'
 )
SELECT COALESCE((SELECT jsonb_agg(jsonb_build_object('key',environment_key,'namespace',namespace_identity) ORDER BY environment_key) FROM env),'[]'),
       ARRAY(SELECT DISTINCT descriptor FROM terraform_resource_instances WHERE application_id=$1::uuid ORDER BY descriptor),
       COALESCE((SELECT jsonb_agg(to_jsonb(fleet) ORDER BY "gitRepo",release,namespace) FROM fleet),'[]'),
       ARRAY(SELECT name FROM application_delivery_repositories WHERE application_id=$1::uuid ORDER BY name)`, report.ApplicationID).
		Scan(&environments, &report.Resources, &fleet, &report.GitRepos)
	if err != nil {
		return purgeError("read safe Application inventory", err)
	}
	if json.Unmarshal(environments, &report.Environments) != nil || json.Unmarshal(fleet, &report.Fleet) != nil {
		return errors.New("postgres: could not decode safe Application inventory")
	}
	return nil
}

func purgeError(stage string, err error) error {
	var detail *pgconn.PgError
	if errors.As(err, &detail) && detail.ConstraintName != "" {
		return fmt.Errorf("postgres: %s failed (%s, %s)", stage, detail.Code, detail.ConstraintName)
	}
	return fmt.Errorf("postgres: %s failed (%s)", stage, sqlState(err))
}
