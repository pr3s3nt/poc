package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"orchestrator/internal/adapters/store"
	"orchestrator/internal/application/authentication"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/seed"
	"orchestrator/internal/seed/seedtest"
	"strings"
	"testing"
)

func TestCatalogSourcesRoleCASAndCredentialURLGuards(t *testing.T) {
	st := store.New()
	opts := seedtest.Defaults()
	if err := seedtest.Apply(context.Background(), st, opts); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveApplication(context.Background(), application.Application{Key: opts.ApplicationKey, OrganizationKey: opts.OrganizationKey, Name: "Catalog", Subdomain: "catalog"}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveEnvironment(context.Background(), environment.Environment{Key: "staging", ApplicationKey: opts.ApplicationKey, NamespaceIdentity: "catalog-staging"}); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(NewServer(Config{Store: st, Authentication: authentication.NewService(st), SeedOptions: opts}))
	defer ts.Close()
	base := ts.URL + "/api/v1/platform/catalog/sources"
	developer := signInClient(t, ts.URL, "developer", seed.FixedTestPassword)
	engineer := signInClient(t, ts.URL, "platform-engineer", seed.FixedTestPassword)
	if status, _ := request(t, developer, http.MethodGet, base, ""); status != 403 {
		t.Fatal("Developer read platform catalog", status)
	}
	valid := `{"repository":"https://example.com/catalog.git","expectedVersion":0}`
	if status, _ := request(t, developer, http.MethodPut, base+"/main", valid); status != 403 {
		t.Fatal("Developer wrote platform catalog", status)
	}
	status, body := request(t, engineer, http.MethodPut, base+"/main", valid)
	if status != 200 || !strings.Contains(body, `"branch":"main"`) || strings.Contains(body, "observedTags") {
		t.Fatal(status, body)
	}
	if status, _ := request(t, engineer, http.MethodPut, base+"/main", valid); status != 409 {
		t.Fatal("stale source save accepted", status)
	}
	invalid := `{"repository":"https://reader:secret-sentinel@example.com/catalog.git","expectedVersion":0}`
	status, body = request(t, engineer, http.MethodPut, base+"/unsafe", invalid)
	if status != 400 || strings.Contains(body, "secret-sentinel") {
		t.Fatal("unsafe URL or diagnostic", status, body)
	}
	if status, _ := request(t, engineer, http.MethodPost, base+"/main/refresh", ""); status != 503 {
		t.Fatal("missing discovery reported success", status)
	}
	if status, _ := request(t, developer, http.MethodGet, ts.URL+"/api/v1/applications/"+opts.ApplicationKey+"/environments/staging/catalog", ""); status != 200 {
		t.Fatal("Environment catalog permission/path failed", status)
	}
}

func TestCatalogReleaseDetailsAreScopedReadOnlyAlongsideEditors(t *testing.T) {
	_, ts, st := bundleTestServer(t, nil)
	opts := seedtest.Defaults()
	pe := bundleClient(t, ts.URL, "platform-engineer")
	base := ts.URL + "/api/v1/platform/catalog/sources/test-fixture/releases/v1.0.0"
	for _, user := range []string{"developer", "other-admin"} {
		status, _ := request(t, bundleClient(t, ts.URL, user), http.MethodGet, base, "")
		want := http.StatusForbidden
		if user == "other-admin" {
			want = http.StatusNotFound
		}
		if status != want {
			t.Fatalf("%s: %d", user, status)
		}
	}
	before, _ := st.ListResourceDefinitions(context.Background(), opts.OrganizationKey)
	status, body := request(t, pe, http.MethodGet, base, "")
	if status != 200 || !strings.Contains(body, `"components"`) || !strings.Contains(body, `"snapshot"`) || !strings.Contains(body, `"criteria"`) {
		t.Fatal(status, body)
	}
	for _, forbidden := range []string{"secretRef", "observedTags", "tokenConfigured"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("detail exposes %s", forbidden)
		}
	}
	after, _ := st.ListResourceDefinitions(context.Background(), opts.OrganizationKey)
	if len(before) != len(after) {
		t.Fatal("detail mutated global Definitions")
	}
	for _, route := range []string{"resource-definitions", "template-sources"} {
		status, _ := request(t, pe, http.MethodGet, ts.URL+"/api/v1/"+route, "")
		if status != http.StatusOK {
			t.Fatalf("restored %s: %d", route, status)
		}
	}
}

func TestCatalogModuleMetadataRequiresPlatformRoleAndScopedRelease(t *testing.T) {
	_, ts, _ := bundleTestServer(t, nil)
	base := ts.URL + "/api/v1/platform/catalog/sources/test-fixture/releases/v1.0.0/modules?resourceType=postgres"
	for _, tc := range []struct {
		user   string
		status int
	}{{"platform-engineer", 200}, {"developer", 403}, {"other-admin", 404}} {
		status, body := request(t, bundleClient(t, ts.URL, tc.user), http.MethodGet, base, "")
		if status != tc.status {
			t.Fatal(tc.user, status, body)
		}
		if status == 200 && !strings.Contains(body, `"modules":[]`) {
			t.Fatal("empty module list must be explicit", body)
		}
	}
}
