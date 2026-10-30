package http

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/application/authentication"
	"orchestrator/internal/domain/identity"
	"orchestrator/internal/platform/password"
	"orchestrator/internal/seed"
	"orchestrator/internal/seed/seedtest"
)

const adminTestPassword = "admin-secret-1"

// newUsersServer seeds the fixture accounts plus one ADMIN and serves the API.
func newUsersServer(t *testing.T) (*httptest.Server, *store.Store) {
	t.Helper()
	ctx := context.Background()
	st := store.New()
	opts := seedtest.Defaults()
	if err := seedtest.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	hash, err := password.Hash(adminTestPassword)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveUserAccount(ctx, identity.UserAccount{ID: "admin-1", OrganizationKey: opts.OrganizationKey, Username: "admin", PasswordHash: hash, Role: identity.RoleAdmin, Status: identity.AccountActive}); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(NewServer(Config{Authentication: authentication.NewService(st), Store: st, SeedOptions: opts}))
	t.Cleanup(ts.Close)
	return ts, st
}

func signInClient(t *testing.T, base, username, password string) *http.Client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar}
	resp, err := c.Post(base+"/api/v1/auth/sign-in", "application/json", strings.NewReader(`{"username":"`+username+`","password":"`+password+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("sign-in %s: %d", username, resp.StatusCode)
	}
	return c
}

func request(t *testing.T, c *http.Client, method, url, body string) (int, string) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, _ := http.NewRequest(method, url, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw)
}

// UC-00 BR-09: the user list is Admin-only, scoped to the Organization, and
// never exposes a password hash.
func TestListUsersRequiresAdmin(t *testing.T) {
	ts, _ := newUsersServer(t)
	url := ts.URL + "/api/v1/users"

	anon := &http.Client{}
	if status, body := request(t, anon, http.MethodGet, url, ""); status != http.StatusUnauthorized || !strings.Contains(body, "UNAUTHORIZED") {
		t.Fatalf("anonymous: %d %s", status, body)
	}
	dev := signInClient(t, ts.URL, "developer", seed.FixedTestPassword)
	if status, body := request(t, dev, http.MethodGet, url, ""); status != http.StatusForbidden || !strings.Contains(body, "FORBIDDEN") {
		t.Fatalf("developer: %d %s", status, body)
	}
	admin := signInClient(t, ts.URL, "admin", adminTestPassword)
	status, body := request(t, admin, http.MethodGet, url, "")
	if status != http.StatusOK {
		t.Fatalf("admin: %d %s", status, body)
	}
	for _, username := range []string{"admin", "developer", "platform-engineer"} {
		if !strings.Contains(body, `"username":"`+username+`"`) {
			t.Fatalf("list misses %s: %s", username, body)
		}
	}
	if strings.Contains(body, "assword") || strings.Contains(body, adminTestPassword) {
		t.Fatalf("list leaks password material: %s", body)
	}
}

// UC-00 BR-09: an Admin creates accounts; the backend refuses non-Admins,
// duplicate usernames and invalid input without echoing the password.
func TestCreateUserValidationAndAuthorization(t *testing.T) {
	ts, _ := newUsersServer(t)
	url := ts.URL + "/api/v1/users"
	dev := signInClient(t, ts.URL, "developer", seed.FixedTestPassword)
	if status, body := request(t, dev, http.MethodPost, url, `{"username":"newdev","password":"password-1","role":"DEVELOPER"}`); status != http.StatusForbidden {
		t.Fatalf("developer create: %d %s", status, body)
	}

	admin := signInClient(t, ts.URL, "admin", adminTestPassword)
	for _, probe := range []struct {
		name, body, code, field string
		status                  int
	}{
		{"duplicate username", `{"username":"developer","password":"password-1","role":"DEVELOPER"}`, `"code":"DUPLICATE"`, `"field":"username"`, http.StatusConflict},
		{"bad username", `{"username":"No Good!","password":"password-1","role":"DEVELOPER"}`, `"code":"INVALID_REQUEST"`, `"field":"username"`, http.StatusBadRequest},
		{"bad role", `{"username":"newdev","password":"password-1","role":"SUPERUSER"}`, `"code":"INVALID_REQUEST"`, `"field":"role"`, http.StatusBadRequest},
		{"short password", `{"username":"newdev","password":"short","role":"DEVELOPER"}`, `"code":"INVALID_REQUEST"`, `"field":"password"`, http.StatusBadRequest},
		{"unknown field", `{"username":"newdev","password":"password-1","role":"DEVELOPER","org":"x"}`, `"code":"INVALID_REQUEST"`, ``, http.StatusBadRequest},
	} {
		status, body := request(t, admin, http.MethodPost, url, probe.body)
		if status != probe.status || !strings.Contains(body, probe.code) || (probe.field != `` && !strings.Contains(body, probe.field)) {
			t.Errorf("%s: %d %s, want %d %s %s", probe.name, status, body, probe.status, probe.code, probe.field)
		}
		if strings.Contains(body, "password-1") {
			t.Errorf("%s: response echoes the password: %s", probe.name, body)
		}
	}

	status, body := request(t, admin, http.MethodPost, url, `{"username":"newdev","password":"password-1","role":"DEVELOPER"}`)
	if status != http.StatusCreated || !strings.Contains(body, `"username":"newdev"`) || !strings.Contains(body, `"role":"DEVELOPER"`) {
		t.Fatalf("create: %d %s", status, body)
	}
	if strings.Contains(body, "password-1") || strings.Contains(body, "asswordHash") {
		t.Fatalf("create leaks password material: %s", body)
	}
	signInClient(t, ts.URL, "newdev", "password-1")
}

