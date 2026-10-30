package authentication

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/domain/identity"
	"orchestrator/internal/platform/password"
)

func newTestService(t *testing.T, accounts ...identity.UserAccount) (*Service, *store.Store) {
	t.Helper()
	return newRejectingService(t, nil, accounts...)
}

func newRejectingService(t *testing.T, rejected []string, accounts ...identity.UserAccount) (*Service, *store.Store) {
	t.Helper()
	st := store.New()
	hash, err := password.Hash("test-password")
	if err != nil {
		t.Fatal(err)
	}
	for _, account := range accounts {
		account.PasswordHash = hash
		if err := st.SaveUserAccount(context.Background(), account); err != nil {
			t.Fatal(err)
		}
	}
	return NewService(st, rejected...), st
}

var developer = identity.UserAccount{ID: "user-1", OrganizationKey: "acme", Username: "developer", Role: identity.RoleDeveloper, Status: identity.AccountActive}

func TestSignIn_ActiveAccountCreatesSession(t *testing.T) {
	ctx := context.Background()
	svc, st := newTestService(t, developer)
	token, who, err := svc.SignIn(ctx, " Developer ", "test-password")
	if err != nil {
		t.Fatal(err)
	}
	if who.UserID != "user-1" || who.OrganizationKey != "acme" || who.Role != identity.RoleDeveloper {
		t.Fatalf("identity = %+v", who)
	}
	if len(token) < 32 {
		t.Fatalf("token is not opaque random: %q", token)
	}
	session, err := st.GetSessionByTokenHash(ctx, hashToken(token))
	if err != nil {
		t.Fatal(err)
	}
	if session.TokenHash == token || strings.Contains(session.TokenHash, token) {
		t.Fatal("raw session token must not be persisted")
	}
	if session.UserAccountID != "user-1" || !session.ExpiresAt.After(time.Now()) {
		t.Fatalf("session = %+v", session)
	}
	restored, err := svc.IdentityForToken(ctx, token)
	if err != nil || restored != who {
		t.Fatalf("restore = %+v, %v", restored, err)
	}
}

func TestSignIn_InvalidPasswordOrDisabledAccountHasNoSession(t *testing.T) {
	ctx := context.Background()
	disabled := identity.UserAccount{ID: "user-2", OrganizationKey: "acme", Username: "retired", Role: identity.RoleDeveloper, Status: identity.AccountDisabled}
	svc, _ := newTestService(t, developer, disabled)
	for _, tc := range []struct{ username, password string }{
		{"developer", "wrong-password"},
		{"retired", "test-password"},
		{"unknown", "test-password"},
		{"developer", ""},
	} {
		token, _, err := svc.SignIn(ctx, tc.username, tc.password)
		if !errors.Is(err, ErrInvalidCredentials) || token != "" {
			t.Fatalf("%s: token=%q err=%v", tc.username, token, err)
		}
	}
}

func TestIdentityForToken_RejectsExpiredSession(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService(t, developer)
	start := time.Date(2026, 9, 29, 8, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return start }
	token, _, err := svc.SignIn(ctx, "developer", "test-password")
	if err != nil {
		t.Fatal(err)
	}
	svc.now = func() time.Time { return start.Add(8*time.Hour - time.Second) }
	if _, err := svc.IdentityForToken(ctx, token); err != nil {
		t.Fatalf("session must be valid before expiry: %v", err)
	}
	svc.now = func() time.Time { return start.Add(8 * time.Hour) }
	if _, err := svc.IdentityForToken(ctx, token); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expired session error = %v", err)
	}
}

func TestSignOut_RevokesSession(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService(t, developer)
	token, _, err := svc.SignIn(ctx, "developer", "test-password")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SignOut(ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.IdentityForToken(ctx, token); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("revoked session error = %v", err)
	}
	if err := svc.SignOut(ctx, "unknown-token"); err != nil {
		t.Fatalf("unknown token sign-out must be idempotent: %v", err)
	}
}

func TestIdentityForToken_RejectsAccountDisabledAfterSignIn(t *testing.T) {
	ctx := context.Background()
	svc, st := newTestService(t, developer)
	token, _, err := svc.SignIn(ctx, "developer", "test-password")
	if err != nil {
		t.Fatal(err)
	}
	account, _ := st.GetUserAccount(ctx, developer.ID)
	account.Status = identity.AccountDisabled
	if err := st.SaveUserAccount(ctx, account); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.IdentityForToken(ctx, token); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("disabled account session error = %v", err)
	}
}

