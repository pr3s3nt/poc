package postgres

import (
	"context"
	"testing"
)

func TestWebPurgeUpgradeRollsBackOnUnexpectedGrant(t *testing.T) {
	ctx := context.Background()
	owner, console, roles := legacyDatabase(t, versionElevenSQL)
	sqlOn(t, owner,
		`INSERT INTO environments(id,application_id,environment_key,name,environment_type,namespace_identity,version)
VALUES('77777777-7777-4777-8777-777777777777','33333333-3333-4333-8333-333333333333','staging','Staging','staging','retained-ns',3)`,
		`INSERT INTO terraform_module_bundles(id,source_id,module_id,revision,subdirectory,content_sha256,terraform_version,terraform_binary_sha256,provider_lock_digest,contract,provider_requirements,status,created_at,updated_at)
VALUES('88888888-8888-4888-8888-888888888888','platform','fixture-module','0123456789abcdef0123456789abcdef01234567','.',repeat('a',64),'1.9.8',repeat('b',64),repeat('c',64),'{}','{}','VERIFIED',now(),now())`,
		`INSERT INTO terraform_resource_instances(id,organization_id,application_id,environment_id,target_generation,descriptor,scope_type,scope_id,state_key,module_bundle_id,target_ref,physical_intent,created_by_run_id,status,version,created_at,updated_at)
VALUES('99999999-9999-4999-8999-999999999999','22222222-2222-4222-8222-222222222222','33333333-3333-4333-8333-333333333333','77777777-7777-4777-8777-777777777777',0,'db.default#shared.data','SHARED','retained-app.staging',repeat('d',64),'88888888-8888-4888-8888-888888888888','{}','{}','retained-run','READY',1,now(),now())`,
		`INSERT INTO terraform_states(resource_instance_id,organization_id,ciphertext,nonce,key_id,aad_revision,lineage,serial,version,updated_at)
VALUES('99999999-9999-4999-8999-999999999999','22222222-2222-4222-8222-222222222222',decode('cafe','hex'),decode(repeat('00',12),'hex'),'fixture-key',3,'retained-lineage',14,3,now())`,
	)
	const stateIdentity = `SELECT s.resource_instance_id||'/'||s.lineage||'/'||s.serial||'/'||encode(s.ciphertext,'hex')||'/'||i.state_key
FROM terraform_states s JOIN terraform_resource_instances i ON i.id=s.resource_instance_id`
	before := scalar[string](t, owner, stateIdentity)
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
	if scalar[string](t, owner, stateIdentity) != before {
		t.Fatal("retained state changed")
	}
	if scalar[string](t, owner, "SELECT id::text FROM applications WHERE application_key='retained-app'") != "33333333-3333-4333-8333-333333333333" {
		t.Fatal("retained app identity changed")
	}
}
