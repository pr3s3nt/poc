package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/bootstrap"
	appdomain "orchestrator/internal/domain/application"
	"orchestrator/internal/domain/identity"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/platform/password"
	"orchestrator/internal/seed"
)

// UC-00/UC-01 onboarding over HTTP: sign-in, session restore, sign-out and
// self-service Application creation scoped by the session Organization.

func buildOnboardingApp(t *testing.T, profile, statePath string) (*bootstrap.App, *httptest.Server) {
	t.Helper()
	seedOptions := seed.Defaults()
	seedOptions.Profile = profile
	seedOptions.Region = "us-east-1"
	seedOptions.AccountID = "000000000000"
	app, err := bootstrap.Build(context.Background(), bootstrap.Options{Seed: seedOptions, Adapters: bootstrap.AdapterFake, StatePath: statePath})
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	server := httptest.NewServer(app.Server)
	t.Cleanup(server.Close)
	return app, server
}

func newJarClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Jar: jar}
}

// call sends a JSON request and tolerates empty bodies such as 204 responses.
func call(t *testing.T, client *http.Client, method, url, body string) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(resp.Body)
	return resp, buf.String()
}

func decode(t *testing.T, body string) map[string]any {
	t.Helper()
	out := map[string]any{}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("decode %q: %v", body, err)
	}
	return out
}

