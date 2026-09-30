package postgres

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	appcreate "orchestrator/internal/application/application"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/seed"
)

func TestCreateApplicationPersistsExactlyTwoEnvironmentsAndUniqueGuards(t *testing.T) {
	url := os.Getenv("ORCHESTRATOR_POSTGRES_TEST_URL")
	if url == "" {
		t.Skip("ORCHESTRATOR_POSTGRES_TEST_URL is not set")
	}
	ctx := context.Background()
	first, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	opts := seed.Defaults()
	if err = seed.Apply(ctx, first, opts); err != nil {
		t.Fatal(err)
	}
	suffix := strings.Split(ids.New(), "-")[0]
	result, err := appcreate.NewService(first).Create(ctx, appcreate.CreateCommand{OrganizationKey: opts.OrganizationKey, Name: "Onboarding " + suffix, Subdomain: "onboarding-" + suffix})
	if err != nil {
		t.Fatal(err)
	}

	// The database unique constraints are the final duplicate guard.
	clash := result.Application
	clash.ID, clash.Key, clash.Name = ids.New(), ids.New(), "Other "+suffix
	if err := first.SaveApplication(ctx, clash); !errors.Is(err, persistence.ErrImmutable) {
		t.Fatalf("duplicate subdomain error = %v", err)
	}
	clash.Subdomain = "other-" + suffix
	clash.Name = result.Application.Name
	if err := first.SaveApplication(ctx, clash); !errors.Is(err, persistence.ErrImmutable) {
		t.Fatalf("duplicate organization name error = %v", err)
	}
	// The (organization_id, lower(name)) index rejects a different casing
	// directly, without relying on the service pre-check.
	clash.Name = strings.ToUpper(result.Application.Name)
	if err := first.SaveApplication(ctx, clash); !errors.Is(err, persistence.ErrImmutable) {
		t.Fatalf("case-insensitive duplicate name error = %v", err)
	}
	first.Close()

	second, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	app, err := second.GetApplication(ctx, result.Application.Key)
	if err != nil || app.Subdomain != "onboarding-"+suffix {
		t.Fatalf("application after restart = %+v, %v", app, err)
	}
	envs, err := second.ListEnvironments(ctx, app.Key)
	if err != nil || len(envs) != 2 {
		t.Fatalf("environments after restart = %+v, %v", envs, err)
	}
	for _, env := range envs {
		set, err := second.GetDeploymentSet(ctx, env.CurrentDeploymentSetID)
		if err != nil || len(set.Document.Modules) != 0 {
			t.Fatalf("%s deployment set = %+v, %v", env.Key, set, err)
		}
	}
}

// TestMigration3CaseInsensitiveNameIndex checks the migrated schema without
// dropping indexes or editing the migration ledger of the shared database.
func TestMigration3CaseInsensitiveNameIndex(t *testing.T) {
	url := os.Getenv("ORCHESTRATOR_POSTGRES_TEST_URL")
	if url == "" {
		t.Skip("ORCHESTRATOR_POSTGRES_TEST_URL is not set")
	}
	ctx := context.Background()
	store, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	var applied bool
	if err := store.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=3)`).Scan(&applied); err != nil {
		t.Fatal(err)
	}
	if !applied {
		t.Fatal("migration 3 must be recorded in schema_migrations")
	}
	var definition string
	if err := store.pool.QueryRow(ctx, `SELECT indexdef FROM pg_indexes WHERE tablename='applications' AND indexname='applications_organization_lower_name_key'`).Scan(&definition); err != nil {
		t.Fatalf("case-insensitive name index: %v", err)
	}
	normalized := strings.ToLower(strings.Join(strings.Fields(definition), " "))
	if !strings.Contains(normalized, "create unique index") || !strings.Contains(normalized, "(organization_id, lower(name))") {
		t.Fatalf("unexpected index definition: %s", definition)
	}

	opts := seed.Defaults()
	if err = seed.Apply(ctx, store, opts); err != nil {
		t.Fatal(err)
	}
	app, err := store.GetApplication(ctx, opts.ApplicationKey)
	if err != nil {
		t.Fatal(err)
	}
	suffix := strings.Split(ids.New(), "-")[0]
	variant := app
	variant.ID, variant.Key, variant.Name, variant.Subdomain = ids.New(), ids.New(), strings.ToUpper(app.Name), "variant-"+suffix
	if variant.Name == app.Name {
		t.Fatalf("seed name %q has no case variant", app.Name)
	}
	if err := store.SaveApplication(ctx, variant); !errors.Is(err, persistence.ErrImmutable) {
		t.Fatalf("case-variant name insert error = %v", err)
	}
	if _, err := store.GetApplication(ctx, variant.Key); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("rejected case variant must not persist: %v", err)
	}
}
