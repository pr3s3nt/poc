package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence/persistencetest"
)

// catalogOf returns the structural dump of the connection's current schema.
func catalogOf(t *testing.T, url string) []string {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	var schema string
	if err := tx.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	lines, err := catalogDump(ctx, tx, schema)
	if err != nil {
		t.Fatal(err)
	}
	return lines
}

func sqlOn(t *testing.T, url string, statements ...string) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	for _, sql := range statements {
		if _, err := conn.Exec(ctx, sql); err != nil {
			t.Fatalf("%v\n%s", err, sql)
		}
	}
}

func scalar[T any](t *testing.T, url, sql string) T {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	var v T
	if err := conn.QueryRow(ctx, sql).Scan(&v); err != nil {
		t.Fatalf("%v\n%s", err, sql)
	}
	return v
}

func equalLines(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// An empty database initializes with the complete baseline and no migration
// ledger; every later boot validates it, changes nothing and is idempotent.
func TestFreshInitializationAndIdempotentReopen(t *testing.T) {
	ctx := context.Background()
	url := persistencetest.FreshPostgresDatabase(t)
	first, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	first.Close()
	if n := scalar[int](t, url, `SELECT count(*) FROM pg_class WHERE relname='schema_migrations' AND relnamespace=current_schema()::regnamespace`); n != 0 {
		t.Fatalf("a migration ledger exists: %d", n)
	}
	if got := scalar[string](t, url, `SELECT baseline FROM schema_baseline`); got != baselineIdentity {
		t.Fatalf("marker = %q", got)
	}
	stamp := scalar[string](t, url, `SELECT created_at::text FROM schema_baseline`)
	before := catalogOf(t, url)
	for i := 0; i < 3; i++ {
		st, err := Open(ctx, url)
		if err != nil {
			t.Fatalf("boot %d: %v", i+2, err)
		}
		st.Close()
	}
	if !equalLines(before, catalogOf(t, url)) {
		t.Fatal("validation changed the schema")
	}
	if scalar[string](t, url, `SELECT created_at::text FROM schema_baseline`) != stamp || scalar[int](t, url, `SELECT count(*) FROM schema_baseline`) != 1 {
		t.Fatal("validation rewrote the baseline marker")
	}
	// The private reference schema built for validation is never left behind.
	if n := scalar[int](t, url, `SELECT count(*) FROM pg_namespace WHERE nspname LIKE 'orch_baseline_ref_%'`); n != 0 {
		t.Fatalf("reference schemas left behind: %d", n)
	}
}

// Many backends starting on one empty database initialize it exactly once,
// atomically under the advisory lock; all of them end up serving.
func TestConcurrentFreshInitialization(t *testing.T) {
	ctx := context.Background()
	url := persistencetest.FreshPostgresDatabase(t)
	const backends = 8
	stores := make([]*Store, backends)
	errs := make([]error, backends)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range stores {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			stores[i], errs[i] = Open(ctx, url)
		}(i)
	}
	close(start)
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("backend %d: %v", i, err)
		}
		defer stores[i].Close()
	}
	if n := scalar[int](t, url, `SELECT count(*) FROM schema_baseline`); n != 1 {
		t.Fatalf("marker rows = %d", n)
	}
	reopened, err := Open(ctx, url)
	if err != nil {
		t.Fatalf("validation after concurrent initialization: %v", err)
	}
	reopened.Close()
}

// A failed initialization is atomic: nothing of the baseline survives, and a
// later boot with the real baseline succeeds.
func TestFailedInitializationLeavesEmptyDatabase(t *testing.T) {
	ctx := context.Background()
	url := persistencetest.FreshPostgresDatabase(t)
	good := baselineSQL
	baselineSQL = good + "\nCREATE TABLE organizations (id uuid);\n"
	t.Cleanup(func() { baselineSQL = good })
	if st, err := Open(ctx, url); err == nil {
		st.Close()
		t.Fatal("broken baseline initialized")
	}
	baselineSQL = good
	if n := scalar[int](t, url, `SELECT count(*) FROM pg_class WHERE relnamespace=current_schema()::regnamespace`); n != 0 {
		t.Fatalf("partial baseline left %d relations", n)
	}
	st, err := Open(ctx, url)
	if err != nil {
		t.Fatalf("initialization after a failed attempt: %v", err)
	}
	st.Close()
}

