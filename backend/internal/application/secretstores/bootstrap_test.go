package secretstores

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"orchestrator/internal/adapters/credentialmemory"
	"orchestrator/internal/adapters/store"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/secretstore"
	"orchestrator/internal/platform/ids"
	configport "orchestrator/internal/ports/configuration"
	"orchestrator/internal/ports/credentials"
	"orchestrator/internal/ports/persistence"
)

type countingVerifier struct {
	calls atomic.Int32
	err   error
	last  atomic.Value // token of the last request
}

func (v *countingVerifier) Verify(_ context.Context, req configport.VerifyRequest) (map[string]any, error) {
	v.calls.Add(1)
	v.last.Store(req.Token)
	if v.err != nil {
		return nil, v.err
	}
	return map[string]any{"verified": true, "probe": "REMOVED", "kubernetesAuth": "ABSENT"}, nil
}

type bootstrapFixture struct {
	st       persistence.Store
	creds    *credentialmemory.Store
	verifier *countingVerifier
	svc      *Service
}

func newBootstrapFixture(t *testing.T) *bootstrapFixture {
	t.Helper()
	st := store.New()
	if err := st.SaveOrganization(context.Background(), application.Organization{ID: ids.New(), Key: "acme", Name: "Acme"}); err != nil {
		t.Fatal(err)
	}
	creds, verifier := credentialmemory.New(), &countingVerifier{}
	return &bootstrapFixture{st: st, creds: creds, verifier: verifier, svc: NewService(st, verifier, creds)}
}

func bootstrapCommand(token string) RegisterCommand {
	return RegisterCommand{Name: "Platform Vault", BackendAddress: "http://vault:8200", WorkloadAddress: "http://vault.vault.svc:8200", Token: token}
}

