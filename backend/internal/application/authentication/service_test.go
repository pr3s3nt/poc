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
