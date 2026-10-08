package vault

import (
	"context"
	"errors"
	"strings"
	"testing"

	"orchestrator/internal/adapters/credentialmemory"
	"orchestrator/internal/adapters/store"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/secretstore"
	"orchestrator/internal/platform/ids"
	configport "orchestrator/internal/ports/configuration"
)

func registerStore(t *testing.T, st *store.Store, creds *credentialmemory.Store, org, key string, fv *fakeVault, token string, auth string) secretstore.Store {
	t.Helper()
	ctx := context.Background()
	ref, err := creds.Put(ctx, org, secretstore.CredentialScope(key), []byte(token))
	if err != nil {
		t.Fatal(err)
	}
	record := secretstore.Store{Key: key, OrganizationKey: org, Name: key, Provider: secretstore.ProviderVaultKV2, Status: secretstore.StatusReady,
		BackendAddress: fv.server.URL, WorkloadAddress: "http://vault-" + key + ".vault.svc:8200", Mount: "kv", AuthMount: auth, CredentialRef: ref}
	if err := st.CreateSecretStore(ctx, record); err != nil {
		t.Fatal(err)
	}
	return record
}

func newRegistryFixture(t *testing.T) (*Registry, *store.Store, *credentialmemory.Store) {
	t.Helper()
	st := store.New()
	for _, org := range []string{"acme", "globex"} {
		if err := st.SaveOrganization(context.Background(), application.Organization{ID: ids.New(), Key: org, Name: org}); err != nil {
			t.Fatal(err)
		}
	}
	creds := credentialmemory.New()
	return NewRegistry(st, creds, nil), st, creds
}

