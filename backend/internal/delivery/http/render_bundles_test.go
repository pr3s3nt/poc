package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/application/authentication"
	"orchestrator/internal/domain/identity"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/platform/password"
	"orchestrator/internal/seed"
)

func bundleTestServer(t *testing.T, bundles map[string]resource.RenderBundle) (*Server, *httptest.Server, *store.Store) {
	t.Helper()
	st := store.New()
	opts := seed.Defaults()
	if err := seed.Apply(context.Background(), st, opts); err != nil {
		t.Fatal(err)
	}
	hash, err := password.Hash(seed.FixedTestPassword)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range []identity.UserAccount{
		{ID: "other-admin", OrganizationKey: "other-org", Username: "other-admin", PasswordHash: hash, Role: identity.RoleAdmin, Status: identity.AccountActive},
	} {
		if err := st.SaveUserAccount(context.Background(), a); err != nil {
			t.Fatal(err)
		}
	}
	s := NewServer(Config{RenderBundles: bundles, Authentication: authentication.NewService(st), Store: st, SeedOptions: opts})
	ts := httptest.NewServer(s)
	t.Cleanup(ts.Close)
	return s, ts, st
}

func bundleClient(t *testing.T, base, user string) *http.Client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar}
	resp, err := c.Post(base+"/api/v1/auth/sign-in", "application/json", strings.NewReader(`{"username":"`+user+`","password":"`+seed.FixedTestPassword+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("sign-in %s: %d", user, resp.StatusCode)
	}
	return c
}

func getBundles(t *testing.T, c *http.Client, base string) (int, string) {
	t.Helper()
	resp, err := c.Get(base + "/api/v1/render-bundles")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		sb.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return resp.StatusCode, sb.String()
}

func TestRenderBundlesAuthAndRole(t *testing.T) {
	_, ts, _ := bundleTestServer(t, map[string]resource.RenderBundle{"b": {ID: "b", Version: "1", Digest: "d", BinaryDigest: "bd"}})
	if code, _ := getBundles(t, &http.Client{}, ts.URL); code != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", code)
	}
	if code, body := getBundles(t, bundleClient(t, ts.URL, "developer"), ts.URL); code != http.StatusForbidden || strings.Contains(body, `"b"`) {
		t.Fatalf("developer: %d %s", code, body)
	}
	if code, _ := getBundles(t, bundleClient(t, ts.URL, "platform-engineer"), ts.URL); code != http.StatusOK {
		t.Fatalf("platform engineer: %d", code)
	}
	if code, _ := getBundles(t, bundleClient(t, ts.URL, "other-admin"), ts.URL); code != http.StatusOK {
		t.Fatalf("admin of another organization: %d", code)
	}
}

func TestRenderBundlesEmptyIsArray(t *testing.T) {
	for name, in := range map[string]map[string]resource.RenderBundle{"nil": nil, "empty": {}, "all invalid": {"x": {ID: "x"}}} {
		_, ts, _ := bundleTestServer(t, in)
		code, body := getBundles(t, bundleClient(t, ts.URL, "platform-engineer"), ts.URL)
		if code != http.StatusOK || strings.TrimSpace(body) != `{"renderBundles":[]}` {
			t.Fatalf("%s: %d %s", name, code, body)
		}
	}
}

func TestRenderBundlesSortedSafeMetadataOnly(t *testing.T) {
	bundles := map[string]resource.RenderBundle{
		"zeta":      {ID: "zeta", Version: "2.0", Digest: "SECRET-DIGEST-Z", BinaryDigest: "SECRET-BIN-Z"},
		"alpha":     {ID: "alpha", Version: "1.0", Digest: "SECRET-DIGEST-A", BinaryDigest: "SECRET-BIN-A"},
		"mismatch":  {ID: "other", Version: "1", Digest: "d", BinaryDigest: "b"},
		"no-ver":    {ID: "no-ver", Digest: "d", BinaryDigest: "b"},
		"no-digest": {ID: "no-digest", Version: "1", BinaryDigest: "b"},
		"no-binary": {ID: "no-binary", Version: "1", Digest: "d"},
		"":          {Version: "1", Digest: "d", BinaryDigest: "b"},
	}
	_, ts, _ := bundleTestServer(t, bundles)
	code, body := getBundles(t, bundleClient(t, ts.URL, "platform-engineer"), ts.URL)
	if code != http.StatusOK {
		t.Fatal(code, body)
	}
	for _, leak := range []string{"SECRET", "igest", "inary", "path", "source"} {
		if strings.Contains(body, leak) {
			t.Fatalf("response leaks %q: %s", leak, body)
		}
	}
	var got struct {
		RenderBundles []map[string]string `json:"renderBundles"`
	}
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatal(err)
	}
	want := []map[string]string{{"id": "alpha", "version": "1.0", "status": "AVAILABLE"}, {"id": "zeta", "version": "2.0", "status": "AVAILABLE"}}
	if len(got.RenderBundles) != 2 {
		t.Fatalf("got %s", body)
	}
	for i, w := range want {
		if len(got.RenderBundles[i]) != 3 || got.RenderBundles[i]["id"] != w["id"] || got.RenderBundles[i]["version"] != w["version"] || got.RenderBundles[i]["status"] != "AVAILABLE" {
			t.Fatalf("entry %d: %v", i, got.RenderBundles[i])
		}
	}
}

func TestRenderBundlesReadOnlyAndRegistrationRevalidates(t *testing.T) {
	s, ts, st := bundleTestServer(t, map[string]resource.RenderBundle{"b": {ID: "b", Version: "1", Digest: "d", BinaryDigest: "bd"}})
	pe := bundleClient(t, ts.URL, "platform-engineer")
	before, err := st.ListResourceDefinitions(context.Background(), seed.Defaults().OrganizationKey)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if code, _ := getBundles(t, pe, ts.URL); code != http.StatusOK {
			t.Fatal(code)
		}
	}
	if resp, err := pe.Post(ts.URL+"/api/v1/render-bundles", "application/json", strings.NewReader(`{}`)); err != nil {
		t.Fatal(err)
	} else if resp.Body.Close(); resp.StatusCode < 400 {
		t.Fatalf("POST render-bundles: %d", resp.StatusCode)
	}
	after, _ := st.ListResourceDefinitions(context.Background(), seed.Defaults().OrganizationKey)
	if len(after) != len(before) {
		t.Fatal("GET render-bundles changed persisted Definitions")
	}
	// A listed bundle can become unavailable; registration still rejects it before insert.
	s.catalog.SetRenderBundles(nil)
	body := `{"key":"tpl-gone","resourceType":"workload","executionProfile":"internal-k8s","driverType":"score-k8s","driverInputs":{"values":{"variables":{"render_bundle":"b"}}},"criteria":[{}]}`
	resp, err := pe.Post(ts.URL+"/api/v1/resource-definitions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unavailable bundle registration: %d", resp.StatusCode)
	}
	if final, _ := st.ListResourceDefinitions(context.Background(), seed.Defaults().OrganizationKey); len(final) != len(before) {
		t.Fatal("unavailable registration inserted a Definition")
	}
}
