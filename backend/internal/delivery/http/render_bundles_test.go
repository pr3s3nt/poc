package http

import (
	"context"
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

func bundleTestServer(t *testing.T, bundles map[string]resource.RenderBundle) (*Server, *httptest.Server, *store.Store) {
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
	s := NewServer(Config{RenderBundles: bundles, Authentication: authentication.NewService(st), Store: st, ModuleRegistry: registrytest.Open(t, st), SeedOptions: opts})
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
