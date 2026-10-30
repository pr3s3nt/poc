package postgres

import (
	"bytes"
	"context"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence/persistencetest"
)

// legacyDatabase creates populated, frozen release data with old runtime grants.
// It never touches a retained installation.
func legacyDatabase(t *testing.T, sql string) (string, string, Roles) {
	t.Helper()
	db := rolesOnly(t)
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, db.Owner)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, sql); err != nil {
		t.Fatal(err)
	}
	catalog, err := catalogDump(ctx, tx, "public")
	if err != nil {
		t.Fatal(err)
	}
	identity := "orchestrator-fresh-baseline-v02"
	if sql == versionOneSQL {
		identity = "orchestrator-schema-v1"
		if err = recordVersionOne(ctx, tx, digest(versionOneSQL)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO schema_baseline(baseline,baseline_sha256,catalog_sha256,installation_id,schema_name) VALUES($3,$1,$2,'11111111-1111-4111-8111-111111111111','public')`, digest(sql), digest(strings.Join(catalog, "\n")), identity); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO organizations VALUES('22222222-2222-4222-8222-222222222222','retained-org','Retained')`,
		`INSERT INTO applications(id,application_key,organization_id,name,subdomain,version) VALUES('33333333-3333-4333-8333-333333333333','retained-app','22222222-2222-4222-8222-222222222222','Retained','retained',7)`,
		`INSERT INTO user_accounts(id,organization_id,username,password_hash,role,status) VALUES('44444444-4444-4444-8444-444444444444','22222222-2222-4222-8222-222222222222','retained-user','retained-password-hash','ADMIN','ACTIVE')`,
		`INSERT INTO sessions(id,user_account_id,token_hash,expires_at) VALUES('55555555-5555-4555-8555-555555555555','44444444-4444-4444-8444-444444444444','retained-token-hash',now()+interval '1 hour')`,
		`INSERT INTO render_bundles(id,organization_id,source_url,source_path,source_revision,content_sha256,engine_contract,renderer_version,renderer_binary_sha256,contract,created_at) VALUES('66666666-6666-4666-8666-666666666666','22222222-2222-4222-8222-222222222222','https://example.invalid/retained.git','.',repeat('a',40),repeat('b',64),'git-render-v1','0.15.0',repeat('c',64),'{}',now())`,
		`REVOKE ALL ON DATABASE ` + pgx.Identifier{db.Name}.Sanitize() + ` FROM PUBLIC`,
		`REVOKE ALL ON SCHEMA public FROM PUBLIC`,
		`REVOKE ALL ON ALL FUNCTIONS IN SCHEMA public FROM PUBLIC`,
		`GRANT CONNECT ON DATABASE ` + pgx.Identifier{db.Name}.Sanitize() + ` TO ` + pgx.Identifier{db.ConsoleRole}.Sanitize() + `,` + pgx.Identifier{db.StateRole}.Sanitize(),
		`GRANT USAGE ON SCHEMA public TO ` + pgx.Identifier{db.ConsoleRole}.Sanitize() + `,` + pgx.Identifier{db.StateRole}.Sanitize(),
	} {
		if _, err = tx.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	for role, name := range map[RuntimeRole]string{ConsoleRuntime: db.ConsoleRole, StateRuntime: db.StateRole} {
		for _, g := range grantsOf(role) {
			if g.table == "platform_git_settings" || (g.table == "schema_upgrade_history" && sql != versionOneSQL) {
				continue
			}
			// Source Applications use whole-table grants already, no new-column backfill privileges.
			statement := `GRANT ` + g.privilege
			if len(g.columns) > 0 {
				statement += ` (` + strings.Join(g.columns, ",") + `)`
			}
			statement += ` ON ` + g.table + ` TO ` + pgx.Identifier{name}.Sanitize()
			if _, err = tx.Exec(ctx, statement); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	// Runtime URLs are kept in the test only; never logged.
	t.Run("old runtime requires upgrade", func(t *testing.T) {
		if _, err := Open(ctx, db.Console); err != ErrUpgradeRequired {
			t.Fatalf("expected upgrade required, got %v", err)
		}
	})
	return db.Owner, db.Console, setupRoles(db)
}

func TestUpgradeSupportedPopulatedSources(t *testing.T) {
	for name, source := range map[string]string{"staging-1516dec": sourceOldSQL, "preview-2a111db": sourceCurrentSQL, "version-1": versionOneSQL} {
		t.Run(name, func(t *testing.T) {
			owner, console, roles := legacyDatabase(t, source)
			stamp := scalar[string](t, owner, `SELECT created_at::text FROM schema_baseline`)
			var wg sync.WaitGroup
			errs := make(chan error, 2)
			for i := 0; i < 2; i++ {
				wg.Add(1)
				go func() { defer wg.Done(); _, err := Setup(context.Background(), owner, roles); errs <- err }()
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				if err != nil {
					t.Fatal(err)
				}
			}
			if n := scalar[int](t, owner, `SELECT count(*) FROM schema_upgrade_history`); n != 2 {
				t.Fatal(n)
			}
			if got := scalar[string](t, owner, `SELECT installation_id::text FROM schema_baseline`); got != "11111111-1111-4111-8111-111111111111" {
				t.Fatal("installation changed")
			}
			if scalar[string](t, owner, `SELECT created_at::text FROM schema_baseline`) != stamp {
				t.Fatal("creation time changed")
			}
			if n := scalar[int](t, owner, `SELECT count(*) FROM applications WHERE application_key='retained-app' AND version=7 AND status='ACTIVE' AND deletion='{}'::jsonb`); n != 1 {
				t.Fatal("retained application changed")
			}
			if changed, err := Setup(context.Background(), owner, roles); err != nil || changed {
				t.Fatalf("repeat = %v %v", changed, err)
			}
			store, err := Open(context.Background(), console)
			if err != nil {
				t.Fatal(err)
			}
			account, err := store.GetUserAccountByUsername(context.Background(), "retained-user")
			if err != nil || account.PasswordHash != "retained-password-hash" {
				t.Fatal("retained login data changed")
			}
			if _, err = store.GetSessionByTokenHash(context.Background(), "retained-token-hash"); err != nil {
				t.Fatal("retained session missing")
			}
			if n := scalar[int](t, owner, `SELECT count(*) FROM render_bundles WHERE source_kind='git' AND source_url='https://example.invalid/retained.git'`); n != 1 {
				t.Fatal("retained render source changed")
			}
			persistencetest.Registration(t, store)
			fx := persistencetest.WorkloadSnapshots(t, store)
			store.Close()
			reopened, err := Open(context.Background(), console)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			persistencetest.AssertReloaded(t, reopened, fx)
		})
	}
}

func TestActiveBaselineMatchesFrozenVersion(t *testing.T) {
	db := rolesOnly(t)
	if _, err := Setup(context.Background(), db.Owner, setupRoles(db)); err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(context.Background(), db.Owner)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(context.Background())
	tx, err := conn.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if err := compareSchemaSQL(context.Background(), tx, "public", versionSQL); err != nil {
		t.Fatal(err)
	}
}

func TestUpgradeRollbackOnGrantFailure(t *testing.T) {
	owner, _, roles := legacyDatabase(t, sourceOldSQL)
	// A valid ordinary role with wrong privileges is refused at final verification.
	conn, err := pgx.Connect(context.Background(), owner)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(context.Background())
	if _, err = conn.Exec(context.Background(), `GRANT DELETE ON schema_baseline TO `+pgx.Identifier{roles.Console}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	if _, err = Setup(context.Background(), owner, roles); err == nil {
		t.Fatal("expected grant verification failure")
	}
	if got := scalar[string](t, owner, `SELECT baseline_sha256 FROM schema_baseline`); got != digest(sourceOldSQL) {
		t.Fatal("marker committed after failure")
	}
	if scalar[bool](t, owner, `SELECT to_regclass('schema_upgrade_history') IS NOT NULL`) {
		t.Fatal("DDL committed after failure")
	}
	if n := scalar[int](t, owner, `SELECT count(*) FROM applications`); n != 1 {
		t.Fatal("data changed")
	}
}

func TestUpgradePreservesStateAndRuntimeIsolation(t *testing.T) {
	for name, source := range map[string]string{"staging": sourceOldSQL, "preview": sourceCurrentSQL, "version-1": versionOneSQL} {
		t.Run(name, func(t *testing.T) {
			f := newStateFixture(t)
			f.console.Close()
			f.state.Close()
			ctx := context.Background()
			owner := connectAs(t, f.db.Owner)
			tx, err := owner.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			if _, err = tx.Exec(ctx, `DROP TABLE platform_git_settings`); err != nil {
				t.Fatal(err)
			}
			if source != versionOneSQL {
				if _, err = tx.Exec(ctx, `DROP TABLE schema_upgrade_history`); err != nil {
					t.Fatal(err)
				}
			}
			if source == sourceOldSQL {
				for _, q := range []string{
					`DROP INDEX applications_subdomain_key`,
					`DROP INDEX applications_organization_lower_name_key`,
					`ALTER TABLE applications DROP COLUMN status, DROP COLUMN deletion, ADD CONSTRAINT applications_subdomain_key UNIQUE(subdomain), ADD CONSTRAINT applications_organization_id_name_key UNIQUE(organization_id,name)`,
					`CREATE UNIQUE INDEX applications_organization_lower_name_key ON applications(organization_id,lower(name))`,
					`ALTER TABLE render_bundles DROP CONSTRAINT render_bundles_check, DROP COLUMN source_kind, DROP CONSTRAINT render_bundles_source_url_check, ADD CONSTRAINT render_bundles_source_url_check CHECK (source_url ~ '^https://' AND length(source_url)<=2048)`,
				} {
					if _, err = tx.Exec(ctx, q); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err = compareSchemaSQL(ctx, tx, "public", source); err != nil {
				t.Fatal(err)
			}
			catalog, err := catalogDump(ctx, tx, "public")
			if err != nil {
				t.Fatal(err)
			}
			identity := "orchestrator-fresh-baseline-v02"
			if source == versionOneSQL {
				identity = "orchestrator-schema-v1"
				if _, err = tx.Exec(ctx, `DELETE FROM schema_upgrade_history WHERE version=2`); err != nil {
					t.Fatal(err)
				}
			}
			if _, err = tx.Exec(ctx, `UPDATE schema_baseline SET baseline=$3,baseline_sha256=$1,catalog_sha256=$2`, digest(source), digest(strings.Join(catalog, "\n")), identity); err != nil {
				t.Fatal(err)
			}
			if err = tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			before := retainedFingerprints(t, f.db.Owner)
			if _, err = Setup(ctx, f.db.Owner, setupRoles(f.db)); err != nil {
				t.Fatal(err)
			}
			after := retainedFingerprints(t, f.db.Owner)
			for table, hash := range before {
				if after[table] != hash {
					t.Errorf("retained rows changed in %s", table)
				}
			}
			console, err := Open(ctx, f.db.Console)
			if err != nil {
				t.Fatal(err)
			}
			defer console.Close()
			state, err := OpenState(ctx, f.db.State)
			if err != nil {
				t.Fatal(err)
			}
			defer state.Close()
			if state.Identity() != console.Identity() {
				t.Fatal("installation identity differs")
			}
			denied(t, connectAs(t, f.db.Console), "history write", `DELETE FROM schema_upgrade_history`)
			denied(t, connectAs(t, f.db.State), "history write", `UPDATE schema_upgrade_history SET version=3 WHERE version=2`)
			denied(t, connectAs(t, f.db.State), "Git credentials metadata", `SELECT credential_ref FROM platform_git_settings`)
			denied(t, connectAs(t, f.db.Console), "private ciphertext", `SELECT ciphertext FROM terraform_states`)
		})
	}
}

func retainedFingerprints(t *testing.T, url string) map[string]string {
	t.Helper()
	conn := connectAs(t, url)
	rows, err := conn.Query(context.Background(), `SELECT tablename FROM pg_tables WHERE schemaname='public' AND tablename NOT IN ('schema_baseline','schema_upgrade_history')`)
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for rows.Next() {
		var table string
		if err = rows.Scan(&table); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, table)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	result := map[string]string{}
	for _, table := range tables {
		expr := `to_jsonb(t)`
		if table == "applications" {
			expr += ` - 'status' - 'deletion'`
		}
		if table == "render_bundles" {
			expr += ` - 'source_kind'`
		}
		var hash string
		q := `SELECT md5(COALESCE(jsonb_agg(x ORDER BY x::text)::text,'')) FROM (SELECT ` + expr + ` AS x FROM ` + pgx.Identifier{table}.Sanitize() + ` t) data`
		if err = conn.QueryRow(context.Background(), q).Scan(&hash); err != nil {
			t.Fatal(err)
		}
		result[table] = hash
	}
	return result
}

func TestUpgradeRejectsUnknownOrDamagedSource(t *testing.T) {
	for name, mutation := range map[string]string{
		"unknown":        `UPDATE schema_baseline SET baseline_sha256=repeat('0',64)`,
		"drift":          `ALTER TABLE applications ADD COLUMN unexpected text`,
		"forged-catalog": `ALTER TABLE applications ADD COLUMN unexpected text`,
	} {
		t.Run(name, func(t *testing.T) {
			owner, _, roles := legacyDatabase(t, sourceOldSQL)
			conn := connectAs(t, owner)
			if _, err := conn.Exec(context.Background(), mutation); err != nil {
				t.Fatal(err)
			}
			if name == "forged-catalog" {
				tx, err := conn.Begin(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				catalog, err := catalogDump(context.Background(), tx, "public")
				if err != nil {
					t.Fatal(err)
				}
				if _, err = tx.Exec(context.Background(), `UPDATE schema_baseline SET catalog_sha256=$1`, digest(strings.Join(catalog, "\n"))); err != nil {
					t.Fatal(err)
				}
				if err = tx.Commit(context.Background()); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := Setup(context.Background(), owner, roles); err == nil {
				t.Fatal("unexpected adoption of unsupported schema")
			}
			if scalar[bool](t, owner, `SELECT to_regclass('schema_upgrade_history') IS NOT NULL`) {
				t.Fatal("DDL changed on rejection")
			}
		})
	}
}

func TestBaselineCommentChangesDoNotRequireUpgrade(t *testing.T) {
	original := baselineSQL
	defer func() { baselineSQL = original }()
	baselineSQL = "-- formatting-only change\n\n" + original
	db := rolesOnly(t)
	if _, err := Setup(context.Background(), db.Owner, setupRoles(db)); err != nil {
		t.Fatal(err)
	}
	baselineSQL = original + "\n-- another comment\n"
	if changed, err := Setup(context.Background(), db.Owner, setupRoles(db)); err != nil || changed {
		t.Fatalf("comment-only update = %v %v", changed, err)
	}
	store, err := Open(context.Background(), db.Console)
	if err != nil {
		t.Fatal(err)
	}
	store.Close()
}

func TestUpgradeHistoryTamperingIsRejected(t *testing.T) {
	for name, mutation := range map[string]string{
		"checksum":       `UPDATE schema_upgrade_history SET migration_sha256=repeat('0',64)`,
		"future-version": `UPDATE schema_upgrade_history SET version=3 WHERE version=2`,
		"source":         `UPDATE schema_upgrade_history SET source_sha256=repeat('0',64)`,
		"missing":        `DELETE FROM schema_upgrade_history`,
	} {
		t.Run(name, func(t *testing.T) {
			db := freshDatabase(t)
			sqlOn(t, db.Admin, mutation)
			if _, err := Open(context.Background(), db.Console); err == nil {
				t.Fatal("runtime accepted altered history")
			}
			if _, err := Setup(context.Background(), db.Owner, setupRoles(db)); err == nil {
				t.Fatal("owner accepted altered history")
			}
		})
	}
}

func TestUpgradeBackupRestore(t *testing.T) {
	ownerURL, _, roles := legacyDatabase(t, sourceOldSQL)
	source, err := url.Parse(ownerURL)
	if err != nil {
		t.Fatal(err)
	}
	container := os.Getenv("ORCHESTRATOR_POSTGRES_TEST_CONTAINER")
	runTool := func(tool, database string, input []byte, args ...string) []byte {
		t.Helper()
		var cmd *exec.Cmd
		if container != "" {
			dockerArgs := []string{"exec", "-i", container, tool, "-U", source.User.Username(), "-d", database}
			cmd = exec.Command("docker", append(dockerArgs, args...)...)
		} else {
			if _, err := exec.LookPath(tool); err != nil {
				t.Fatal("backup/restore test requires PostgreSQL client tools or ORCHESTRATOR_POSTGRES_TEST_CONTAINER")
			}
			cmd = exec.Command(tool, args...)
			password, _ := source.User.Password()
			cmd.Env = append(os.Environ(), "PGHOST="+source.Hostname(), "PGPORT="+source.Port(), "PGUSER="+source.User.Username(), "PGPASSWORD="+password, "PGDATABASE="+database, "PGSSLMODE=disable")
		}
		cmd.Stdin = bytes.NewReader(input)
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("%s failed; private output omitted", tool)
		}
		return output
	}
	backup := runTool("pg_dump", strings.TrimPrefix(source.Path, "/"), nil, "-Fc")
	runTool("pg_restore", strings.TrimPrefix(source.Path, "/"), backup, "--list")
	before := retainedFingerprints(t, ownerURL)
	if _, err := Setup(context.Background(), ownerURL, roles); err != nil {
		t.Fatal(err)
	}
	admin := connectAs(t, persistencetest.PostgresTestURL(t))
	restoredName := "restore_" + strings.ReplaceAll(ids.New(), "-", "")
	if _, err = admin.Exec(context.Background(), `CREATE DATABASE `+pgx.Identifier{restoredName}.Sanitize()+` OWNER `+pgx.Identifier{source.User.Username()}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, err := admin.Exec(context.Background(), `DROP DATABASE `+pgx.Identifier{restoredName}.Sanitize()+` WITH (FORCE)`)
		if err != nil {
			t.Error("restore fixture cleanup failed")
		}
	})
	for _, q := range []string{`REVOKE ALL ON DATABASE ` + pgx.Identifier{restoredName}.Sanitize() + ` FROM PUBLIC`, `GRANT CONNECT ON DATABASE ` + pgx.Identifier{restoredName}.Sanitize() + ` TO ` + pgx.Identifier{roles.Console}.Sanitize() + `,` + pgx.Identifier{roles.State}.Sanitize()} {
		if _, err = admin.Exec(context.Background(), q); err != nil {
			t.Fatal(err)
		}
	}
	runTool("pg_restore", restoredName, backup, "--no-owner", "--exit-on-error")
	source.Path = "/" + restoredName
	restoredURL := source.String()
	if scalar[string](t, restoredURL, `SELECT baseline_sha256 FROM schema_baseline`) != digest(sourceOldSQL) {
		t.Fatal("backup did not restore the source version")
	}
	after := retainedFingerprints(t, restoredURL)
	for table, hash := range before {
		if after[table] != hash {
			t.Errorf("restored data differs in %s", table)
		}
	}
	if _, err = Setup(context.Background(), restoredURL, roles); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeHistoryGrantFailureIsClassified(t *testing.T) {
	db := freshDatabase(t)
	sqlOn(t, db.Admin, `REVOKE SELECT ON schema_upgrade_history FROM `+pgx.Identifier{db.ConsoleRole}.Sanitize())
	if _, err := Open(context.Background(), db.Console); !errors.Is(err, ErrRuntimeRole) {
		t.Fatalf("expected runtime grant error, got %v", err)
	}
}

func TestVersionTwoRollbackPreservesVersionOne(t *testing.T) {
	owner, _, roles := legacyDatabase(t, versionOneSQL)
	sqlOn(t, owner, `GRANT DELETE ON schema_baseline TO `+pgx.Identifier{roles.Console}.Sanitize())
	if _, err := Setup(context.Background(), owner, roles); err == nil {
		t.Fatal("expected grant verification failure")
	}
	if scalar[string](t, owner, `SELECT baseline_sha256 FROM schema_baseline`) != digest(versionOneSQL) {
		t.Fatal("source marker changed")
	}
	if scalar[bool](t, owner, `SELECT to_regclass('platform_git_settings') IS NOT NULL`) {
		t.Fatal("migration DDL was committed")
	}
	if scalar[int](t, owner, `SELECT count(*) FROM schema_upgrade_history`) != 1 {
		t.Fatal("history changed")
	}
	if scalar[int](t, owner, `SELECT count(*) FROM applications WHERE version=7`) != 1 {
		t.Fatal("retained data changed")
	}
}
