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
