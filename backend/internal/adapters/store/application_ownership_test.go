package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/identity"
	"orchestrator/internal/ports/persistence"
)

func TestApplicationOwnerSurvivesSnapshotAndCannotChange(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.json")
	st, err := NewWithSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveOrganization(ctx, application.Organization{ID: "org", Key: "org", Name: "Org"}); err != nil {
		t.Fatal(err)
	}
	for _, user := range []identity.UserAccount{
		{ID: "owner", OrganizationKey: "org", Username: "owner", Role: identity.RoleDeveloper, Status: identity.AccountActive, PasswordHash: "fixture-hash"},
		{ID: "other", OrganizationKey: "org", Username: "other", Role: identity.RoleDeveloper, Status: identity.AccountActive, PasswordHash: "fixture-hash"},
		{ID: "foreign", OrganizationKey: "foreign", Username: "foreign", Role: identity.RoleDeveloper, Status: identity.AccountActive, PasswordHash: "fixture-hash"},
	} {
		if err := st.SaveUserAccount(ctx, user); err != nil {
			t.Fatal(err)
		}
	}
	app := application.Application{ID: "app", Key: "app", OrganizationKey: "org", OwnerUserID: "owner", Name: "App", Subdomain: "app", Version: 1}
	if err := st.SaveApplication(ctx, app); err != nil {
		t.Fatal(err)
	}
	st, err = NewWithSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.GetApplication(ctx, "app")
	if err != nil || got.OwnerUserID != "owner" {
		t.Fatalf("owner lost: %+v %v", got, err)
	}
	for _, owner := range []string{"other", "foreign", ""} {
		app.OwnerUserID = owner
		if err := st.SaveApplication(ctx, app); !errors.Is(err, persistence.ErrImmutable) {
			t.Fatalf("owner changed to %q: %v", owner, err)
		}
	}
	app.Key, app.ID, app.OwnerUserID = "new", "new", "foreign"
	if err := st.SaveApplication(ctx, app); err == nil {
		t.Fatal("foreign owner accepted")
	}
	app.OwnerUserID = "missing"
	if err := st.SaveApplication(ctx, app); err == nil {
		t.Fatal("missing owner accepted")
	}
}
