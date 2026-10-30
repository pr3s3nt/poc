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
var versionOneSQL string

//go:embed migrations/version-002.sql
var versionSQL string

//go:embed migrations/002-git-settings.sql
var gitSettingsUpgradeSQL string

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

// recordUpgrade records both transitions for a fresh current installation.
func recordUpgrade(ctx context.Context, tx pgx.Tx, source string) error {
	if source == digest(versionSQL) {
		source = digest(versionOneSQL)
	}
	if err := recordVersionOne(ctx, tx, source); err != nil {
		return err
	}
	return recordVersionTwo(ctx, tx)
}
func recordVersionOne(ctx context.Context, tx pgx.Tx, source string) error {
	_, err := tx.Exec(ctx, `INSERT INTO schema_upgrade_history(version,migration_sha256,source_sha256) VALUES(1,$1,$2)`, migrationChecksum(), source)
	if err != nil {
		return fmt.Errorf("postgres: record upgrade 1 (%s)", sqlState(err))
	}
	return nil
}
func recordVersionTwo(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `INSERT INTO schema_upgrade_history(version,migration_sha256,source_sha256) VALUES(2,$1,$2)`, digest(gitSettingsUpgradeSQL), digest(versionOneSQL))
	if err != nil {
		return fmt.Errorf("postgres: record upgrade 2 (%s)", sqlState(err))
	}
	return nil
}
func verifyUpgradeHistory(ctx context.Context, tx pgx.Tx) error {
	return verifyHistoryThrough(ctx, tx, 2)
}
func verifyHistoryThrough(ctx context.Context, tx pgx.Tx, want int) error {
	rows, err := tx.Query(ctx, `SELECT version,migration_sha256,source_sha256 FROM schema_upgrade_history ORDER BY version`)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "42501" {
			return err
		}
		return resetRequired("upgrade history is missing or unreadable")
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var v int
		var checksum, source string
		if err := rows.Scan(&v, &checksum, &source); err != nil {
			return resetRequired("upgrade history is unreadable")
		}
		count++
		valid := v == count && v <= want
		switch v {
		case 1:
			valid = valid && checksum == migrationChecksum() && (source == digest(versionOneSQL) || supportedSource(source) != "")
		case 2:
			valid = valid && checksum == digest(gitSettingsUpgradeSQL) && source == digest(versionOneSQL)
		default:
			valid = false
		}
		if !valid {
			return resetRequired("upgrade history does not match this release")
		}
	}
	if rows.Err() != nil || count != want {
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
	versionOne := identity == "orchestrator-schema-v1" && sum == digest(versionOneSQL)
	if versionOne {
		source = versionOneSQL
	}
	if (!versionOne && (identity != "orchestrator-fresh-baseline-v02" || source == "")) || name != schema {
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
	if versionOne {
		if err = verifyHistoryThrough(ctx, tx, 1); err != nil {
			return false, err
		}
	} else {
		if sum == digest(sourceOldSQL) {
			if _, err = tx.Exec(ctx, productUpgradeSQL); err != nil {
				return false, fmt.Errorf("postgres: apply upgrade 1 (%s)", sqlState(err))
			}
		}
		if _, err = tx.Exec(ctx, historyUpgradeSQL); err != nil {
			return false, fmt.Errorf("postgres: create upgrade history (%s)", sqlState(err))
		}
		if err = compareSchemaSQL(ctx, tx, schema, versionOneSQL); err != nil {
			return false, err
		}
		if err = recordVersionOne(ctx, tx, sum); err != nil {
			return false, err
		}
	}
	if _, err = tx.Exec(ctx, gitSettingsUpgradeSQL); err != nil {
		return false, fmt.Errorf("postgres: apply upgrade 2 (%s)", sqlState(err))
	}
	if err = compareSchemaSQL(ctx, tx, schema, versionSQL); err != nil {
		return false, err
	}
	if err = recordVersionTwo(ctx, tx); err != nil {
		return false, err
	}
	// Only the new table grants change. Existing grants are verified by Setup.
	if _, err = tx.Exec(ctx, `REVOKE ALL ON platform_git_settings FROM PUBLIC`); err != nil {
		return false, err
	}
	if _, err = tx.Exec(ctx, `GRANT SELECT,INSERT,UPDATE,DELETE ON platform_git_settings TO `+pgx.Identifier{roles.Console}.Sanitize()); err != nil {
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
