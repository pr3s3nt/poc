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

func (o oneStore) GetSecretStore(_ context.Context, org, key string) (secretstore.Store, error) {
	if key != o.rec.Key {
		return secretstore.Store{}, persistence.ErrNotFound
	}
	return o.rec, nil
}

func inspectFixture(t *testing.T, handler http.HandlerFunc, token string) (StoreInspector, string) {
	t.Helper()
	server := httptest.NewServer(handler)
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

func vaultHandler(token string, sealed bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/v1/sys/health"):
			if sealed {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/v1/auth/token/lookup-self":
			if r.Header.Get("X-Vault-Token") != token {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			_, _ = w.Write([]byte(`{"data":{"expire_time":"2099-01-01T00:00:00Z"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

func TestInspectReportsRealStoreAvailabilityTokenAndSealedState(t *testing.T) {
	ctx := context.Background()
	in, key := inspectFixture(t, vaultHandler("good", false), "good")
	if f := in.Inspect(ctx, "acme", key); f.Verdict != domain.VerdictPass || f.Reason != "STORE_AVAILABLE" || f.Facts["tokenAccepted"] != "true" {
		t.Fatalf("available store: %+v", f)
	}
	in, key = inspectFixture(t, vaultHandler("other", false), "stale-token")
	if f := in.Inspect(ctx, "acme", key); f.Verdict != domain.VerdictFail || f.Reason != "STORE_TOKEN_REJECTED" {
		t.Fatalf("rejected token is cached READY but not usable: %+v", f)
	}
	in, key = inspectFixture(t, vaultHandler("good", true), "good")
	if f := in.Inspect(ctx, "acme", key); f.Verdict != domain.VerdictFail || f.Reason != "STORE_SEALED" {
		t.Fatalf("sealed: %+v", f)
	}
	if f := in.Inspect(ctx, "acme", "missing"); f.Verdict != domain.VerdictFail || f.Reason != "SECRET_STORE_NOT_FOUND" {
		t.Fatalf("missing: %+v", f)
	}
	// The saved token is never part of a finding.
	for _, f := range []domain.Finding{in.Inspect(ctx, "acme", key)} {
		for k, v := range f.Facts {
			if strings.Contains(v, "good") || !domain.ValidFact(k, v) {
				t.Fatalf("fact %s=%s", k, v)
			}
		}
	}
}
