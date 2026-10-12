package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"orchestrator/internal/adapters/configmemory"
	"orchestrator/internal/adapters/store"
	"orchestrator/internal/application/authentication"
	appconfig "orchestrator/internal/application/configuration"
	domain "orchestrator/internal/domain/configuration"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/identity"
	"orchestrator/internal/platform/password"
	"orchestrator/internal/seed"
	"orchestrator/internal/seed/seedtest"
)

func TestRenameOptInDecoderIsStrict(t *testing.T) {
	cases := []struct {
		name  string
		body  string
		valid bool
		want  appconfig.ReferenceGuards
	}{
		{"omitted", `{"newName":"N","version":1}`, true, appconfig.ReferenceGuards{}},
		{"false", `{"newName":"N","version":1,"updateReferences":false}`, true, appconfig.ReferenceGuards{}},
		{"false ignores guards", `{"newName":"N","version":1,"updateReferences":false,"environmentVersion":3}`, true, appconfig.ReferenceGuards{}},
		{"true", `{"newName":"N","version":1,"updateReferences":true,"environmentVersion":3,"draftVersion":0}`, true, appconfig.ReferenceGuards{UpdateReferences: true, EnvironmentVersion: 3}},
		{"true missing env", `{"newName":"N","version":1,"updateReferences":true,"draftVersion":0}`, false, appconfig.ReferenceGuards{}},
		{"true missing draft", `{"newName":"N","version":1,"updateReferences":true,"environmentVersion":1}`, false, appconfig.ReferenceGuards{}},
		{"true null env", `{"newName":"N","version":1,"updateReferences":true,"environmentVersion":null,"draftVersion":0}`, false, appconfig.ReferenceGuards{}},
		{"true null draft", `{"newName":"N","version":1,"updateReferences":true,"environmentVersion":1,"draftVersion":null}`, false, appconfig.ReferenceGuards{}},
		{"string guard", `{"newName":"N","version":1,"updateReferences":true,"environmentVersion":"1","draftVersion":0}`, false, appconfig.ReferenceGuards{}},
		{"float guard", `{"newName":"N","version":1,"updateReferences":true,"environmentVersion":1.5,"draftVersion":0}`, false, appconfig.ReferenceGuards{}},
		{"exponent guard", `{"newName":"N","version":1,"updateReferences":true,"environmentVersion":1e2,"draftVersion":0}`, false, appconfig.ReferenceGuards{}},
		{"negative guard", `{"newName":"N","version":1,"updateReferences":true,"environmentVersion":-1,"draftVersion":0}`, false, appconfig.ReferenceGuards{}},
		{"bool guard", `{"newName":"N","version":1,"updateReferences":true,"environmentVersion":true,"draftVersion":0}`, false, appconfig.ReferenceGuards{}},
		{"string flag", `{"newName":"N","version":1,"updateReferences":"true"}`, false, appconfig.ReferenceGuards{}},
		{"null flag", `{"newName":"N","version":1,"updateReferences":null}`, false, appconfig.ReferenceGuards{}},
		{"number flag", `{"newName":"N","version":1,"updateReferences":1}`, false, appconfig.ReferenceGuards{}},
	}
	for _, tc := range cases {
		var req renameConfigurationKeyRequest
		if err := json.Unmarshal([]byte(tc.body), &req); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		got, valid := req.referenceGuards()
		if valid != tc.valid || (valid && got != tc.want) {
			t.Errorf("%s: got %+v valid=%v, want %+v valid=%v", tc.name, got, valid, tc.want, tc.valid)
		}
	}
}

func patchRename(t *testing.T, c *http.Client, url, body string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPatch, url, strings.NewReader(body))
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

