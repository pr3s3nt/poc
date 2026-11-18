package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"orchestrator/internal/adapters/store"
	"orchestrator/internal/application/authentication"
	"orchestrator/internal/application/installationsetup"
	"strings"
	"testing"
)

func TestSetupHTTPClosesAfterInitializationAndEnforcesPlatformRole(t *testing.T) {
	st := store.New()
	token := strings.Repeat("x", 32)
	s := NewServer(Config{Store: st, Authentication: authentication.NewService(st), Setup: installationsetup.New(st, token)})
	request := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	initial := request("GET", "/api/v1/setup", "")
	if initial.Code != 200 || !strings.Contains(initial.Body.String(), `"available":true`) || initial.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(initial)
	}
	claim := installationsetup.Claim{Token: token, OrganizationKey: "team", OrganizationName: "Team", Username: "admin", Password: "private-password", ConfirmPassword: "private-password"}
	raw, _ := json.Marshal(claim)
	if w := request("POST", "/api/v1/setup", string(raw)); w.Code != http.StatusNoContent {
		t.Fatal(w)
	}
	if w := request("POST", "/api/v1/setup", string(raw)); w.Code != http.StatusForbidden {
		t.Fatal(w)
	}
	if w := request("GET", "/api/v1/setup", ""); strings.Contains(w.Body.String(), `"available":true`) {
		t.Fatal("reopened")
	}
	for _, path := range []string{"/api/v1/platform/setup", "/api/v1/platform/setup/discover", "/api/v1/platform/setup/preview"} {
		method := "POST"
		if strings.HasSuffix(path, "/setup") {
			method = "GET"
		}
		if w := request(method, path, `{}`); w.Code != 401 {
			t.Fatal(path, w)
		}
	}
	tokenSession, _, err := authentication.NewService(st).SignIn(context.Background(), "admin", "private-password")
	if err != nil {
		t.Fatal(err)
	}
	// A malformed review remains a bounded client error, never a handler panic.
	r := httptest.NewRequest("POST", "/api/v1/platform/setup/preview", strings.NewReader(`{"documents":[]}`))
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: tokenSession})
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatal(w)
	}
}