func TestUC00SignInSessionRestoreAndSignOut(t *testing.T) {
	_, server := buildOnboardingApp(t, "test", "")
	client := newJarClient(t)
	protected := []string{"/api/v1/auth/session", "/api/v1/applications"}
	for _, path := range protected {
		if resp, _ := call(t, client, http.MethodGet, server.URL+path, ""); resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("anonymous %s = %d", path, resp.StatusCode)
		}
	}

	for _, creds := range []string{`{"username":"developer","password":"wrong"}`, `{"username":"nobody","password":"test-password"}`} {
		resp, body := call(t, client, http.MethodPost, server.URL+"/api/v1/auth/sign-in", creds)
		if resp.StatusCode != http.StatusUnauthorized || len(resp.Cookies()) != 0 || decode(t, body)["error"] != "invalid credentials" {
			t.Fatalf("invalid sign-in = %d %v %s", resp.StatusCode, resp.Cookies(), body)
		}
	}

	resp, body := call(t, client, http.MethodPost, server.URL+"/api/v1/auth/sign-in", `{"username":"developer","password":"test-password"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("sign-in = %d %s", resp.StatusCode, body)
	}
	cookies := resp.Cookies()
	if len(cookies) != 1 || cookies[0].Name != "orchestrator_session" || !cookies[0].HttpOnly || cookies[0].Path != "/" || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatalf("session cookie = %+v", cookies)
	}
	if strings.Contains(body, cookies[0].Value) || strings.Contains(body, "test-password") || strings.Contains(strings.ToLower(body), "hash") {
		t.Fatalf("sign-in response leaks token or password: %s", body)
	}
	user := decode(t, body)["user"].(map[string]any)
	if user["Username"] != "developer" || user["Role"] != string(identity.RoleDeveloper) || user["OrganizationKey"] != "acme" {
		t.Fatalf("sign-in identity = %v", user)
	}

	resp, body = call(t, client, http.MethodGet, server.URL+"/api/v1/auth/session", "")
	if resp.StatusCode != http.StatusOK || decode(t, body)["user"].(map[string]any)["Username"] != "developer" || strings.Contains(body, cookies[0].Value) {
		t.Fatalf("session restore = %d %s", resp.StatusCode, body)
	}

	stolen := cookies[0].Value
	resp, _ = call(t, client, http.MethodPost, server.URL+"/api/v1/auth/sign-out", "")
	if resp.StatusCode != http.StatusNoContent || len(resp.Cookies()) != 1 || resp.Cookies()[0].MaxAge >= 0 {
		t.Fatalf("sign-out = %d %+v", resp.StatusCode, resp.Cookies())
	}
	for _, path := range protected {
		if resp, _ := call(t, client, http.MethodGet, server.URL+path, ""); resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("%s after sign-out = %d", path, resp.StatusCode)
		}
	}
	// A copied token must also stop working: sign-out revokes it server side.
	req, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/applications", nil)
	req.AddCookie(&http.Cookie{Name: "orchestrator_session", Value: stolen})
	replay, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	replay.Body.Close()
	if replay.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked token replay = %d", replay.StatusCode)
	}
}

func TestProductionProfile_DoesNotSeedFixedTestAccounts(t *testing.T) {
	ctx := context.Background()
	statePath := filepath.Join(t.TempDir(), "state.json")
	localApp, local := buildOnboardingApp(t, "local", statePath)
	for _, id := range seed.FixedTestAccountIDs() {
		if _, err := localApp.Store.GetUserAccount(ctx, id); err != nil {
			t.Fatalf("local profile must seed fixed account %s: %v", id, err)
		}
	}
	resp, _ := call(t, newJarClient(t), http.MethodPost, local.URL+"/api/v1/auth/sign-in", `{"username":"developer","password":"test-password"}`)
	if resp.StatusCode != http.StatusOK || len(resp.Cookies()) != 1 {
		t.Fatalf("local sign-in = %d", resp.StatusCode)
	}
	localToken := resp.Cookies()[0].Value
	local.Close()

	fresh, freshServer := buildOnboardingApp(t, "production", "")
	if _, err := fresh.Store.GetUserAccountByUsername(ctx, "developer"); err == nil {
		t.Fatal("production profile must not seed fixed test accounts")
	}
	// A production account that merely shares the username is not a fixed
	// seeded account and must keep working.
	hash, err := password.Hash("production-password")
	if err != nil {
		t.Fatal(err)
	}
	if err := fresh.Store.SaveUserAccount(ctx, identity.UserAccount{ID: "7c9e6679-7425-40de-944b-e07fc1f90ae7", OrganizationKey: "acme", Username: "developer", PasswordHash: hash, Role: identity.RoleDeveloper, Status: identity.AccountActive}); err != nil {
		t.Fatal(err)
	}
	if resp, body := call(t, newJarClient(t), http.MethodPost, freshServer.URL+"/api/v1/auth/sign-in", `{"username":"developer","password":"production-password"}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("production account named developer = %d %s", resp.StatusCode, body)
	}

	// Reusing state written by a local profile must not re-enable the fixed
	// accounts, neither for sign-in nor for a session created before restart.
	_, production := buildOnboardingApp(t, "production", statePath)
	for _, username := range []string{"developer", "platform-engineer"} {
		resp, _ := call(t, newJarClient(t), http.MethodPost, production.URL+"/api/v1/auth/sign-in", `{"username":"`+username+`","password":"test-password"}`)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("production sign-in as fixed %s = %d", username, resp.StatusCode)
		}
	}
	req, _ := http.NewRequest(http.MethodGet, production.URL+"/api/v1/auth/session", nil)
	req.AddCookie(&http.Cookie{Name: "orchestrator_session", Value: localToken})
	restore, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	restore.Body.Close()
	if restore.StatusCode != http.StatusUnauthorized {
		t.Fatalf("production restore of fixed-account session = %d", restore.StatusCode)
	}
}