func TestRegistryDeliversBundleToSelectedStoreReadingRefsThroughOwners(t *testing.T) {
	ctx := context.Background()
	reg, st, creds := newRegistryFixture(t)
	oldVault, newVault := newFakeVault(t, "old-token"), newFakeVault(t, "new-token")
	newVault.auth = "k8s-alt"
	registerStore(t, st, creds, "acme", "old", oldVault, "old-token", "kubernetes")
	registerStore(t, st, creds, "acme", "new", newVault, "new-token", "k8s-alt")

	oldProvider, err := reg.Provider(ctx, "acme", "old")
	if err != nil {
		t.Fatal(err)
	}
	oldRef, err := oldProvider.WriteValue(ctx, "app-1", "staging", "old-secret-value")
	if err != nil {
		t.Fatal(err)
	}
	newProvider, _ := reg.Provider(ctx, "acme", "new")
	newRef, _ := newProvider.WriteValue(ctx, "app-1", "staging", "new-secret-value")

	bundle, err := reg.PrepareBundle(ctx, configport.BundleRequest{
		OrganizationKey: "acme", ApplicationKey: "app-1", EnvironmentKey: "staging", WorkloadID: "web", Namespace: "app-staging", RevisionID: "rev-1", DeploymentID: "dep-1", DeliveryStoreKey: "new",
		Refs: map[string]map[string]configport.StoreRef{"main": {"OLD": {StoreKey: "old", ValueRef: oldRef}, "NEW": {StoreKey: "new", ValueRef: newRef}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if bundle.StoreKey != "new" || bundle.AuthMount != "k8s-alt" || bundle.Address != "http://vault-new.vault.svc:8200" || bundle.Mount != "kv" {
		t.Fatalf("bundle must pin the delivery store identity: %+v", bundle)
	}
	// The object lands in the delivery store only; the old store was only read.
	data := newVault.values["orchestrator/apps/app-1/envs/staging/values/dep-1"]
	if !strings.Contains(data, "old-secret-value") || !strings.Contains(data, "new-secret-value") {
		t.Fatalf("bundle content: %s", data)
	}
	if _, wrote := oldVault.values["orchestrator/apps/app-1/envs/staging/values/dep-1"]; wrote {
		t.Fatal("bundle must not be written to the previous store")
	}
	if len(newVault.roles) != 1 || len(newVault.policies) != 1 {
		t.Fatalf("role and policy belong in the delivery store: roles=%d policies=%d", len(newVault.roles), len(oldVault.policies))
	}
	if len(oldVault.roles) != 0 || len(oldVault.policies) != 0 {
		t.Fatal("old store gained workload access objects")
	}
}

func TestRegistryScopesStoresToTheOrganizationAndFailsClosed(t *testing.T) {
	ctx := context.Background()
	reg, st, creds := newRegistryFixture(t)
	fv := newFakeVault(t, "tok")
	registerStore(t, st, creds, "acme", "one", fv, "tok", "kubernetes")
	if _, err := reg.Provider(ctx, "globex", "one"); !errors.Is(err, configport.ErrStoreUnavailable) {
		t.Fatalf("foreign organization: %v", err)
	}
	if _, err := reg.Provider(ctx, "acme", "missing"); !errors.Is(err, configport.ErrStoreUnavailable) {
		t.Fatalf("missing store: %v", err)
	}
	// A ref outside the Application Environment scope is rejected before any read.
	_, err := reg.PrepareBundle(ctx, configport.BundleRequest{
		OrganizationKey: "acme", ApplicationKey: "app-1", EnvironmentKey: "staging", WorkloadID: "web", Namespace: "ns", RevisionID: "r", DeploymentID: "d", DeliveryStoreKey: "one",
		Refs: map[string]map[string]configport.StoreRef{"main": {"X": {StoreKey: "one", ValueRef: "kv2://kv/orchestrator/apps/other/envs/staging/values/abc"}}},
	})
	if err == nil {
		t.Fatal("cross-Application ref accepted")
	}
	// A ref without store identity never falls back to another store.
	_, err = reg.PrepareBundle(ctx, configport.BundleRequest{
		OrganizationKey: "acme", ApplicationKey: "app-1", EnvironmentKey: "staging", WorkloadID: "web", Namespace: "ns", RevisionID: "r", DeploymentID: "d2", DeliveryStoreKey: "one",
		Refs: map[string]map[string]configport.StoreRef{"main": {"X": {ValueRef: "kv2://kv/orchestrator/apps/app-1/envs/staging/values/abc"}}},
	})
	if !errors.Is(err, configport.ErrStoreUnavailable) {
		t.Fatalf("storeless ref: %v", err)
	}
	// A deleted credential makes the store unavailable; no host fallback exists.
	creds2 := credentialmemory.New()
	reg2 := NewRegistry(st, creds2, nil)
	if _, err := reg2.Provider(ctx, "acme", "one"); !errors.Is(err, configport.ErrStoreUnavailable) {
		t.Fatalf("missing credential: %v", err)
	}
	// The legacy store resolves only with the flag configuration it mirrors.
	legacy := secretstore.Store{Key: LegacyStoreKey, OrganizationKey: "acme", Name: "legacy", Provider: secretstore.ProviderVaultKV2, Status: secretstore.StatusReady, Legacy: true,
		BackendAddress: fv.server.URL, WorkloadAddress: fv.server.URL, Mount: "kv", AuthMount: "kubernetes"}
	if err := st.CreateSecretStore(ctx, legacy); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Provider(ctx, "acme", LegacyStoreKey); !errors.Is(err, configport.ErrStoreUnavailable) {
		t.Fatalf("legacy store without flags must fail safely: %v", err)
	}
	withFlags := NewRegistry(st, nil, &LegacyConfig{Address: fv.server.URL, AgentAddress: fv.server.URL, Mount: "kv", AuthMount: "kubernetes", Token: "tok"})
	provider, err := withFlags.Provider(ctx, "acme", LegacyStoreKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.WriteValue(ctx, "app-1", "staging", "v"); err != nil {
		t.Fatal(err)
	}
}
