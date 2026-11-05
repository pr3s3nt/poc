package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"orchestrator/internal/adapters/postgres/postgrestest"
	"orchestrator/internal/adapters/store"
	appcreate "orchestrator/internal/application/application"
	"orchestrator/internal/application/authentication"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/identity"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/platform/password"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/seed"
	"orchestrator/internal/seed/seedtest"
)

func TestApplicationOwnershipHTTP(t *testing.T) {
	testApplicationOwnershipHTTP(t, store.New())
}

func TestApplicationOwnershipHTTPPostgres(t *testing.T) {
	db := postgrestest.Fresh(t)
	testApplicationOwnershipHTTP(t, postgrestest.OpenConsole(t, db))
}

func testApplicationOwnershipHTTP(t *testing.T, st persistence.Store) {
	t.Helper()
	ctx := context.Background()
	if err := st.SaveOrganization(ctx, application.Organization{ID: ids.New(), Key: "foreign", Name: "Foreign"}); err != nil {
		t.Fatal(err)
	}
	devTwoID := ids.New()
	opts := seedtest.Defaults()
	if err := seedtest.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	hash, err := password.Hash(seed.FixedTestPassword)
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range []identity.UserAccount{
		{ID: devTwoID, Username: "dev-two", OrganizationKey: opts.OrganizationKey, Role: identity.RoleDeveloper},
		{ID: ids.New(), Username: "admin", OrganizationKey: opts.OrganizationKey, Role: identity.RoleAdmin},
		{ID: ids.New(), Username: "foreign-platform", OrganizationKey: "foreign", Role: identity.RolePlatformEngineer},
	} {
		u.PasswordHash, u.Status = hash, identity.AccountActive
		if err := st.SaveUserAccount(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	server := NewServer(Config{Store: st, Authentication: authentication.NewService(st), Applications: appcreate.NewService(st), SeedOptions: opts})
	ts := httptest.NewServer(server)
	defer ts.Close()
	devOne := signInClient(t, ts.URL, "developer", seed.FixedTestPassword)
	devTwo := signInClient(t, ts.URL, "dev-two", seed.FixedTestPassword)
	platform := signInClient(t, ts.URL, "platform-engineer", seed.FixedTestPassword)
	admin := signInClient(t, ts.URL, "admin", seed.FixedTestPassword)
	foreign := signInClient(t, ts.URL, "foreign-platform", seed.FixedTestPassword)
	create := func(client *http.Client, name string) string {
		t.Helper()
		status, body := request(t, client, "POST", ts.URL+"/api/v1/applications", `{"name":"`+name+`","subdomain":"`+name+`"}`)
		if status != 201 {
			t.Fatalf("create: %d %s", status, body)
		}
		var view struct {
			Application struct {
				Key string `json:"key"`
			} `json:"application"`
		}
		if err := json.Unmarshal([]byte(body), &view); err != nil {
			t.Fatal(err)
		}
		return view.Application.Key
	}
	one, two := create(devOne, "one"), create(devTwo, "two")
	for key, want := range map[string]string{one: seed.FixedDeveloperAccountID, two: devTwoID} {
		app, err := st.GetApplication(ctx, key)
		if err != nil || app.OwnerUserID != want {
			t.Fatalf("owner was not derived from session: %+v %v", app, err)
		}
	}
	if err := st.SaveApplication(ctx, application.Application{ID: ids.New(), Key: "retained", OrganizationKey: opts.OrganizationKey, Name: "Retained", Subdomain: "retained"}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name            string
		client          *http.Client
		visible, hidden []string
	}{
		{"first developer", devOne, []string{one, opts.ApplicationKey}, []string{two, "retained"}},
		{"second developer", devTwo, []string{two}, []string{one, opts.ApplicationKey, "retained"}},
		{"platform", platform, []string{one, two, "retained", opts.ApplicationKey}, nil},
		{"admin", admin, []string{one, two, "retained", opts.ApplicationKey}, nil},
		{"foreign platform", foreign, nil, []string{one, two, "retained", opts.ApplicationKey}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, body := request(t, tc.client, "GET", ts.URL+"/api/v1/applications", "")
			if status != 200 {
				t.Fatalf("list: %d %s", status, body)
			}
			var list struct {
				Applications []struct {
					Key string `json:"key"`
				} `json:"applications"`
			}
			if err := json.Unmarshal([]byte(body), &list); err != nil {
				t.Fatal(err)
			}
			got := map[string]bool{}
			for _, a := range list.Applications {
				got[a.Key] = true
			}
			if len(got) != len(tc.visible) {
				t.Fatalf("unexpected list: %s", body)
			}
			for _, key := range tc.visible {
				if !got[key] {
					t.Errorf("missing visible app %s", key)
				}
				if status, body := request(t, tc.client, "GET", ts.URL+"/api/v1/applications/"+key, ""); status != 200 {
					t.Errorf("visible detail: %d %s", status, body)
				}
			}
			for _, key := range tc.hidden {
				if got[key] {
					t.Errorf("listed hidden app %s", key)
				}
				if status, body := request(t, tc.client, "GET", ts.URL+"/api/v1/applications/"+key, ""); status != 404 {
					t.Errorf("hidden detail: %d %s", status, body)
				}
			}
		})
	}
	// Forging a creator or role is rejected by the public creation contract.
	for _, extra := range []string{`"ownerUserId":"dev-two"`, `"role":"PLATFORM_ENGINEER"`, `"organizationKey":"foreign"`} {
		if status, body := request(t, devOne, "POST", ts.URL+"/api/v1/applications", `{"name":"forged","subdomain":"forged",`+extra+`}`); status != 400 {
			t.Fatalf("forged creator: %d %s", status, body)
		}
	}
	// Every nested endpoint must deny before decoding or dispatching side effects.
	endpoints := []struct{ method, path string }{
		{"GET", ""}, {"HEAD", ""},
		{"GET", "/environments/staging/configuration"},
		{"PUT", "/environments/staging/configuration/keys/SECRET"},
		{"GET", "/environments/staging/workloads"},
		{"PUT", "/environments/staging/workloads/backend"},
		{"GET", "/environments/staging/workload-templates"},
		{"GET", "/environments/staging/catalog"},
		{"GET", "/environments/staging/definition-versions"},
		{"GET", "/environments/staging/deployments"},
		{"POST", "/environments/staging/preview"},
		{"POST", "/environments/staging/deploy"},
		{"GET", "/environments/staging/operations"},
		{"POST", "/environments/staging/operations/op/recover"},
		{"GET", "/environments/staging/resources"},
		{"GET", "/environments/staging/observability/logs"},
		{"GET", "/environments/staging/observability/metrics"},
		{"GET", "/environments/staging/workloads/backup/jobs"},
		{"POST", "/deletion-preview"}, {"POST", "/deletions"}, {"GET", "/deletions/op"},
	}
	for _, endpoint := range endpoints {
		url := ts.URL + "/api/v1/applications/" + one + endpoint.path
		if status, body := request(t, devTwo, endpoint.method, url, "malformed request"); status != 404 {
			t.Errorf("%s %s: %d %s", endpoint.method, endpoint.path, status, body)
		}
		if status, body := request(t, &http.Client{}, endpoint.method, url, ""); status != 401 {
			t.Errorf("anonymous %s %s: %d %s", endpoint.method, endpoint.path, status, body)
		}
	}
	// A new nested route automatically inherits authorization; handlers cannot
	// accidentally expose app data by omitting their own scope check.
	calls := 0
	server.mux.HandleFunc("GET /api/v1/applications/{id}/future-read", func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(204) })
	for _, client := range []*http.Client{devOne, platform, admin} {
		if status, _ := request(t, client, "GET", ts.URL+"/api/v1/applications/"+one+"/future-read", ""); status != 204 {
			t.Fatalf("owner/platform nested access: %d", status)
		}
	}
	for _, client := range []*http.Client{devTwo, foreign} {
		if status, _ := request(t, client, "GET", ts.URL+"/api/v1/applications/"+one+"/future-read", ""); status != 404 {
			t.Fatalf("nested access escaped: %d", status)
		}
	}
	if calls != 3 {
		t.Fatalf("unauthorized handler executed: %d", calls)
	}
	// Hiding an Application also hides its lifecycle, even when it is deleting.
	app, _ := st.GetApplication(ctx, one)
	app.Status = application.StatusDeleting
	if err := st.SaveApplication(ctx, app); err != nil {
		t.Fatal(err)
	}
	if status, body := request(t, devTwo, "POST", ts.URL+"/api/v1/applications/"+one+"/environments/staging/deploy", ""); status != 404 || strings.Contains(body, "DELETING") {
		t.Fatalf("lifecycle leak: %d %s", status, body)
	}
}