// writeLegacyAccountState writes JSON state shaped like a database seeded
// before fixed accounts had stable IDs: random IDs for developer and
// platform-engineer, each with the given password.
func writeLegacyAccountState(t *testing.T, statePath string, passwords map[string]string) map[string]string {
	t.Helper()
	ctx := context.Background()
	st, err := store.NewWithSnapshot(statePath)
	if err != nil {
		t.Fatal(err)
	}
	opts := seed.Defaults()
	opts.Profile = "production"
	if err := seed.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	accountIDs := map[string]string{}
	err = st.Transact(ctx, func(ctx context.Context) error {
		for username, raw := range passwords {
			hash, err := password.Hash(raw)
			if err != nil {
				return err
			}
			id := ids.New()
			role := identity.RoleDeveloper
			if username == "platform-engineer" {
				role = identity.RolePlatformEngineer
			}
			if err := st.SaveUserAccount(ctx, identity.UserAccount{ID: id, OrganizationKey: opts.OrganizationKey, Username: username, PasswordHash: hash, Role: role, Status: identity.AccountActive}); err != nil {
				return err
			}
			accountIDs[username] = id
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for username, id := range accountIDs {
		for _, fixed := range seed.FixedTestAccountIDs() {
			if id == fixed {
				t.Fatalf("legacy %s must not use a stable fixed ID", username)
			}
		}
	}
	return accountIDs
}

func signInStatus(t *testing.T, baseURL, username, rawPassword string) (int, string) {
	t.Helper()
	resp, _ := call(t, newJarClient(t), http.MethodPost, baseURL+"/api/v1/auth/sign-in", `{"username":"`+username+`","password":"`+rawPassword+`"}`)
	token := ""
	if len(resp.Cookies()) == 1 {
		token = resp.Cookies()[0].Value
	}
	return resp.StatusCode, token
}

func restoreStatus(t *testing.T, baseURL, token string) int {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, baseURL+"/api/v1/auth/session", nil)
	req.AddCookie(&http.Cookie{Name: "orchestrator_session", Value: token})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func TestProductionProfile_RejectsLegacyFixedAccountsWithRandomIDs(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "legacy.json")
	writeLegacyAccountState(t, statePath, map[string]string{"developer": seed.FixedTestPassword, "platform-engineer": seed.FixedTestPassword})

	// The legacy accounts work under the local profile, which keeps their IDs.
	_, local := buildOnboardingApp(t, "local", statePath)
	tokens := map[string]string{}
	for _, username := range []string{"developer", "platform-engineer"} {
		status, token := signInStatus(t, local.URL, username, seed.FixedTestPassword)
		if status != http.StatusOK || token == "" {
			t.Fatalf("local sign-in as legacy %s = %d", username, status)
		}
		tokens[username] = token
	}
	local.Close()

	_, production := buildOnboardingApp(t, "production", statePath)
	for username, token := range tokens {
		if status, _ := signInStatus(t, production.URL, username, seed.FixedTestPassword); status != http.StatusUnauthorized {
			t.Fatalf("production sign-in as legacy fixed %s = %d", username, status)
		}
		if status := restoreStatus(t, production.URL, token); status != http.StatusUnauthorized {
			t.Fatalf("production restore of legacy fixed %s session = %d", username, status)
		}
	}
}

func TestProductionProfile_AllowsFixedUsernameWithOtherPassword(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "production.json")
	writeLegacyAccountState(t, statePath, map[string]string{"developer": "production-password", "platform-engineer": "another-production-password"})

	_, production := buildOnboardingApp(t, "production", statePath)
	for username, raw := range map[string]string{"developer": "production-password", "platform-engineer": "another-production-password"} {
		status, token := signInStatus(t, production.URL, username, raw)
		if status != http.StatusOK || token == "" {
			t.Fatalf("production account %s with its own password = %d", username, status)
		}
		if status := restoreStatus(t, production.URL, token); status != http.StatusOK {
			t.Fatalf("production account %s session restore = %d", username, status)
		}
		if status, _ := signInStatus(t, production.URL, username, seed.FixedTestPassword); status != http.StatusUnauthorized {
			t.Fatalf("fixed password must not work for production %s = %d", username, status)
		}
	}
}

func TestUC01CreateApplicationRequiresExactlyOneJSONObject(t *testing.T) {
	app, server := buildOnboardingApp(t, "test", "")
	client := authenticatedClient(t, server.URL)
	before, _ := app.Store.ListApplications(context.Background())
	for _, body := range []string{
		`{"name":"Evil","subdomain":"evil","organizationKey":"globex"}`,
		`{"name":"Broken","subdomain":"broken"`,
		`not json`,
		``,
		`null`,
		`["name","subdomain"]`,
		`{"name":"Twice","subdomain":"twice"}{"name":"Again","subdomain":"again"}`,
		`{"name":"Trailing","subdomain":"trailing"} []`,
		`{"name":"Trailing","subdomain":"trailing"} x`,
	} {
		resp, out := call(t, client, http.MethodPost, server.URL+"/api/v1/applications", body)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("%q = %d %s", body, resp.StatusCode, out)
		}
	}
	if apps, _ := app.Store.ListApplications(context.Background()); len(apps) != len(before) {
		t.Fatalf("rejected bodies must not create Applications: %d -> %d", len(before), len(apps))
	}
	if resp, out := call(t, client, http.MethodPost, server.URL+"/api/v1/applications", " \n{\"name\":\"Ok\",\"subdomain\":\"ok\"}\n "); resp.StatusCode != http.StatusCreated {
		t.Fatalf("surrounding whitespace is valid: %d %s", resp.StatusCode, out)
	}
}