// An old-layout database (migration ledger, retired Application columns,
// text identifiers) is rejected before serving. Removing the ledger, or adding
// a baseline marker, never makes the old schema eligible, and no data or object
// is touched or echoed.
func TestOldLayoutRequiresResetAndIsNeverEligible(t *testing.T) {
	ctx := context.Background()
	url := persistencetest.FreshPostgresDatabase(t)
	sqlOn(t, url,
		`CREATE TABLE schema_migrations(version bigint PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`,
		`INSERT INTO schema_migrations(version) SELECT generate_series(1,10)`,
		`CREATE TABLE organizations(id uuid PRIMARY KEY, organization_key text UNIQUE NOT NULL, name text NOT NULL, default_connection_key text NOT NULL, default_connection_id uuid)`,
		`CREATE TABLE user_accounts(id text PRIMARY KEY)`,
		`CREATE TABLE applications(id uuid PRIMARY KEY, application_key text, name text, execution_profile text, connection_id uuid, region text, runtime_status text, configuration_provider text)`,
		`INSERT INTO organizations VALUES('11111111-1111-4111-8111-111111111111','secret-org-key','secret-org-name','x',NULL)`,
	)
	before := catalogOf(t, url)
	check := func(label string) {
		t.Helper()
		st, err := Open(ctx, url)
		if err == nil {
			st.Close()
			t.Fatalf("%s: old layout opened", label)
		}
		if !errors.Is(err, ErrResetRequired) || strings.Contains(err.Error(), "secret-org") {
			t.Fatalf("%s: error = %v", label, err)
		}
		if !equalLines(before, catalogOf(t, url)) {
			t.Fatalf("%s: schema was altered", label)
		}
		if n := scalar[int](t, url, `SELECT count(*) FROM organizations WHERE name='secret-org-name'`); n != 1 {
			t.Fatalf("%s: data changed", label)
		}
	}
	check("with ledger")
	sqlOn(t, url, `DROP TABLE schema_migrations`)
	before = catalogOf(t, url)
	check("ledger removed")
	sqlOn(t, url,
		`CREATE TABLE schema_baseline(singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton), baseline text NOT NULL, created_at timestamptz NOT NULL DEFAULT now())`,
		`INSERT INTO schema_baseline(baseline) VALUES('`+baselineIdentity+`')`)
	before = catalogOf(t, url)
	check("marker added to the old schema")
}

