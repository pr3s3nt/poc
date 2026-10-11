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
	return NewRegistry(st, creds), st, creds
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
	reg2 := NewRegistry(st, creds2)
	if _, err := reg2.Provider(ctx, "acme", "one"); !errors.Is(err, configport.ErrStoreUnavailable) {
		t.Fatalf("missing credential: %v", err)
	}
}

// Missing, empty or wrong-scope credentials and an absent credential store all
// fail closed. The reserved platform-vault key and a matching endpoint grant
// nothing: there is no flag, token-file or global fallback (T23).
func TestRegistryCredentialResolutionFailsClosed(t *testing.T) {
	ctx := context.Background()
	fv := newFakeVault(t, "tok")
	record := func(key, ref string) secretstore.Store {
		return secretstore.Store{Key: key, OrganizationKey: "acme", Name: key, Provider: secretstore.ProviderVaultKV2, Status: secretstore.StatusReady,
			BackendAddress: fv.server.URL, WorkloadAddress: fv.server.URL, Mount: "kv", AuthMount: "kubernetes", CredentialRef: ref}
	}
	cases := map[string]func(t *testing.T, st *store.Store, creds *credentialmemory.Store) (string, *Registry){
		"missing credential reference": func(t *testing.T, st *store.Store, creds *credentialmemory.Store) (string, *Registry) {
			if err := st.CreateSecretStore(ctx, record("platform-vault", "")); err != nil {
				t.Fatal(err)
			}
			return "platform-vault", NewRegistry(st, creds)
		},
		"empty token": func(t *testing.T, st *store.Store, creds *credentialmemory.Store) (string, *Registry) {
			ref, err := creds.Put(ctx, "acme", secretstore.CredentialScope("blank"), []byte("  \n"))
			if err != nil {
				t.Fatal(err)
			}
			if err := st.CreateSecretStore(ctx, record("blank", ref)); err != nil {
				t.Fatal(err)
			}
			return "blank", NewRegistry(st, creds)
		},
		"credential of another store scope": func(t *testing.T, st *store.Store, creds *credentialmemory.Store) (string, *Registry) {
			ref, err := creds.Put(ctx, "acme", secretstore.CredentialScope("other"), []byte("tok"))
			if err != nil {
				t.Fatal(err)
			}
			if err := st.CreateSecretStore(ctx, record("mine", ref)); err != nil {
				t.Fatal(err)
			}
			return "mine", NewRegistry(st, creds)
		},
		"credential of another organization": func(t *testing.T, st *store.Store, creds *credentialmemory.Store) (string, *Registry) {
			ref, err := creds.Put(ctx, "globex", secretstore.CredentialScope("mine"), []byte("tok"))
			if err != nil {
				t.Fatal(err)
			}
			if err := st.CreateSecretStore(ctx, record("mine", ref)); err != nil {
				t.Fatal(err)
			}
			return "mine", NewRegistry(st, creds)
		},
		"no credential store": func(t *testing.T, st *store.Store, creds *credentialmemory.Store) (string, *Registry) {
			ref, _ := creds.Put(ctx, "acme", secretstore.CredentialScope("mine"), []byte("tok"))
			if err := st.CreateSecretStore(ctx, record("mine", ref)); err != nil {
				t.Fatal(err)
			}
			return "mine", NewRegistry(st, nil)
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			_, st, creds := newRegistryFixture(t)
			key, reg := setup(t, st, creds)
			if _, err := reg.Provider(ctx, "acme", key); !errors.Is(err, configport.ErrStoreUnavailable) {
				t.Fatalf("provider: %v", err)
			}
			if err := reg.CheckWorkloadAuth(ctx, "acme", key); !errors.Is(err, configport.ErrStoreUnavailable) {
				t.Fatalf("workload auth: %v", err)
			}
			_, err := reg.PrepareBundle(ctx, configport.BundleRequest{OrganizationKey: "acme", ApplicationKey: "app-1", EnvironmentKey: "staging", WorkloadID: "web", Namespace: "ns", RevisionID: "r", DeploymentID: "d", DeliveryStoreKey: key})
			if !errors.Is(err, configport.ErrStoreUnavailable) {
				t.Fatalf("bundle: %v", err)
			}
			if fv.writes != 0 {
				t.Fatalf("a store without a usable credential reached Vault: %d writes", fv.writes)
			}
		})
	}
}
