package postgres

import (
	"context"
	"testing"
)

func TestWebPurgeUpgradeRollsBackOnUnexpectedGrant(t *testing.T) {
	ctx := context.Background()
	owner, console, roles := legacyDatabase(t, versionElevenSQL)
	sqlOn(t, owner, "GRANT UPDATE ON schema_baseline TO "+roles.Console)
	if _, err := Setup(ctx, owner, roles); err == nil {
		t.Fatal("upgrade accepted unexpected runtime grants")
	}
	if scalar[string](t, owner, "SELECT baseline FROM schema_baseline") != "orchestrator-schema-v11" ||
		scalar[int](t, owner, "SELECT max(version) FROM schema_upgrade_history") != 11 ||
		!scalar[bool](t, owner, "SELECT to_regclass('application_purge_policy') IS NULL") {
		t.Fatal("failed upgrade left schema changes")
	}
	sqlOn(t, owner, "REVOKE UPDATE ON schema_baseline FROM "+roles.Console)
	if changed, err := Setup(ctx, owner, roles); err != nil || !changed {
		t.Fatalf("upgrade retry = %v, %v", changed, err)
	}
	store, err := Open(ctx, console)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if enabled, err := store.WebApplicationPurgeEnabled(ctx); err != nil || enabled {
		t.Fatalf("purge after setup = %v, %v, expected disabled", enabled, err)
	}
	if outcome, err := store.ExecuteWebApplicationPurge(ctx, "retained-org", "retained-app", "33333333-3333-4333-8333-333333333333", "Retained", true); err != nil || outcome != "DISABLED" {
		t.Fatalf("direct runtime invocation = %s, %v", outcome, err)
	}
	for _, query := range []string{
		"SELECT has_function_privilege('" + roles.State + "','purge_one_application(text,text,uuid,text,boolean)','EXECUTE')",
		"SELECT has_function_privilege('" + roles.Console + "','application_purge_scope_matches(uuid)','EXECUTE')",
		"SELECT has_schema_privilege('" + roles.Console + "','public','CREATE')",
		"SELECT has_any_column_privilege('" + roles.Console + "','terraform_states','SELECT')",
		"SELECT has_table_privilege('" + roles.Console + "','application_purge_policy','UPDATE')",
	} {
		if scalar[bool](t, owner, query) {
			t.Fatal("unexpected database privilege")
		}
	}
	if scalar[string](t, owner, "SELECT id::text FROM applications WHERE application_key='retained-app'") != "33333333-3333-4333-8333-333333333333" {
		t.Fatal("retained app identity changed")
	}
}