func TestNewService_RejectedAccountIDsRefuseOnlyThoseAccounts(t *testing.T) {
	ctx := context.Background()
	// A session created before the restart must not be restored afterwards.
	open, st := newTestService(t, developer)
	token, _, err := open.SignIn(ctx, "developer", "test-password")
	if err != nil {
		t.Fatal(err)
	}
	restricted := NewService(st, strings.ToUpper(developer.ID))
	if _, _, err := restricted.SignIn(ctx, "developer", "test-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("rejected account sign-in error = %v", err)
	}
	if _, err := restricted.IdentityForToken(ctx, token); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("rejected account session restore error = %v", err)
	}

	// Same username, different stable ID: a legitimate account stays usable.
	other := developer
	other.ID = "user-production"
	svc, _ := newRejectingService(t, []string{developer.ID}, other)
	token, who, err := svc.SignIn(ctx, "developer", "test-password")
	if err != nil || who.UserID != "user-production" {
		t.Fatalf("same-username account sign-in = %+v, %v", who, err)
	}
	if _, err := svc.IdentityForToken(ctx, token); err != nil {
		t.Fatalf("same-username account session restore: %v", err)
	}
}

var admin = identity.UserAccount{ID: "admin-1", OrganizationKey: "acme", Username: "admin", Role: identity.RoleAdmin, Status: identity.AccountActive}

func TestChangePassword_RevokesEverySession(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService(t, developer)
	first, _, err := svc.SignIn(ctx, "developer", "test-password")
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := svc.SignIn(ctx, "developer", "test-password")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ChangePassword(ctx, developer.ID, "wrong", "new-secret-1"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong current error = %v", err)
	}
	if err := svc.ChangePassword(ctx, developer.ID, "test-password", "short"); !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("short password error = %v", err)
	}
	if err := svc.ChangePassword(ctx, developer.ID, "test-password", "new-secret-1"); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{first, second} {
		if _, err := svc.IdentityForToken(ctx, token); !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("session survived the password change: %v", err)
		}
	}
	if _, _, err := svc.SignIn(ctx, "developer", "test-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("old password error = %v", err)
	}
	if _, _, err := svc.SignIn(ctx, "developer", "new-secret-1"); err != nil {
		t.Fatalf("new password sign-in: %v", err)
	}
}

func TestCreateUser_RequiresAdminAndValidInput(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService(t, developer, admin)
	devCaller := Identity{UserID: developer.ID, OrganizationKey: "acme", Username: "developer", Role: identity.RoleDeveloper}
	if _, err := svc.CreateUser(ctx, devCaller, "newdev", "password-1", identity.RoleDeveloper); !errors.Is(err, ErrForbidden) {
		t.Fatalf("developer create error = %v", err)
	}
	adminCaller := Identity{UserID: admin.ID, OrganizationKey: "acme", Username: "admin", Role: identity.RoleAdmin}
	if _, err := svc.CreateUser(ctx, adminCaller, "developer", "password-1", identity.RoleDeveloper); !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf("duplicate error = %v", err)
	}
	if _, err := svc.CreateUser(ctx, adminCaller, "UPPER", "password-1", identity.RoleDeveloper); err != nil {
		t.Fatalf("uppercase username should normalize: %v", err)
	}
	if _, err := svc.CreateUser(ctx, adminCaller, "no good!", "password-1", identity.RoleDeveloper); !errors.Is(err, ErrInvalidUsername) {
		t.Fatalf("invalid username error = %v", err)
	}
	if _, err := svc.CreateUser(ctx, adminCaller, "newdev", "short", identity.RoleDeveloper); !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("short password error = %v", err)
	}
	if _, err := svc.CreateUser(ctx, adminCaller, "newdev", "password-1", identity.Role("SUPERUSER")); !errors.Is(err, ErrInvalidRole) {
		t.Fatalf("invalid role error = %v", err)
	}
	account, err := svc.CreateUser(ctx, adminCaller, "newdev", "password-1", identity.RoleDeveloper)
	if err != nil {
		t.Fatal(err)
	}
	if account.OrganizationKey != "acme" || account.Status != identity.AccountActive || account.Role != identity.RoleDeveloper {
		t.Fatalf("account = %+v", account)
	}
	if account.PasswordHash == "password-1" {
		t.Fatal("password persisted in clear")
	}
	if _, _, err := svc.SignIn(ctx, "newdev", "password-1"); err != nil {
		t.Fatalf("created account sign-in: %v", err)
	}
}

