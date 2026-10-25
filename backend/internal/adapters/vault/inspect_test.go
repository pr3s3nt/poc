package vault

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"orchestrator/internal/adapters/credentialmemory"
	domain "orchestrator/internal/domain/diagnostics"
	"orchestrator/internal/domain/secretstore"
	"orchestrator/internal/ports/persistence"
)

type oneStore struct{ rec secretstore.Store }

func (o oneStore) GetSecretStore(_ context.Context, _, key string) (secretstore.Store, error) {
	if key != o.rec.Key {
		return secretstore.Store{}, persistence.ErrNotFound
	}
	return o.rec, nil
}

// inspectFixture serves the fake Vault KV v2 plus the health endpoint, whose
// answer is chosen by the test.
func inspectFixture(t *testing.T, fv *fakeVault, token string, health int) (StoreInspector, string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v1/sys/health") {
			w.WriteHeader(health)
			return
		}
		fv.handle(w, r)
	}))
	t.Cleanup(server.Close)
	creds := credentialmemory.New()
	rec := secretstore.Store{Key: "vault-a", OrganizationKey: "acme", Name: "Vault", Provider: secretstore.ProviderVaultKV2, BackendAddress: server.URL, WorkloadAddress: server.URL, Mount: "kv", AuthMount: "kubernetes", Status: secretstore.StatusReady}
	ref, err := creds.Put(context.Background(), "acme", rec.CredentialScope(), []byte(token))
	if err != nil {
		t.Fatal(err)
	}
	rec.CredentialRef = ref
	return StoreInspector{Stores: oneStore{rec}, Credentials: creds}, "vault-a"
}

func TestInspectDistinguishesAvailabilityTokenCapabilityAndWorkloadAuthEvidence(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name    string
		setup   func(*fakeVault)
		token   string
		health  int
		verdict domain.Verdict
		reason  string
	}{
		{"ready", func(*fakeVault) {}, "good", 200, domain.VerdictPass, "STORE_AVAILABLE"},
		{"token rejected", func(*fakeVault) {}, "stale-token", 200, domain.VerdictFail, "STORE_TOKEN_REJECTED"},
		{"sealed", func(*fakeVault) {}, "good", 503, domain.VerdictFail, "STORE_SEALED"},
		{"uninitialized", func(*fakeVault) {}, "good", 501, domain.VerdictFail, "STORE_UNINITIALIZED"},
		// A valid token on a store whose mount or grants are wrong is not usable.
		{"mount is not KV v2", func(f *fakeVault) { f.notKV2 = true }, "good", 200, domain.VerdictFail, "STORE_NOT_KV2"},
		{"token lacks policy capability", func(f *fakeVault) { f.capabilities = map[string][]string{"sys/policies": {"read"}} }, "good", 200, domain.VerdictFail, "STORE_CAPABILITY_MISSING"},
		{"token lacks value capability", func(f *fakeVault) { f.capabilities = map[string][]string{"/values/": {"read"}} }, "good", 200, domain.VerdictFail, "STORE_CAPABILITY_MISSING"},
		// Unreadable Kubernetes auth config: the workload login path is unobserved.
		{"auth not observable", func(f *fakeVault) { f.authMissing = true }, "good", 200, domain.VerdictUnknown, "STORE_AUTH_UNOBSERVED"},
	}
	for _, c := range cases {
		fv := newFakeVault(t, "good")
		c.setup(fv)
		in, key := inspectFixture(t, fv, c.token, c.health)
		f := in.Inspect(ctx, "acme", key)
		if f.Verdict != c.verdict || f.Reason != c.reason {
			t.Errorf("%s: %+v", c.name, f)
		}
		for k, v := range f.Facts {
			if strings.Contains(v, c.token) || !domain.ValidFact(k, v) {
				t.Errorf("%s: unsafe fact %s=%s", c.name, k, v)
			}
		}
		// The inspector never creates policies, roles or values.
		if fv.writes != 0 || len(fv.policies) != 0 || len(fv.roles) != 0 || fv.valueCount() != 0 {
			t.Errorf("%s: the inspector wrote to the store", c.name)
		}
	}
	fv := newFakeVault(t, "good")
	in, _ := inspectFixture(t, fv, "good", 200)
	if f := in.Inspect(ctx, "acme", "missing"); f.Verdict != domain.VerdictFail || f.Reason != "SECRET_STORE_NOT_FOUND" {
		t.Fatalf("missing: %+v", f)
	}
}
