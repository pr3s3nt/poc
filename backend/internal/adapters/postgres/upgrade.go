package postgres

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Frozen references and transitions must never be edited after release.
//
//go:embed migrations/version-001.sql
var versionSQL string

//go:embed migrations/source-1516dec.sql
var sourceOldSQL string

//go:embed migrations/source-2a111db.sql
var sourceCurrentSQL string

//go:embed migrations/001-product.sql
var productUpgradeSQL string

//go:embed migrations/001-history.sql
var historyUpgradeSQL string

var ErrUpgradeRequired = errors.New("postgres: database upgrade required; stop old writers, take a verified backup and run owner setup (-setup-database) before starting this version")

func supportedSource(sum string) string {
	for _, sql := range []string{sourceOldSQL, sourceCurrentSQL} {
		if digest(sql) == sum {
			return sql
		}
	}
	return ""
}
func migrationChecksum() string { return digest(productUpgradeSQL + "\n" + historyUpgradeSQL) }

func recordUpgrade(ctx context.Context, tx pgx.Tx, source string) error {
	_, err := tx.Exec(ctx, `INSERT INTO schema_upgrade_history(version,migration_sha256,source_sha256) VALUES(1,$1,$2)`, migrationChecksum(), source)
	if err != nil {
		return fmt.Errorf("postgres: record upgrade (%s)", sqlState(err))
	}
	return nil
}
func verifyUpgradeHistory(ctx context.Context, tx pgx.Tx) error {
	var count int
	var version int64
	var checksum, source string
	err := tx.QueryRow(ctx, `SELECT count(*),COALESCE(max(version),0),COALESCE(min(migration_sha256),''),COALESCE(min(source_sha256),'') FROM schema_upgrade_history`).Scan(&count, &version, &checksum, &source)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "42501" {
			return err // verifyMarkerRuntime classifies missing runtime grants.
		}
		return resetRequired("upgrade history is missing or unreadable")
	}
	if count != 1 || version != 1 || checksum != migrationChecksum() || (source != digest(versionSQL) && supportedSource(source) == "") {
		return resetRequired("upgrade history does not match this release")
	}
	return nil
}

// compareSchemaSQL builds a private reference in the same transaction; no data
// from the user's schema is copied. Only structural metadata is compared.
func compareSchemaSQL(ctx context.Context, tx pgx.Tx, schema, sql string) error {
	actual, err := catalogDump(ctx, tx, schema)
	if err != nil {
		return err
	}
	const ref = "orch_upgrade_reference"
	// CREATE refuses any preexisting object: never drop an object we did not create.
	if _, err = tx.Exec(ctx, `CREATE SCHEMA `+ref); err != nil {
		return fmt.Errorf("postgres: create upgrade reference (%s)", sqlState(err))
	}
	if err = useSchema(ctx, tx, ref); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, sql); err != nil {
		return fmt.Errorf("postgres: build upgrade reference (%s)", sqlState(err))
	}
	expected, err := catalogDump(ctx, tx, ref)
	if err != nil {
		return err
	}
	if diff := firstDifference(expected, actual); diff != "" {
		return resetRequired("schema differs at " + diff)
	}
	if _, err = tx.Exec(ctx, `DROP SCHEMA `+ref+` CASCADE`); err != nil {
		return fmt.Errorf("postgres: remove upgrade reference (%s)", sqlState(err))
	}
	return useSchema(ctx, tx, schema)
}

// upgradeDatabase runs only inside Setup's owner transaction and advisory lock.
func upgradeDatabase(ctx context.Context, tx pgx.Tx, schema string, roles Roles) (bool, error) {
	exists, err := relationExists(ctx, tx, schema, "schema_baseline")
	if err != nil {
		return false, err
	}
	if !exists {
		return false, resetRequired("existing schema has no baseline marker")
	}
	if err = useSchema(ctx, tx, schema); err != nil {
		return false, err
	}
	var identity, sum, catalog, name string
	// A malformed marker is rejected without exposing driver details or row values.
	err = tx.QueryRow(ctx, `SELECT baseline,baseline_sha256,catalog_sha256,schema_name FROM schema_baseline WHERE singleton`).Scan(&identity, &sum, &catalog, &name)
	if err != nil {
		return false, resetRequired("baseline marker is malformed")
	}
	if identity == baselineIdentity {
		return false, nil
	}
	source := supportedSource(sum)
	if identity != "orchestrator-fresh-baseline-v02" || source == "" || name != schema {
		return false, resetRequired("unsupported source version; a reviewed migration is required")
	}
	var count int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM schema_baseline`).Scan(&count); err != nil || count != 1 {
		return false, resetRequired("baseline marker is malformed")
	}
	actual, err := catalogDump(ctx, tx, schema)
	if err != nil {
		return false, err
	}
	if digest(strings.Join(actual, "\n")) != catalog {
		return false, resetRequired("source catalog differs from its recorded identity")
	}
	if err = compareSchemaSQL(ctx, tx, schema, source); err != nil {
		return false, err
	}
	if sum == digest(sourceOldSQL) {
		if _, err = tx.Exec(ctx, productUpgradeSQL); err != nil {
			return false, fmt.Errorf("postgres: apply upgrade 1 (%s); transaction will roll back", sqlState(err))
		}
	}
	if _, err = tx.Exec(ctx, historyUpgradeSQL); err != nil {
		return false, fmt.Errorf("postgres: create upgrade history (%s)", sqlState(err))
	}
	if err = compareSchemaSQL(ctx, tx, schema, versionSQL); err != nil {
		return false, err
	}
	if err = recordUpgrade(ctx, tx, sum); err != nil {
		return false, err
	}
	actual, err = catalogDump(ctx, tx, schema)
	if err != nil {
		return false, err
	}
	if _, err = tx.Exec(ctx, `UPDATE schema_baseline SET baseline=$1,baseline_sha256=$2,catalog_sha256=$3`, baselineIdentity, digest(versionSQL), digest(strings.Join(actual, "\n"))); err != nil {
		return false, fmt.Errorf("postgres: update version marker (%s)", sqlState(err))
	}
	for _, role := range []string{roles.Console, roles.State} {
		if _, err = tx.Exec(ctx, `GRANT SELECT ON schema_upgrade_history TO `+pgx.Identifier{role}.Sanitize()); err != nil {
			return false, fmt.Errorf("postgres: grant upgrade history (%s)", sqlState(err))
		}
	}
	return true, nil
}