func TestRenameHandlerStrictDecodingAndSafeEnvelopes(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	opts := seed.Defaults()
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
	if _, err := cfg.Put(ctx, app, env, "OLD", domain.Variable, "v", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := cfg.Put(ctx, app, env, "DUP", domain.Variable, "d", 1); err != nil {
		t.Fatal(err)
	}
	draftScore := map[string]any{"apiVersion": "score.dev/v1b1", "metadata": map[string]any{"name": "api"},
		"containers": map[string]any{"web": map[string]any{"image": "i", "variables": map[string]any{"A": "${resources.env.OLD}"}}},
		"resources":  map[string]any{"env": map[string]any{"type": "environment"}}}
	current, _ := st.GetEnvironment(ctx, app, env)
	if err := st.SaveWorkloadDraft(ctx, current.DraftVersion, environment.WorkloadDraft{ApplicationKey: app, EnvironmentKey: env, WorkloadID: "api", State: environment.DraftUpsert, Score: draftScore}); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(NewServer(Config{Authentication: authentication.NewService(st), Store: st, SeedOptions: opts, Configurations: cfg}))
	defer ts.Close()
	url := ts.URL + "/api/v1/applications/" + app + "/environments/" + env + "/configuration/keys/OLD"

	view, _ := cfg.List(ctx, app, env)
	good := fmt.Sprintf(`"newName":"NEW","version":%d,"updateReferences":true,"environmentVersion":%d,"draftVersion":%d`, view.Version, view.EnvironmentVersion, view.DraftVersion)
	snapshot := func() string {
		scope, _ := st.GetConfigurationScope(ctx, app, env)
		e, _ := st.GetEnvironment(ctx, app, env)
		drafts, _ := st.ListWorkloadDrafts(ctx, app, env)
		b, _ := json.Marshal([]any{scope, e.DraftVersion, e.Version, drafts})
		return string(b)
	}
	before := snapshot()
	dev := bundleClient(t, ts.URL, "developer")
	probes := []struct {
		name, body string
		status     int
		code       string
	}{
		{"trailing JSON", `{` + good + `}{"x":1}`, 400, "INVALID_REQUEST"},
		{"trailing garbage", `{` + good + `} junk`, 400, "INVALID_REQUEST"},
		{"unknown field", `{` + good + `,"extra-canary":1}`, 400, "INVALID_REQUEST"},
		{"top-level null", `null`, 400, "INVALID_REQUEST"},
		{"array", `[]`, 400, "INVALID_REQUEST"},
		{"missing guard", fmt.Sprintf(`{"newName":"NEW","version":%d,"updateReferences":true,"draftVersion":%d}`, view.Version, view.DraftVersion), 400, "INVALID_REQUEST"},
		{"null guard", fmt.Sprintf(`{"newName":"NEW","version":%d,"updateReferences":true,"environmentVersion":null,"draftVersion":%d}`, view.Version, view.DraftVersion), 400, "INVALID_REQUEST"},
		{"string guard", fmt.Sprintf(`{"newName":"NEW","version":%d,"updateReferences":true,"environmentVersion":"%d","draftVersion":%d}`, view.Version, view.EnvironmentVersion, view.DraftVersion), 400, "INVALID_REQUEST"},
		{"float guard", fmt.Sprintf(`{"newName":"NEW","version":%d,"updateReferences":true,"environmentVersion":1.5,"draftVersion":%d}`, view.Version, view.DraftVersion), 400, "INVALID_REQUEST"},
		{"string flag", `{"newName":"NEW","version":1,"updateReferences":"true"}`, 400, "INVALID_REQUEST"},
		{"zero environment guard", fmt.Sprintf(`{"newName":"NEW","version":%d,"updateReferences":true,"environmentVersion":0,"draftVersion":%d}`, view.Version, view.DraftVersion), 409, "STALE_VERSION"},
		{"stale draft guard", fmt.Sprintf(`{"newName":"NEW","version":%d,"updateReferences":true,"environmentVersion":%d,"draftVersion":%d}`, view.Version, view.EnvironmentVersion, view.DraftVersion+1), 409, "STALE_VERSION"},
		{"duplicate", fmt.Sprintf(`{"newName":"DUP","version":%d,"updateReferences":true,"environmentVersion":%d,"draftVersion":%d}`, view.Version, view.EnvironmentVersion, view.DraftVersion), 409, "DUPLICATE"},
	}
	for _, probe := range probes {
		status, body := patchRename(t, dev, url, probe.body)
		if status != probe.status || body["code"] != probe.code {
			t.Errorf("%s: %d %v, want %d %s", probe.name, status, body, probe.status, probe.code)
		}
		if raw, _ := json.Marshal(body); strings.Contains(string(raw), "canary") || strings.Contains(string(raw), "OLD") {
			t.Errorf("%s: envelope echoes input: %s", probe.name, raw)
		}
		if snapshot() != before {
			t.Fatalf("%s wrote state", probe.name)
		}
	}
	if status, _ := patchRename(t, &http.Client{}, url, `{`+good+`}`); status != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", status)
	}
	if status, _ := patchRename(t, bundleClient(t, ts.URL, "other-admin"), url, `{`+good+`}`); status != http.StatusNotFound {
		t.Fatalf("other organization: %d", status)
	}
	if snapshot() != before {
		t.Fatal("rejected requests wrote state")
	}
	status, body := patchRename(t, dev, url, `{`+good+`}`)
	if status != http.StatusOK || body["version"] != float64(view.Version+1) || body["draftVersion"] != float64(view.DraftVersion+1) || body["environmentVersion"] != float64(view.EnvironmentVersion) {
		t.Fatalf("opt-in: %d %v", status, body)
	}
	drafts, _ := st.ListWorkloadDrafts(ctx, app, env)
	if raw, _ := json.Marshal(drafts); !strings.Contains(string(raw), "${resources.env.NEW}") || strings.Contains(string(raw), "resources.env.OLD") {
		t.Fatalf("draft not rewritten: %s", raw)
	}
}
