package postgres

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"orchestrator/internal/platform/ids"
)

// baselineSQL is the complete current schema for empty installations.
// Retained supported schemas use frozen transitions in upgrade.go.
//
//go:embed baseline.sql
var baselineSQL string

// baselineIdentity is the technical marker stored by a fresh initialization.
// It identifies the frozen current schema version; history is recorded separately.
const baselineIdentity = "orchestrator-schema-v1"

// baselineLockKey serializes initialization and validation across instances.
const baselineLockKey = 1862274311

// markerColumns are the columns of schema_baseline that runtime validation reads.
var markerColumns = []string{"singleton", "baseline", "baseline_sha256", "catalog_sha256", "installation_id", "schema_name", "created_at"}

// ErrResetRequired means the database is not an empty schema and does not hold
// a supported schema. The legacy exported name is retained for callers; errors
// now require preservation/diagnosis, never deletion. No row values are exposed.
var ErrResetRequired = errors.New("postgres: database schema is unsupported or damaged; preserve the database and consult the PostgreSQL upgrade runbook")

// ErrNotInitialized means a runtime login found an empty schema. Runtime
// logins never create the baseline; the owner setup must run first.
var ErrNotInitialized = errors.New("postgres: the database schema is empty; run the owner setup (orchestrator -setup-database) on this fresh database before starting the runtime")

// ErrRuntimeRole means a runtime connection does not use the configured
// least-privilege login: it is the owner, a superuser or otherwise elevated,
// a member of another role, able to create objects, or its privileges differ
// from the Console/State grants. The runtime never falls back to another login.
var ErrRuntimeRole = errors.New("postgres: the connection is not the configured least-privilege runtime login")

// ErrSetupRefused means the owner setup was refused before any change.
var ErrSetupRefused = errors.New("postgres: owner setup refused")

func resetRequired(reason string) error { return fmt.Errorf("%w: %s", ErrResetRequired, reason) }

func runtimeRole(role RuntimeRole, reason string) error {
	return fmt.Errorf("%w (%s): %s", ErrRuntimeRole, role, reason)
}

func setupRefused(reason string) error { return fmt.Errorf("%w: %s", ErrSetupRefused, reason) }

// Roles names the two runtime logins the owner setup grants. Both must exist
// already (created by the database administrator), be distinct ordinary
// LOGIN roles without elevated attributes or memberships, and differ from
// the owner.
type Roles struct {
	Console string
	State   string
}

