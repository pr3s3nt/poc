package postgres

import (
	"context"
	"github.com/jackc/pgx/v5"
	"strings"
	"testing"
)

func monitoringPrior(t *testing.T, priorSQL string) (string, string, Roles) {
	t.Helper()
	db := rolesOnly(t)
	ctx := context.Background()
	c, e := pgx.Connect(ctx, db.Owner)
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close(ctx)
	tx, e := c.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, priorSQL); e != nil {
		t.Fatal(e)
	}
	versions := []func(context.Context, pgx.Tx) error{func(ctx context.Context, tx pgx.Tx) error { return recordVersionOne(ctx, tx, digest(versionOneSQL)) }, recordVersionTwo, recordVersionThree, recordVersionFour, recordVersionFive, recordVersionSix, recordVersionSeven, recordVersionEight}
	if priorSQL == versionTenSQL {
		versions = append(versions, recordVersionNine, recordVersionTen)
	}
	for _, f := range versions {
		if e = f(ctx, tx); e != nil {
			t.Fatal(e)
		}
	}
	dump, e := catalogDump(ctx, tx, "public")
	if e != nil {
		t.Fatal(e)
	}
	priorVersion := "orchestrator-schema-v8"
	if priorSQL == versionTenSQL {
		priorVersion = "orchestrator-schema-v10"
	}
	if _, e = tx.Exec(ctx, `INSERT INTO schema_baseline(baseline,baseline_sha256,catalog_sha256,installation_id,schema_name) VALUES($1,$2,$3,'11111111-1111-4111-8111-111111111111','public')`, priorVersion, digest(priorSQL), digest(strings.Join(dump, "\n"))); e != nil {
		t.Fatal(e)
	}
	appInsert := `INSERT INTO applications(id,application_key,organization_id,name,version) VALUES('33333333-3333-4333-8333-333333333333','retained-app','22222222-2222-4222-8222-222222222222','Retained',7)`
	if priorSQL == versionEightSQL {
		appInsert = `INSERT INTO applications(id,application_key,organization_id,name,subdomain,version) VALUES('33333333-3333-4333-8333-333333333333','retained-app','22222222-2222-4222-8222-222222222222','Retained','retained',7)`
	}
	for _, q := range []string{
		`INSERT INTO organizations VALUES('22222222-2222-4222-8222-222222222222','retained-org','Retained')`,
		appInsert,
		`REVOKE ALL ON DATABASE ` + pgx.Identifier{db.Name}.Sanitize() + ` FROM PUBLIC`,
		`REVOKE ALL ON SCHEMA public FROM PUBLIC`,
		`REVOKE ALL ON ALL FUNCTIONS IN SCHEMA public FROM PUBLIC`,
		`GRANT CONNECT ON DATABASE ` + pgx.Identifier{db.Name}.Sanitize() + ` TO ` + pgx.Identifier{db.ConsoleRole}.Sanitize() + `,` + pgx.Identifier{db.StateRole}.Sanitize(),
		`GRANT USAGE ON SCHEMA public TO ` + pgx.Identifier{db.ConsoleRole}.Sanitize() + `,` + pgx.Identifier{db.StateRole}.Sanitize(),
	} {
		if _, e = tx.Exec(ctx, q); e != nil {
			t.Fatal(e)
		}
	}
	for role, name := range map[RuntimeRole]string{ConsoleRuntime: db.ConsoleRole, StateRuntime: db.StateRole} {
		for _, g := range grantsOf(role) {
			if priorSQL == versionEightSQL && (g.table == "endpoint_hosts" || g.table == "endpoint_paths") {
				continue
			}
			if g.table == "monitoring_connections" || g.table == "monitoring_bindings" {
				continue
			}
			q := `GRANT ` + g.privilege
			if len(g.columns) > 0 {
				q += ` (` + strings.Join(g.columns, ",") + `)`
			}
			q += ` ON ` + g.table + ` TO ` + pgx.Identifier{name}.Sanitize()
			if _, e = tx.Exec(ctx, q); e != nil {
				t.Fatal(e)
			}
		}
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	return db.Owner, db.Console, Roles{Console: db.ConsoleRole, State: db.StateRole}
}
func TestMonitoringUpgradePreservesPriorVersions(t *testing.T) {
	for name, priorSQL := range map[string]string{"v8": versionEightSQL, "v10": versionTenSQL} {
		t.Run(name, func(t *testing.T) {
			owner, console, roles := monitoringPrior(t, priorSQL)
			ctx := context.Background()
			before := scalar[string](t, owner, `SELECT installation_id::text||created_at::text FROM schema_baseline`)
			if _, e := Setup(ctx, owner, roles); e != nil {
				t.Fatal(e)
			}
			if after := scalar[string](t, owner, `SELECT installation_id::text||created_at::text FROM schema_baseline`); after != before {
				t.Fatal("installation identity changed")
			}
			if scalar[int](t, owner, `SELECT version FROM applications WHERE application_key='retained-app'`) != 7 {
				t.Fatal("retained app changed")
			}
			if scalar[int](t, owner, `SELECT count(*) FROM schema_upgrade_history`) != 11 {
				t.Fatal("missing migration receipt")
			}
			if _, e := Setup(ctx, owner, roles); e != nil {
				t.Fatal("repeat setup", e)
			}
			st, e := Open(ctx, console)
			if e != nil {
				t.Fatal(e)
			}
			st.Close()
			if scalar[bool](t, owner, `SELECT has_table_privilege('`+roles.State+`','monitoring_connections','SELECT')`) {
				t.Fatal("State runtime gained monitoring access")
			}
			if scalar[bool](t, owner, `SELECT has_schema_privilege('`+roles.Console+`','public','CREATE')`) {
				t.Fatal("Console gained DDL")
			}
		})
	}
}
func TestMonitoringUpgradeRollback(t *testing.T) {
	owner, _, roles := monitoringPrior(t, versionTenSQL)
	sqlOn(t, owner, `GRANT DELETE ON schema_baseline TO `+pgx.Identifier{roles.Console}.Sanitize())
	if _, e := Setup(context.Background(), owner, roles); e == nil {
		t.Fatal("unexpected setup success")
	}
	if scalar[string](t, owner, `SELECT baseline FROM schema_baseline`) != "orchestrator-schema-v10" || scalar[int](t, owner, `SELECT count(*) FROM schema_upgrade_history`) != 10 {
		t.Fatal("failed upgrade committed")
	}
	if scalar[bool](t, owner, `SELECT to_regclass('monitoring_connections') IS NOT NULL`) {
		t.Fatal("DDL was not rolled back")
	}
}
