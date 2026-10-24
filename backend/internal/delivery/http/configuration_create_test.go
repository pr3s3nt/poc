package http

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"orchestrator/internal/adapters/configmemory"
	"orchestrator/internal/adapters/store"
	"orchestrator/internal/application/authentication"
	appconfig "orchestrator/internal/application/configuration"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/identity"
	"orchestrator/internal/platform/password"
	"orchestrator/internal/seed"
	"orchestrator/internal/seed/seedtest"
)

func postCreateKey(t *testing.T, c *http.Client, url, body string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

// OC-12 create-only: strict body, 201 with the safe Configuration projection,
// typed errors and no write on any rejection.
func TestCreateConfigurationKeyHandler(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	opts := seedtest.Defaults()
	if err := seedtest.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	hash, _ := password.Hash(seed.FixedTestPassword)
	if err := st.SaveUserAccount(ctx, identity.UserAccount{ID: "other-admin", OrganizationKey: "other-org", Username: "other-admin", PasswordHash: hash, Role: identity.RoleAdmin, Status: identity.AccountActive}); err != nil {
		t.Fatal(err)
	}
	cfg := appconfig.NewService(st, configmemory.NewRegistry(st), nil)
	app, env := opts.ApplicationKey, "staging"
	if err := st.SaveEnvironment(ctx, environment.Environment{ApplicationKey: app, Key: env, NamespaceIdentity: app + "-staging"}); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(NewServer(Config{Authentication: authentication.NewService(st), Store: st, SeedOptions: opts, Configurations: cfg}))
	defer ts.Close()
	url := ts.URL + "/api/v1/applications/" + app + "/environments/" + env + "/configuration/keys"
	dev := bundleClient(t, ts.URL, "developer")

	snapshot := func() string {
		scope, _ := st.GetConfigurationScope(ctx, app, env)
		b, _ := json.Marshal(scope)
		return string(b)
	}
	before := snapshot()
	for _, probe := range []struct {
		name, body string
		status     int
		code       string
	}{
		{"missing name", `{"kind":"VARIABLE","value":"v","version":0}`, 400, "INVALID_REQUEST"},
		{"missing kind", `{"name":"A","value":"v","version":0}`, 400, "INVALID_REQUEST"},
		{"missing value", `{"name":"A","kind":"VARIABLE","version":0}`, 400, "INVALID_REQUEST"},
		{"missing version", `{"name":"A","kind":"VARIABLE","value":"v"}`, 400, "INVALID_REQUEST"},
		{"null value", `{"name":"A","kind":"VARIABLE","value":null,"version":0}`, 400, "INVALID_REQUEST"},
		{"null version", `{"name":"A","kind":"VARIABLE","value":"v","version":null}`, 400, "INVALID_REQUEST"},
		{"string version", `{"name":"A","kind":"VARIABLE","value":"v","version":"0"}`, 400, "INVALID_REQUEST"},
		{"negative version", `{"name":"A","kind":"VARIABLE","value":"v","version":-1}`, 400, "INVALID_REQUEST"},
		{"float version", `{"name":"A","kind":"VARIABLE","value":"v","version":0.5}`, 400, "INVALID_REQUEST"},
		{"number value", `{"name":"A","kind":"VARIABLE","value":1,"version":0}`, 400, "INVALID_REQUEST"},
		{"unknown field", `{"name":"A","kind":"VARIABLE","value":"v","version":0,"canary":1}`, 400, "INVALID_REQUEST"},
		{"trailing JSON", `{"name":"A","kind":"VARIABLE","value":"v","version":0}{}`, 400, "INVALID_REQUEST"},
		{"null body", `null`, 400, "INVALID_REQUEST"},
		{"bad kind", `{"name":"A","kind":"OTHER","value":"v","version":0}`, 400, "INVALID_VARIABLE"},
		{"bad name", `{"name":"1A","kind":"VARIABLE","value":"v","version":0}`, 400, "INVALID_VARIABLE"},
		{"empty value", `{"name":"A","kind":"VARIABLE","value":"","version":0}`, 400, "INVALID_VARIABLE"},
		{"stale version", `{"name":"A","kind":"VARIABLE","value":"v","version":9}`, 409, "STALE_VERSION"},
		{"secret without store", `{"name":"A","kind":"SECRET","value":"canary-secret","version":0}`, 422, "SECRET_STORE_REQUIRED"},
	} {
		status, body := postCreateKey(t, dev, url, probe.body)
		if status != probe.status || body["code"] != probe.code {
			t.Errorf("%s: %d %v, want %d %s", probe.name, status, body, probe.status, probe.code)
		}
		if raw, _ := json.Marshal(body); strings.Contains(string(raw), "canary") {
			t.Errorf("%s: envelope echoes input: %s", probe.name, raw)
		}
		if snapshot() != before {
			t.Fatalf("%s wrote state", probe.name)
		}
	}
	if status, _ := postCreateKey(t, &http.Client{}, url, `{"name":"A","kind":"VARIABLE","value":"v","version":0}`); status != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", status)
	}
	if status, _ := postCreateKey(t, bundleClient(t, ts.URL, "other-admin"), url, `{"name":"A","kind":"VARIABLE","value":"v","version":0}`); status != http.StatusNotFound {
		t.Fatalf("other organization: %d", status)
	}

	status, body := postCreateKey(t, dev, url, `{"name":"REGION","kind":"VARIABLE","value":"eu","version":0}`)
	if status != http.StatusCreated || body["version"] != float64(1) {
		t.Fatalf("create: %d %v", status, body)
	}
	after := snapshot()
	for _, kind := range []string{"VARIABLE", "SECRET"} {
		status, body = postCreateKey(t, dev, url, `{"name":"REGION","kind":"`+kind+`","value":"other","version":1}`)
		if status != http.StatusConflict || body["code"] != "DUPLICATE" {
			t.Fatalf("duplicate %s: %d %v", kind, status, body)
		}
	}
	if snapshot() != after {
		t.Fatal("a duplicate create changed the scope")
	}
	view, _ := cfg.List(ctx, app, env)
	if len(view.Keys) != 1 || view.Keys[0].Value == nil || *view.Keys[0].Value != "eu" {
		t.Fatalf("existing key was overwritten: %+v", view.Keys)
	}
}
