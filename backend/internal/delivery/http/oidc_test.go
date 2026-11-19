package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/application/authentication"
	"orchestrator/internal/domain/identity"
)

type httpOIDCProvider struct{}

func (httpOIDCProvider) AuthorizationURL(state, nonce, verifier string) string {
	return "https://idp.test/auth?state=" + state
}
func (httpOIDCProvider) Exchange(context.Context, string, string, string) (authentication.Grant, error) {
	return authentication.Grant{Subject: "employee-id", Username: "employee", Role: identity.RoleDeveloper, RefreshToken: "private-provider-refresh", ExpiresAt: time.Now().Add(time.Hour)}, nil
}
func (httpOIDCProvider) Refresh(context.Context, string) (authentication.Grant, error) {
	return httpOIDCProvider{}.Exchange(context.Background(), "", "", "")
}
func (httpOIDCProvider) Revoke(context.Context, string) error { return nil }
func TestOIDCControllerCookieBindingAndNoLocalEndpoints(t *testing.T) {
	auth := authentication.NewService(store.New())
	auth.EnableOIDC(&authentication.Federation{Provider: httpOIDCProvider{}, Issuer: "https://idp.test", OrganizationKey: "acme", Secure: true})
	s := &Server{auth: auth}
	config := httptest.NewRecorder()
	s.handleAuthConfig(config, httptest.NewRequest("GET", "/api/v1/auth/config", nil))
	if !strings.Contains(config.Body.String(), `"oidc"`) || strings.Contains(config.Body.String(), "private") {
		t.Fatal(config.Body.String())
	}
	for _, handler := range []http.HandlerFunc{s.handleSignIn, s.handleChangePassword, s.handleListUsers, s.handleCreateUser, s.handleResetUserPassword} {
		w := httptest.NewRecorder()
		handler(w, httptest.NewRequest("POST", "/", strings.NewReader(`{}`)))
		if w.Code != 403 {
			t.Fatal("local endpoint", w.Code)
		}
	}
	start := httptest.NewRecorder()
	s.handleOIDCStart(start, httptest.NewRequest("GET", "/api/v1/auth/oidc/start", nil))
	if start.Code != 303 {
		t.Fatal(start.Code)
	}
	cookies := start.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].Secure || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatal("transaction cookie security")
	}
	location, _ := url.Parse(start.Header().Get("Location"))
	callback := "/api/v1/auth/oidc/callback?state=" + location.Query().Get("state") + "&code=private-code"
	req := httptest.NewRequest("GET", callback, nil)
	req.AddCookie(cookies[0])
	done := httptest.NewRecorder()
	s.handleOIDCCallback(done, req)
	if done.Code != 303 || done.Header().Get("Location") != "/ui/" {
		t.Fatal("callback failed", done.Code)
	}
	var sessionCookieValue string
	for _, c := range done.Result().Cookies() {
		if c.Name == sessionCookie {
			if !c.Secure || !c.HttpOnly {
				t.Fatal("session cookie security")
			}
			sessionCookieValue = c.Value
		}
	}
	if sessionCookieValue == "" {
		t.Fatal("missing session cookie")
	}
	session := httptest.NewRequest("GET", "/api/v1/auth/session", nil)
	session.AddCookie(&http.Cookie{Name: sessionCookie, Value: sessionCookieValue})
	reply := httptest.NewRecorder()
	s.handleSession(reply, session)
	if reply.Code != 200 || strings.Contains(reply.Body.String(), "private-provider-refresh") {
		t.Fatal("session exposes provider credential")
	}
	replay := httptest.NewRecorder()
	s.handleOIDCCallback(replay, req)
	if replay.Header().Get("Location") != "/ui/?sso_error=1" {
		t.Fatal("callback replay accepted")
	}
	wrong := httptest.NewRecorder()
	s.handleOIDCCallback(wrong, httptest.NewRequest("GET", callback, nil))
	if wrong.Header().Get("Location") != "/ui/?sso_error=1" {
		t.Fatal("missing binding accepted")
	}
}
