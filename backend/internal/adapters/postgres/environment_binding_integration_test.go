package postgres

import (
	"context"
	"orchestrator/internal/domain/application"
	"testing"

	"orchestrator/internal/domain/environment"
)

// A fresh database holds identity-only Applications: Save/Get/List carry no
// target, new Environments stay unconfigured and reopening is idempotent.
func TestApplicationIdentityOnlyFreshAndReopen(t *testing.T) {
	ctx := context.Background()
	url := freshDatabase(t).Console
	st, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveOrganization(ctx, application.Organization{ID: "55555555-5555-4555-8555-555555555555", Key: "acme", Name: "Acme"}); err != nil {
		t.Fatal(err)
	}
	fresh := application.Application{ID: "cccccccc-cccc-4ccc-8ccc-cccccccccccc", Key: "cccccccc-cccc-4ccc-8ccc-cccccccccccc", OrganizationKey: "acme", Name: "Fresh", Subdomain: "fresh", Version: 1}
	if err := st.SaveApplication(ctx, fresh); err != nil {
		t.Fatal(err)
	}
	// A new Application has the explicit UC-18 lifecycle default after persistence.
	fresh.Status = application.StatusActive
	for _, key := range []string{"staging", "production"} {
		if err := st.SaveEnvironment(ctx, environment.Environment{Key: key, ApplicationID: fresh.ID, ApplicationKey: fresh.Key, Name: key, Type: key, NamespaceIdentity: "app-fresh-" + key, Version: 1}); err != nil {
			t.Fatal(err)
		}
	}
	var columns int
	if err := st.pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_name='applications' AND column_name IN ('execution_profile','connection_id','region','runtime_status','configuration_provider')`).Scan(&columns); err != nil || columns != 0 {
		t.Fatalf("retired Application columns remain: %d %v", columns, err)
	}
	st.Close()
	for i := 0; i < 2; i++ {
		reopened, err := Open(ctx, url)
		if err != nil {
			t.Fatalf("reopen %d: %v", i, err)
		}
		if got, err := reopened.GetApplication(ctx, fresh.Key); err != nil || got != fresh {
			t.Fatalf("reopen %d: application = %+v %v", i, got, err)
		}
		if list, err := reopened.ListApplications(ctx); err != nil || len(list) != 1 || list[0] != fresh {
			t.Fatalf("reopen %d: list = %+v %v", i, list, err)
		}
		for _, key := range []string{"staging", "production"} {
			e, err := reopened.GetEnvironment(ctx, fresh.Key, key)
			if err != nil || e.InfrastructureScope != environment.ScopeEnvironment {
				t.Fatalf("reopen %d: %s must stay UNCONFIGURED: %+v %v", i, key, e, err)
			}
		}
		var markers, ledgers int
		if err := reopened.pool.QueryRow(ctx, `SELECT count(*), (SELECT count(*) FROM pg_class WHERE relname='schema_migrations') FROM schema_baseline`).Scan(&markers, &ledgers); err != nil || markers != 1 || ledgers != 0 {
			t.Fatalf("baseline marker rows = %d, ledgers = %d (%v)", markers, ledgers, err)
		}
		reopened.Close()
	}
}
