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
