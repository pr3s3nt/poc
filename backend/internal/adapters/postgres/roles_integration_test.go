package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/persistence/persistencetest"
	"orchestrator/internal/ports/terraformstate"
)

// These option-A tests (ADR-014) log in as the separate owner, Console and
// State roles with their own passwords; nothing uses SET ROLE under a
// superuser. They need ORCHESTRATOR_POSTGRES_TEST_URL and skip otherwise, or
// fail under REQUIRE_POSTGRES=1.

func connectAs(t *testing.T, url string) *pgx.Conn {
	t.Helper()
	conn, err := pgx.Connect(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { conn.Close(context.Background()) })
	return conn
}

func codeOf(t *testing.T, err error) string {
	t.Helper()
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	if err == nil {
		return ""
	}
	return "non-SQL error: " + err.Error()
}

// denied runs sql on conn (in a rolled-back transaction) and requires the
// database itself to refuse it with insufficient_privilege (42501).
func denied(t *testing.T, conn *pgx.Conn, label, sql string, args ...any) {
	t.Helper()
	ctx := context.Background()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, sql, args...)
	if err == nil {
		for rows.Next() {
		}
		rows.Close()
		err = rows.Err()
	}
	if code := codeOf(t, err); code != "42501" {
		t.Errorf("%s: want permission denied (42501), got %q", label, code)
	}
}

// allowed runs sql on conn in a rolled-back transaction and requires success.
func allowed(t *testing.T, conn *pgx.Conn, label, sql string, args ...any) int {
	t.Helper()
	ctx := context.Background()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, sql, args...)
	if err != nil {
		t.Errorf("%s: %v", label, err)
		return 0
	}
	return int(tag.RowsAffected())
}

// stateFixture is a set-up database with one instance, an encrypted-state
// row, a state lock and a capability, written through the State login under
// the Environment claim taken by the Console login.
type stateFixture struct {
	db      persistencetest.PostgresDatabase
	console *Store
	state   *StateStore
	op      environment.Operation
	inst    tfresource.Instance
	hash    string
}

