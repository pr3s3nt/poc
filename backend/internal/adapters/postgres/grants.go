package postgres

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
)

// RuntimeRole names one of the two least-privilege runtime logins of option A
// (ADR-014): the Console login serves product queries and commands, the State
// login serves only the private Terraform state repository. The owner role
// that creates the baseline is never a runtime role.
type RuntimeRole string

// Runtime roles.
const (
	ConsoleRuntime RuntimeRole = "console"
	StateRuntime   RuntimeRole = "state"
)

// grant is one privilege of a runtime role on one table. A nil column list
// grants the privilege on the whole table; otherwise only on those columns.
type grant struct {
	table     string
	privilege string // SELECT, INSERT, UPDATE or DELETE
	columns   []string
}

// productTables are the Console's product tables. The Console reads and
// writes them as its commands and queries need (CRUD); the State login has no
// privilege on them beyond the scope columns listed in stateGrants.
var productTables = []string{
	"organizations", "user_accounts", "sessions", "connections", "applications",
	"secret_store_connections", "environments", "environment_operations",
	"environment_transitions", "deployments", "deployment_sets",
	"deployment_delta_snapshots", "deployment_plans", "resource_types",
	"resource_definitions", "matching_criteria", "active_resources",
	"deployment_resources", "configuration_revisions",
	"configuration_revision_entries", "workload_drafts", "workload_instances",
	"deployment_workloads",
}

// consoleInstanceColumns and consoleAttemptColumns are the only instance and
// attempt columns the Console may read (the safe Deployment progress
// projection). target_ref and physical_intent, the module and target
// snapshots, observed physical IDs, state serial and the recovery handle stay
// private to the State login.
var (
	consoleInstanceColumns = []string{"id", "organization_id", "application_id", "environment_id", "target_generation", "descriptor", "scope_type", "scope_id", "state_key", "active_resource_id", "module_bundle_id", "created_by_run_id", "status", "version", "created_at", "updated_at"}
	consoleAttemptColumns  = []string{"id", "resource_instance_id", "environment_operation_id", "deployment_id", "run_id", "attempt_number", "action", "ownership", "stage", "status", "safe_error_code", "deadline", "created_at", "updated_at"}
)

// consoleGrants is the complete privilege set of the Console login. It has no
// privilege on terraform_states, and on terraform_executor_capabilities only
// the operation column plus DELETE, which is what revoking every capability
// of a released or recovered operation needs: a DELETE can neither filter on
// nor return the token hash without SELECT on it. Lock rows expose only the
// safe lock metadata release/recovery needs, not the lock_metadata document.
// Terraform instance, edge and attempt metadata are read-only; they are
// written by the State login. Verified module bundles are insert-only for the
// Console, which runs registration and platform preparation; the baseline
// trigger keeps them immutable and no runtime login may update or delete them.
func consoleGrants() []grant {
	var out []grant
	out = append(out, grant{table: "schema_baseline", privilege: "SELECT"})
	for _, table := range productTables {
		for _, privilege := range []string{"SELECT", "INSERT", "UPDATE", "DELETE"} {
			out = append(out, grant{table: table, privilege: privilege})
		}
	}
	return append(out,
		grant{table: "terraform_module_bundles", privilege: "SELECT"},
		grant{table: "terraform_module_bundles", privilege: "INSERT"},
		grant{table: "terraform_resource_instances", privilege: "SELECT", columns: consoleInstanceColumns},
		grant{table: "terraform_resource_dependencies", privilege: "SELECT"},
		grant{table: "terraform_resource_attempts", privilege: "SELECT", columns: consoleAttemptColumns},
		grant{table: "terraform_state_locks", privilege: "SELECT", columns: []string{"resource_instance_id", "lock_id", "environment_operation_id", "owner", "fencing_generation", "deadline"}},
		grant{table: "terraform_executor_capabilities", privilege: "SELECT", columns: []string{"environment_operation_id"}},
		grant{table: "terraform_executor_capabilities", privilege: "DELETE"},
	)
}