func (f *bootstrapFixture) legacyRecord(t *testing.T) secretstore.Store {
	t.Helper()
	legacy := secretstore.Store{Key: BootstrapKey, OrganizationKey: "acme", Name: "Platform Vault (legacy)", Provider: secretstore.ProviderVaultKV2, Status: secretstore.StatusReady,
		BackendAddress: "http://vault:8200", WorkloadAddress: "http://vault.vault.svc:8200", Mount: "kv", AuthMount: "kubernetes", Legacy: true, Verification: map[string]any{"legacy": true, "verified": false}}
	if err := f.st.CreateSecretStore(context.Background(), legacy); err != nil {
		t.Fatal(err)
	}
	got, err := f.st.GetSecretStore(context.Background(), "acme", BootstrapKey)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func (f *bootstrapFixture) stored(t *testing.T) secretstore.Store {
	t.Helper()
	got, err := f.st.GetSecretStore(context.Background(), "acme", BootstrapKey)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func (f *bootstrapFixture) token(t *testing.T, record secretstore.Store) string {
	t.Helper()
	raw, err := f.creds.Get(context.Background(), "acme", record.CredentialScope(), record.CredentialRef)
	if err != nil {
		t.Fatalf("credential of the stored record is unreadable: %v", err)
	}
	return string(raw)
}

func TestBootstrapFirstSeedIsAnOrdinaryVerifiedStore(t *testing.T) {
	f := newBootstrapFixture(t)
	outcome, err := f.svc.Bootstrap(context.Background(), "acme", bootstrapCommand("app-token"))
	if err != nil || outcome != BootstrapCreated {
		t.Fatalf("bootstrap: %v %v", outcome, err)
	}
	got := f.stored(t)
	if got.Key != "platform-vault" || got.Name != "Platform Vault" || got.Legacy || got.Status != secretstore.StatusReady || got.Mount != "kv" || got.AuthMount != "kubernetes" {
		t.Fatalf("record: %+v", got)
	}
	if got.CredentialRef == "" || f.token(t, got) != "app-token" || f.creds.Len() != 1 || f.verifier.calls.Load() != 1 {
		t.Fatalf("credential/verification: ref=%q creds=%d verify=%d", got.CredentialRef, f.creds.Len(), f.verifier.calls.Load())
	}
	if got.Verification["probe"] != "REMOVED" || got.Verification["bootstrap"] != BootstrapMarker {
		t.Fatalf("verification must come from the verifier: %+v", got.Verification)
	}
	if strings.Contains(got.CredentialRef, "app-token") {
		t.Fatal("credential reference leaks the token")
	}
	list, _ := f.svc.Choices(context.Background(), "acme")
	if len(list) != 1 {
		t.Fatalf("a Developer must be able to choose the store: %+v", list)
	}
}

func TestBootstrapRestartIsIdempotent(t *testing.T) {
	f := newBootstrapFixture(t)
	if _, err := f.svc.Bootstrap(context.Background(), "acme", bootstrapCommand("app-token")); err != nil {
		t.Fatal(err)
	}
	first := f.stored(t)
	for i := 0; i < 3; i++ {
		outcome, err := f.svc.Bootstrap(context.Background(), "acme", bootstrapCommand("app-token"))
		if err != nil || outcome != BootstrapUnchanged {
			t.Fatalf("restart %d: %v %v", i, outcome, err)
		}
	}
	if got := f.stored(t); got.ID != first.ID || got.CredentialRef != first.CredentialRef || f.creds.Len() != 1 || f.verifier.calls.Load() != 1 {
		t.Fatalf("restart created work: %+v creds=%d verify=%d", got, f.creds.Len(), f.verifier.calls.Load())
	}
	if all, _ := f.st.ListSecretStores(context.Background(), "acme"); len(all) != 1 {
		t.Fatalf("stores: %+v", all)
	}
}

func TestBootstrapConvertsLegacyRecordKeepingIdentity(t *testing.T) {
	f := newBootstrapFixture(t)
	legacy := f.legacyRecord(t)
	outcome, err := f.svc.Bootstrap(context.Background(), "acme", bootstrapCommand("app-token"))
	if err != nil || outcome != BootstrapConverted {
		t.Fatalf("convert: %v %v", outcome, err)
	}
	got := f.stored(t)
	if got.ID != legacy.ID || !got.CreatedAt.Equal(legacy.CreatedAt) || got.Key != legacy.Key || got.Legacy || got.Name != "Platform Vault" ||
		got.BackendAddress != legacy.BackendAddress || got.WorkloadAddress != legacy.WorkloadAddress || got.Mount != legacy.Mount || got.AuthMount != legacy.AuthMount {
		t.Fatalf("identity not preserved: %+v vs %+v", got, legacy)
	}
	if f.token(t, got) != "app-token" || f.creds.Len() != 1 || f.verifier.calls.Load() != 1 {
		t.Fatalf("conversion must verify and persist exactly once: creds=%d verify=%d", f.creds.Len(), f.verifier.calls.Load())
	}
}

func TestBootstrapFailsClosedWithoutPartialState(t *testing.T) {
	t.Run("verification failure keeps legacy record and stores nothing", func(t *testing.T) {
		f := newBootstrapFixture(t)
		legacy := f.legacyRecord(t)
		f.verifier.err = configport.ErrTokenRejected
		_, err := f.svc.Bootstrap(context.Background(), "acme", bootstrapCommand("secret-token-value"))
		if !errors.Is(err, ErrVerification) || strings.Contains(err.Error(), "secret-token-value") {
			t.Fatalf("error: %v", err)
		}
		if got := f.stored(t); !got.Legacy || got.CredentialRef != "" || got.ID != legacy.ID || f.creds.Len() != 0 {
			t.Fatalf("partial state: %+v creds=%d", got, f.creds.Len())
		}
	})
	t.Run("first seed verification failure creates no record", func(t *testing.T) {
		f := newBootstrapFixture(t)
		f.verifier.err = configport.ErrProbeCleanup
		if _, err := f.svc.Bootstrap(context.Background(), "acme", bootstrapCommand("t")); !errors.Is(err, ErrVerification) {
			t.Fatalf("error: %v", err)
		}
		if _, err := f.st.GetSecretStore(context.Background(), "acme", BootstrapKey); !errors.Is(err, persistence.ErrNotFound) || f.creds.Len() != 0 {
			t.Fatalf("a failed seed left state: %v creds=%d", err, f.creds.Len())
		}
	})
	t.Run("credential persistence failure", func(t *testing.T) {
		f := newBootstrapFixture(t)
		legacy := f.legacyRecord(t)
		f.creds.FailPut = errors.New("down")
		if _, err := f.svc.Bootstrap(context.Background(), "acme", bootstrapCommand("t")); !errors.Is(err, ErrCredentialStore) {
			t.Fatalf("error: %v", err)
		}
		if got := f.stored(t); !got.Legacy || got.ID != legacy.ID || f.creds.Len() != 0 {
			t.Fatalf("partial state: %+v creds=%d", got, f.creds.Len())
		}
	})
	t.Run("record admission failure removes only the attempt credential", func(t *testing.T) {
		f := newBootstrapFixture(t)
		legacy := f.legacyRecord(t)
		f.svc = NewService(failingAdmission{Store: f.st, err: errors.New("db down")}, f.verifier, f.creds)
		if _, err := f.svc.Bootstrap(context.Background(), "acme", bootstrapCommand("t")); err == nil {
			t.Fatal("expected failure")
		}
		if got := f.stored(t); !got.Legacy || got.ID != legacy.ID || f.creds.Len() != 0 {
			t.Fatalf("rollback incomplete: %+v creds=%d", got, f.creds.Len())
		}
	})
	t.Run("missing credential store or verifier", func(t *testing.T) {
		f := newBootstrapFixture(t)
		if _, err := NewService(f.st, f.verifier, nil).Bootstrap(context.Background(), "acme", bootstrapCommand("t")); !errors.Is(err, ErrCredentialStore) {
			t.Fatalf("credential store: %v", err)
		}
		if _, err := NewService(f.st, nil, f.creds).Bootstrap(context.Background(), "acme", bootstrapCommand("t")); !errors.Is(err, ErrVerification) {
			t.Fatalf("verifier: %v", err)
		}
	})
	t.Run("invalid input is rejected before any remote work", func(t *testing.T) {
		f := newBootstrapFixture(t)
		bad := bootstrapCommand("t")
		bad.BackendAddress = "ftp://vault"
		if _, err := f.svc.Bootstrap(context.Background(), "acme", bad); !errors.Is(err, ErrInvalid) || f.verifier.calls.Load() != 0 {
			t.Fatalf("invalid: %v verify=%d", err, f.verifier.calls.Load())
		}
		if _, err := f.svc.Bootstrap(context.Background(), "acme", bootstrapCommand(" ")); !errors.Is(err, ErrInvalid) {
			t.Fatalf("empty token: %v", err)
		}
	})
}

// failingAdmission fails only the final admission write.
type failingAdmission struct {
	persistence.Store
	err error
}

func (f failingAdmission) AdmitManagedSecretStore(context.Context, persistence.ManagedStoreAdmission) (secretstore.Store, error) {
	return secretstore.Store{}, f.err
}

func TestBootstrapNeverOverwritesConflictingRegistrations(t *testing.T) {
	cases := map[string]func(*secretstore.Store){
		"backend address": func(s *secretstore.Store) { s.BackendAddress = "http://elsewhere:8200" },
		"workload":        func(s *secretstore.Store) { s.WorkloadAddress = "http://elsewhere.svc:8200" },
		"mount":           func(s *secretstore.Store) { s.Mount = "other" },
		"auth mount":      func(s *secretstore.Store) { s.AuthMount = "other" },
	}
	for label, mutate := range cases {
		t.Run(label, func(t *testing.T) {
			f := newBootstrapFixture(t)
			existing := secretstore.Store{Key: BootstrapKey, OrganizationKey: "acme", Name: "Mine", Provider: secretstore.ProviderVaultKV2, Status: secretstore.StatusReady,
				BackendAddress: "http://vault:8200", WorkloadAddress: "http://vault.vault.svc:8200", Mount: "kv", AuthMount: "kubernetes", Legacy: true}
			mutate(&existing)
			if err := f.st.CreateSecretStore(context.Background(), existing); err != nil {
				t.Fatal(err)
			}
			if _, err := f.svc.Bootstrap(context.Background(), "acme", bootstrapCommand("t")); !errors.Is(err, ErrBootstrapConflict) {
				t.Fatalf("conflict: %v", err)
			}
			if got := f.stored(t); !got.Legacy || got.Name != "Mine" || f.creds.Len() != 0 || f.verifier.calls.Load() != 0 {
				t.Fatalf("conflict changed state: %+v creds=%d verify=%d", got, f.creds.Len(), f.verifier.calls.Load())
			}
		})
	}
	t.Run("user registration with another token", func(t *testing.T) {
		f := newBootstrapFixture(t)
		registered, err := f.svc.Register(context.Background(), "acme", bootstrapCommand("user-token"))
		if err != nil || registered.Key != BootstrapKey {
			t.Fatalf("register: %+v %v", registered, err)
		}
		if _, err := f.svc.Bootstrap(context.Background(), "acme", bootstrapCommand("bootstrap-token")); !errors.Is(err, ErrBootstrapConflict) {
			t.Fatalf("unmanaged store must not be refreshed: %v", err)
		}
		if got := f.stored(t); got.CredentialRef != registered.CredentialRef || f.token(t, got) != "user-token" || f.creds.Len() != 1 {
			t.Fatalf("user registration changed: %+v", got)
		}
		if outcome, err := f.svc.Bootstrap(context.Background(), "acme", bootstrapCommand("user-token")); err != nil || outcome != BootstrapUnchanged {
			t.Fatalf("identical registration: %v %v", outcome, err)
		}
	})
}

func TestBootstrapTokenReplacementSwapsCredentialSafely(t *testing.T) {
	f := newBootstrapFixture(t)
	if _, err := f.svc.Bootstrap(context.Background(), "acme", bootstrapCommand("old-token")); err != nil {
		t.Fatal(err)
	}
	first := f.stored(t)

	f.verifier.err = configport.ErrTokenRejected
	if _, err := f.svc.Bootstrap(context.Background(), "acme", bootstrapCommand("bad-token")); !errors.Is(err, ErrVerification) {
		t.Fatalf("rejected replacement: %v", err)
	}
	if got := f.stored(t); got.CredentialRef != first.CredentialRef || f.token(t, got) != "old-token" || f.creds.Len() != 1 {
		t.Fatalf("failed replacement changed state: %+v creds=%d", got, f.creds.Len())
	}

	f.verifier.err = nil
	outcome, err := f.svc.Bootstrap(context.Background(), "acme", bootstrapCommand("new-token"))
	if err != nil || outcome != BootstrapRefreshed {
		t.Fatalf("replacement: %v %v", outcome, err)
	}
	got := f.stored(t)
	if got.ID != first.ID || got.CredentialRef == first.CredentialRef || f.token(t, got) != "new-token" || f.creds.Len() != 1 {
		t.Fatalf("replacement: %+v creds=%d", got, f.creds.Len())
	}
	if _, err := f.creds.Get(context.Background(), "acme", got.CredentialScope(), first.CredentialRef); !errors.Is(err, credentials.ErrNotFound) {
		t.Fatalf("replaced credential must be removed: %v", err)
	}
	if last, _ := f.verifier.last.Load().(string); last != "new-token" {
		t.Fatalf("the verifier must see the replacement token, saw %q", last)
	}
	if outcome, err := f.svc.Bootstrap(context.Background(), "acme", bootstrapCommand("new-token")); err != nil || outcome != BootstrapUnchanged {
		t.Fatalf("restart after replacement: %v %v", outcome, err)
	}
}

func TestBootstrapRepairsMissingCredentialObject(t *testing.T) {
	f := newBootstrapFixture(t)
	if _, err := f.svc.Bootstrap(context.Background(), "acme", bootstrapCommand("tok")); err != nil {
		t.Fatal(err)
	}
	first := f.stored(t)
	_ = f.creds.Delete(context.Background(), "acme", first.CredentialScope(), first.CredentialRef)
	if outcome, err := f.svc.Bootstrap(context.Background(), "acme", bootstrapCommand("tok")); err != nil || outcome != BootstrapRefreshed {
		t.Fatalf("repair: %v %v", outcome, err)
	}
	if got := f.stored(t); f.token(t, got) != "tok" || f.creds.Len() != 1 {
		t.Fatalf("repair: %+v creds=%d", got, f.creds.Len())
	}
}

func TestBootstrapConcurrentStartupsAdmitOneStoreAndOneCredential(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		f := newBootstrapFixture(t)
		var prior secretstore.Store
		if legacy {
			prior = f.legacyRecord(t)
		}
		var wg sync.WaitGroup
		errs := make(chan error, 12)
		for i := 0; i < 12; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := f.svc.Bootstrap(context.Background(), "acme", bootstrapCommand("tok"))
				errs <- err
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatalf("legacy=%v: %v", legacy, err)
			}
		}
		got := f.stored(t)
		if all, _ := f.st.ListSecretStores(context.Background(), "acme"); len(all) != 1 || got.Legacy || f.token(t, got) != "tok" {
			t.Fatalf("legacy=%v: stores=%+v", legacy, all)
		}
		if f.creds.Len() != 1 {
			t.Fatalf("legacy=%v: losers must clean exactly their own credentials, %d remain", legacy, f.creds.Len())
		}
		if legacy && got.ID != prior.ID {
			t.Fatalf("legacy row identity lost: %s vs %s", got.ID, prior.ID)
		}
	}
}