func TestListUsers_AdminSeesOwnOrganizationOnly(t *testing.T) {
	ctx := context.Background()
	other := identity.UserAccount{ID: "user-9", OrganizationKey: "other-org", Username: "outsider", Role: identity.RoleDeveloper, Status: identity.AccountActive}
	svc, _ := newTestService(t, developer, admin, other)
	devCaller := Identity{UserID: developer.ID, OrganizationKey: "acme", Username: "developer", Role: identity.RoleDeveloper}
	if _, err := svc.ListUsers(ctx, devCaller); !errors.Is(err, ErrForbidden) {
		t.Fatalf("developer list error = %v", err)
	}
	adminCaller := Identity{UserID: admin.ID, OrganizationKey: "acme", Username: "admin", Role: identity.RoleAdmin}
	accounts, err := svc.ListUsers(ctx, adminCaller)
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 2 {
		t.Fatalf("accounts = %+v", accounts)
	}
	if accounts[0].Username != "admin" || accounts[1].Username != "developer" {
		t.Fatalf("order = %+v", accounts)
	}
}

func TestBootstrapAdmin(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService(t)
	if _, err := svc.BootstrapAdmin(ctx, "acme", "no good!", "password-1"); !errors.Is(err, ErrInvalidUsername) {
		t.Fatalf("invalid username error = %v", err)
	}
	if _, err := svc.BootstrapAdmin(ctx, "acme", "admin", "short"); !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("short password error = %v", err)
	}
	account, err := svc.BootstrapAdmin(ctx, "acme", "Admin", "password-1")
	if err != nil {
		t.Fatal(err)
	}
	if account.Role != identity.RoleAdmin || account.Username != "admin" || account.Status != identity.AccountActive {
		t.Fatalf("account = %+v", account)
	}
	if _, err := svc.BootstrapAdmin(ctx, "acme", "admin", "password-2"); !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf("duplicate error = %v", err)
	}
	_, who, err := svc.SignIn(ctx, "admin", "password-1")
	if err != nil || who.Role != identity.RoleAdmin {
		t.Fatalf("bootstrap admin sign-in = %+v, %v", who, err)
	}
}

// UC-00 BR-11: only an Admin resets another account's password, the target
// must live in the caller's Organization, and a successful reset revokes every
// session of the target while storing the password as a hash only.
func TestResetUserPassword(t *testing.T) {
	ctx := context.Background()
	admin := identity.UserAccount{ID: "admin-1", OrganizationKey: "acme", Username: "admin", Role: identity.RoleAdmin, Status: identity.AccountActive}
	foreign := identity.UserAccount{ID: "user-9", OrganizationKey: "other-org", Username: "outsider", Role: identity.RoleDeveloper, Status: identity.AccountActive}
	svc, st := newTestService(t, developer, admin, foreign)
	adminIdentity := Identity{UserID: admin.ID, OrganizationKey: "acme", Username: "admin", Role: identity.RoleAdmin}

	token, _, err := svc.SignIn(ctx, "developer", "test-password")
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.ResetUserPassword(ctx, Identity{UserID: developer.ID, OrganizationKey: "acme", Role: identity.RoleDeveloper}, developer.ID, "reset-secret-1"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("non-admin reset err = %v", err)
	}
	if err := svc.ResetUserPassword(ctx, adminIdentity, developer.ID, "short"); !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("short password err = %v", err)
	}
	if err := svc.ResetUserPassword(ctx, adminIdentity, "missing", "reset-secret-1"); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("unknown target err = %v", err)
	}
	if err := svc.ResetUserPassword(ctx, adminIdentity, foreign.ID, "reset-secret-1"); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("cross-organization target err = %v", err)
	}
	// Failed attempts change nothing.
	if _, _, err := svc.SignIn(ctx, "developer", "test-password"); err != nil {
		t.Fatalf("failed attempts must not change the password: %v", err)
	}
	if _, err := svc.IdentityForToken(ctx, token); err != nil {
		t.Fatalf("failed attempts must keep sessions: %v", err)
	}

	if err := svc.ResetUserPassword(ctx, adminIdentity, developer.ID, "reset-secret-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.IdentityForToken(ctx, token); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("target session must be revoked: %v", err)
	}
	if _, _, err := svc.SignIn(ctx, "developer", "test-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("old password err = %v", err)
	}
	if _, _, err := svc.SignIn(ctx, "developer", "reset-secret-1"); err != nil {
		t.Fatalf("new password must sign in: %v", err)
	}
	account, err := st.GetUserAccount(ctx, developer.ID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(account.PasswordHash, "reset-secret-1") {
		t.Fatal("password must be stored as a hash only")
	}
}