func TestUC01CreateApplicationOverHTTP(t *testing.T) {
	app, server := buildOnboardingApp(t, "test", "")
	client := authenticatedClient(t, server.URL)
	deploymentsBefore := len(app.FakeDeploy.Applied)

	if resp, _ := call(t, http.DefaultClient, http.MethodPost, server.URL+"/api/v1/applications", `{"name":"Anon","subdomain":"anon"}`); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous create = %d", resp.StatusCode)
	}
	for _, body := range []string{
		`{"name":"Evil","subdomain":"evil","organizationKey":"globex"}`,
		`{"name":"Evil","subdomain":"evil","role":"ADMIN"}`,
		`{"name":"Evil","subdomain":"evil","executionProfile":"aws-eks","connectionKey":"aws-account"}`,
	} {
		if resp, out := call(t, client, http.MethodPost, server.URL+"/api/v1/applications", body); resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("trusted field accepted: %s = %d %s", body, resp.StatusCode, out)
		}
	}

	resp, body := call(t, client, http.MethodPost, server.URL+"/api/v1/applications", `{"name":"Catalog","subdomain":"Catalog"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create = %d %s", resp.StatusCode, body)
	}
	created := decode(t, body)["application"].(map[string]any)
	key := created["key"].(string)
	if created["subdomain"] != "catalog" || created["executionProfile"] != "internal-k8s" || created["runtimeStatus"] != "READY" || len(key) != 36 {
		t.Fatalf("created application = %v", created)
	}
	assertEnvironments := func(view map[string]any) {
		t.Helper()
		envs := view["environments"].([]any)
		keys := []string{}
		for _, raw := range envs {
			env := raw.(map[string]any)
			keys = append(keys, env["key"].(string))
			if env["currentDeploymentSetId"] == "" || env["namespaceIdentity"] == "" {
				t.Fatalf("environment = %v", env)
			}
		}
		if strings.Join(keys, ",") != "staging,production" && strings.Join(keys, ",") != "production,staging" {
			t.Fatalf("environments = %v", keys)
		}
	}
	assertEnvironments(created)
	resp, body = call(t, client, http.MethodGet, server.URL+"/api/v1/applications/"+key, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get = %d %s", resp.StatusCode, body)
	}
	assertEnvironments(decode(t, body)["application"].(map[string]any))

	for _, env := range []string{"staging", "production"} {
		resp, body = call(t, client, http.MethodGet, server.URL+"/api/v1/applications/"+key+"/environments/"+env+"/workloads", "")
		if resp.StatusCode != http.StatusOK || len(decode(t, body)["workloads"].([]any)) != 0 {
			t.Fatalf("%s workloads = %d %s", env, resp.StatusCode, body)
		}
	}
	resp, body = call(t, client, http.MethodGet, server.URL+"/api/v1/deployments?application="+key, "")
	deployments, _ := decode(t, body)["deployments"].([]any)
	if resp.StatusCode != http.StatusOK || len(deployments) != 0 || len(app.FakeDeploy.Applied) != deploymentsBefore {
		t.Fatalf("create must have no deploy side effect: %d %s", resp.StatusCode, body)
	}

	for _, tc := range []struct {
		body, field string
		status      int
	}{
		{`{"name":"catalog","subdomain":"other"}`, "name", http.StatusConflict},
		{`{"name":"Other","subdomain":"CATALOG"}`, "subdomain", http.StatusConflict},
		{`{"name":"","subdomain":"other"}`, "name", http.StatusBadRequest},
		{`{"name":"Other","subdomain":"not_a_label"}`, "subdomain", http.StatusBadRequest},
	} {
		resp, out := call(t, client, http.MethodPost, server.URL+"/api/v1/applications", tc.body)
		if resp.StatusCode != tc.status || decode(t, out)["field"] != tc.field {
			t.Fatalf("%s = %d %s", tc.body, resp.StatusCode, out)
		}
	}
}

func TestAuthenticationMiddleware_UsesSessionOrganizationAndRole(t *testing.T) {
	app, server := buildOnboardingApp(t, "test", "")
	ctx := context.Background()
	hash, err := password.Hash("test-password")
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Store.SaveOrganization(ctx, appdomain.Organization{ID: "org-globex", Key: "globex", Name: "Globex", DefaultConnectionKey: "globex-cluster"}); err != nil {
		t.Fatal(err)
	}
	if err := app.Store.SaveConnection(ctx, appdomain.Connection{ID: "conn-globex", Key: "globex-cluster", OrganizationKey: "globex", Kind: appdomain.ConnectionKubernetes, Status: appdomain.ConnectionVerifying}); err != nil {
		t.Fatal(err)
	}
	if err := app.Store.SaveUserAccount(ctx, identity.UserAccount{ID: "user-globex", OrganizationKey: "globex", Username: "globex-developer", PasswordHash: hash, Role: identity.RoleDeveloper, Status: identity.AccountActive}); err != nil {
		t.Fatal(err)
	}
	acme := authenticatedClient(t, server.URL)
	globex := authenticatedClientAs(t, server.URL, "globex-developer")
	for client, want := range map[*http.Client]string{globex: "globex/DEVELOPER", authenticatedClientAs(t, server.URL, "platform-engineer"): "acme/PLATFORM_ENGINEER"} {
		_, body := call(t, client, http.MethodGet, server.URL+"/api/v1/auth/session", "")
		user := decode(t, body)["user"].(map[string]any)
		if got := user["OrganizationKey"].(string) + "/" + user["Role"].(string); got != want {
			t.Fatalf("session identity = %s, want %s", got, want)
		}
	}

	resp, body := call(t, acme, http.MethodPost, server.URL+"/api/v1/applications", `{"name":"Catalog","subdomain":"catalog"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("acme create = %d %s", resp.StatusCode, body)
	}
	key := decode(t, body)["application"].(map[string]any)["key"].(string)

	resp, body = call(t, globex, http.MethodGet, server.URL+"/api/v1/applications", "")
	if resp.StatusCode != http.StatusOK || len(decode(t, body)["applications"].([]any)) != 0 {
		t.Fatalf("globex list must be empty: %d %s", resp.StatusCode, body)
	}
	if resp, _ := call(t, globex, http.MethodGet, server.URL+"/api/v1/applications/"+key, ""); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-organization get = %d", resp.StatusCode)
	}
	// Globex default target is not READY, so its Developer cannot create.
	resp, body = call(t, globex, http.MethodPost, server.URL+"/api/v1/applications", `{"name":"Catalog","subdomain":"globex-catalog"}`)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("not-ready target create = %d %s", resp.StatusCode, body)
	}
	resp, body = call(t, acme, http.MethodGet, server.URL+"/api/v1/applications", "")
	found := false
	for _, raw := range decode(t, body)["applications"].([]any) {
		found = found || raw.(map[string]any)["key"] == key
	}
	if resp.StatusCode != http.StatusOK || !found {
		t.Fatalf("acme list must include its Application: %s", body)
	}
}
