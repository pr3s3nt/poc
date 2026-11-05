package application

import (
	"orchestrator/internal/domain/identity"
	"testing"
)

func TestApplicationAccess(t *testing.T) {
	app := Application{OrganizationKey: "org", OwnerUserID: "owner"}
	for _, tc := range []struct {
		name, org, user string
		role            identity.Role
		want            bool
	}{
		{"owner", "org", "owner", identity.RoleDeveloper, true},
		{"other developer", "org", "other", identity.RoleDeveloper, false},
		{"platform", "org", "other", identity.RolePlatformEngineer, true},
		{"admin", "org", "other", identity.RoleAdmin, true},
		{"foreign owner", "foreign", "owner", identity.RoleDeveloper, false},
		{"foreign platform", "foreign", "owner", identity.RolePlatformEngineer, false},
		{"foreign admin", "foreign", "owner", identity.RoleAdmin, false},
		{"unknown role", "org", "owner", identity.Role("UNKNOWN"), false},
		{"empty identity", "org", "", identity.RoleDeveloper, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := app.AccessibleBy(tc.org, tc.user, tc.role); got != tc.want {
				t.Fatalf("access=%v want %v", got, tc.want)
			}
		})
	}
	app.OwnerUserID = ""
	if app.AccessibleBy("org", "", identity.RoleDeveloper) || app.AccessibleBy("org", "owner", identity.RoleDeveloper) {
		t.Fatal("ownerless app exposed to developer")
	}
	if !app.AccessibleBy("org", "platform", identity.RolePlatformEngineer) {
		t.Fatal("platform cannot see retained app")
	}
}
