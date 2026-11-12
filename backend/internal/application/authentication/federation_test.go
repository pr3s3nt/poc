package authentication

import (
	"context"
	"errors"
	"net/url"
	"sync"
	"testing"
	"time"

	"orchestrator/internal/domain/identity"
)

type providerStub struct {
	grant     Grant
	err       error
	refreshes int
	revoked   bool
	mu        sync.Mutex
}

func (p *providerStub) AuthorizationURL(state, nonce, verifier string) string {
	return "https://idp.test/auth?state=" + state
}
func (p *providerStub) Exchange(context.Context, string, string, string) (Grant, error) {
	return p.grant, p.err
}
func (p *providerStub) Refresh(context.Context, string) (Grant, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.refreshes++
	return p.grant, p.err
}
func (p *providerStub) Revoke(context.Context, string) error { p.revoked = true; return nil }
func federationFixture(t *testing.T) (*Service, *providerStub) {
	t.Helper()
	s, _ := newTestService(t)
	p := &providerStub{grant: Grant{Subject: "subject-1", Username: "employee", Role: identity.RoleDeveloper, RefreshToken: "private-refresh", ExpiresAt: time.Now().Add(time.Hour)}}
	s.EnableOIDC(&Federation{Provider: p, Issuer: "https://idp.test/realm", OrganizationKey: "acme"})
	return s, p
}
func loginFederated(t *testing.T, s *Service) (string, Identity) {
	t.Helper()
	u, b, e := s.BeginOIDC()
	if e != nil {
		t.Fatal(e)
	}
	parsed, _ := url.Parse(u)
	token, who, e := s.CompleteOIDC(context.Background(), parsed.Query().Get("state"), b, "code")
	if e != nil {
		t.Fatal(e)
	}
	return token, who
}
func TestOIDCBrowserBindingAndReplay(t *testing.T) {
	s, p := federationFixture(t)
	u, b, _ := s.BeginOIDC()
	parsed, _ := url.Parse(u)
	state := parsed.Query().Get("state")
	if _, _, e := s.CompleteOIDC(context.Background(), state, "another-browser", "code"); !errors.Is(e, ErrUnauthorized) {
		t.Fatal(e)
	}
	if _, _, e := s.CompleteOIDC(context.Background(), state, b, "code"); !errors.Is(e, ErrUnauthorized) {
		t.Fatal("consumed transaction replay", e)
	}
	u, b, _ = s.BeginOIDC()
	parsed, _ = url.Parse(u)
	state = parsed.Query().Get("state")
	now := s.now()
	s.now = func() time.Time { return now.Add(6 * time.Minute) }
	if _, _, e := s.CompleteOIDC(context.Background(), state, b, "code"); !errors.Is(e, ErrUnauthorized) {
		t.Fatal("expired transaction", e)
	}
	s.now = func() time.Time { return now }
	p.grant.Role = ""
	u, b, _ = s.BeginOIDC()
	parsed, _ = url.Parse(u)
	if _, _, e := s.CompleteOIDC(context.Background(), parsed.Query().Get("state"), b, "code"); !errors.Is(e, ErrForbidden) {
		t.Fatal("missing role", e)
	}
}
func TestOIDCStableIdentityAndNoLocalBypass(t *testing.T) {
	s, _ := federationFixture(t)
	token, who := loginFederated(t, s)
	_, again := loginFederated(t, s)
	if who.UserID != again.UserID || who.OrganizationKey != "acme" || who.AuthProvider != "oidc" {
		t.Fatal(who, again)
	}
	a, e := s.store.GetUserAccount(context.Background(), who.UserID)
	if e != nil || a.PasswordHash != "" || a.OIDCSubject != "subject-1" {
		t.Fatal("external account persistence", e)
	}
	if _, _, e := s.SignIn(context.Background(), "employee", "anything"); !errors.Is(e, ErrForbidden) {
		t.Fatal("local bypass", e)
	}
	if _, e := s.IdentityForToken(context.Background(), token); e != nil {
		t.Fatal(e)
	}
	restarted := NewService(s.store)
	restarted.EnableOIDC(&Federation{Provider: s.federation.Provider, Issuer: s.federation.Issuer, OrganizationKey: "acme"})
	if _, e := restarted.IdentityForToken(context.Background(), token); !errors.Is(e, ErrUnauthorized) {
		t.Fatal("restart must require login", e)
	}
}
func TestOIDCRefreshChangesRoleAndFailsClosed(t *testing.T) {
	for _, failure := range []string{"provider", "subject", "role"} {
		t.Run(failure, func(t *testing.T) {
			s, p := federationFixture(t)
			token, _ := loginFederated(t, s)
			now := s.now()
			s.now = func() time.Time { return now.Add(31 * time.Second) }
			p.grant.Role = identity.RolePlatformEngineer
			who, e := s.IdentityForToken(context.Background(), token)
			if e != nil || who.Role != identity.RolePlatformEngineer || p.refreshes != 1 {
				t.Fatal(who, e)
			}
			s.now = func() time.Time { return now.Add(62 * time.Second) }
			switch failure {
			case "provider":
				p.err = errors.New("disabled or unavailable")
			case "subject":
				p.grant.Subject = "other"
			case "role":
				p.grant.Role = ""
			}
			if _, e = s.IdentityForToken(context.Background(), token); !errors.Is(e, ErrUnauthorized) {
				t.Fatal("refresh accepted", e)
			}
			p.err = nil
			p.grant.Subject = "subject-1"
			p.grant.Role = identity.RoleAdmin
			if _, e = s.IdentityForToken(context.Background(), token); !errors.Is(e, ErrUnauthorized) {
				t.Fatal("failed session resurrected", e)
			}
		})
	}
}
func TestOIDCLogoutRevokesProviderAndSession(t *testing.T) {
	s, p := federationFixture(t)
	token, _ := loginFederated(t, s)
	if e := s.SignOut(context.Background(), token); e != nil {
		t.Fatal(e)
	}
	if !p.revoked {
		t.Fatal("no revocation")
	}
	if _, e := s.IdentityForToken(context.Background(), token); e == nil {
		t.Fatal("session survived")
	}
}
func TestOIDCUsernameCollisionDoesNotLink(t *testing.T) {
	s, p := federationFixture(t)
	_, who := loginFederated(t, s)
	p.grant.Subject = "another-subject"
	u, b, _ := s.BeginOIDC()
	parsed, _ := url.Parse(u)
	if _, _, e := s.CompleteOIDC(context.Background(), parsed.Query().Get("state"), b, "code"); !errors.Is(e, ErrForbidden) {
		t.Fatal(e)
	}
	a, _ := s.store.GetUserAccount(context.Background(), who.UserID)
	if a.OIDCSubject != "subject-1" {
		t.Fatal("identity overwritten")
	}
}

