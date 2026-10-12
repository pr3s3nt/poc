package postgres

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// baselineSQL is the complete current schema (T28). It is applied only to an
// empty schema; no earlier layout is upgraded or repaired.
//
//go:embed baseline.sql
var baselineSQL string

// baselineIdentity is the technical marker stored by a fresh initialization.
// It identifies the baseline; it is not a migration ledger and drives no upgrade.
const baselineIdentity = "orchestrator-fresh-baseline-t28"

// baselineLockKey serializes initialization and validation across instances.
const baselineLockKey = 1862274311

// ErrResetRequired means the database is not an empty schema and does not hold
// the exact current baseline. Nothing is converted or repaired; the development
// database must be reset. The text carries no row values, SQL or driver detail.
var ErrResetRequired = errors.New("postgres: the database does not hold the current development baseline; reset the development database (schema conversion is unsupported)")

func resetRequired(reason string) error { return fmt.Errorf("%w: %s", ErrResetRequired, reason) }

// initialize creates the baseline in an empty schema, atomically under an
// advisory transaction lock, or validates that the existing schema is exactly
// the current baseline. It never issues DDL against a nonempty schema.
func initialize(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("postgres: begin baseline initialization: %w", err)
	}
	defer tx.Rollback(ctx) // no-op after commit
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, baselineLockKey); err != nil {
		return fmt.Errorf("postgres: lock baseline initialization: %w", err)
	}
	var schema string
	if err = tx.QueryRow(ctx, `SELECT COALESCE(current_schema(),'')`).Scan(&schema); err != nil || schema == "" {
		return errors.New("postgres: no usable schema on the search path")
	}
	var objects int64
	if err = tx.QueryRow(ctx, `SELECT
	  (SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=$1)
	+ (SELECT count(*) FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname=$1)
	+ (SELECT count(*) FROM pg_type t JOIN pg_namespace n ON n.oid=t.typnamespace WHERE n.nspname=$1 AND t.typtype IN ('e','d','r','m'))`, schema).Scan(&objects); err != nil {
		return fmt.Errorf("postgres: inspect schema: %w", err)
	}
	if objects == 0 {
		if _, err = tx.Exec(ctx, baselineSQL); err != nil {
			return fmt.Errorf("postgres: create baseline: %w", err)
		}
		if _, err = tx.Exec(ctx, `INSERT INTO schema_baseline(baseline) VALUES($1)`, baselineIdentity); err != nil {
			return fmt.Errorf("postgres: record baseline: %w", err)
		}
		if err = tx.Commit(ctx); err != nil {
			return fmt.Errorf("postgres: commit baseline: %w", err)
		}
		return nil
	}
	return verifyBaseline(ctx, tx, schema)
}

// verifyBaseline checks the existing schema against the baseline. It reads the
// catalog of the schema and of a reference copy of the baseline that is built
// in a private schema of this transaction, which is always rolled back by the
// caller. The existing schema is never altered.
func verifyBaseline(ctx context.Context, tx pgx.Tx, schema string) error {
	if exists, err := relationExists(ctx, tx, schema, "schema_migrations"); err != nil {
		return err
	} else if exists {
		return resetRequired("migration ledger present")
	}
	if exists, err := relationExists(ctx, tx, schema, "schema_baseline"); err != nil {
		return err
	} else if !exists {
		return resetRequired("existing schema has no baseline marker")
	}
	actual, err := catalogDump(ctx, tx, schema)
	if err != nil {
		return err
	}
	var suffix [8]byte
	if _, err = rand.Read(suffix[:]); err != nil {
		return fmt.Errorf("postgres: reference schema name: %w", err)
	}
	reference := "orch_baseline_ref_" + hex.EncodeToString(suffix[:])
	if _, err = tx.Exec(ctx, `CREATE SCHEMA `+pgx.Identifier{reference}.Sanitize()); err != nil {
		return fmt.Errorf("postgres: verify baseline: cannot build the reference schema (%s)", sqlState(err))
	}
	return compareWithReference(ctx, tx, schema, reference, actual)
}

func compareWithReference(ctx context.Context, tx pgx.Tx, schema, reference string, actual []string) error {
	if err := useSchema(ctx, tx, reference); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, baselineSQL); err != nil {
		return fmt.Errorf("postgres: verify baseline: cannot build the reference schema (%s)", sqlState(err))
	}
	expected, err := catalogDump(ctx, tx, reference)
	if err != nil {
		return err
	}
	if diff := firstDifference(expected, actual); diff != "" {
		return resetRequired("schema differs at " + diff)
	}
	if err = useSchema(ctx, tx, schema); err != nil {
		return err
	}
	var rows int64
	var identity string
	if err = tx.QueryRow(ctx, `SELECT count(*), COALESCE(min(baseline),'') FROM schema_baseline`).Scan(&rows, &identity); err != nil {
		return fmt.Errorf("postgres: read baseline marker: %w", err)
	}
	if rows != 1 || identity != baselineIdentity {
		return resetRequired("baseline marker does not match")
	}
	return nil
}

