package seed_test

import (
	"context"
	"sync"
	"testing"

	"orchestrator/internal/adapters/postgres"
	"orchestrator/internal/ports/persistence/persistencetest"
	"orchestrator/internal/seed"
)

// The catalog-free fixture on REAL PostgreSQL: concurrent seeding (several
// processes starting at once) creates exactly one Application with exactly
// one staging and one production Environment, the exact owned namespace
// identity survives a reopen, and no catalog row, Connection or binding
// appears.
func TestCatalogNoneFixtureOnPostgresIsAtomicRaceFreeAndSurvivesReopen(t *testing.T) {
	db := persistencetest.FreshPostgresRoles(t)
	ctx := context.Background()
	if _, err := postgres.Setup(ctx, db.Owner, postgres.Roles{Console: db.ConsoleRole, State: db.StateRole}); err != nil {
		t.Fatal(err)
	}
	o := seed.Defaults()
	o.Profile, o.Catalog, o.NamespaceIdentity = seed.ProfileAcceptanceV2, seed.CatalogNone, "orch-v2-test-pgrun1"

	const writers = 6
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			st, err := postgres.Open(ctx, db.Console)
			if err != nil {
				errs <- err
				return
			}
			defer st.Close()
			errs <- seed.Apply(ctx, st, o)
		}()
	}
	wg.Wait()
	close(errs)
	succeeded := 0
	for err := range errs {
		if err == nil {
			succeeded++
		}
	}
	if succeeded == 0 {
		t.Fatal("no concurrent seed succeeded")
	}
	// Reopen with a new pool and a final idempotent Apply: the state is exactly one fixture.
	st, err := postgres.Open(ctx, db.Console)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := seed.Apply(ctx, st, o); err != nil {
		t.Fatalf("idempotent re-seed after the race: %v", err)
	}
	apps, err := st.ListApplications(ctx)
	if err != nil || len(apps) != 1 {
		t.Fatalf("applications %v %v", apps, err)
	}
	staging, err := st.GetEnvironment(ctx, o.ApplicationKey, "staging")
	if err != nil || staging.NamespaceIdentity != "orch-v2-test-pgrun1" || staging.Configured() {
		t.Fatalf("staging %+v %v", staging, err)
	}
	if _, err := st.GetEnvironment(ctx, o.ApplicationKey, "production"); err != nil {
		t.Fatal(err)
	}
	if defs, _ := st.ListResourceDefinitions(ctx, o.OrganizationKey); len(defs) != 0 {
		t.Fatalf("definitions %d", len(defs))
	}
	if types, _ := st.ListResourceTypes(ctx, o.OrganizationKey); len(types) != 0 {
		t.Fatalf("types %d", len(types))
	}
	if conns, _ := st.ListConnections(ctx, o.OrganizationKey); len(conns) != 0 {
		t.Fatalf("connections %d", len(conns))
	}
}
