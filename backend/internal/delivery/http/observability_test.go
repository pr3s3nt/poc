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
	obs "orchestrator/internal/application/observability"
	"orchestrator/internal/domain/identity"
	"orchestrator/internal/platform/password"
	"orchestrator/internal/ports/persistence/persistencetest"
	"orchestrator/internal/seed"
)

func TestObservabilityAuthenticatedStoredHistory(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	fixture := persistencetest.Telemetry(t, st)
	hash, err := password.Hash(seed.FixedTestPassword)
	if err != nil {
		t.Fatal(err)
	}
	for _, user := range []struct{ name, org string }{{"telemetry-user", fixture.Scope.OrganizationKey}, {"foreign-user", "foreign"}} {
		if err := st.SaveUserAccount(ctx, identity.UserAccount{ID: user.name, OrganizationKey: user.org, Username: user.name, PasswordHash: hash, Role: identity.RoleDeveloper, Status: identity.AccountActive}); err != nil {
			t.Fatal(err)
		}
	}
	ts := httptest.NewServer(NewServer(Config{Store: st, Authentication: authentication.NewService(st), Observability: obs.New(st, nil)}))
	defer ts.Close()
	base := ts.URL + "/api/v1/applications/" + fixture.Scope.ApplicationKey + "/environments/staging/observability/"
	user := bundleClient(t, ts.URL, "telemetry-user")
	foreign := bundleClient(t, ts.URL, "foreign-user")
	read := func(client *http.Client, url string) (int, []byte) {
		t.Helper()
		response, err := client.Get(url)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		raw, _ := io.ReadAll(response.Body)
		return response.StatusCode, raw
	}
	if status, _ := read(http.DefaultClient, base+"logs"); status != 401 {
		t.Fatalf("unauthenticated: %d", status)
	}
	if status, _ := read(foreign, base+"logs"); status != 404 {
		t.Fatalf("foreign: %d", status)
	}
	jobs := strings.TrimSuffix(base, "observability/") + "workloads/backup/jobs"
	if status, _ := read(http.DefaultClient, jobs); status != 401 {
		t.Fatalf("unauthenticated jobs: %d", status)
	}
	if status, _ := read(foreign, jobs); status != 404 {
		t.Fatalf("foreign jobs: %d", status)
	}
	if status, _ := read(user, jobs); status != 503 {
		t.Fatalf("jobs without reader must be unavailable, not empty success: %d", status)
	}
	if status, _ := read(user, jobs+"?namespace=foreign"); status != 400 {
		t.Fatalf("jobs target override: %d", status)
	}
	for _, kind := range []string{"overview", "logs", "metrics"} {
		status, raw := read(user, base+kind)
		if status != 200 {
			t.Fatalf("%s %d %s", kind, status, raw)
		}
		var view obs.View
		if err := json.Unmarshal(raw, &view); err != nil {
			t.Fatal(err)
		}
		if !view.HasHistory || view.State != "UNAVAILABLE" {
			t.Fatalf("history projection: %s", raw)
		}
		for _, private := range []string{"checkpoints", "leaseUntil", "targetHash", "owner", "fence"} {
			if strings.Contains(string(raw), `"`+private+`"`) {
				t.Fatalf("private field: %s", private)
			}
		}
	}
	for _, query := range []string{"?from=invalid", "?level=stderr", "?limit=201", "?limit=1&limit=2", "?focus=cpu", "?cursor=invalid"} {
		if status, _ := read(user, base+"logs"+query); status != 400 {
			t.Fatalf("query %s: %d", query, status)
		}
	}
	if status, raw := read(user, base+"logs?level=ERROR&limit=1&q=duplicate"); status != 200 || !strings.Contains(string(raw), `"logCount":2`) || !strings.Contains(string(raw), `"nextCursor"`) {
		t.Fatalf("filtered page %d: %s", status, raw)
	}
}
