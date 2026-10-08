package bootstrap

import (
	"context"
	"errors"
	"fmt"

	"orchestrator/internal/adapters/vault"
	"orchestrator/internal/domain/secretstore"
	"orchestrator/internal/ports/persistence"
)

// seedLegacyStore creates the explicit per-Organization platform store from the
// flag-configured Vault of earlier releases and stamps its identity on legacy
// configuration entries (ADR-012). It performs no Vault network I/O, never
// selects the store for a new Environment and is idempotent. Without a
// configured legacy Vault nothing is fabricated: old refs stay unreadable and
// reads fail with an actionable message.
func seedLegacyStore(ctx context.Context, st persistence.Store, opts Options, legacy *vault.LegacyConfig) error {
	if legacy == nil {
		return nil
	}
	orgs := map[string]bool{}
	if opts.Seed.OrganizationKey != "" {
		orgs[opts.Seed.OrganizationKey] = true
	}
	apps, err := st.ListApplications(ctx)
	if err != nil {
		return fmt.Errorf("bootstrap: list applications for the legacy store: %w", err)
	}
	for _, app := range apps {
		orgs[app.OrganizationKey] = true
	}
	workload := legacy.AgentAddress
	if workload == "" {
		workload = legacy.Address
	}
	for org := range orgs {
		if _, err := st.GetOrganization(ctx, org); err != nil {
			continue
		}
		record := secretstore.Store{
			Key: vault.LegacyStoreKey, OrganizationKey: org, Name: "Platform Vault (legacy)", Provider: secretstore.ProviderVaultKV2, Status: secretstore.StatusReady,
			BackendAddress: legacy.Address, WorkloadAddress: workload, Mount: legacy.Mount, AuthMount: legacy.AuthMount, Legacy: true,
			Verification: map[string]any{"legacy": true, "verified": false},
		}
		if err := st.CreateSecretStore(ctx, record); err != nil && !errors.Is(err, persistence.ErrDuplicate) {
			return fmt.Errorf("bootstrap: create the legacy secret store: %w", err)
		}
		if err := st.BackfillLegacySecretStore(ctx, org, vault.LegacyStoreKey); err != nil {
			return fmt.Errorf("bootstrap: backfill legacy configuration: %w", err)
		}
	}
	return nil
}