// Any structural damage to a marked baseline is detected before serving
// without a repair: types, nullability, defaults, keys, FKs, deferrability,
// check constraints, partial/expression indexes, triggers and their function.
func TestSchemaDriftIsDetectedWithoutRepair(t *testing.T) {
	ctx := context.Background()
	drift := map[string][]string{
		"dropped column":            {`ALTER TABLE environments DROP COLUMN generation_high`},
		"extra legacy column":       {`ALTER TABLE secret_store_connections ADD COLUMN legacy boolean NOT NULL DEFAULT false`},
		"wrong column type":         {`ALTER TABLE sessions ALTER COLUMN id TYPE text`},
		"nullability":               {`ALTER TABLE connections ALTER COLUMN name DROP NOT NULL`},
		"changed default":           {`ALTER TABLE environments ALTER COLUMN runtime_status SET DEFAULT 'PENDING'`},
		"dropped foreign key":       {`ALTER TABLE environments DROP CONSTRAINT environments_active_operation_fk`},
		"not deferrable":            {`ALTER TABLE environments ALTER CONSTRAINT environments_current_set_fk NOT DEFERRABLE`},
		"dropped unique":            {`ALTER TABLE connections DROP CONSTRAINT connections_organization_id_connection_key_key`},
		"dropped primary key":       {`ALTER TABLE deployment_plans DROP CONSTRAINT deployment_plans_pkey`},
		"dropped check":             {`ALTER TABLE configuration_revision_entries DROP CONSTRAINT configuration_revision_entries_kind_check`},
		"widened scope check":       {`ALTER TABLE environments DROP CONSTRAINT environments_infrastructure_scope_check`, `ALTER TABLE environments ADD CONSTRAINT environments_infrastructure_scope_check CHECK (infrastructure_scope IN ('ENVIRONMENT','LEGACY_APPLICATION'))`},
		"widened auth check":        {`ALTER TABLE connections DROP CONSTRAINT connections_authentication_type_check`, `ALTER TABLE connections ADD CONSTRAINT connections_authentication_type_check CHECK (authentication_type IN ('KUBECONFIG','HOST_CONTEXT','AWS_ACCESS_KEY'))`},
		"dropped partial index":     {`DROP INDEX environment_operations_one_holder`},
		"partial index widened":     {`DROP INDEX environment_operations_one_holder`, `CREATE UNIQUE INDEX environment_operations_one_holder ON environment_operations(environment_id)`},
		"dropped expression idx":    {`DROP INDEX applications_organization_lower_name_key`},
		"dropped generation idx":    {`DROP INDEX workload_instances_env_generation_workload_key`},
		"dropped snapshot trigger":  {`DROP TRIGGER deployments_require_snapshot ON deployments`},
		"disabled snapshot trigger": {`ALTER TABLE deployments DISABLE TRIGGER deployments_require_snapshot`},
		"altered trigger function":  {`CREATE OR REPLACE FUNCTION require_deployment_snapshot() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NEW; END $$`},
		"permanent binding trigger": {
			`CREATE FUNCTION environments_binding_immutable() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NEW; END $$`,
			`CREATE TRIGGER environments_binding_immutable BEFORE UPDATE ON environments FOR EACH ROW EXECUTE FUNCTION environments_binding_immutable()`},
		"stray table":          {`CREATE TABLE stray(id integer)`},
		"stray ledger":         {`CREATE TABLE schema_migrations(version bigint PRIMARY KEY)`},
		"wrong marker":         {`UPDATE schema_baseline SET baseline='something-else'`},
		"missing marker":       {`DELETE FROM schema_baseline`},
		"dropped marker table": {`DROP TABLE schema_baseline`},
	}
	for name, statements := range drift {
		t.Run(name, func(t *testing.T) {
			url := persistencetest.FreshPostgresDatabase(t)
			st, err := Open(ctx, url)
			if err != nil {
				t.Fatal(err)
			}
			if err := st.SaveOrganization(ctx, testOrganization("secret-drift-org")); err != nil {
				t.Fatal(err)
			}
			st.Close()
			sqlOn(t, url, statements...)
			before := catalogOf(t, url)
			reopened, err := Open(ctx, url)
			if err == nil {
				reopened.Close()
				t.Fatal("damaged baseline served")
			}
			if !errors.Is(err, ErrResetRequired) || strings.Contains(err.Error(), "secret-drift-org") {
				t.Fatalf("error = %v", err)
			}
			if !equalLines(before, catalogOf(t, url)) {
				t.Fatal("startup repaired or altered the schema")
			}
			if n := scalar[int](t, url, `SELECT count(*) FROM pg_namespace WHERE nspname LIKE 'orch_baseline_ref_%'`); n != 0 {
				t.Fatalf("a rejected Open left %d reference schemas", n)
			}
		})
	}
}

