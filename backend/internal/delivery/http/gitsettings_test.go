package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"orchestrator/internal/adapters/credentialmemory"
	"orchestrator/internal/adapters/store"
	"orchestrator/internal/application/authentication"
	"orchestrator/internal/application/gitaccess"
	access "orchestrator/internal/ports/gitaccess"
	"orchestrator/internal/seed"
	"orchestrator/internal/seed/seedtest"
)

type gitTesterStub struct {
	calls int
	token string
}

func (g *gitTesterStub) TestGit(_ context.Context, _, _ string, a access.Access) error {
	g.calls++
	g.token = a.Token
	return nil
}
func TestGitSettingsRoleScopeAndWriteOnlyToken(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	opts := seedtest.Defaults()
	if err := seedtest.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	tester := &gitTesterStub{}
	ts := httptest.NewServer(NewServer(Config{Store: st, Authentication: authentication.NewService(st), GitAccess: gitaccess.New(st, credentialmemory.New()), GitTester: tester, SeedOptions: opts}))
	defer ts.Close()
	base := ts.URL + "/api/v1/platform/settings/git"
	d := `{"url":"https://git.example.com/team","username":"reader","authenticated":true,"token":"secret-sentinel","caPem":"","expectedVersion":0}`
	for _, tc := range []struct {
		user   string
		status int
	}{{"developer", http.StatusForbidden}, {"platform-engineer", http.StatusOK}} {
		client := signInClient(t, ts.URL, tc.user, seed.FixedTestPassword)
		status, body := request(t, client, http.MethodPut, base, d)
		if status != tc.status {
			t.Fatalf("%s status %d: %s", tc.user, status, body)
		}
		if strings.Contains(body, "secret-sentinel") || strings.Contains(body, "credentialRef") {
			t.Fatal("credential leaked")
		}
		if tc.status == http.StatusOK {
			status, body = request(t, client, http.MethodGet, base, "")
			if status != http.StatusOK || !strings.Contains(body, `"tokenConfigured":true`) || strings.Contains(body, "secret-sentinel") {
				t.Fatal("unsafe settings view")
			}
			status, _ = request(t, client, http.MethodPut, base, d)
			if status != http.StatusConflict {
				t.Fatal("stale update accepted")
			}
			test := `{"url":"https://git.example.com/team","username":"reader","authenticated":true,"token":"","caPem":"","expectedVersion":1,"repositoryUrl":"https://git.example.com/team/catalog.git","ref":"main"}`
			status, _ = request(t, client, http.MethodPost, base+"/test", test)
			if status != http.StatusOK || tester.calls != 1 || tester.token != "secret-sentinel" {
				t.Fatal("draft test did not use retained credential")
			}
		}
	}
}