// Setup is the explicit owner step of option A. Connected as the owner role
// of a fresh database, it creates the baseline in the empty schema, records
// the marker (baseline and catalog digests, installation ID, schema), removes
// PUBLIC access to the database, schema and functions and grants the Console
// and State logins exactly their privilege sets, all in one transaction under
// an advisory lock. On a schema that already holds the baseline it changes
// nothing when current: it verifies the baseline and both logins hold their
// grants (no regrant) and reports initialized=false. Any other schema is
// upgraded transactionally when recognized; unknown/damaged sources are refused.
func Setup(ctx context.Context, ownerURL string, roles Roles) (initialized bool, err error) {
	pool, err := connect(ctx, ownerURL)
	if err != nil {
		return false, err
	}
	defer pool.Close()
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, fmt.Errorf("postgres: begin owner setup: %w", err)
	}
	defer tx.Rollback(ctx) // no-op after commit
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, baselineLockKey); err != nil {
		return false, fmt.Errorf("postgres: lock owner setup: %w", err)
	}
	var schema, owner string
	var ownerSuper, sameSession bool
	if err = tx.QueryRow(ctx, `SELECT COALESCE(current_schema(),''),current_user,r.rolsuper,session_user=current_user FROM pg_roles r WHERE r.rolname=current_user`).Scan(&schema, &owner, &ownerSuper, &sameSession); err != nil {
		return false, fmt.Errorf("postgres: inspect owner: %w", err)
	}
	switch {
	case schema == "":
		return false, setupRefused("no usable schema on the owner's search path")
	case ownerSuper || !sameSession:
		return false, setupRefused("the owner must be an ordinary (non-superuser) role connected as itself")
	case roles.Console == "" || roles.State == "":
		return false, setupRefused("both the Console and the State runtime role are required")
	case roles.Console == roles.State:
		return false, setupRefused("the Console and the State runtime roles must differ")
	case roles.Console == owner || roles.State == owner:
		return false, setupRefused("a runtime role must not be the owner")
	}
	consoleOID, err := candidateRole(ctx, tx, roles.Console, ConsoleRuntime)
	if err != nil {
		return false, err
	}
	stateOID, err := candidateRole(ctx, tx, roles.State, StateRuntime)
	if err != nil {
		return false, err
	}
	objects, err := schemaObjects(ctx, tx, schema)
	if err != nil {
		return false, err
	}
	if objects != 0 {
		upgraded, upgradeErr := upgradeDatabase(ctx, tx, schema, roles)
		if upgradeErr != nil {
			return false, upgradeErr
		}
		if err = verifyBaseline(ctx, tx, schema); err != nil {
			return false, err
		}
		for role, oid := range map[RuntimeRole]uint32{ConsoleRuntime: consoleOID, StateRuntime: stateOID} {
			if err := verifyPrivileges(ctx, tx, schema, oid, role); err != nil {
				return false, setupRefused("existing grants of the " + string(role) + " role differ and are not changed: " + err.Error())
			}
		}
		if err = tx.Commit(ctx); err != nil {
			return false, fmt.Errorf("postgres: commit upgrade (%s)", sqlState(err))
		}
		return upgraded, nil
	}
	if _, err = tx.Exec(ctx, baselineSQL); err != nil {
		return false, fmt.Errorf("postgres: create baseline: %w", err)
	}
	catalog, err := catalogDump(ctx, tx, schema)
	if err != nil {
		return false, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO schema_baseline(baseline,baseline_sha256,catalog_sha256,installation_id,schema_name) VALUES($1,$2,$3,$4::uuid,$5)`,
		baselineIdentity, digest(versionSQL), digest(strings.Join(catalog, "\n")), ids.New(), schema); err != nil {
		return false, fmt.Errorf("postgres: record baseline: %w", err)
	}
	if err = recordUpgrade(ctx, tx, digest(versionSQL)); err != nil {
		return false, err
	}
	database := pgx.Identifier{currentDatabase(ctx, tx)}.Sanitize()
	nsp := pgx.Identifier{schema}.Sanitize()
	console, state := pgx.Identifier{roles.Console}.Sanitize(), pgx.Identifier{roles.State}.Sanitize()
	statements := []string{
		`REVOKE ALL ON DATABASE ` + database + ` FROM PUBLIC`,
		`REVOKE ALL ON SCHEMA ` + nsp + ` FROM PUBLIC`,
		`REVOKE ALL ON ALL TABLES IN SCHEMA ` + nsp + ` FROM PUBLIC`,
		`REVOKE ALL ON ALL FUNCTIONS IN SCHEMA ` + nsp + ` FROM PUBLIC`,
		`GRANT CONNECT ON DATABASE ` + database + ` TO ` + console + `,` + state,
		`GRANT USAGE ON SCHEMA ` + nsp + ` TO ` + console + `,` + state,
	}
	statements = append(statements, grantStatements(ConsoleRuntime, roles.Console)...)
	statements = append(statements, grantStatements(StateRuntime, roles.State)...)
	for _, statement := range statements {
		if _, err = tx.Exec(ctx, statement); err != nil {
			return false, fmt.Errorf("postgres: grant runtime privileges (%s)", sqlState(err))
		}
	}
	// A REVOKE the owner may not perform only warns; verifying the result
	// inside the transaction makes such a database fail before commit.
	for role, oid := range map[RuntimeRole]uint32{ConsoleRuntime: consoleOID, StateRuntime: stateOID} {
		if err := verifyPrivileges(ctx, tx, schema, oid, role); err != nil {
			return false, setupRefused("the owner cannot establish the " + string(role) + " grants (the owner must own the fresh database and schema): " + err.Error())
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("postgres: commit baseline: %w", err)
	}
	return true, nil
}

func currentDatabase(ctx context.Context, tx pgx.Tx) string {
	var name string
	_ = tx.QueryRow(ctx, `SELECT current_database()`).Scan(&name)
	return name
}

func digest(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// roleFacts are the catalog attributes that decide whether a role is an
// ordinary least-privilege login.
type roleFacts struct {
	oid                                                 uint32
	super, createRole, createDB, replication, bypassRLS bool
	login, databaseOwner, ownsObjects                   bool
	memberships                                         int64
}

func roleFactsOf(ctx context.Context, tx pgx.Tx, name string) (roleFacts, bool, error) {
	var f roleFacts
	err := tx.QueryRow(ctx, `SELECT r.oid,r.rolsuper,r.rolcreaterole,r.rolcreatedb,r.rolreplication,r.rolbypassrls,r.rolcanlogin,
		d.datdba=r.oid,
		EXISTS(SELECT 1 FROM pg_class WHERE relowner=r.oid) OR EXISTS(SELECT 1 FROM pg_proc WHERE proowner=r.oid)
		  OR EXISTS(SELECT 1 FROM pg_namespace WHERE nspowner=r.oid) OR EXISTS(SELECT 1 FROM pg_type WHERE typowner=r.oid),
		(SELECT count(*) FROM pg_auth_members m WHERE m.member=r.oid)
		FROM pg_roles r JOIN pg_database d ON d.datname=current_database() WHERE r.rolname=$1`, name).
		Scan(&f.oid, &f.super, &f.createRole, &f.createDB, &f.replication, &f.bypassRLS, &f.login, &f.databaseOwner, &f.ownsObjects, &f.memberships)
	if errors.Is(err, pgx.ErrNoRows) {
		return f, false, nil
	}
	if err != nil {
		return f, false, fmt.Errorf("postgres: inspect role (%s)", sqlState(err))
	}
	return f, true, nil
}

// leastPrivilegeProblem returns why a role is not an ordinary runtime login.
func (f roleFacts) leastPrivilegeProblem() string {
	switch {
	case f.super || f.createRole || f.createDB || f.replication || f.bypassRLS:
		return "the role is a superuser or has CREATEROLE/CREATEDB/REPLICATION/BYPASSRLS"
	case !f.login:
		return "the role cannot log in"
	case f.databaseOwner || f.ownsObjects:
		return "the role owns the database or database objects (an owner is never a runtime login)"
	case f.memberships != 0:
		return "the role is a member of another role"
	}
	return ""
}

func candidateRole(ctx context.Context, tx pgx.Tx, name string, role RuntimeRole) (uint32, error) {
	f, ok, err := roleFactsOf(ctx, tx, name)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, setupRefused("the " + string(role) + " runtime role does not exist; the database administrator creates it first")
	}
	if problem := f.leastPrivilegeProblem(); problem != "" {
		return 0, setupRefused("the " + string(role) + " runtime role is not a least-privilege login: " + problem)
	}
	return f.oid, nil
}

func schemaObjects(ctx context.Context, tx pgx.Tx, schema string) (int64, error) {
	var objects int64
	if err := tx.QueryRow(ctx, `SELECT
	  (SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=$1)
	+ (SELECT count(*) FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname=$1)
	+ (SELECT count(*) FROM pg_type t JOIN pg_namespace n ON n.oid=t.typnamespace WHERE n.nspname=$1 AND t.typtype IN ('e','d','r','m'))`, schema).Scan(&objects); err != nil {
		return 0, fmt.Errorf("postgres: inspect schema: %w", err)
	}
	return objects, nil
}

// Identity is the installation a validated runtime connection serves. The
// Console and State connections of one process must report the same value.
type Identity struct {
	SystemIdentifier string
	DatabaseOID      uint32
	Schema           string
	Installation     string
}

// openRuntime connects one runtime login and validates it read-only.
func openRuntime(ctx context.Context, databaseURL string, role RuntimeRole) (db, Identity, error) {
	pool, err := connect(ctx, databaseURL)
	if err != nil {
		return db{}, Identity{}, err
	}
	identity, err := validateRuntime(ctx, pool, role)
	if err != nil {
		pool.Close()
		return db{}, Identity{}, err
	}
	return db{pool: pool}, identity, nil
}

// validateRuntime checks, in one read-only transaction and without any DDL,
// that the login is an ordinary role that owns nothing, may create nothing
// and is no member of another role; that the schema holds the baseline the
// owner setup recorded (identity, baseline and catalog digests); and that
// the login holds exactly the privileges of role.
func validateRuntime(ctx context.Context, pool *pgxpool.Pool, role RuntimeRole) (Identity, error) {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return Identity{}, fmt.Errorf("postgres: begin runtime validation: %w", err)
	}
	defer tx.Rollback(ctx) // read-only; nothing to keep
	var name string
	var sameSession bool
	if err = tx.QueryRow(ctx, `SELECT current_user,session_user=current_user`).Scan(&name, &sameSession); err != nil {
		return Identity{}, fmt.Errorf("postgres: inspect runtime login: %w", err)
	}
	facts, ok, err := roleFactsOf(ctx, tx, name)
	if err != nil {
		return Identity{}, err
	}
	if !ok || !sameSession {
		return Identity{}, runtimeRole(role, "the session runs as another role")
	}
	if problem := facts.leastPrivilegeProblem(); problem != "" {
		return Identity{}, runtimeRole(role, problem)
	}
	var schema string
	if err = tx.QueryRow(ctx, `SELECT COALESCE(current_schema(),'')`).Scan(&schema); err != nil {
		return Identity{}, fmt.Errorf("postgres: inspect schema: %w", err)
	}
	if schema == "" {
		return Identity{}, runtimeRole(role, "no usable schema on the search path")
	}
	objects, err := schemaObjects(ctx, tx, schema)
	if err != nil {
		return Identity{}, err
	}
	if objects == 0 {
		return Identity{}, ErrNotInitialized
	}
	marker, err := verifyMarkerRuntime(ctx, tx, schema, role)
	if err != nil {
		return Identity{}, err
	}
	catalog, err := catalogDump(ctx, tx, schema)
	if err != nil {
		return Identity{}, err
	}
	if digest(strings.Join(catalog, "\n")) != marker.catalog {
		return Identity{}, resetRequired("schema differs from the baseline recorded by the owner setup")
	}
	if err = verifyPrivileges(ctx, tx, schema, facts.oid, role); err != nil {
		return Identity{}, runtimeRole(role, err.Error())
	}
	identity := Identity{Schema: schema, Installation: marker.installation}
	if err = tx.QueryRow(ctx, `SELECT (SELECT system_identifier::text FROM pg_control_system()),(SELECT oid FROM pg_database WHERE datname=current_database())`).Scan(&identity.SystemIdentifier, &identity.DatabaseOID); err != nil {
		return Identity{}, fmt.Errorf("postgres: inspect installation: %w", err)
	}
	return identity, nil
}

type baselineMarker struct{ catalog, installation string }

// verifyMarkerRuntime checks the marker of an existing schema as a runtime
// login: a ledger or a missing/old marker requires a reset; a marker the
// login may not read is a grants problem.
func verifyMarkerRuntime(ctx context.Context, tx pgx.Tx, schema string, role RuntimeRole) (baselineMarker, error) {
	if exists, err := relationExists(ctx, tx, schema, "schema_migrations"); err != nil {
		return baselineMarker{}, err
	} else if exists {
		return baselineMarker{}, resetRequired("migration ledger present")
	}
	if exists, err := relationExists(ctx, tx, schema, "schema_baseline"); err != nil {
		return baselineMarker{}, err
	} else if !exists {
		return baselineMarker{}, resetRequired("existing schema has no baseline marker")
	}
	var columns int64
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM pg_attribute a JOIN pg_class c ON c.oid=a.attrelid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=$1 AND c.relname='schema_baseline' AND a.attnum>0 AND NOT a.attisdropped AND a.attname=ANY($2)`, schema, markerColumns).Scan(&columns); err != nil {
		return baselineMarker{}, fmt.Errorf("postgres: inspect schema: %w", err)
	}
	if columns != int64(len(markerColumns)) {
		return baselineMarker{}, resetRequired("baseline marker does not match")
	}
	if err := useSchema(ctx, tx, schema); err != nil {
		return baselineMarker{}, err
	}
	m, err := verifyMarker(ctx, tx, schema)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "42501" {
		return baselineMarker{}, runtimeRole(role, "the baseline marker is not readable")
	}
	return m, err
}

