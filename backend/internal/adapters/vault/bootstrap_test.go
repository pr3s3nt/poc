package vault

import (
	"context"
	"errors"
	"strings"
	"testing"

	"orchestrator/internal/application/secretstores"
	"orchestrator/internal/domain/secretstore"
)

func bootstrapCmd(fv *fakeVault, token string) secretstores.RegisterCommand {
	return secretstores.RegisterCommand{Name: "Platform Vault", BackendAddress: fv.server.URL, WorkloadAddress: "http://vault.vault.svc:8200", Token: token}
}

// The real verifier admits the bootstrap store; runtime then resolves it only
// through the persisted credential reference (no legacy configuration).
func TestBootstrapWithRealVerifierSeedsStoreUsedByRegistry(t *testing.T) {
	ctx := context.Background()
	reg, st, creds := newRegistryFixture(t)
	fv := newFakeVault(t, "app-token")
	svc := secretstores.NewService(st, Verifier{}, creds)

	outcome, err := svc.Bootstrap(ctx, "acme", bootstrapCmd(fv, "app-token"))
	if err != nil || outcome != secretstores.BootstrapCreated {
		t.Fatalf("bootstrap: %v %v", outcome, err)
	}
	if fv.valueCount() != 0 {
		t.Fatal("verification probe object was not removed")
	}
	got, err := st.GetSecretStore(ctx, "acme", "platform-vault")
	if err != nil || got.Legacy || got.CredentialRef == "" || got.Verification["probe"] != "REMOVED" || got.Verification["capabilities"] != "VERIFIED" || got.Verification["kubernetesAuth"] != "CONFIGURED" {
		t.Fatalf("record: %+v %v", got, err)
	}
	if reg.legacy != nil {
		t.Fatal("registry must have no legacy configuration")
	}
	p, err := reg.Provider(ctx, "acme", "platform-vault")
	if err != nil {
		t.Fatal(err)
	}
	ref, err := p.WriteValue(ctx, "app-1", "staging", "runtime-value")
	if err != nil {
		t.Fatalf("runtime write through the persisted credential: %v", err)
	}
	if value, err := p.ReadValue(ctx, ref); err != nil || value != "runtime-value" {
		t.Fatalf("runtime read: %q %v", value, err)
	}

	// Restart: no probe, no new credential.
	writesBefore := fv.writes
	if outcome, err := svc.Bootstrap(ctx, "acme", bootstrapCmd(fv, "app-token")); err != nil || outcome != secretstores.BootstrapUnchanged || fv.writes != writesBefore || creds.Len() != 1 {
		t.Fatalf("restart: %v %v writes=%d creds=%d", outcome, err, fv.writes-writesBefore, creds.Len())
	}
}

func TestBootstrapWithRealVerifierRejectsUnusableVault(t *testing.T) {
	ctx := context.Background()
	for name, setup := range map[string]func(*fakeVault){
		"wrong token":         func(f *fakeVault) { f.token = "other" },
		"not KV v2":           func(f *fakeVault) { f.notKV2 = true },
		"missing capability":  func(f *fakeVault) { f.capabilities = map[string][]string{"/metadata/": {"read"}} },
		"probe write fails":   func(f *fakeVault) { f.failWrite = true },
		"probe not removable": func(f *fakeVault) { f.noDelete = true },
	} {
		t.Run(name, func(t *testing.T) {
			_, st, creds := newRegistryFixture(t)
			fv := newFakeVault(t, "app-token")
			setup(fv)
			legacy := secretstore.Store{Key: "platform-vault", OrganizationKey: "acme", Name: "Platform Vault (legacy)", Provider: secretstore.ProviderVaultKV2, Status: secretstore.StatusReady,
				BackendAddress: fv.server.URL, WorkloadAddress: "http://vault.vault.svc:8200", Mount: "kv", AuthMount: "kubernetes", Legacy: true}
			if err := st.CreateSecretStore(ctx, legacy); err != nil {
				t.Fatal(err)
			}
			_, err := secretstores.NewService(st, Verifier{}, creds).Bootstrap(ctx, "acme", bootstrapCmd(fv, "app-token"))
			if !errors.Is(err, secretstores.ErrVerification) || strings.Contains(err.Error(), "app-token") {
				t.Fatalf("expected a safe verification error, got %v", err)
			}
			got, _ := st.GetSecretStore(ctx, "acme", "platform-vault")
			if !got.Legacy || got.CredentialRef != "" || creds.Len() != 0 {
				t.Fatalf("failed bootstrap left state: %+v creds=%d", got, creds.Len())
			}
		})
	}
}
