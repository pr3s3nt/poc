package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"orchestrator/internal/application/secretstores"
	"orchestrator/internal/ports/persistence"
)

// PlatformVaultName is the display name of the bootstrap store.
const PlatformVaultName = "Platform Vault"

// seedPlatformVault admits the bundled Compose Vault as an ordinary verified
// store before the server accepts requests (UC-04 SS-07/08). It runs the same
// validation, verifier and credential persistence as registration. Missing
// inputs, verification failure or an identity conflict stop startup; nothing
// is fabricated READY and no environment is selected.
func seedPlatformVault(ctx context.Context, st persistence.Store, svc *secretstores.Service, opts Options) error {
	cfg := opts.PlatformVault
	if cfg.Address == "" || cfg.TokenFile == "" {
		return fmt.Errorf("bootstrap: the platform Vault bootstrap needs an address and a token file")
	}
	raw, err := os.ReadFile(cfg.TokenFile)
	if err != nil {
		return fmt.Errorf("bootstrap: read the platform Vault bootstrap token file")
	}
	token := strings.TrimSpace(string(raw))
	if token == "" {
		return fmt.Errorf("bootstrap: the platform Vault bootstrap token file is empty")
	}
	workload := cfg.WorkloadAddress
	if workload == "" {
		workload = cfg.Address
	}
	cmd := secretstores.RegisterCommand{
		Name: PlatformVaultName, BackendAddress: cfg.Address, WorkloadAddress: workload,
		Mount: cfg.Mount, AuthMount: cfg.AuthMount, Token: token,
	}

	// The seed Organization is always bootstrapped. Any other Organization is
	// bootstrapped only to upgrade or refresh its existing Compose-managed record.
	orgs := []string{opts.Seed.OrganizationKey}
	apps, err := st.ListApplications(ctx)
	if err != nil {
		return fmt.Errorf("bootstrap: list applications for the platform Vault: %w", err)
	}
	seen := map[string]bool{opts.Seed.OrganizationKey: true}
	for _, app := range apps {
		if seen[app.OrganizationKey] {
			continue
		}
		seen[app.OrganizationKey] = true
		existing, err := st.GetSecretStore(ctx, app.OrganizationKey, secretstores.BootstrapKey)
		switch {
		case errors.Is(err, persistence.ErrNotFound):
		case err != nil:
			return fmt.Errorf("bootstrap: look up the platform Vault store of organization %q: %w", app.OrganizationKey, err)
		case existing.Legacy || existing.Verification["bootstrap"] == secretstores.BootstrapMarker:
			orgs = append(orgs, app.OrganizationKey)
		}
	}
	for _, org := range orgs {
		outcome, err := svc.Bootstrap(ctx, org, cmd)
		if err != nil {
			var cleanup *secretstores.CleanupError
			if errors.As(err, &cleanup) || errors.Is(err, secretstores.ErrBootstrapConflict) || errors.Is(err, secretstores.ErrVerification) ||
				errors.Is(err, secretstores.ErrInvalid) || errors.Is(err, secretstores.ErrCredentialStore) {
				return fmt.Errorf("bootstrap: platform Vault for organization %q: %w", org, err)
			}
			return fmt.Errorf("bootstrap: platform Vault for organization %q failed", org)
		}
		log.Printf("orchestrator: platform Vault store %s (%s) for organization %s", secretstores.BootstrapKey, outcome, org)
	}
	return nil
}