// verifyMarker reads the single marker row and checks the baseline identity,
// the digest of the embedded baseline and the recorded schema.
func verifyMarker(ctx context.Context, tx pgx.Tx, schema string) (baselineMarker, error) {
	var rows int64
	var identity, baselineSum, schemaName string
	var m baselineMarker
	err := tx.QueryRow(ctx, `SELECT count(*),COALESCE(min(baseline),''),COALESCE(min(baseline_sha256),''),COALESCE(min(catalog_sha256),''),COALESCE(min(installation_id::text),''),COALESCE(min(schema_name),'') FROM schema_baseline`).
		Scan(&rows, &identity, &baselineSum, &m.catalog, &m.installation, &schemaName)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "42501" {
			return m, err
		}
		return m, fmt.Errorf("postgres: read baseline marker: %w", err)
	}
	if rows == 1 && identity == "orchestrator-fresh-baseline-v02" && supportedSource(baselineSum) != "" {
		return m, ErrUpgradeRequired
	}
	if rows != 1 || identity != baselineIdentity || baselineSum != digest(versionSQL) || schemaName != schema {
		return m, resetRequired("baseline marker does not match")
	}
	if err := verifyUpgradeHistory(ctx, tx); err != nil {
		return m, err
	}
	return m, nil
}

// verifyBaseline checks the existing schema against the baseline. It reads the
// catalog of the schema and of a reference copy of the baseline that is built
// in a private schema of this transaction, which is always rolled back by the
// caller. The existing schema is never altered. Only the owner setup runs it.
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
	if err = compareWithReference(ctx, tx, schema, reference, actual); err != nil {
		return err
	}
	// The reference copy must not count as an object outside the baseline
	// schema in the grant verification that follows in this transaction.
	if _, err = tx.Exec(ctx, `DROP SCHEMA `+pgx.Identifier{reference}.Sanitize()+` CASCADE`); err != nil {
		return fmt.Errorf("postgres: verify baseline: cannot drop the reference schema (%s)", sqlState(err))
	}
	return useSchema(ctx, tx, schema)
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
	m, err := verifyMarker(ctx, tx, schema)
	if err != nil {
		return err
	}
	if m.catalog != digest(strings.Join(actual, "\n")) {
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