// stateGrants is the complete privilege set of the State login: the state
// repository's own tables plus read access to exactly the scope columns that
// the claim owner/fence check and the instance/operation scope triggers read.
// PostgreSQL requires UPDATE on at least one column for a FOR SHARE row lock,
// so the State login may update environment_operations.updated_at, the only
// column it may write there; it never changes owner, fence or status. It has
// no DDL, no DELETE of state, attempts or instances, and no product access.
func stateGrants() []grant {
	return []grant{
		{table: "schema_baseline", privilege: "SELECT"},
		{table: "applications", privilege: "SELECT", columns: []string{"id", "organization_id"}},
		{table: "environments", privilege: "SELECT", columns: []string{"id", "application_id", "active_operation_id", "generation_high"}},
		{table: "active_resources", privilege: "SELECT", columns: []string{"id", "organization_id"}},
		{table: "environment_operations", privilege: "SELECT", columns: []string{"id", "environment_id", "owner_instance", "fence", "status"}},
		{table: "environment_operations", privilege: "UPDATE", columns: []string{"updated_at"}},
		{table: "terraform_resource_instances", privilege: "SELECT"},
		{table: "terraform_resource_instances", privilege: "INSERT"},
		{table: "terraform_resource_instances", privilege: "UPDATE", columns: []string{"status", "version", "updated_at"}},
		{table: "terraform_resource_dependencies", privilege: "SELECT"},
		{table: "terraform_resource_dependencies", privilege: "INSERT"},
		{table: "terraform_resource_dependencies", privilege: "DELETE"},
		{table: "terraform_resource_attempts", privilege: "SELECT"},
		{table: "terraform_resource_attempts", privilege: "INSERT"},
		{table: "terraform_resource_attempts", privilege: "UPDATE", columns: []string{"stage", "status", "observed_physical_ids", "state_serial", "safe_error_code", "recovery_handle", "updated_at"}},
		{table: "terraform_states", privilege: "SELECT"},
		{table: "terraform_states", privilege: "INSERT"},
		{table: "terraform_states", privilege: "UPDATE", columns: []string{"ciphertext", "nonce", "key_id", "aad_revision", "lineage", "serial", "version", "updated_at"}},
		{table: "terraform_state_locks", privilege: "SELECT"},
		{table: "terraform_state_locks", privilege: "INSERT"},
		{table: "terraform_state_locks", privilege: "UPDATE", columns: []string{"heartbeat_at"}},
		{table: "terraform_state_locks", privilege: "DELETE"},
		{table: "terraform_executor_capabilities", privilege: "SELECT"},
		{table: "terraform_executor_capabilities", privilege: "INSERT"},
	}
}

func grantsOf(role RuntimeRole) []grant {
	if role == StateRuntime {
		return stateGrants()
	}
	return consoleGrants()
}

// grantStatements renders the GRANT statements of role for roleName.
func grantStatements(role RuntimeRole, roleName string) []string {
	var out []string
	for _, g := range grantsOf(role) {
		cols := ""
		if g.columns != nil {
			quoted := make([]string, len(g.columns))
			for i, c := range g.columns {
				quoted[i] = pgx.Identifier{c}.Sanitize()
			}
			cols = " (" + strings.Join(quoted, ",") + ")"
		}
		out = append(out, "GRANT "+g.privilege+cols+" ON TABLE "+pgx.Identifier{g.table}.Sanitize()+" TO "+pgx.Identifier{roleName}.Sanitize())
	}
	return out
}

// expected reports whether role must hold privilege on table.column; an
// empty column asks for the table-level privilege.
func expected(grants []grant, table, column, privilege string) bool {
	for _, g := range grants {
		if g.table != table || g.privilege != privilege {
			continue
		}
		if column == "" {
			if g.columns == nil {
				return true
			}
			continue
		}
		if g.columns == nil || slices.Contains(g.columns, column) {
			return true
		}
	}
	return false
}

