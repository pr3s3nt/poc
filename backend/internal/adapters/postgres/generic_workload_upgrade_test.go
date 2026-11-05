package postgres

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestGenericWorkloadUpgradePreservesInstallationAndRuntimeGrants(t *testing.T) {
	owner, runtime, roles := legacyDatabase(t, versionSevenSQL)
	ctx := context.Background()

	prior, err := pgx.Connect(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`INSERT INTO connections(id,organization_id,connection_key,name,kind,authentication_type,config,secret_ref,status,verification) VALUES('70000000-0000-4000-8000-000000000001','22222222-2222-4222-8222-222222222222','retained-cluster','Retained cluster','KUBERNETES','KUBECONFIG','{}','opaque-reference','READY','{}')`,
		`INSERT INTO resource_types(id,organization_id,resource_type_key,input_schema,output_schema) VALUES('70000000-0000-4000-8000-000000000002','22222222-2222-4222-8222-222222222222','redis','[]','[]')`,
		`INSERT INTO resource_definitions(id,organization_id,definition_key,resource_type_id,definition_name,driver_type,driver_inputs,provision_rules) VALUES('70000000-0000-4000-8000-000000000003','22222222-2222-4222-8222-222222222222','retained-terraform','70000000-0000-4000-8000-000000000002','Retained Terraform','terraform','{}','{}')`,
		`INSERT INTO environments(id,application_id,environment_key,name,environment_type,namespace_identity,version) VALUES('70000000-0000-4000-8000-000000000004','33333333-3333-4333-8333-333333333333','staging','Staging','staging','retained-ns',3)`,
		`INSERT INTO environment_definition_selections(environment_id,revision,document) VALUES('70000000-0000-4000-8000-000000000004',4,'{"revision":4,"desired":[{"driverType":"score-k8s","key":"old"},{"driverType":"terraform","key":"retained"}],"baseline":[],"applied":[]}')`,
		`INSERT INTO terraform_module_bundles(id,source_id,module_id,revision,subdirectory,content_sha256,terraform_version,terraform_binary_sha256,provider_lock_digest,contract,provider_requirements,status,created_at,updated_at) VALUES('70000000-0000-4000-8000-000000000005','platform','redis','0123456789abcdef0123456789abcdef01234567','.',repeat('a',64),'1.9.8',repeat('b',64),repeat('c',64),'{}','{}','VERIFIED',now(),now())`,
		`INSERT INTO terraform_resource_instances(id,organization_id,application_id,environment_id,target_generation,descriptor,scope_type,scope_id,state_key,module_bundle_id,target_ref,physical_intent,created_by_run_id,status,version,created_at,updated_at) VALUES('70000000-0000-4000-8000-000000000006','22222222-2222-4222-8222-222222222222','33333333-3333-4333-8333-333333333333','70000000-0000-4000-8000-000000000004',0,'redis.default#shared.cache','SHARED','cache',repeat('d',64),'70000000-0000-4000-8000-000000000005','{}','{}','retained-run','READY',5,now(),now())`,
		`INSERT INTO terraform_states(resource_instance_id,organization_id,ciphertext,nonce,key_id,aad_revision,lineage,serial,version,updated_at) VALUES('70000000-0000-4000-8000-000000000006','22222222-2222-4222-8222-222222222222',decode('aabbcc','hex'),decode(repeat('00',12),'hex'),'retained-key',2,'retained-lineage',4,2,now())`,
	} {
		if _, err = prior.Exec(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	prior.Close(ctx)
	preserved := map[string]string{}
	for _, table := range []string{"connections", "resource_definitions", "terraform_resource_instances", "terraform_states"} {
		preserved[table] = scalar[string](t, owner, `SELECT row_to_json(record)::text FROM `+table+` record LIMIT 1`)
	}
	initialized, err := Setup(ctx, owner, roles)
	if err != nil {
		t.Fatal(err)
	}
	if !initialized {
		t.Fatal("upgrade not executed")
	}
	if scalar[string](t, owner, `SELECT installation_id::text FROM schema_baseline`) != "11111111-1111-4111-8111-111111111111" {
		t.Fatal("installation changed")
	}
	if scalar[int](t, owner, `SELECT version FROM applications WHERE application_key='retained-app'`) != 7 {
		t.Fatal("application changed")
	}
	for table, want := range preserved {
		if got := scalar[string](t, owner, `SELECT row_to_json(record)::text FROM `+table+` record LIMIT 1`); got != want {
			t.Fatalf("%s changed during workload upgrade", table)
		}
	}
	if scalar[int](t, owner, `SELECT count(*) FROM template_sources`) != 0 {
		t.Fatal("obsolete source retained")
	}
	if scalar[int](t, owner, `SELECT revision FROM environment_definition_selections`) != 5 || scalar[int](t, owner, `SELECT (document->>'revision')::int FROM environment_definition_selections`) != 5 {
		t.Fatal("selection revision disagrees with its persisted document")
	}
	if scalar[string](t, owner, `SELECT document->'desired'->0->>'key' FROM environment_definition_selections`) != "retained" || scalar[int](t, owner, `SELECT jsonb_array_length(document->'desired') FROM environment_definition_selections`) != 1 {
		t.Fatal("retired workload selection was not removed independently of Terraform")
	}
	if initialized, err = Setup(ctx, owner, roles); err != nil || initialized {
		t.Fatalf("repeat setup: changed=%v err=%v", initialized, err)
	}
	st, err := Open(ctx, runtime)
	if err != nil {
		t.Fatal(err)
	}
	st.Close()
	c, err := pgx.Connect(ctx, runtime)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close(ctx)
	if _, err = c.Exec(ctx, `CREATE TABLE forbidden_ddl(id int)`); err == nil {
		t.Fatal("runtime acquired DDL")
	}
	if scalar[int](t, owner, `SELECT max(version) FROM schema_upgrade_history`) != 9 {
		t.Fatal("history not advanced")
	}
}

func TestGenericWorkloadUpgradeRollsBackCleanupAndDDL(t *testing.T) {
	owner, _, roles := legacyDatabase(t, versionSevenSQL)
	ctx := context.Background()
	c, err := pgx.Connect(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close(ctx)
	if _, err = c.Exec(ctx, `GRANT DELETE ON schema_baseline TO `+pgx.Identifier{roles.Console}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	if _, err = Setup(ctx, owner, roles); err == nil {
		t.Fatal("invalid runtime privileges accepted")
	}
	if scalar[string](t, owner, `SELECT baseline FROM schema_baseline`) != "orchestrator-schema-v7" {
		t.Fatal("marker committed after failure")
	}
	if scalar[int](t, owner, `SELECT count(*) FROM render_bundles`) != 1 {
		t.Fatal("obsolete source cleanup committed after failure")
	}
	if scalar[bool](t, owner, `SELECT to_regclass('template_sources') IS NOT NULL`) {
		t.Fatal("DDL committed after failure")
	}
}
