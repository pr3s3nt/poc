package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"orchestrator/internal/adapters/terraform/registry/registrytest"
	"strings"
	"testing"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/application/authentication"
	"orchestrator/internal/domain/identity"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/platform/password"
	"orchestrator/internal/seed"
	"orchestrator/internal/seed/seedtest"
)

func bundleTestServer(t *testing.T, bundles map[string]resource.TemplateSource) (*Server, *httptest.Server, *store.Store) {
	t.Helper()
	st := store.New()
	opts := seedtest.Defaults()
	if err := seedtest.Apply(context.Background(), st, opts); err != nil {
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
	s := NewServer(Config{TemplateSources: bundles, Authentication: authentication.NewService(st), Store: st, ModuleRegistry: registrytest.Open(t, st), SeedOptions: opts})
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
	resp, err := c.Get(base + "/api/v1/template-sources")
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

func TestTemplateSourcesAuthAndRole(t *testing.T) {
	_, ts, _ := bundleTestServer(t, map[string]resource.TemplateSource{"b": {ID: "b", Version: "1", Digest: "d", BinaryDigest: "bd"}})
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

func TestTemplateSourcesEmptyIsArray(t *testing.T) {
	for name, in := range map[string]map[string]resource.TemplateSource{"nil": nil, "empty": {}, "all invalid": {"x": {ID: "x"}}} {
		_, ts, _ := bundleTestServer(t, in)
		code, body := getBundles(t, bundleClient(t, ts.URL, "platform-engineer"), ts.URL)
		if code != http.StatusOK || strings.TrimSpace(body) != `{"templateSources":[]}` {
			t.Fatalf("%s: %d %s", name, code, body)
		}
	}
}

func TestTemplateSourcesSortedSafeMetadataOnly(t *testing.T) {
	bundles := map[string]resource.TemplateSource{
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
		TemplateSources []map[string]string `json:"templateSources"`
	}
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatal(err)
	}
	want := []map[string]string{{"id": "alpha", "version": "1.0", "status": "AVAILABLE"}, {"id": "zeta", "version": "2.0", "status": "AVAILABLE"}}
	if len(got.TemplateSources) != 2 {
		t.Fatalf("got %s", body)
	}
	for i, w := range want {
		if len(got.TemplateSources[i]) != 3 || got.TemplateSources[i]["id"] != w["id"] || got.TemplateSources[i]["version"] != w["version"] || got.TemplateSources[i]["status"] != "AVAILABLE" {
			t.Fatalf("entry %d: %v", i, got.TemplateSources[i])
		}
	}
}

func TestTemplateSourcesReadOnlyAndRegistrationRevalidates(t *testing.T) {
	s, ts, st := bundleTestServer(t, map[string]resource.TemplateSource{"b": {ID: "b", Version: "1", Digest: "d", BinaryDigest: "bd"}})
	pe := bundleClient(t, ts.URL, "platform-engineer")
	before, err := st.ListResourceDefinitions(context.Background(), seedtest.Defaults().OrganizationKey)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if code, _ := getBundles(t, pe, ts.URL); code != http.StatusOK {
			t.Fatal(code)
		}
	}
	if resp, err := pe.Post(ts.URL+"/api/v1/template-sources", "application/json", strings.NewReader(`{}`)); err != nil {
		t.Fatal(err)
	} else if resp.Body.Close(); resp.StatusCode < 400 {
		t.Fatalf("POST template-sources: %d", resp.StatusCode)
	}
	after, _ := st.ListResourceDefinitions(context.Background(), seedtest.Defaults().OrganizationKey)
	if len(after) != len(before) {
		t.Fatal("GET template-sources changed persisted Definitions")
	}
	// A listed bundle can become unavailable; registration still rejects it before insert.
	s.catalog.SetTemplateSources(nil)
	body := `{"key":"tpl-gone","resourceType":"workload","executionProfile":"internal-k8s","driverType":"template","driverInputs":{"values":{"variables":{"template_source":"b"}}},"criteria":[{}]}`
	resp, err := pe.Post(ts.URL+"/api/v1/resource-definitions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unavailable bundle registration: %d", resp.StatusCode)
	}
	if final, _ := st.ListResourceDefinitions(context.Background(), seedtest.Defaults().OrganizationKey); len(final) != len(before) {
		t.Fatal("unavailable registration inserted a Definition")
	}
}