func TestBootstrapLoserKeepsWinnersCredential(t *testing.T) {
	f := newBootstrapFixture(t)
	// The winner publishes between this attempt's read and its admission.
	racing := &racingVerifier{inner: f.verifier, onVerify: func() {
		winner := NewService(f.st, f.verifier, f.creds)
		if _, err := winner.Bootstrap(context.Background(), "acme", bootstrapCommand("tok")); err != nil {
			t.Error(err)
		}
	}}
	loser := NewService(f.st, racing, f.creds)
	racing.once.Store(true)
	outcome, err := loser.Bootstrap(context.Background(), "acme", bootstrapCommand("tok"))
	if err != nil || outcome != BootstrapUnchanged {
		t.Fatalf("loser: %v %v", outcome, err)
	}
	got := f.stored(t)
	if f.token(t, got) != "tok" || f.creds.Len() != 1 {
		t.Fatalf("winner credential must survive: %+v creds=%d", got, f.creds.Len())
	}
}

type racingVerifier struct {
	inner    *countingVerifier
	once     atomic.Bool
	onVerify func()
}

func (r *racingVerifier) Verify(ctx context.Context, req configport.VerifyRequest) (map[string]any, error) {
	out, err := r.inner.Verify(ctx, req)
	if r.once.CompareAndSwap(true, false) {
		r.onVerify()
	}
	return out, err
}