// verifyPrivileges compares every privilege the role identified by roleOID
// holds in schema (directly, through PUBLIC or through any membership) with
// the grant set of role: column SELECT/INSERT/UPDATE/REFERENCES, table
// DELETE/TRUNCATE/TRIGGER, any grant option, function EXECUTE, schema
// USAGE/CREATE and database CONNECT/CREATE/TEMP. Both a missing and an extra
// privilege fail. The reason names only catalog objects.
func verifyPrivileges(ctx context.Context, tx pgx.Tx, schema string, roleOID uint32, role RuntimeRole) error {
	grants := grantsOf(role)
	mismatch := func(what string) error { return fmt.Errorf("privileges differ from the %s grants at %s", role, what) }

	var usage, usageGrant, create, connect, dbCreate, temp bool
	if err := tx.QueryRow(ctx, `SELECT has_schema_privilege($2::oid,n.oid,'USAGE'),has_schema_privilege($2::oid,n.oid,'USAGE WITH GRANT OPTION'),has_schema_privilege($2::oid,n.oid,'CREATE'),
		has_database_privilege($2::oid,current_database(),'CONNECT'),has_database_privilege($2::oid,current_database(),'CREATE'),has_database_privilege($2::oid,current_database(),'TEMP')
		FROM pg_namespace n WHERE n.nspname=$1`, schema, roleOID).Scan(&usage, &usageGrant, &create, &connect, &dbCreate, &temp); err != nil {
		return fmt.Errorf("inspect schema privileges (%s)", sqlState(err))
	}
	switch {
	case !usage || usageGrant:
		return mismatch("schema usage")
	case create:
		return mismatch("schema create")
	case !connect:
		return mismatch("database connect")
	case dbCreate || temp:
		return mismatch("database create/temporary")
	}

	rows, err := tx.Query(ctx, `SELECT c.relname,a.attname,
		has_column_privilege($2::oid,c.oid,a.attnum,'SELECT'),has_column_privilege($2::oid,c.oid,a.attnum,'INSERT'),
		has_column_privilege($2::oid,c.oid,a.attnum,'UPDATE'),has_column_privilege($2::oid,c.oid,a.attnum,'REFERENCES'),
		has_column_privilege($2::oid,c.oid,a.attnum,'SELECT WITH GRANT OPTION,INSERT WITH GRANT OPTION,UPDATE WITH GRANT OPTION,REFERENCES WITH GRANT OPTION')
		FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace JOIN pg_attribute a ON a.attrelid=c.oid
		WHERE n.nspname=$1 AND c.relkind IN ('r','p','v','m','f') AND a.attnum>0 AND NOT a.attisdropped`, schema, roleOID)
	if err != nil {
		return fmt.Errorf("inspect column privileges (%s)", sqlState(err))
	}
	type colPriv struct {
		table, column                 string
		sel, ins, upd, ref, withGrant bool
	}
	var cols []colPriv
	for rows.Next() {
		var p colPriv
		if err := rows.Scan(&p.table, &p.column, &p.sel, &p.ins, &p.upd, &p.ref, &p.withGrant); err != nil {
			rows.Close()
			return fmt.Errorf("inspect column privileges (%s)", sqlState(err))
		}
		cols = append(cols, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("inspect column privileges (%s)", sqlState(err))
	}
	for _, p := range cols {
		where := p.table + "." + p.column
		if p.ref || p.withGrant {
			return mismatch(where)
		}
		for privilege, have := range map[string]bool{"SELECT": p.sel, "INSERT": p.ins, "UPDATE": p.upd} {
			if have != expected(grants, p.table, p.column, privilege) {
				return mismatch(where + " " + privilege)
			}
		}
	}

	rows, err = tx.Query(ctx, `SELECT c.relname,has_table_privilege($2::oid,c.oid,'DELETE'),has_table_privilege($2::oid,c.oid,'TRUNCATE'),has_table_privilege($2::oid,c.oid,'TRIGGER'),
		has_table_privilege($2::oid,c.oid,'DELETE WITH GRANT OPTION,TRUNCATE WITH GRANT OPTION,TRIGGER WITH GRANT OPTION,SELECT WITH GRANT OPTION,INSERT WITH GRANT OPTION,UPDATE WITH GRANT OPTION,REFERENCES WITH GRANT OPTION')
		FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
		WHERE n.nspname=$1 AND c.relkind IN ('r','p','v','m','f','S')`, schema, roleOID)
	if err != nil {
		return fmt.Errorf("inspect table privileges (%s)", sqlState(err))
	}
	type tablePriv struct {
		table                      string
		del, truncate, trig, grant bool
	}
	var tables []tablePriv
	for rows.Next() {
		var p tablePriv
		if err := rows.Scan(&p.table, &p.del, &p.truncate, &p.trig, &p.grant); err != nil {
			rows.Close()
			return fmt.Errorf("inspect table privileges (%s)", sqlState(err))
		}
		tables = append(tables, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("inspect table privileges (%s)", sqlState(err))
	}
	for _, p := range tables {
		if p.truncate || p.trig || p.grant || p.del != expected(grants, p.table, "", "DELETE") {
			return mismatch(p.table)
		}
	}

	var executable int64
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname=$1 AND has_function_privilege($2::oid,p.oid,'EXECUTE')`, schema, roleOID).Scan(&executable); err != nil {
		return fmt.Errorf("inspect function privileges (%s)", sqlState(err))
	}
	if executable != 0 {
		return mismatch("schema functions")
	}

	// Outside its schema the login may create nothing and use no relation or
	// user function: a view or function elsewhere (for example one owned by
	// another role over terraform_states) would bypass the column grants.
	var elsewhere int64
	if err := tx.QueryRow(ctx, `SELECT
		  (SELECT count(*) FROM pg_namespace n WHERE has_schema_privilege($2::oid,n.oid,'CREATE'))
		+ (SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
		    WHERE n.nspname<>$1 AND n.nspname NOT IN ('pg_catalog','information_schema') AND n.nspname NOT LIKE 'pg\_toast%' AND n.nspname NOT LIKE 'pg\_temp\_%'
		      AND c.relkind IN ('r','p','v','m','f','S')
		      AND (has_table_privilege($2::oid,c.oid,'SELECT,INSERT,UPDATE,DELETE,TRUNCATE,REFERENCES,TRIGGER') OR has_any_column_privilege($2::oid,c.oid,'SELECT,INSERT,UPDATE,REFERENCES')))
		+ (SELECT count(*) FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace
		    WHERE n.nspname<>$1 AND n.nspname NOT IN ('pg_catalog','information_schema') AND has_function_privilege($2::oid,p.oid,'EXECUTE'))`, schema, roleOID).Scan(&elsewhere); err != nil {
		return fmt.Errorf("inspect privileges outside the schema (%s)", sqlState(err))
	}
	if elsewhere != 0 {
		return mismatch("objects outside the baseline schema")
	}
	return nil
}
