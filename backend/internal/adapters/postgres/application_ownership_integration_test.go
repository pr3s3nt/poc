package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/identity"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
)

func TestApplicationOwnershipUpgradeAndPersistence(t *testing.T) {
	owner, runtime, roles := legacyDatabase(t, versionEightSQL)
	ctx := context.Background()
	stamp := scalar[string](t, owner, `SELECT created_at::text FROM schema_baseline`)
	if changed, err := Setup(ctx, owner, roles); err != nil || !changed {
		t.Fatalf("upgrade: changed=%v err=%v", changed, err)
	}
	if scalar[string](t, owner, `SELECT installation_id::text FROM schema_baseline`) != "11111111-1111-4111-8111-111111111111" || scalar[string](t, owner, `SELECT created_at::text FROM schema_baseline`) != stamp {
		t.Fatal("installation identity changed")
	}
	if scalar[int](t, owner, `SELECT count(*) FROM schema_upgrade_history`) != 9 {
		t.Fatal("history missing")
	}
	if changed, err := Setup(ctx, owner, roles); err != nil || changed {
		t.Fatalf("repeat: changed=%v err=%v", changed, err)
	}
	st, err := Open(ctx, runtime)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	retained, err := st.GetApplication(ctx, "retained-app")
	if err != nil || retained.OwnerUserID != "" || retained.Version != 7 {
		t.Fatalf("retained app changed: %+v %v", retained, err)
	}
	if retained.AccessibleBy("retained-org", "44444444-4444-4444-8444-444444444444", identity.RoleDeveloper) {
		t.Fatal("retained app exposed to Developer")
	}
	if !retained.AccessibleBy("retained-org", "platform", identity.RolePlatformEngineer) {
		t.Fatal("retained app hidden from platform")
	}
	account := identity.UserAccount{ID: ids.New(), OrganizationKey: "retained-org", Username: "new-owner", PasswordHash: "test-fixture-hash", Role: identity.RoleDeveloper, Status: identity.AccountActive}
	if err := st.SaveUserAccount(ctx, account); err != nil {
		t.Fatal(err)
	}
	app := application.Application{ID: ids.New(), Key: "owned", OrganizationKey: "retained-org", OwnerUserID: account.ID, Name: "Owned", Subdomain: "owned", Version: 1}
	if err := st.SaveApplication(ctx, app); err != nil {
		t.Fatal(err)
	}
	app.Name = "Renamed"
	if err := st.SaveApplication(ctx, app); err != nil {
		t.Fatal(err)
	}
	st.Close()
	st, err = Open(ctx, runtime)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	got, err := st.GetApplication(ctx, "owned")
	if err != nil || got.OwnerUserID != account.ID || got.Name != "Renamed" {
		t.Fatalf("owner lost after reopen: %+v %v", got, err)
	}
	list, err := st.ListApplications(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range list {
		if a.Key == app.Key {
			found = a.OwnerUserID == account.ID
		}
	}
	if !found {
		t.Fatal("list lost owner")
	}
	app.OwnerUserID = "44444444-4444-4444-8444-444444444444"
	if err := st.SaveApplication(ctx, app); !errors.Is(err, persistence.ErrImmutable) {
		t.Fatalf("owner change accepted: %v", err)
	}
	if err := st.SaveOrganization(ctx, application.Organization{ID: ids.New(), Key: "foreign", Name: "Foreign"}); err != nil {
		t.Fatal(err)
	}
	account.ID, account.Username, account.OrganizationKey = ids.New(), "foreign-owner", "foreign"
	if err := st.SaveUserAccount(ctx, account); err != nil {
		t.Fatal(err)
	}
	app.ID, app.Key, app.Name, app.Subdomain, app.OwnerUserID = ids.New(), "foreign-owner-app", "Foreign owner app", "foreign-owner-app", account.ID
	if err := st.SaveApplication(ctx, app); err == nil {
		t.Fatal("foreign Organization owner accepted")
	}
	c, err := pgx.Connect(ctx, runtime)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close(ctx)
	for _, sql := range []string{
		`UPDATE applications SET owner_user_id=NULL WHERE application_key='owned'`,
		`UPDATE applications SET owner_user_id='44444444-4444-4444-8444-444444444444' WHERE application_key='retained-app'`,
		`ALTER TABLE applications DISABLE TRIGGER application_owner_immutable`,
		`CREATE TABLE forbidden_ownership_ddl(id int)`,
	} {
		if _, err := c.Exec(ctx, sql); err == nil {
			t.Fatal("runtime bypass accepted")
		}
	}
}

func TestApplicationOwnershipUpgradeRollsBack(t *testing.T) {
	owner, _, roles := legacyDatabase(t, versionEightSQL)
	ctx := context.Background()
	c, err := pgx.Connect(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close(ctx)
	if _, err := c.Exec(ctx, `GRANT DELETE ON schema_baseline TO `+pgx.Identifier{roles.Console}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	if _, err := Setup(ctx, owner, roles); err == nil {
		t.Fatal("invalid runtime grants accepted")
	}
	if scalar[string](t, owner, `SELECT baseline FROM schema_baseline`) != "orchestrator-schema-v8" {
		t.Fatal("marker changed despite rollback")
	}
	if scalar[int](t, owner, `SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND table_name='applications' AND column_name='owner_user_id'`) != 0 {
		t.Fatal("owner DDL committed despite failure")
	}
	if scalar[int](t, owner, `SELECT count(*) FROM schema_upgrade_history`) != 8 {
		t.Fatal("history committed despite failure")
	}
}