func relationExists(ctx context.Context, tx pgx.Tx, schema, name string) (bool, error) {
	var exists bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=$1 AND c.relname=$2 AND c.relkind IN ('r','p','v','m','f'))`, schema, name).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("postgres: inspect schema: %w", err)
	}
	return exists, nil
}

// useSchema makes schema the only search path entry for the rest of the
// transaction, so catalog deparsing and unqualified DDL address that schema.
func useSchema(ctx context.Context, tx pgx.Tx, schema string) error {
	if _, err := tx.Exec(ctx, `SELECT set_config('search_path', $1, true)`, pgx.Identifier{schema}.Sanitize()); err != nil {
		return fmt.Errorf("postgres: select schema: %w", err)
	}
	return nil
}

// catalogQueries describe every structural property that must match: the
// relations, typed/nullable/defaulted columns, primary/unique/foreign/check
// constraints with deferrability, indexes (including partial and expression
// indexes), triggers and trigger functions. Each yields one comparable line.
var catalogQueries = []string{
	`SELECT 'relation '||c.relname||' kind='||c.relkind::text
	   FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
	  WHERE n.nspname=$1 AND c.relkind IN ('r','p','v','m','S','f')`,
	`SELECT 'column '||c.relname||'.'||a.attname||' type='||format_type(a.atttypid,a.atttypmod)||' notnull='||a.attnotnull::text||' default='||COALESCE(pg_get_expr(d.adbin,d.adrelid),'')
	   FROM pg_attribute a JOIN pg_class c ON c.oid=a.attrelid JOIN pg_namespace n ON n.oid=c.relnamespace
	   LEFT JOIN pg_attrdef d ON d.adrelid=a.attrelid AND d.adnum=a.attnum
	  WHERE n.nspname=$1 AND c.relkind IN ('r','p') AND a.attnum>0 AND NOT a.attisdropped`,
	`SELECT 'constraint '||c.relname||'.'||k.conname||' type='||k.contype::text||' '||pg_get_constraintdef(k.oid)||' deferrable='||k.condeferrable::text||' deferred='||k.condeferred::text||' validated='||k.convalidated::text
	   FROM pg_constraint k JOIN pg_class c ON c.oid=k.conrelid JOIN pg_namespace n ON n.oid=c.relnamespace
	  WHERE n.nspname=$1 AND k.contype IN ('p','u','f','c','x')`,
	`SELECT 'index '||t.relname||'.'||i.relname||' '||pg_get_indexdef(x.indexrelid)||' valid='||x.indisvalid::text||' ready='||x.indisready::text
	   FROM pg_index x JOIN pg_class i ON i.oid=x.indexrelid JOIN pg_class t ON t.oid=x.indrelid JOIN pg_namespace n ON n.oid=t.relnamespace
	  WHERE n.nspname=$1`,
	`SELECT 'trigger '||c.relname||'.'||g.tgname||' '||pg_get_triggerdef(g.oid)||' enabled='||g.tgenabled::text
	   FROM pg_trigger g JOIN pg_class c ON c.oid=g.tgrelid JOIN pg_namespace n ON n.oid=c.relnamespace
	  WHERE n.nspname=$1 AND NOT g.tgisinternal`,
	`SELECT 'function '||p.proname||'('||pg_get_function_identity_arguments(p.oid)||') lang='||l.lanname||' secdef='||p.prosecdef::text||' body='||md5(p.prosrc)
	   FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace JOIN pg_language l ON l.oid=p.prolang
	  WHERE n.nspname=$1`,
}

// catalogDump returns the sorted structural description of schema with the
// schema qualifier removed, so two schemas with equal structure compare equal.
func catalogDump(ctx context.Context, tx pgx.Tx, schema string) ([]string, error) {
	if err := useSchema(ctx, tx, schema); err != nil {
		return nil, err
	}
	qualifiers := []string{schema + ".", pgx.Identifier{schema}.Sanitize() + "."}
	var lines []string
	for _, query := range catalogQueries {
		rows, err := tx.Query(ctx, query, schema)
		if err != nil {
			return nil, fmt.Errorf("postgres: read catalog: %w", err)
		}
		for rows.Next() {
			var line string
			if err = rows.Scan(&line); err != nil {
				rows.Close()
				return nil, fmt.Errorf("postgres: read catalog: %w", err)
			}
			for _, q := range qualifiers {
				line = strings.ReplaceAll(line, q, "")
			}
			lines = append(lines, line)
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return nil, fmt.Errorf("postgres: read catalog: %w", err)
		}
	}
	sort.Strings(lines)
	return lines, nil
}

// firstDifference names the first known baseline object (kind and name only)
// that is missing or different. An object that exists only in the database may
// carry arbitrary names, so it is reported generically. It never returns
// definitions or row values.
func firstDifference(expected, actual []string) string {
	have := map[string]bool{}
	for _, line := range actual {
		have[line] = true
	}
	want := map[string]bool{}
	for _, line := range expected {
		want[line] = true
	}
	for _, line := range expected {
		if !have[line] {
			return objectName(line)
		}
	}
	for _, line := range actual {
		if !want[line] {
			return "an unexpected object"
		}
	}
	return ""
}

func objectName(line string) string {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return "unknown object"
	}
	name := fields[1]
	if i := strings.IndexByte(name, '('); i >= 0 {
		name = name[:i]
	}
	return fields[0] + " " + name
}

func sqlState(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return "SQLSTATE " + pgErr.Code
	}
	return "unclassified"
}