func newStateFixture(t *testing.T) stateFixture {
	t.Helper()
	ctx := context.Background()
	db := freshDatabase(t)
	console, err := Open(ctx, db.Console)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(console.Close)
	state, err := OpenState(ctx, db.State)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(state.Close)
	if console.Identity() != state.Identity() || console.Identity().Installation == "" {
		t.Fatalf("identities differ: %+v %+v", console.Identity(), state.Identity())
	}
	if err := console.SaveOrganization(ctx, application.Organization{ID: ids.New(), Key: "acme", Name: "acme"}); err != nil {
		t.Fatal(err)
	}
	appID := ids.New()
	if err := console.SaveApplication(ctx, application.Application{ID: appID, Key: "app", OrganizationKey: "acme", Name: "App", Subdomain: "app", Version: 1}); err != nil {
		t.Fatal(err)
	}
	if err := console.SaveEnvironment(ctx, environment.Environment{ID: ids.New(), Key: "staging", ApplicationID: appID, ApplicationKey: "app", Name: "staging", Type: "staging", NamespaceIdentity: "ns-app-staging", Version: 1}); err != nil {
		t.Fatal(err)
	}
	bundle := ids.New()
	// The module registry is a later task; the administrator arranges one bundle row.
	sqlOn(t, db.Admin, `INSERT INTO terraform_module_bundles(id,source_id,module_id,revision,subdirectory,content_sha256,terraform_version,terraform_binary_sha256,provider_lock_digest,contract,provider_requirements,status,created_at,updated_at) VALUES('`+bundle+`','platform','redis','0123456789abcdef0123456789abcdef01234567','.',repeat('a',64),'1.9.8',repeat('b',64),repeat('c',64),'{}','{}','VERIFIED',now(),now())`)
	op, err := console.ClaimEnvironment(ctx, persistence.OperationClaim{ApplicationKey: "app", EnvironmentKey: "staging", Owner: "role-test", Kind: environment.OpDeploy, Deadline: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	owner := persistence.WithOwner(ctx, persistence.Owner{OperationID: op.ID, Owner: op.Owner, Fence: op.Fence})
	inst, err := state.RegisterInstance(owner, terraformstate.NewInstance{Descriptor: "redis.in-cluster#cache", ScopeType: "SHARED", ScopeID: "cache", ModuleBundleID: bundle, TargetRef: map[string]any{"connection": "k8s-4f"}, PhysicalIntent: map[string]any{"name": "redis"}, CreatedByRunID: "run-1"})
	if err != nil {
		t.Fatal(err)
	}
	hash := strings.Repeat("ab", 32)
	if err := state.InsertCapability(owner, tfresource.Capability{TokenHash: hash, InstanceID: inst.ID, Action: tfresource.CapabilityCreate, ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	err = state.Transact(owner, func(ctx context.Context) error {
		if _, err := state.InsertLock(ctx, tfresource.Lock{InstanceID: inst.ID, LockID: "lock-1", OperationID: op.ID, Owner: op.Owner, Fence: op.Fence, AcquiredAt: now, Deadline: now.Add(time.Hour)}); err != nil {
			return err
		}
		return state.PutState(ctx, tfresource.StateBlob{InstanceID: inst.ID, OrganizationID: inst.Identity.OrganizationID, Ciphertext: []byte("opaque-ciphertext"), Nonce: make([]byte, 12), KeyID: "key-1", AADRevision: 1, Lineage: "lin", Serial: 1, Version: 1, UpdatedAt: now}, 0)
	})
	if err != nil {
		t.Fatal(err)
	}
	return stateFixture{db: db, console: console, state: state, op: op, inst: inst, hash: hash}
}

// The Console login cannot read state ciphertext, nonce or key data, nor
// capability token hashes, through any statement form, and cannot escalate
// to the State or owner role, create views/functions/temporary objects, grant
// itself access or change ownership. It can read the safe lock columns and
// revoke an operation's capabilities, which release/recovery need.
func TestConsoleLoginCannotReadStateOrCapabilityHashes(t *testing.T) {
	f := newStateFixture(t)
	console := connectAs(t, f.db.Console)
	for label, sql := range map[string]string{
		"state ciphertext":          `SELECT ciphertext FROM terraform_states`,
		"state row":                 `SELECT * FROM terraform_states`,
		"state count":               `SELECT count(*) FROM terraform_states`,
		"state nonce/key":           `SELECT nonce,key_id FROM terraform_states`,
		"state metadata":            `SELECT lineage,serial FROM terraform_states`,
		"state copy":                `COPY terraform_states TO STDOUT`,
		"state update":              `UPDATE terraform_states SET serial=0`,
		"state delete":              `DELETE FROM terraform_states`,
		"state truncate":            `TRUNCATE terraform_states`,
		"state via join":            `SELECT s.ciphertext FROM terraform_resource_instances i JOIN terraform_states s ON s.resource_instance_id=i.id`,
		"capability hash":           `SELECT token_hash FROM terraform_executor_capabilities`,
		"capability row":            `SELECT * FROM terraform_executor_capabilities`,
		"capability copy":           `COPY terraform_executor_capabilities TO STDOUT`,
		"capability hash filter":    `DELETE FROM terraform_executor_capabilities WHERE token_hash LIKE 'a%'`,
		"capability hash returning": `DELETE FROM terraform_executor_capabilities WHERE environment_operation_id='` + f.op.ID + `' RETURNING token_hash`,
		"capability insert":         `INSERT INTO terraform_executor_capabilities(token_hash,organization_id,resource_instance_id,environment_operation_id,fencing_generation,action,expires_at) SELECT repeat('c',64),organization_id,id,'` + f.op.ID + `',1,'CREATE',now() FROM terraform_resource_instances`,
		"lock metadata document":    `SELECT lock_metadata FROM terraform_state_locks`,
		"lock delete":               `DELETE FROM terraform_state_locks`,
		"instance write":            `UPDATE terraform_resource_instances SET status='READY'`,
		"instance row lock":         `SELECT id FROM terraform_resource_instances FOR SHARE`,
		"attempt write":             `UPDATE terraform_resource_attempts SET status='FAILED'`,
		"marker write":              `UPDATE schema_baseline SET baseline='x'`,
		"escalate to state":         `SET ROLE ` + pgx.Identifier{f.db.StateRole}.Sanitize(),
		"escalate to owner":         `SET ROLE ` + pgx.Identifier{f.db.OwnerRole}.Sanitize(),
		"session authorization":     `SET SESSION AUTHORIZATION ` + pgx.Identifier{f.db.OwnerRole}.Sanitize(),
		"view over state":           `CREATE VIEW leak AS SELECT * FROM terraform_states`,
		"definer function":          `CREATE FUNCTION leak() RETURNS SETOF bytea LANGUAGE sql SECURITY DEFINER AS 'SELECT ciphertext FROM terraform_states'`,
		"temporary table":           `CREATE TEMPORARY TABLE leak(x bytea)`,
		"schema":                    `CREATE SCHEMA leak`,
		"table":                     `CREATE TABLE leak(x int)`,
		"grant to self":             `GRANT SELECT ON terraform_states TO ` + pgx.Identifier{f.db.ConsoleRole}.Sanitize(),
		"alter owner":               `ALTER TABLE terraform_states OWNER TO ` + pgx.Identifier{f.db.ConsoleRole}.Sanitize(),
		"disable retention trigger": `ALTER TABLE terraform_states DISABLE TRIGGER terraform_states_retained`,
		"become superuser":          `ALTER ROLE ` + pgx.Identifier{f.db.ConsoleRole}.Sanitize() + ` SUPERUSER`,
		"create role":               `CREATE ROLE leak LOGIN`,
		"grant role":                `GRANT ` + pgx.Identifier{f.db.StateRole}.Sanitize() + ` TO ` + pgx.Identifier{f.db.ConsoleRole}.Sanitize(),
		"server file":               `SELECT pg_read_file('postgresql.conf')`,
		"large object import":       `SELECT lo_import('/etc/passwd')`,
		"retention function":        `SELECT terraform_retained()`,
	} {
		denied(t, console, label, sql)
	}
	// A GRANT on a table where the login holds some privilege only warns; the
	// hash column stays unreadable afterwards.
	ctx := context.Background()
	_, _ = console.Exec(ctx, `GRANT SELECT ON terraform_executor_capabilities TO `+pgx.Identifier{f.db.ConsoleRole}.Sanitize())
	var readable bool
	if err := console.QueryRow(ctx, `SELECT has_column_privilege('terraform_executor_capabilities','token_hash','SELECT') OR has_column_privilege('terraform_states','ciphertext','SELECT')`).Scan(&readable); err != nil || readable {
		t.Fatalf("console can read a hash or ciphertext after a self-grant: %v %v", readable, err)
	}
	// Planner statistics never expose a column the login cannot read.
	sqlOn(t, f.db.Admin, `ANALYZE terraform_states`, `ANALYZE terraform_executor_capabilities`)
	var stats int
	if err := console.QueryRow(ctx, `SELECT count(*) FROM pg_stats WHERE tablename IN ('terraform_states','terraform_executor_capabilities') AND attname IN ('ciphertext','nonce','token_hash')`).Scan(&stats); err != nil || stats != 0 {
		t.Fatalf("console sees %d statistics rows of restricted columns: %v", stats, err)
	}

	// What release/recovery needs stays possible.
	allowed(t, console, "safe lock metadata", `SELECT resource_instance_id,lock_id,environment_operation_id,owner,fencing_generation,deadline FROM terraform_state_locks`)
	allowed(t, console, "capability operation column", `SELECT environment_operation_id FROM terraform_executor_capabilities`)
	allowed(t, console, "instance metadata", `SELECT id,state_key,version,status FROM terraform_resource_instances`)
	if n := allowed(t, console, "capability revocation", `DELETE FROM terraform_executor_capabilities WHERE environment_operation_id=$1::uuid`, f.op.ID); n != 1 {
		t.Fatalf("revocation removed %d capabilities", n)
	}
}

// The State login reaches only the state repository tables and the scope
// columns its owner/fence checks read. It cannot read product rows (password
// or session hashes, Connections), write claims, purge state or run DDL.
func TestStateLoginIsConfinedToTheStateRepository(t *testing.T) {
	f := newStateFixture(t)
	state := connectAs(t, f.db.State)
	for label, sql := range map[string]string{
		"password hashes":        `SELECT password_hash FROM user_accounts`,
		"session hashes":         `SELECT token_hash FROM sessions`,
		"connections":            `SELECT secret_ref FROM connections`,
		"organizations":          `SELECT * FROM organizations`,
		"environment row":        `SELECT * FROM environments`,
		"operation pins":         `SELECT pins FROM environment_operations`,
		"product insert":         `INSERT INTO organizations(id,organization_key,name,default_connection_key) VALUES(gen_random_uuid(),'x','x','x')`,
		"claim status":           `UPDATE environment_operations SET status='SUCCEEDED'`,
		"claim fence":            `UPDATE environment_operations SET fence=fence+1`,
		"claim owner":            `UPDATE environment_operations SET owner_instance='thief'`,
		"active claim":           `UPDATE environments SET active_operation_id=NULL`,
		"state purge":            `DELETE FROM terraform_states`,
		"state truncate":         `TRUNCATE terraform_states`,
		"state identity rewrite": `UPDATE terraform_states SET organization_id=organization_id`,
		"attempt purge":          `DELETE FROM terraform_resource_attempts`,
		"instance purge":         `DELETE FROM terraform_resource_instances`,
		"instance identity":      `UPDATE terraform_resource_instances SET state_key=state_key`,
		"attempt provenance":     `UPDATE terraform_resource_attempts SET run_id='x'`,
		"capability delete":      `DELETE FROM terraform_executor_capabilities`,
		"module write":           `UPDATE terraform_module_bundles SET status='REVOKED'`,
		"escalate to console":    `SET ROLE ` + pgx.Identifier{f.db.ConsoleRole}.Sanitize(),
		"escalate to owner":      `SET ROLE ` + pgx.Identifier{f.db.OwnerRole}.Sanitize(),
		"table":                  `CREATE TABLE leak(x int)`,
		"temporary table":        `CREATE TEMPORARY TABLE leak(x int)`,
		"alter":                  `ALTER TABLE terraform_states DISABLE TRIGGER terraform_states_retained`,
		"create role":            `CREATE ROLE leak LOGIN`,
	} {
		denied(t, state, label, sql)
	}
	allowed(t, state, "state ciphertext", `SELECT ciphertext,nonce,key_id FROM terraform_states`)
	allowed(t, state, "capability hash", `SELECT token_hash FROM terraform_executor_capabilities`)
	allowed(t, state, "claim check row lock", `SELECT owner_instance,fence,status FROM environment_operations WHERE id=$1::uuid FOR SHARE`, f.op.ID)
}

// A runtime connection refuses every login but its own: the owner, the
// administrator (superuser) and the other runtime login fail closed; no store
// falls back to another role.
func TestRuntimeRefusesOwnerSuperuserAndCrossedLogins(t *testing.T) {
	ctx := context.Background()
	db := freshDatabase(t)
	for label, open := range map[string]func() error{
		"console as owner": func() error { return closeOpen(Open(ctx, db.Owner)) },
		"console as admin": func() error { return closeOpen(Open(ctx, db.Admin)) },
		"console as state": func() error { return closeOpen(Open(ctx, db.State)) },
		"state as owner":   func() error { return closeOpenState(OpenState(ctx, db.Owner)) },
		"state as admin":   func() error { return closeOpenState(OpenState(ctx, db.Admin)) },
		"state as console": func() error { return closeOpenState(OpenState(ctx, db.Console)) },
	} {
		if err := open(); !errors.Is(err, ErrRuntimeRole) {
			t.Errorf("%s: %v", label, err)
		}
	}
	wrong := strings.Replace(db.Console, ":", ":x", 2) // breaks the password, not the user
	if err := closeOpen(Open(ctx, wrong)); err == nil {
		t.Error("a wrong password opened")
	}
	if err := closeOpen(Open(ctx, "")); err == nil {
		t.Error("an empty URL opened")
	}
	if err := closeOpen(Open(ctx, db.Console)); err != nil {
		t.Fatalf("console login: %v", err)
	}
	if err := closeOpenState(OpenState(ctx, db.State)); err != nil {
		t.Fatalf("state login: %v", err)
	}
}

func closeOpen(st *Store, err error) error {
	if st != nil {
		st.Close()
	}
	return err
}

func closeOpenState(st *StateStore, err error) error {
	if st != nil {
		st.Close()
	}
	return err
}

// A grant, membership or attribute added by anyone after the owner setup
// makes the affected runtime login fail closed, and a repeated owner setup
// reports the difference without regranting.
func TestRuntimeRefusesElevatedOrChangedGrants(t *testing.T) {
	ctx := context.Background()
	type change struct {
		sql   func(db persistencetest.PostgresDatabase) []string
		state bool // the State login is the one affected
	}
	q := func(name string) string { return pgx.Identifier{name}.Sanitize() }
	changes := map[string]change{
		"console reads state": {sql: func(db persistencetest.PostgresDatabase) []string {
			return []string{`GRANT SELECT ON terraform_states TO ` + q(db.ConsoleRole)}
		}},
		"console reads token hash": {sql: func(db persistencetest.PostgresDatabase) []string {
			return []string{`GRANT SELECT(token_hash) ON terraform_executor_capabilities TO ` + q(db.ConsoleRole)}
		}},
		"console reads lock document": {sql: func(db persistencetest.PostgresDatabase) []string {
			return []string{`GRANT SELECT(lock_metadata) ON terraform_state_locks TO ` + q(db.ConsoleRole)}
		}},
		"public reads state": {sql: func(db persistencetest.PostgresDatabase) []string {
			return []string{`GRANT SELECT(ciphertext) ON terraform_states TO PUBLIC`}
		}},
		"console member of state": {sql: func(db persistencetest.PostgresDatabase) []string {
			return []string{`GRANT ` + q(db.StateRole) + ` TO ` + q(db.ConsoleRole)}
		}},
		"console member of owner": {sql: func(db persistencetest.PostgresDatabase) []string {
			return []string{`GRANT ` + q(db.OwnerRole) + ` TO ` + q(db.ConsoleRole)}
		}},
		"console creates in schema": {sql: func(db persistencetest.PostgresDatabase) []string {
			return []string{`GRANT CREATE ON SCHEMA public TO ` + q(db.ConsoleRole)}
		}},
		"console temporary objects": {sql: func(db persistencetest.PostgresDatabase) []string {
			return []string{`GRANT TEMPORARY ON DATABASE ` + q(db.Name) + ` TO ` + q(db.ConsoleRole)}
		}},
		"console creates roles": {sql: func(db persistencetest.PostgresDatabase) []string {
			return []string{`ALTER ROLE ` + q(db.ConsoleRole) + ` CREATEROLE`}
		}},
		"console owns a schema": {sql: func(db persistencetest.PostgresDatabase) []string {
			return []string{`CREATE SCHEMA side AUTHORIZATION ` + q(db.ConsoleRole)}
		}},
		"console grant option": {sql: func(db persistencetest.PostgresDatabase) []string {
			return []string{`GRANT SELECT ON organizations TO ` + q(db.ConsoleRole) + ` WITH GRANT OPTION`}
		}},
		"console executes function": {sql: func(db persistencetest.PostgresDatabase) []string {
			return []string{`GRANT EXECUTE ON FUNCTION terraform_retained() TO ` + q(db.ConsoleRole)}
		}},
		"console truncates": {sql: func(db persistencetest.PostgresDatabase) []string {
			return []string{`GRANT TRUNCATE ON organizations TO ` + q(db.ConsoleRole)}
		}},
		"console lost a grant": {sql: func(db persistencetest.PostgresDatabase) []string {
			return []string{`REVOKE DELETE ON terraform_executor_capabilities FROM ` + q(db.ConsoleRole)}
		}},
		"view elsewhere over state": {sql: func(db persistencetest.PostgresDatabase) []string {
			return []string{`CREATE SCHEMA side`, `CREATE VIEW side.leak AS SELECT ciphertext FROM public.terraform_states`, `GRANT USAGE ON SCHEMA side TO ` + q(db.ConsoleRole), `GRANT SELECT ON side.leak TO ` + q(db.ConsoleRole)}
		}},
		"definer function elsewhere": {sql: func(db persistencetest.PostgresDatabase) []string {
			return []string{`CREATE SCHEMA side`, `CREATE FUNCTION side.leak() RETURNS SETOF bytea LANGUAGE sql SECURITY DEFINER AS 'SELECT ciphertext FROM public.terraform_states'`, `GRANT USAGE ON SCHEMA side TO ` + q(db.ConsoleRole)}
		}},
		"state reads product": {state: true, sql: func(db persistencetest.PostgresDatabase) []string {
			return []string{`GRANT SELECT ON user_accounts TO ` + q(db.StateRole)}
		}},
		"state writes claims": {state: true, sql: func(db persistencetest.PostgresDatabase) []string {
			return []string{`GRANT UPDATE(fence) ON environment_operations TO ` + q(db.StateRole)}
		}},
		"state purges state": {state: true, sql: func(db persistencetest.PostgresDatabase) []string {
			return []string{`GRANT DELETE ON terraform_states TO ` + q(db.StateRole)}
		}},
		"state lost a grant": {state: true, sql: func(db persistencetest.PostgresDatabase) []string {
			return []string{`REVOKE INSERT ON terraform_states FROM ` + q(db.StateRole)}
		}},
		"state member of console": {state: true, sql: func(db persistencetest.PostgresDatabase) []string {
			return []string{`GRANT ` + q(db.ConsoleRole) + ` TO ` + q(db.StateRole)}
		}},
		"state bypasses RLS": {state: true, sql: func(db persistencetest.PostgresDatabase) []string {
			return []string{`ALTER ROLE ` + q(db.StateRole) + ` BYPASSRLS`}
		}},
	}
	for name, c := range changes {
		t.Run(name, func(t *testing.T) {
			db := freshDatabase(t)
			sqlOn(t, db.Admin, c.sql(db)...)
			var err error
			if c.state {
				err = closeOpenState(OpenState(ctx, db.State))
			} else {
				err = closeOpen(Open(ctx, db.Console))
			}
			if !errors.Is(err, ErrRuntimeRole) {
				t.Fatalf("changed login opened or failed otherwise: %v", err)
			}
			if _, err := Setup(ctx, db.Owner, setupRoles(db)); err == nil {
				t.Fatal("owner setup accepted the changed grants")
			}
			// The repeated owner setup did not regrant: the login still fails.
			if c.state {
				err = closeOpenState(OpenState(ctx, db.State))
			} else {
				err = closeOpen(Open(ctx, db.Console))
			}
			if err == nil {
				t.Fatal("a repeated owner setup repaired the grants")
			}
		})
	}
}

// The owner setup refuses before any change unless the owner is an ordinary
// role that owns the fresh database and both runtime roles are distinct,
// existing, ordinary LOGIN roles without memberships.
func TestOwnerSetupRefusesUnsafeRoles(t *testing.T) {
	ctx := context.Background()
	db := rolesOnly(t)
	q := func(name string) string { return pgx.Identifier{name}.Sanitize() }
	for label, roles := range map[string]Roles{
		"same runtime role":  {Console: db.ConsoleRole, State: db.ConsoleRole},
		"missing state role": {Console: db.ConsoleRole},
		"unknown role":       {Console: db.ConsoleRole, State: "no_such_role_x"},
		"owner as console":   {Console: db.OwnerRole, State: db.StateRole},
		"superuser as state": {Console: db.ConsoleRole, State: scalar[string](t, db.Admin, `SELECT current_user`)},
	} {
		if _, err := Setup(ctx, db.Owner, roles); !errors.Is(err, ErrSetupRefused) {
			t.Errorf("%s: %v", label, err)
		}
	}
	if _, err := Setup(ctx, db.Admin, setupRoles(db)); !errors.Is(err, ErrSetupRefused) {
		t.Errorf("superuser owner: %v", err)
	}
	sqlOn(t, db.Admin, `ALTER ROLE `+q(db.StateRole)+` CREATEDB`)
	if _, err := Setup(ctx, db.Owner, setupRoles(db)); !errors.Is(err, ErrSetupRefused) {
		t.Errorf("elevated state role: %v", err)
	}
	sqlOn(t, db.Admin, `ALTER ROLE `+q(db.StateRole)+` NOCREATEDB`, `GRANT `+q(db.OwnerRole)+` TO `+q(db.ConsoleRole))
	if _, err := Setup(ctx, db.Owner, setupRoles(db)); !errors.Is(err, ErrSetupRefused) {
		t.Errorf("console member of owner: %v", err)
	}
	sqlOn(t, db.Admin, `REVOKE `+q(db.OwnerRole)+` FROM `+q(db.ConsoleRole))
	if n := scalar[int](t, db.Admin, `SELECT count(*) FROM pg_class WHERE relnamespace=current_schema()::regnamespace`); n != 0 {
		t.Fatalf("a refused setup created %d relations", n)
	}

	// An owner that does not own the database cannot remove PUBLIC access;
	// the setup verifies its grants before commit and rolls back.
	other := rolesOnly(t)
	sqlOn(t, other.Admin, `ALTER DATABASE `+q(other.Name)+` OWNER TO CURRENT_USER`, `ALTER SCHEMA public OWNER TO `+q(other.OwnerRole))
	if _, err := Setup(ctx, other.Owner, setupRoles(other)); !errors.Is(err, ErrSetupRefused) {
		t.Fatalf("owner without the database: %v", err)
	}
	if n := scalar[int](t, other.Admin, `SELECT count(*) FROM pg_class WHERE relnamespace=current_schema()::regnamespace`); n != 0 {
		t.Fatalf("a refused setup left %d relations", n)
	}
}

// The Console and State connections must serve one installation: another
// database or another schema gives a different identity, and a runtime login
// whose search path leads to an empty schema is not initialized.
func TestRuntimeIdentityBindsDatabaseAndSchema(t *testing.T) {
	ctx := context.Background()
	a, b := freshDatabase(t), freshDatabase(t)
	consoleA, err := Open(ctx, a.Console)
	if err != nil {
		t.Fatal(err)
	}
	defer consoleA.Close()
	stateB, err := OpenState(ctx, b.State)
	if err != nil {
		t.Fatal(err)
	}
	defer stateB.Close()
	if consoleA.Identity() == stateB.Identity() || consoleA.Identity().SystemIdentifier != stateB.Identity().SystemIdentifier {
		t.Fatalf("identities of two databases: %+v %+v", consoleA.Identity(), stateB.Identity())
	}

	// A second installation in another schema of the same database.
	c := rolesOnly(t)
	sqlOn(t, c.Admin, `CREATE SCHEMA orch AUTHORIZATION `+pgx.Identifier{c.OwnerRole}.Sanitize())
	withPath := func(url, path string) string { return url + "&search_path=" + path }
	if _, err := Setup(ctx, withPath(c.Owner, "orch"), setupRoles(c)); err != nil {
		t.Fatal(err)
	}
	if err := closeOpen(Open(ctx, c.Console)); !errors.Is(err, ErrNotInitialized) {
		t.Fatalf("console on the empty public schema: %v", err)
	}
	console, err := Open(ctx, withPath(c.Console, "orch"))
	if err != nil {
		t.Fatal(err)
	}
	defer console.Close()
	state, err := OpenState(ctx, withPath(c.State, "orch"))
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	if console.Identity() != state.Identity() || console.Identity().Schema != "orch" {
		t.Fatalf("same installation, different identities: %+v %+v", console.Identity(), state.Identity())
	}
	if err := closeOpenState(OpenState(ctx, c.State)); !errors.Is(err, ErrNotInitialized) {
		t.Fatalf("state on another schema of the same database: %v", err)
	}
	// Grants of these logins on a second installation would be objects outside
	// their baseline schema: the owner setup refuses to create it.
	if _, err := Setup(ctx, c.Owner, setupRoles(c)); !errors.Is(err, ErrSetupRefused) {
		t.Fatalf("second installation for the same logins: %v", err)
	}
}