// UC-00 BR-08: a successful password change ends every session of the account;
// the old password stops working and the new one signs in.
func TestChangePasswordRevokesSessions(t *testing.T) {
	ts, _ := newUsersServer(t)
	url := ts.URL + "/api/v1/auth/change-password"

	first := signInClient(t, ts.URL, "developer", seed.FixedTestPassword)
	second := signInClient(t, ts.URL, "developer", seed.FixedTestPassword)

	if status, body := request(t, first, http.MethodPost, url, `{"currentPassword":"wrong","newPassword":"new-secret-1","confirmPassword":"new-secret-1"}`); status != http.StatusUnauthorized || !strings.Contains(body, "INVALID_CREDENTIALS") {
		t.Fatalf("wrong current: %d %s", status, body)
	}
	if status, body := request(t, first, http.MethodPost, url, `{"currentPassword":"`+seed.FixedTestPassword+`","newPassword":"new-secret-1","confirmPassword":"different-1"}`); status != http.StatusBadRequest || !strings.Contains(body, `"field":"confirmPassword"`) {
		t.Fatalf("confirm mismatch: %d %s", status, body)
	}
	if status, body := request(t, first, http.MethodPost, url, `{"currentPassword":"`+seed.FixedTestPassword+`","newPassword":"short","confirmPassword":"short"}`); status != http.StatusBadRequest || !strings.Contains(body, `"field":"newPassword"`) {
		t.Fatalf("short new: %d %s", status, body)
	}
	// Failed attempts leave both sessions alive.
	for name, c := range map[string]*http.Client{"first": first, "second": second} {
		if status, _ := request(t, c, http.MethodGet, ts.URL+"/api/v1/auth/session", ""); status != http.StatusOK {
			t.Fatalf("%s session died on a failed attempt", name)
		}
	}

	status, body := request(t, first, http.MethodPost, url, `{"currentPassword":"`+seed.FixedTestPassword+`","newPassword":"new-secret-1","confirmPassword":"new-secret-1"}`)
	if status != http.StatusNoContent || body != "" {
		t.Fatalf("change: %d %s", status, body)
	}
	// Every session of the account is revoked, including the current one.
	for name, c := range map[string]*http.Client{"first": first, "second": second} {
		if status, _ := request(t, c, http.MethodGet, ts.URL+"/api/v1/auth/session", ""); status != http.StatusUnauthorized {
			t.Fatalf("%s session survived the password change", name)
		}
	}
	// Other accounts keep their sessions.
	admin := signInClient(t, ts.URL, "admin", adminTestPassword)
	if status, _ := request(t, admin, http.MethodGet, ts.URL+"/api/v1/auth/session", ""); status != http.StatusOK {
		t.Fatal("admin session was revoked by another account's password change")
	}
	// Old password rejected, new password accepted.
	jar, _ := cookiejar.New(nil)
	old := &http.Client{Jar: jar}
	resp, err := old.Post(ts.URL+"/api/v1/auth/sign-in", "application/json", strings.NewReader(`{"username":"developer","password":"`+seed.FixedTestPassword+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("old password still works: %d", resp.StatusCode)
	}
	signInClient(t, ts.URL, "developer", "new-secret-1")
}

// Change password without a session is unauthorized.
func TestChangePasswordRequiresSignIn(t *testing.T) {
	ts, _ := newUsersServer(t)
	status, body := request(t, &http.Client{}, http.MethodPost, ts.URL+"/api/v1/auth/change-password", `{"currentPassword":"a","newPassword":"b","confirmPassword":"b"}`)
	if status != http.StatusUnauthorized || !strings.Contains(body, "UNAUTHORIZED") {
		t.Fatalf("%d %s", status, body)
	}
}

// UC-00 BR-11: an Admin resets the password of an account of the same
// Organization without the current one; every session of the target is
// revoked, the old password stops working and the new one signs in.
func TestResetUserPasswordRevokesTargetSessions(t *testing.T) {
	ts, _ := newUsersServer(t)
	url := ts.URL + "/api/v1/users/" + seed.FixedDeveloperAccountID + "/reset-password"

	first := signInClient(t, ts.URL, "developer", seed.FixedTestPassword)
	second := signInClient(t, ts.URL, "developer", seed.FixedTestPassword)
	admin := signInClient(t, ts.URL, "admin", adminTestPassword)

	// The target cannot reset through the Admin API, and the caller must be signed in.
	if status, body := request(t, first, http.MethodPost, url, `{"newPassword":"reset-secret-1","confirmPassword":"reset-secret-1"}`); status != http.StatusForbidden || !strings.Contains(body, "FORBIDDEN") {
		t.Fatalf("developer reset: %d %s", status, body)
	}
	if status, body := request(t, &http.Client{}, http.MethodPost, url, `{"newPassword":"reset-secret-1","confirmPassword":"reset-secret-1"}`); status != http.StatusUnauthorized || !strings.Contains(body, "UNAUTHORIZED") {
		t.Fatalf("anonymous reset: %d %s", status, body)
	}
	if status, body := request(t, admin, http.MethodPost, url, `{"newPassword":"reset-secret-1","confirmPassword":"different-1"}`); status != http.StatusBadRequest || !strings.Contains(body, `"field":"confirmPassword"`) {
		t.Fatalf("confirm mismatch: %d %s", status, body)
	}
	if status, body := request(t, admin, http.MethodPost, url, `{"newPassword":"short","confirmPassword":"short"}`); status != http.StatusBadRequest || !strings.Contains(body, `"field":"newPassword"`) {
		t.Fatalf("short new: %d %s", status, body)
	}
	if status, body := request(t, admin, http.MethodPost, ts.URL+"/api/v1/users/no-such-user/reset-password", `{"newPassword":"reset-secret-1","confirmPassword":"reset-secret-1"}`); status != http.StatusNotFound || !strings.Contains(body, "NOT_FOUND") {
		t.Fatalf("unknown account: %d %s", status, body)
	}
	// Failed attempts leave both target sessions alive.
	for name, c := range map[string]*http.Client{"first": first, "second": second} {
		if status, _ := request(t, c, http.MethodGet, ts.URL+"/api/v1/auth/session", ""); status != http.StatusOK {
			t.Fatalf("%s session died on a failed reset attempt", name)
		}
	}

	status, body := request(t, admin, http.MethodPost, url, `{"newPassword":"reset-secret-1","confirmPassword":"reset-secret-1"}`)
	if status != http.StatusNoContent || body != "" {
		t.Fatalf("reset: %d %s", status, body)
	}
	// Every session of the target account is revoked; the Admin session survives.
	for name, c := range map[string]*http.Client{"first": first, "second": second} {
		if status, _ := request(t, c, http.MethodGet, ts.URL+"/api/v1/auth/session", ""); status != http.StatusUnauthorized {
			t.Fatalf("%s session survived the password reset", name)
		}
	}
	if status, _ := request(t, admin, http.MethodGet, ts.URL+"/api/v1/auth/session", ""); status != http.StatusOK {
		t.Fatal("admin session was revoked by the reset of another account")
	}
	// Old password rejected, new password accepted.
	jar, _ := cookiejar.New(nil)
	old := &http.Client{Jar: jar}
	resp, err := old.Post(ts.URL+"/api/v1/auth/sign-in", "application/json", strings.NewReader(`{"username":"developer","password":"`+seed.FixedTestPassword+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("old password still works: %d", resp.StatusCode)
	}
	signInClient(t, ts.URL, "developer", "reset-secret-1")
}
