package bootstrap

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"orchestrator/internal/adapters/credentialmemory"
	"orchestrator/internal/adapters/vault"
	"orchestrator/internal/application/secretstores"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/secretstore"
	"orchestrator/internal/platform/ids"
	configport "orchestrator/internal/ports/configuration"
	"orchestrator/internal/seed"
)

type countVerifier struct {
	calls atomic.Int32
	err   error
}

func (v *countVerifier) Verify(context.Context, configport.VerifyRequest) (map[string]any, error) {
	v.calls.Add(1)
	if v.err != nil {
		return nil, v.err
	}
	return map[string]any{"verified": true, "probe": "REMOVED", "kubernetesAuth": "ABSENT"}, nil
}

func writeToken(t *testing.T, value string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte(value+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func platformOptions(statePath string, creds *credentialmemory.Store, verifier configport.Verifier, tokenFile string) Options {
	return Options{
		Adapters: AdapterFake, Seed: seed.Defaults(), StatePath: statePath,
		ConnectionCredentialsOverride: creds, SecretStoreVerifierOverride: verifier,
		PlatformVault: &PlatformVaultBootstrap{Address: "http://vault:8200", WorkloadAddress: "http://vault.vault.svc:8200", TokenFile: tokenFile},
	}
}

func TestPlatformVaultBootstrapSeedsAnOrdinaryStoreResolvedWithoutLegacyConfig(t *testing.T) {
	ctx := context.Background()
	state := filepath.Join(t.TempDir(), "state.json")
	creds, verifier := credentialmemory.New(), &countVerifier{}
	opts := platformOptions(state, creds, verifier, writeToken(t, "bootstrap-token"))
	app, err := Build(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	org := opts.Seed.OrganizationKey
	got, err := app.Store.GetSecretStore(ctx, org, "platform-vault")
	if err != nil || got.Legacy || got.Name != "Platform Vault" || got.CredentialRef == "" || got.Status != "READY" || got.Verification["probe"] != "REMOVED" {
		t.Fatalf("seeded store: %+v %v", got, err)
	}
	if raw, err := creds.Get(ctx, org, got.CredentialScope(), got.CredentialRef); err != nil || string(raw) != "bootstrap-token" {
		t.Fatalf("credential: %v", err)
	}
	// Runtime resolves through the credential reference; no legacy Vault exists.
	if _, err := app.StoreRegistry.Provider(ctx, org, "platform-vault"); err != nil {
		t.Fatalf("runtime resolution: %v", err)
	}
	// Without the persisted credential the store cannot resolve: the token
	// file is bootstrap input, not a runtime fallback.
	if err := creds.Delete(ctx, org, got.CredentialScope(), got.CredentialRef); err != nil {
		t.Fatal(err)
	}
	fresh := platformOptions(state, creds, verifier, writeToken(t, "bootstrap-token"))
	fresh.PlatformVault = nil
	reopened, err := Build(ctx, fresh)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := vault.NewRegistry(reopened.Store, creds, nil).Provider(ctx, org, "platform-vault"); err == nil {
		t.Fatal("runtime must not fall back to the bootstrap token file")
	}
}

func TestPlatformVaultBootstrapRestartAndLegacyUpgrade(t *testing.T) {
	ctx := context.Background()
	state := filepath.Join(t.TempDir(), "state.json")
	creds, verifier := credentialmemory.New(), &countVerifier{}
	org := seed.Defaults().OrganizationKey

	// An earlier Compose release used the legacy flag-configured store.
	legacyToken := writeToken(t, "legacy-token")
	legacyOpts := Options{Adapters: AdapterFake, Seed: seed.Defaults(), StatePath: state, ConnectionCredentialsOverride: creds, SecretStoreVerifierOverride: verifier,
		VaultAddress: "http://vault:8200", VaultTokenFile: legacyToken, VaultAgentAddress: "http://vault.vault.svc:8200"}
	legacyApp, err := Build(ctx, legacyOpts)
	if err != nil {
		t.Fatal(err)
	}
	before, err := legacyApp.Store.GetSecretStore(ctx, org, "platform-vault")
	if err != nil || !before.Legacy {
		t.Fatalf("legacy seed: %+v %v", before, err)
	}

	opts := platformOptions(state, creds, verifier, writeToken(t, "bootstrap-token"))
	app, err := Build(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	after, err := app.Store.GetSecretStore(ctx, org, "platform-vault")
	if err != nil || after.Legacy || after.ID != before.ID || after.Key != before.Key || after.CredentialRef == "" || verifier.calls.Load() != 1 || creds.Len() != 1 {
		t.Fatalf("upgrade: %+v %v verify=%d creds=%d", after, err, verifier.calls.Load(), creds.Len())
	}

	// Restart: same record, no new credential, no new verification.
	again, err := Build(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := again.Store.GetSecretStore(ctx, org, "platform-vault"); got.CredentialRef != after.CredentialRef || creds.Len() != 1 || verifier.calls.Load() != 1 {
		t.Fatalf("restart: %+v creds=%d verify=%d", got, creds.Len(), verifier.calls.Load())
	}
	if _, err := again.StoreRegistry.Provider(ctx, org, "platform-vault"); err != nil {
		t.Fatalf("runtime resolution after restart: %v", err)
	}
}

func TestPlatformVaultBootstrapFailsClosed(t *testing.T) {
	ctx := context.Background()
	build := func(mutate func(*Options), verifier configport.Verifier) error {
		opts := platformOptions("", credentialmemory.New(), verifier, writeToken(t, "super-secret-token"))
		mutate(&opts)
		_, err := Build(ctx, opts)
		return err
	}
	for name, mutate := range map[string]func(*Options){
		"missing address":     func(o *Options) { o.PlatformVault.Address = "" },
		"missing token file":  func(o *Options) { o.PlatformVault.TokenFile = "" },
		"unreadable token":    func(o *Options) { o.PlatformVault.TokenFile = filepath.Join(t.TempDir(), "absent") },
		"empty token":         func(o *Options) { o.PlatformVault.TokenFile = writeToken(t, "") },
		"legacy combined":     func(o *Options) { o.VaultAddress = "http://vault:8200"; o.VaultTokenFile = writeToken(t, "x") },
		"no credential store": func(o *Options) { o.ConnectionCredentialsOverride = nil },
		"invalid address":     func(o *Options) { o.PlatformVault.Address = "not a url" },
	} {
		err := build(mutate, &countVerifier{})
		if err == nil {
			t.Errorf("%s: startup must fail", name)
			continue
		}
		if strings.Contains(err.Error(), "super-secret-token") || strings.Contains(err.Error(), "absent") {
			t.Errorf("%s: error leaks input: %v", name, err)
		}
	}
	err := build(func(*Options) {}, &countVerifier{err: configport.ErrTokenRejected})
	if !errors.Is(err, secretstores.ErrVerification) || strings.Contains(err.Error(), "super-secret-token") {
		t.Fatalf("verification failure: %v", err)
	}
}

func TestPlatformVaultBootstrapConflictStopsStartup(t *testing.T) {
	ctx := context.Background()
	state := filepath.Join(t.TempDir(), "state.json")
	creds, verifier := credentialmemory.New(), &countVerifier{}
	if _, err := Build(ctx, Options{Adapters: AdapterFake, Seed: seed.Defaults(), StatePath: state, ConnectionCredentialsOverride: creds, SecretStoreVerifierOverride: verifier,
		VaultAddress: "http://old-vault:8200", VaultTokenFile: writeToken(t, "t")}); err != nil {
		t.Fatal(err)
	}
	opts := platformOptions(state, creds, verifier, writeToken(t, "t"))
	_, err := Build(ctx, opts)
	if !errors.Is(err, secretstores.ErrBootstrapConflict) {
		t.Fatalf("conflicting endpoint must stop startup: %v", err)
	}
	if creds.Len() != 0 || verifier.calls.Load() != 0 {
		t.Fatalf("conflict did work: creds=%d verify=%d", creds.Len(), verifier.calls.Load())
	}
}

func TestPlatformVaultTokenReplacementRefreshesManagedStoresInEveryOrganization(t *testing.T) {
	ctx := context.Background()
	state := filepath.Join(t.TempDir(), "state.json")
	creds, verifier := credentialmemory.New(), &countVerifier{}
	seedOrg := seed.Defaults().OrganizationKey

	base, err := Build(ctx, Options{Adapters: AdapterFake, Seed: seed.Defaults(), StatePath: state})
	if err != nil {
		t.Fatal(err)
	}
	for _, org := range []string{"globex", "initech"} {
		if err := base.Store.SaveOrganization(ctx, application.Organization{ID: ids.New(), Key: org, Name: org}); err != nil {
			t.Fatal(err)
		}
		appID := ids.New()
		if err := base.Store.SaveApplication(ctx, application.Application{ID: appID, Key: appID, OrganizationKey: org, Name: org + " app", Subdomain: "s-" + appID[:8], Version: 1}); err != nil {
			t.Fatal(err)
		}
	}
	legacyOpts := Options{Adapters: AdapterFake, Seed: seed.Defaults(), StatePath: state, ConnectionCredentialsOverride: creds, SecretStoreVerifierOverride: verifier,
		VaultAddress: "http://vault:8200", VaultTokenFile: writeToken(t, "legacy"), VaultAgentAddress: "http://vault.vault.svc:8200"}
	if _, err := Build(ctx, legacyOpts); err != nil {
		t.Fatal(err)
	}
	legacyApp, err := Build(ctx, legacyOpts)
	if err != nil {
		t.Fatal(err)
	}
	st := legacyApp.Store
	unrelated := secretstore.Store{Key: "team-vault", OrganizationKey: "globex", Name: "Team", Provider: secretstore.ProviderVaultKV2, Status: secretstore.StatusReady,
		BackendAddress: "http://team:8200", WorkloadAddress: "http://team:8200", Mount: "kv", AuthMount: "kubernetes", CredentialRef: "kv2://kv/x/y"}
	if err := st.CreateSecretStore(ctx, unrelated); err != nil {
		t.Fatal(err)
	}
	initechBefore, _ := st.GetSecretStore(ctx, "initech", "platform-vault")
	if !initechBefore.Legacy {
		t.Fatalf("initech should start legacy: %+v", initechBefore)
	}

	first := platformOptions(state, creds, verifier, writeToken(t, "token-1"))
	if _, err := Build(ctx, first); err != nil {
		t.Fatal(err)
	}
	read := func(org string) secretstore.Store {
		app, err := Build(ctx, Options{Adapters: AdapterFake, Seed: seed.Defaults(), StatePath: state})
		if err != nil {
			t.Fatal(err)
		}
		got, err := app.Store.GetSecretStore(ctx, org, "platform-vault")
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	orgs := []string{seedOrg, "globex", "initech"}
	converted := map[string]secretstore.Store{}
	for _, org := range orgs {
		got := read(org)
		if got.Legacy || got.CredentialRef == "" || got.Verification["bootstrap"] != secretstores.BootstrapMarker {
			t.Fatalf("%s not converted: %+v", org, got)
		}
		converted[org] = got
	}
	if creds.Len() != 3 {
		t.Fatalf("credentials after conversion: %d", creds.Len())
	}

	// Another organization with an unmarked ordinary platform-vault must stay untouched.
	app, _ := Build(ctx, Options{Adapters: AdapterFake, Seed: seed.Defaults(), StatePath: state})
	if err := app.Store.SaveOrganization(ctx, application.Organization{ID: ids.New(), Key: "umbrella", Name: "umbrella"}); err != nil {
		t.Fatal(err)
	}
	appID := ids.New()
	_ = app.Store.SaveApplication(ctx, application.Application{ID: appID, Key: appID, OrganizationKey: "umbrella", Name: "u", Subdomain: "u-" + appID[:8], Version: 1})
	umbrellaRef, err := creds.Put(ctx, "umbrella", "ss-platform-vault", []byte("user-token"))
	if err != nil {
		t.Fatal(err)
	}
	userStore := secretstore.Store{Key: "platform-vault", OrganizationKey: "umbrella", Name: "Platform Vault", Provider: secretstore.ProviderVaultKV2, Status: secretstore.StatusReady,
		BackendAddress: "http://vault:8200", WorkloadAddress: "http://vault.vault.svc:8200", Mount: "kv", AuthMount: "kubernetes", CredentialRef: umbrellaRef}
	if err := app.Store.CreateSecretStore(ctx, userStore); err != nil {
		t.Fatal(err)
	}
	calls := verifier.calls.Load()
	credsBefore := creds.Len()

	second := platformOptions(state, creds, verifier, writeToken(t, "token-2"))
	if _, err := Build(ctx, second); err != nil {
		t.Fatal(err)
	}
	for _, org := range orgs {
		got := read(org)
		before := converted[org]
		if got.CredentialRef == before.CredentialRef || got.ID != before.ID || got.Key != before.Key || got.Legacy || got.Name != before.Name ||
			got.BackendAddress != before.BackendAddress || got.WorkloadAddress != before.WorkloadAddress || got.Mount != before.Mount || got.AuthMount != before.AuthMount || !got.CreatedAt.Equal(before.CreatedAt) {
			t.Fatalf("%s not refreshed with identity preserved: %+v vs %+v", org, got, before)
		}
		if raw, err := creds.Get(ctx, org, got.CredentialScope(), got.CredentialRef); err != nil || string(raw) != "token-2" {
			t.Fatalf("%s credential: %v", org, err)
		}
		if _, err := creds.Get(ctx, org, got.CredentialScope(), before.CredentialRef); err == nil {
			t.Fatalf("%s old credential not removed", org)
		}
	}
	if verifier.calls.Load() != calls+3 || creds.Len() != credsBefore {
		t.Fatalf("verify=%d (+%d) creds=%d (was %d)", verifier.calls.Load(), verifier.calls.Load()-calls, creds.Len(), credsBefore)
	}
	untouched := read("umbrella")
	if untouched.CredentialRef != umbrellaRef || untouched.Name != "Platform Vault" {
		t.Fatalf("unrelated organization changed: %+v", untouched)
	}
	reread, _ := Build(ctx, Options{Adapters: AdapterFake, Seed: seed.Defaults(), StatePath: state})
	if got, err := reread.Store.GetSecretStore(ctx, "globex", "team-vault"); err != nil || got.CredentialRef != unrelated.CredentialRef || got.Name != "Team" {
		t.Fatalf("unrelated store changed: %+v %v", got, err)
	}
}

func TestPlatformVaultBootstrapUsesRealVerifierInFakeAdapterMode(t *testing.T) {
	ctx := context.Background()
	for _, mode := range []AdapterMode{"", AdapterFake} {
		creds := credentialmemory.New()
		opts := platformOptions("", creds, nil, writeToken(t, "super-secret-token"))
		opts.SecretStoreVerifierOverride = nil
		opts.Adapters = mode
		opts.PlatformVault.Address = "http://127.0.0.1:1"
		opts.PlatformVault.WorkloadAddress = ""
		_, err := Build(ctx, opts)
		if !errors.Is(err, secretstores.ErrVerification) || strings.Contains(err.Error(), "super-secret-token") {
			t.Fatalf("mode %q: an unreachable Vault must fail verification safely: %v", mode, err)
		}
		if creds.Len() != 0 {
			t.Fatalf("mode %q: failed bootstrap left %d credentials", mode, creds.Len())
		}
	}
}
