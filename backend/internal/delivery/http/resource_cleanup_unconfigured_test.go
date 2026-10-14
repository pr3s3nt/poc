package http

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/application/authentication"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/seed/seedtest"
)

// TestResourceRoutesWithoutTheResourceService: a server built without the
// optional ResourceCleanup service (an older launcher or test wiring) answers
// the canonical 503 on valid scoped reads and mutations instead of panicking.
// Authentication and scope come first (401, 404) and a Developer is denied 403
// on both mutations before the missing service or the body matter. There is no
// in-memory or State fallback.
func TestResourceRoutesWithoutTheResourceService(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	opts := seedtest.Defaults()
	if err := seedtest.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	app, env := opts.ApplicationKey, "staging"
	if err := st.SaveEnvironment(ctx, environment.Environment{ApplicationKey: app, Key: env, NamespaceIdentity: app + "-staging"}); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(NewServer(Config{Authentication: authentication.NewService(st), Store: st, SeedOptions: opts}))
	defer ts.Close()
	const id = "11111111-1111-4111-8111-111111111111"
	scope := ts.URL + "/api/v1/applications/" + app + "/environments/" + env
	call := func(c *http.Client, method, url, body string) (int, string) {
		t.Helper()
		req, _ := http.NewRequest(method, url, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v (the handler must not panic)", method, url, err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(raw)
	}
	type route struct{ method, path, body string }
	reads := []route{
		{http.MethodGet, scope + "/resources", ""},
		{http.MethodGet, scope + "/resources/" + id, ""},
		{http.MethodGet, scope + "/resource-cleanups/" + id, ""},
	}
	mutations := []route{
		{http.MethodPost, scope + "/resources/" + id + "/deletion-preview", `{}`},
		{http.MethodPost, scope + "/resources/" + id + "/deletions", `{"token":"x","confirmationName":"n","selectedResourceIds":["` + id + `"]}`},
	}
	pe := bundleClient(t, ts.URL, "platform-engineer")
	dev := bundleClient(t, ts.URL, "developer")

	for _, r := range append(append([]route{}, reads...), mutations...) {
		if status, _ := call(&http.Client{}, r.method, r.path, r.body); status != http.StatusUnauthorized {
			t.Errorf("signed out %s %s = %d, want 401", r.method, r.path, status)
		}
	}
	for _, r := range reads {
		for name, c := range map[string]*http.Client{"developer": dev, "platform-engineer": pe} {
			status, body := call(c, r.method, r.path, r.body)
			if status != http.StatusServiceUnavailable {
				t.Errorf("%s %s %s = %d %s, want 503", name, r.method, r.path, status, body)
			}
			var envelope map[string]any
			if json.Unmarshal([]byte(body), &envelope) != nil || envelope["code"] == nil {
				t.Errorf("%s %s: not the canonical error envelope: %s", name, r.path, body)
			}
		}
	}
	for _, r := range mutations {
		// A Developer is denied before the missing service and before the body is read.
		for _, body := range []string{r.body, `not json`, ``} {
			if status, _ := call(dev, r.method, r.path, body); status != http.StatusForbidden {
				t.Errorf("developer %s %q = %d, want 403", r.path, body, status)
			}
		}
		if status, body := call(pe, r.method, r.path, r.body); status != http.StatusServiceUnavailable {
			t.Errorf("platform-engineer %s = %d %s, want 503", r.path, status, body)
		}
	}
	// A foreign or unknown scope stays 404 for everyone, before the service matters.
	foreign := ts.URL + "/api/v1/applications/" + app + "/environments/qa/resources/" + id
	other := ts.URL + "/api/v1/applications/00000000-0000-4000-8000-000000000000/environments/staging/resources"
	for _, path := range []string{foreign, other} {
		for name, c := range map[string]*http.Client{"developer": dev, "platform-engineer": pe} {
			if status, body := call(c, http.MethodGet, path, ""); status != http.StatusNotFound {
				t.Errorf("%s GET %s = %d %s, want 404", name, path, status, body)
			}
		}
	}
}
