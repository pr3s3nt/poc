package postgres

import (
	"context"
	"errors"
	"sync"
	"testing"

	"orchestrator/internal/domain/endpoint"
	"orchestrator/internal/ports/persistence"

	"github.com/jackc/pgx/v5"
)

// populatedVersionNine seeds a retained v9 installation: two Environments, a
// whole-hostname reservation, a subdomain, a pending route flag and a draft
// still declaring the retired publicRoutes (ADR-037).
func populatedVersionNine(t *testing.T) (string, string, Roles) {
	t.Helper()
	owner, console, roles := legacyDatabase(t, versionNineSQL)
	ctx := context.Background()
	c, err := pgx.Connect(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close(ctx)
	for _, q := range []string{
		`INSERT INTO environments(id,application_id,environment_key,name,environment_type,namespace_identity,version,public_routes_pending) VALUES('90000000-0000-4000-8000-000000000001','33333333-3333-4333-8333-333333333333','staging','Staging','staging','retained-ns',3,true)`,
		`INSERT INTO environments(id,application_id,environment_key,name,environment_type,namespace_identity,version) VALUES('90000000-0000-4000-8000-000000000002','33333333-3333-4333-8333-333333333333','production','Production','production','retained-prod-ns',4)`,
		`INSERT INTO endpoint_reservations(hostname,organization_id,application_id,environment_id,target_generation,workload_id) VALUES('shop.apps.example.com','22222222-2222-4222-8222-222222222222','33333333-3333-4333-8333-333333333333','90000000-0000-4000-8000-000000000001',0,'web')`,
		`INSERT INTO workload_drafts(environment_id,workload_id,workload_document,state) VALUES('90000000-0000-4000-8000-000000000001','web','{"apiVersion":"orchestrator.dev/workload/v1","metadata":{"name":"web"},"containers":{"main":{"image":"nginx"}},"service":{"ports":{"http":{"port":80}},"publicRoutes":[{"path":"/","port":"http"}]}}','UPSERT')`,
	} {
		if _, err = c.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	return owner, console, roles
}

func TestRetirePublicRoutesUpgradeFromPopulatedVersionNine(t *testing.T) {
	owner, console, roles := populatedVersionNine(t)
	ctx := context.Background()
	preserved := map[string]string{}
	for table, columns := range map[string]string{
		"organizations": "id,organization_key,name", "applications": "id,application_key,organization_id,name,version,status",
		"environments": "id,application_id,environment_key,version,namespace_identity", "user_accounts": "*", "sessions": "*", "template_sources": "*",
	} {
		preserved[table] = scalar[string](t, owner, `SELECT COALESCE(string_agg(row_to_json(r)::text, ',' ORDER BY row_to_json(r)::text),'') FROM (SELECT `+columns+` FROM `+table+`) r`)
	}
	initialized, err := Setup(ctx, owner, roles)
	if err != nil || !initialized {
		t.Fatalf("upgrade: changed=%v err=%v", initialized, err)
	}
	if got := scalar[string](t, owner, `SELECT baseline FROM schema_baseline`); got != "orchestrator-schema-v11" {
		t.Fatal(got)
	}
	if scalar[string](t, owner, `SELECT installation_id::text FROM schema_baseline`) != "11111111-1111-4111-8111-111111111111" {
		t.Fatal("installation identity changed")
	}
	if scalar[int](t, owner, `SELECT count(*) FROM schema_upgrade_history`) != 11 {
		t.Fatal("history not advanced to 11")
	}
	for table, columns := range map[string]string{
		"organizations": "id,organization_key,name", "applications": "id,application_key,organization_id,name,version,status",
		"environments": "id,application_id,environment_key,version,namespace_identity", "user_accounts": "*", "sessions": "*", "template_sources": "*",
	} {
		if got := scalar[string](t, owner, `SELECT COALESCE(string_agg(row_to_json(r)::text, ',' ORDER BY row_to_json(r)::text),'') FROM (SELECT `+columns+` FROM `+table+`) r`); got != preserved[table] {
			t.Fatalf("%s changed during upgrade", table)
		}
	}
	// The v9 whole-hostname claim survives as the workload's '/' path.
	if got := scalar[string](t, owner, `SELECT h.hostname||'|'||h.environment_id||'|'||p.path||'|'||p.workload_id FROM endpoint_hosts h JOIN endpoint_paths p USING(hostname)`); got != "shop.apps.example.com|90000000-0000-4000-8000-000000000001|/|web" {
		t.Fatal(got)
	}
	for _, retired := range []string{`SELECT to_regclass('endpoint_reservations') IS NULL`,
		`SELECT NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='applications' AND column_name='subdomain')`,
		`SELECT NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='environments' AND column_name='public_routes_pending')`,
		`SELECT NOT (workload_document->'service' ? 'publicRoutes') FROM workload_drafts WHERE workload_id='web'`} {
		if !scalar[bool](t, owner, retired) {
			t.Fatal("retired structure remains:", retired)
		}
	}
	if !scalar[bool](t, owner, `SELECT workload_document->'service'->'ports' ? 'http' FROM workload_drafts WHERE workload_id='web'`) {
		t.Fatal("draft lost its Service ports")
	}
	if initialized, err = Setup(ctx, owner, roles); err != nil || initialized {
		t.Fatalf("repeat setup: changed=%v err=%v", initialized, err)
	}
	for _, table := range []string{"endpoint_hosts", "endpoint_paths"} {
		for _, privilege := range []string{"SELECT", "INSERT", "UPDATE", "DELETE"} {
			if !scalar[bool](t, owner, `SELECT has_table_privilege('`+roles.Console+`','`+table+`','`+privilege+`')`) {
				t.Fatal("console lacks", table, privilege)
			}
			if scalar[bool](t, owner, `SELECT has_table_privilege('`+roles.State+`','`+table+`','`+privilege+`')`) {
				t.Fatal("state gained", table, privilege)
			}
		}
	}
	c, err := pgx.Connect(ctx, console)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close(ctx)
	if _, err = c.Exec(ctx, `CREATE TABLE forbidden_ddl(id int)`); err == nil {
		t.Fatal("runtime acquired DDL")
	}
	st, err := Open(ctx, console)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	web := endpoint.Owner{Organization: "retained-org", Application: "retained-app", Environment: "staging", Workload: "web"}
	if err = st.ReserveEndpoint(ctx, endpoint.Claim{Hostname: "shop.apps.example.com", Path: "/"}, web); err != nil {
		t.Fatal("migrated claim is not the owner's", err)
	}
}

func TestRetirePublicRoutesUpgradeRollsBackOnGrantFailure(t *testing.T) {
	owner, _, roles := populatedVersionNine(t)
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
	if scalar[string](t, owner, `SELECT baseline FROM schema_baseline`) != "orchestrator-schema-v9" {
		t.Fatal("marker committed after failure")
	}
	if scalar[bool](t, owner, `SELECT to_regclass('endpoint_hosts') IS NOT NULL`) || !scalar[bool](t, owner, `SELECT to_regclass('endpoint_reservations') IS NOT NULL`) {
		t.Fatal("DDL committed after failure")
	}
	if scalar[string](t, owner, `SELECT subdomain FROM applications`) != "retained" {
		t.Fatal("subdomain dropped after failure")
	}
}

func TestEndpointLedgerSharesHostnamesByPath(t *testing.T) {
	owner, console, roles := populatedVersionNine(t)
	ctx := context.Background()
	if _, err := Setup(ctx, owner, roles); err != nil {
		t.Fatal(err)
	}
	st, err := Open(ctx, console)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	// Release the migrated claim to start from an empty ledger.
	if err = st.RetainEndpoints(ctx, endpoint.Owner{Organization: "retained-org", Application: "retained-app", Environment: "staging", Workload: "web"}, nil); err != nil {
		t.Fatal(err)
	}
	frontend := endpoint.Owner{Organization: "retained-org", Application: "retained-app", Environment: "staging", Workload: "frontend"}
	backend := frontend
	backend.Workload = "backend"
	production := frontend
	production.Environment = "production"
	root := endpoint.Claim{Hostname: "shop.apps.example.com", Path: "/"}
	api := endpoint.Claim{Hostname: "shop.apps.example.com", Path: "/api"}

	// Concurrent claims of one hostname from two Environments: one wins.
	var wg sync.WaitGroup
	results := make([]error, 8)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			o := frontend
			if i%2 == 1 {
				o = production
			}
			results[i] = st.ReserveEndpoint(ctx, root, o)
		}(i)
	}
	wg.Wait()
	winners := map[string]bool{}
	for i, err := range results {
		switch {
		case err == nil:
			winners[map[bool]string{true: "staging", false: "production"}[i%2 == 0]] = true
		case errors.Is(err, persistence.ErrHostnameTaken):
		default:
			t.Fatal(err)
		}
	}
	if len(winners) != 1 {
		t.Fatal("hostname owners", winners)
	}
	if !winners["staging"] {
		// Normalize the rest of the scenario to a staging-held hostname.
		if err = st.RetainEndpoints(ctx, production, nil); err != nil {
			t.Fatal(err)
		}
		if err = st.ReserveEndpoint(ctx, root, frontend); err != nil {
			t.Fatal(err)
		}
	}
	// Same Environment: another workload shares the hostname on another path.
	if err = st.ReserveEndpoint(ctx, api, backend); err != nil {
		t.Fatal("path sharing refused", err)
	}
	if err = st.CheckEndpoint(ctx, root, backend); !errors.Is(err, persistence.ErrHostnameTaken) {
		t.Fatal("same path claimed by two workloads", err)
	}
	if err = st.ReserveEndpoint(ctx, root, backend); !errors.Is(err, persistence.ErrHostnameTaken) {
		t.Fatal("same path reserved by two workloads", err)
	}
	if err = st.CheckEndpoint(ctx, endpoint.Claim{Hostname: api.Hostname, Path: "/other"}, production); !errors.Is(err, persistence.ErrHostnameTaken) {
		t.Fatal("another Environment used a held hostname", err)
	}
	if err = st.ReserveEndpoint(ctx, root, frontend); err != nil {
		t.Fatal("same-owner retry", err)
	}
	// Releasing one workload keeps the hostname while another path remains.
	if err = st.RetainEndpoints(ctx, frontend, nil); err != nil {
		t.Fatal(err)
	}
	if n := scalar[int](t, owner, `SELECT count(*) FROM endpoint_hosts`); n != 1 {
		t.Fatal("hostname released while a path remained", n)
	}
	if err = st.CheckEndpoint(ctx, root, production); !errors.Is(err, persistence.ErrHostnameTaken) {
		t.Fatal("hostname moved while still held", err)
	}
	if err = st.RetainEndpoints(ctx, backend, []endpoint.Claim{}); err != nil {
		t.Fatal(err)
	}
	if n := scalar[int](t, owner, `SELECT count(*) FROM endpoint_hosts`); n != 0 {
		t.Fatal("empty hostname not released", n)
	}
	if err = st.ReserveEndpoint(ctx, root, endpoint.Owner{Organization: "retained-org", Application: "missing", Environment: "staging", Workload: "web"}); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatal("unknown owner", err)
	}
	if err = st.ReserveEndpoint(ctx, endpoint.Claim{Hostname: "Not_A_Host", Path: "/"}, frontend); err == nil {
		t.Fatal("invalid hostname accepted")
	}
}