// A failed snapshot write must fail the bootstrap, restore the prior record and
// remove only the attempt's credential.
func TestBootstrapSnapshotWriteFailureRollsBackAdmission(t *testing.T) {
	setup := func(t *testing.T) (*bootstrapFixture, string) {
		t.Helper()
		path := filepath.Join(t.TempDir(), "state.json")
		st, err := store.NewWithSnapshot(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := st.SaveOrganization(context.Background(), application.Organization{ID: ids.New(), Key: "acme", Name: "Acme"}); err != nil {
			t.Fatal(err)
		}
		creds, verifier := credentialmemory.New(), &countingVerifier{}
		return &bootstrapFixture{st: st, creds: creds, verifier: verifier, svc: NewService(st, verifier, creds)}, path
	}
	breakSnapshot := func(t *testing.T, path string) {
		t.Helper()
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(path, 0o700); err != nil { // a directory cannot be written as the snapshot file
			t.Fatal(err)
		}
	}
	t.Run("create", func(t *testing.T) {
		f, path := setup(t)
		breakSnapshot(t, path)
		if _, err := f.svc.Bootstrap(context.Background(), "acme", bootstrapCommand("tok")); err == nil {
			t.Fatal("bootstrap succeeded although the record was not persisted")
		}
		if _, err := f.st.GetSecretStore(context.Background(), "acme", BootstrapKey); !errors.Is(err, persistence.ErrNotFound) || f.creds.Len() != 0 {
			t.Fatalf("create left state: %v creds=%d", err, f.creds.Len())
		}
	})
	t.Run("legacy conversion", func(t *testing.T) {
		f, path := setup(t)
		legacy := f.legacyRecord(t)
		breakSnapshot(t, path)
		if _, err := f.svc.Bootstrap(context.Background(), "acme", bootstrapCommand("tok")); err == nil {
			t.Fatal("conversion succeeded although it was not persisted")
		}
		if got := f.stored(t); !got.Legacy || got.ID != legacy.ID || got.CredentialRef != "" || got.Name != legacy.Name || f.creds.Len() != 0 {
			t.Fatalf("conversion left state: %+v creds=%d", got, f.creds.Len())
		}
	})
	t.Run("token refresh", func(t *testing.T) {
		f, path := setup(t)
		if _, err := f.svc.Bootstrap(context.Background(), "acme", bootstrapCommand("old-token")); err != nil {
			t.Fatal(err)
		}
		before := f.stored(t)
		breakSnapshot(t, path)
		if _, err := f.svc.Bootstrap(context.Background(), "acme", bootstrapCommand("new-token")); err == nil {
			t.Fatal("refresh succeeded although it was not persisted")
		}
		got := f.stored(t)
		if got.CredentialRef != before.CredentialRef || f.token(t, got) != "old-token" || f.creds.Len() != 1 {
			t.Fatalf("refresh left state: %+v creds=%d", got, f.creds.Len())
		}
	})
}
