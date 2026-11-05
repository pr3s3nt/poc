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

// populatedVersionEight seeds a retained v8 installation with an Environment.
func populatedVersionEight(t *testing.T) (string, string, Roles) {
	t.Helper()
	owner, console, roles := legacyDatabase(t, versionEightSQL)
	ctx := context.Background()
	c, err := pgx.Connect(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close(ctx)
	for _, q := range []string{
		`INSERT INTO environments(id,application_id,environment_key,name,environment_type,namespace_identity,version) VALUES('90000000-0000-4000-8000-000000000001','33333333-3333-4333-8333-333333333333','staging','Staging','staging','retained-ns',3)`,
		`INSERT INTO environments(id,application_id,environment_key,name,environment_type,namespace_identity,version) VALUES('90000000-0000-4000-8000-000000000002','33333333-3333-4333-8333-333333333333','production','Production','production','retained-prod-ns',4)`,
	} {
		if _, err = c.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	return owner, console, roles
}

func TestEndpointUpgradeFromPopulatedVersionEight(t *testing.T) {
	owner, console, roles := populatedVersionEight(t)
	ctx := context.Background()
	preserved := map[string]string{}
	for _, table := range []string{"organizations", "applications", "environments", "user_accounts", "sessions", "template_sources"} {
		preserved[table] = scalar[string](t, owner, `SELECT COALESCE(string_agg(row_to_json(record)::text, ',' ORDER BY row_to_json(record)::text),'') FROM `+table+` record`)
	}
	initialized, err := Setup(ctx, owner, roles)
	if err != nil || !initialized {
		t.Fatalf("upgrade: changed=%v err=%v", initialized, err)
	}
	if got := scalar[string](t, owner, `SELECT baseline FROM schema_baseline`); got != "orchestrator-schema-v9" {
		t.Fatal(got)
	}
	if scalar[string](t, owner, `SELECT installation_id::text FROM schema_baseline`) != "11111111-1111-4111-8111-111111111111" {
		t.Fatal("installation identity changed")
	}
	if scalar[int](t, owner, `SELECT max(version) FROM schema_upgrade_history`) != 9 || scalar[int](t, owner, `SELECT count(*) FROM schema_upgrade_history`) != 9 {
		t.Fatal("history not advanced to 9")
	}
	for table, want := range preserved {
		if got := scalar[string](t, owner, `SELECT COALESCE(string_agg(row_to_json(record)::text, ',' ORDER BY row_to_json(record)::text),'') FROM `+table+` record`); got != want {
			t.Fatalf("%s changed during endpoint upgrade", table)
		}
	}
	// Repeated owner setup is a verified no-op.
	if initialized, err = Setup(ctx, owner, roles); err != nil || initialized {
		t.Fatalf("repeat setup: changed=%v err=%v", initialized, err)
	}
	// Console owns reservation CRUD; State has no access; runtime has no DDL.
	for _, privilege := range []string{"SELECT", "INSERT", "UPDATE", "DELETE"} {
		if !scalar[bool](t, owner, `SELECT has_table_privilege('`+roles.Console+`','endpoint_reservations','`+privilege+`')`) {
			t.Fatal("console lacks", privilege)
		}
		if scalar[bool](t, owner, `SELECT has_table_privilege('`+roles.State+`','endpoint_reservations','`+privilege+`')`) {
			t.Fatal("state gained", privilege)
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
	staging := endpoint.Owner{Organization: "retained-org", Application: "retained-app", Environment: "staging", Generation: 0, Workload: "web"}
	if err = st.ReserveEndpoint(ctx, "shop.apps.example.com", staging); err != nil {
		t.Fatal(err)
	}
}

func TestEndpointUpgradeRollsBackOnGrantFailure(t *testing.T) {
	owner, _, roles := populatedVersionEight(t)
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
	if scalar[string](t, owner, `SELECT baseline FROM schema_baseline`) != "orchestrator-schema-v8" {
		t.Fatal("marker committed after failure")
	}
	if scalar[bool](t, owner, `SELECT to_regclass('endpoint_reservations') IS NOT NULL`) {
		t.Fatal("DDL committed after failure")
	}
	if scalar[int](t, owner, `SELECT count(*) FROM schema_upgrade_history`) != 8 {
		t.Fatal("history committed after failure")
	}
}

func TestEndpointReservationsArbitrateOwners(t *testing.T) {
	owner, console, roles := populatedVersionEight(t)
	ctx := context.Background()
	if _, err := Setup(ctx, owner, roles); err != nil {
		t.Fatal(err)
	}
	st, err := Open(ctx, console)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	staging := endpoint.Owner{Organization: "retained-org", Application: "retained-app", Environment: "staging", Workload: "web"}
	production := staging
	production.Environment = "production"
	const host = "shop.apps.example.com"

	// Concurrent claims from two owners: exactly one wins.
	var wg sync.WaitGroup
	results := make([]error, 8)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			o := staging
			if i%2 == 1 {
				o = production
			}
			results[i] = st.ReserveEndpoint(ctx, host, o)
		}(i)
	}
	wg.Wait()
	var winner *endpoint.Owner
	for i, err := range results {
		o := staging
		if i%2 == 1 {
			o = production
		}
		switch {
		case err == nil:
			if winner != nil && *winner != o {
				t.Fatal("two owners won one hostname")
			}
			winner = &o
		case errors.Is(err, persistence.ErrHostnameTaken):
		default:
			t.Fatal(err)
		}
	}
	if winner == nil {
		t.Fatal("no owner won")
	}
	loser := production
	if *winner == production {
		loser = staging
	}
	// Same-owner retry is idempotent; the loser is refused in Preview and Deploy.
	if err = st.ReserveEndpoint(ctx, host, *winner); err != nil {
		t.Fatal("same-owner retry", err)
	}
	if err = st.CheckEndpoint(ctx, host, *winner); err != nil {
		t.Fatal(err)
	}
	if err = st.CheckEndpoint(ctx, host, loser); !errors.Is(err, persistence.ErrHostnameTaken) {
		t.Fatal("loser preview", err)
	}
	if n := scalar[int](t, owner, `SELECT count(*) FROM endpoint_reservations`); n != 1 {
		t.Fatal(n)
	}
	// A rename keeps both claims until the old one is released explicitly.
	if err = st.ReserveEndpoint(ctx, "store.apps.example.com", *winner); err != nil {
		t.Fatal(err)
	}
	if err = st.RetainEndpoints(ctx, *winner, []string{"store.apps.example.com"}); err != nil {
		t.Fatal(err)
	}
	if err = st.CheckEndpoint(ctx, host, loser); err != nil {
		t.Fatal("released hostname still held", err)
	}
	// Another owner's release never frees this owner's claim.
	if err = st.RetainEndpoints(ctx, loser, nil); err != nil {
		t.Fatal(err)
	}
	if err = st.CheckEndpoint(ctx, "store.apps.example.com", loser); !errors.Is(err, persistence.ErrHostnameTaken) {
		t.Fatal("foreign release freed a claim", err)
	}
	// Removal releases everything the owner holds.
	if err = st.RetainEndpoints(ctx, *winner, nil); err != nil {
		t.Fatal(err)
	}
	if n := scalar[int](t, owner, `SELECT count(*) FROM endpoint_reservations`); n != 0 {
		t.Fatal(n)
	}
	// Unknown owners cannot reserve; invalid hostnames violate the ledger check.
	if err = st.ReserveEndpoint(ctx, host, endpoint.Owner{Organization: "retained-org", Application: "missing", Environment: "staging", Workload: "web"}); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatal("unknown owner", err)
	}
	if err = st.ReserveEndpoint(ctx, "Not_A_Host", staging); err == nil {
		t.Fatal("invalid hostname accepted")
	}
}