func TestOIDCReplacingCookieDoesNotRevokeNewProviderGrant(t *testing.T) {
	s, p := federationFixture(t)
	old, _ := loginFederated(t, s)
	fresh, _ := loginFederated(t, s)
	if e := s.EndReplacedSession(context.Background(), old); e != nil {
		t.Fatal(e)
	}
	if p.revoked {
		t.Fatal("new login may share the old provider client session")
	}
	if _, e := s.IdentityForToken(context.Background(), old); e == nil {
		t.Fatal("old platform cookie survived")
	}
	if _, e := s.IdentityForToken(context.Background(), fresh); e != nil {
		t.Fatal(e)
	}
}

func TestOIDCOnlyConfiguredLocalAdminMayUsePassword(t *testing.T) {
	owner := identity.UserAccount{ID: "owner", OrganizationKey: "acme", Username: "local-admin", Role: identity.RoleAdmin, Status: identity.AccountActive}
	other := owner
	other.ID = "other"
	other.Username = "other-admin"
	member := developer
	s, _ := newTestService(t, owner, other, member)
	provider := &providerStub{}
	s.EnableOIDC(&Federation{Provider: provider, Issuer: "https://idp.test", OrganizationKey: "acme", LocalTestAdminUsername: "local-admin"})
	ctx := context.Background()
	token, who, err := s.SignIn(ctx, "LOCAL-ADMIN", "test-password")
	if err != nil || who.AuthProvider != "local" || who.Role != identity.RoleAdmin || who.UserManagement {
		t.Fatal("configured local admin", who, err)
	}
	if restored, err := s.IdentityForToken(ctx, token); err != nil || restored != who {
		t.Fatal("local restore", err)
	}
	for _, name := range []string{"developer", "other-admin"} {
		if _, _, err := s.SignIn(ctx, name, "test-password"); !errors.Is(err, ErrForbidden) {
			t.Fatal("non-allowlisted account admitted", name, err)
		}
	}
	if _, err := s.ListUsers(ctx, who); !errors.Is(err, ErrForbidden) {
		t.Fatal("local Admin gained employee management", err)
	}
	if _, err := s.CreateUser(ctx, who, "employee", "password-1", identity.RoleDeveloper); !errors.Is(err, ErrForbidden) {
		t.Fatal("created local employee", err)
	}
	if err := s.ResetUserPassword(ctx, who, other.ID, "password-1"); !errors.Is(err, ErrForbidden) {
		t.Fatal("reset employee password", err)
	}
	if provider.refreshes != 0 {
		t.Fatal("local password touched Keycloak")
	}
	if err := s.ChangePassword(ctx, owner.ID, "test-password", "new-test-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.IdentityForToken(ctx, token); err == nil {
		t.Fatal("password change failed to revoke local session")
	}
	if _, _, err := s.SignIn(ctx, "local-admin", "new-test-password"); err != nil {
		t.Fatal(err)
	}
}
func TestOIDCLocalAdminSessionCannotSurviveLossOfEligibility(t *testing.T) {
	for _, kind := range []string{"disabled-config", "role", "status", "organization", "external"} {
		t.Run(kind, func(t *testing.T) {
			owner := identity.UserAccount{ID: "owner", OrganizationKey: "acme", Username: "local-admin", Role: identity.RoleAdmin, Status: identity.AccountActive}
			s, st := newTestService(t, owner)
			s.EnableOIDC(&Federation{Provider: &providerStub{}, Issuer: "https://idp.test", OrganizationKey: "acme", LocalTestAdminUsername: "local-admin"})
			token, _, err := s.SignIn(context.Background(), "local-admin", "test-password")
			if err != nil {
				t.Fatal(err)
			}
			account, _ := st.GetUserAccount(context.Background(), owner.ID)
			switch kind {
			case "disabled-config":
				s.federation.LocalTestAdminUsername = ""
			case "role":
				account.Role = identity.RoleDeveloper
			case "status":
				account.Status = identity.AccountDisabled
			case "organization":
				account.OrganizationKey = "other"
			case "external":
				account.OIDCIssuer = "https://idp.test"
				account.OIDCSubject = "subject"
			}
			if err := st.SaveUserAccount(context.Background(), account); err != nil {
				t.Fatal(err)
			}
			if _, err := s.IdentityForToken(context.Background(), token); !errors.Is(err, ErrUnauthorized) {
				t.Fatal("ineligible local session accepted", err)
			}
		})
	}
}