// The constraints that carry the current semantics hold in the database
// itself, independent of adapter code.
func TestBaselineEnforcesCurrentConstraints(t *testing.T) {
	ctx := context.Background()
	st, url := openFresh(t)
	const (
		org = "11111111-1111-4111-8111-111111111111"
		app = "22222222-2222-4222-8222-222222222222"
		env = "33333333-3333-4333-8333-333333333333"
		rev = "44444444-4444-4444-8444-444444444444"
	)
	sqlOn(t, url,
		`INSERT INTO organizations(id,organization_key,name,default_connection_key) VALUES('`+org+`','acme','Acme','default')`,
		`INSERT INTO applications(id,application_key,organization_id,name,subdomain,version) VALUES('`+app+`','app','`+org+`','Payments','pay',1)`,
		`INSERT INTO environments(id,application_id,environment_key,name,environment_type,namespace_identity,version) VALUES('`+env+`','`+app+`','dev','dev','dev','app-dev',1)`,
		`INSERT INTO configuration_revisions(id,environment_id,version) VALUES('`+rev+`','`+env+`',1)`,
	)
	entry := func(kind, ref, store, value string) string {
		return fmt.Sprintf(`INSERT INTO configuration_revision_entries(revision_id,key_name,kind,value_ref,store_key,variable_value) VALUES('%s','K%d','%s',%s,'%s',%s)`, rev, len(kind)+len(ref)+len(store)+len(value), kind, ref, store, value)
	}
	for name, sql := range map[string]string{
		"variable with store ref":       entry("VARIABLE", `'kv2://x'`, "", `'v'`),
		"variable with store key":       entry("VARIABLE", `NULL`, "s", `'v'`),
		"variable without value":        entry("VARIABLE", `NULL`, "", `NULL`),
		"variable with empty value":     entry("VARIABLE", `NULL`, "", `''`),
		"secret with inline value":      entry("SECRET", `'kv2://x'`, "s", `'plain'`),
		"secret without ref":            entry("SECRET", `NULL`, "s", `NULL`),
		"secret without store":          entry("SECRET", `'kv2://x'`, "", `NULL`),
		"unknown kind":                  entry("OTHER", `NULL`, "", `'v'`),
		"retired infrastructure scope":  `UPDATE environments SET infrastructure_scope='LEGACY_APPLICATION'`,
		"host context authentication":   `INSERT INTO connections(id,organization_id,connection_key,name,kind,authentication_type,config,secret_ref,status,verification) VALUES('55555555-5555-4555-8555-555555555555','` + org + `','h','h','KUBERNETES','HOST_CONTEXT','{}','','READY','{}')`,
		"kind incompatible auth":        `INSERT INTO connections(id,organization_id,connection_key,name,kind,authentication_type,config,secret_ref,status,verification) VALUES('55555555-5555-4555-8555-555555555556','` + org + `','h2','h2','AWS','KUBECONFIG','{}','','READY','{}')`,
		"non-uuid account id":           `INSERT INTO user_accounts(id,organization_id,username,password_hash,role,status) VALUES('not-a-uuid','` + org + `','u','x','ADMIN','ACTIVE')`,
		"case-variant application name": `INSERT INTO applications(id,application_key,organization_id,name,subdomain,version) VALUES('66666666-6666-4666-8666-666666666666','app2','` + org + `','PAYMENTS','pay2',1)`,
	} {
		if _, err := st.pool.Exec(ctx, sql); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	for name, sql := range map[string]string{
		"valid variable": entry("VARIABLE", `NULL`, "", `'blue'`),
		"valid secret":   entry("SECRET", `'kv2://kv/a'`, "vault-a", `NULL`),
	} {
		if _, err := st.pool.Exec(ctx, sql); err != nil {
			t.Errorf("%s rejected: %v", name, err)
		}
	}
	// One holder per Environment: a second active operation violates the
	// partial unique index while a finished one does not.
	op := func(id, status string) string {
		return fmt.Sprintf(`INSERT INTO environment_operations(id,environment_id,kind,owner_instance,status,pins,started_at,updated_at,heartbeat_at,deadline) VALUES('%s','%s','SWITCH','a','%s','{}',now(),now(),now(),now())`, id, env, status)
	}
	if _, err := st.pool.Exec(ctx, op("77777777-7777-4777-8777-777777777771", "ACTIVE")); err != nil {
		t.Fatal(err)
	}
	if _, err := st.pool.Exec(ctx, op("77777777-7777-4777-8777-777777777772", "RECOVERING")); err == nil {
		t.Error("second holder accepted")
	}
	if _, err := st.pool.Exec(ctx, op("77777777-7777-4777-8777-777777777773", "SUCCEEDED")); err != nil {
		t.Errorf("finished operation rejected: %v", err)
	}
	if n := scalar[int](t, url, `SELECT count(*) FROM pg_trigger WHERE tgname='environments_binding_immutable'`); n != 0 {
		t.Errorf("permanent binding trigger installed: %d", n)
	}
}

func testOrganization(name string) application.Organization {
	return application.Organization{ID: ids.New(), Key: strings.ToLower(ids.New()[:8]), Name: name, DefaultConnectionKey: "default"}
}
