package bootstrap

import (
	"errors"
	"fmt"

	"orchestrator/internal/seed"
)

// validateRealProfile enforces the real v2 acceptance profile: product
// planner -> pinned Terraform with encrypted PostgreSQL state -> required
// pinned score-k8s -> real Kubernetes delivery, scoped Vault Connections. No
// fake or native fallback, no test override and no host credential chain can
// stand in for any of them. It runs before anything is opened.
func validateRealProfile(opts Options) error {
	if opts.Seed.Profile != seed.ProfileAcceptanceV2 {
		return nil
	}
	var problems []string
	add := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }
	if opts.Adapters != AdapterKubernetes && opts.Adapters != AdapterAWS {
		add("adapters must be kubernetes or aws, not %q", opts.Adapters)
	}
	if opts.DatabaseURL == "" || opts.StatePath != "" {
		add("a PostgreSQL Console store is required (no JSON snapshot)")
	}
	if opts.TerraformStateKeyFile == "" || opts.TerraformStateDatabaseURL == "" || opts.TerraformRecoveryDir == "" {
		add("the encrypted PostgreSQL Terraform state backend and recovery directory are required")
	}
	if !opts.RequireScoreRenderer || opts.ScoreK8sPath == "" {
		add("the pinned score-k8s renderer is required (no native fallback)")
	}
	if opts.ConnectionCredentialStore != "vault" || opts.ConnectionVaultAddress == "" || opts.ConnectionVaultTokenFile == "" {
		add("scoped Vault Connection credentials are required")
	}
	if opts.OutputSecretStore != "vault" {
		add("the durable Vault output-secret store is required (the in-process store loses references on restart)")
	}
	if opts.WorkloadDelivery != "" && opts.WorkloadDelivery != "direct" {
		add("workload delivery must be direct")
	}
	overrides := map[string]bool{
		"RegistryOverride": opts.RegistryOverride != nil, "KubeconfigVerifierOverride": opts.KubeconfigVerifierOverride != nil,
		"AWSVerifierOverride": opts.AWSVerifierOverride != nil, "ConnectionCredentialsOverride": opts.ConnectionCredentialsOverride != nil,
		"RendererOverride": opts.RendererOverride != nil, "DeployerOverride": opts.DeployerOverride != nil,
		"StoreRegistryOverride": opts.StoreRegistryOverride != nil, "SecretStoreVerifierOverride": opts.SecretStoreVerifierOverride != nil,
		"CleanupEngineOverride": opts.CleanupEngineOverride != nil, "ClusterReadinessOverride": opts.ClusterReadinessOverride != nil,
	}
	for name, set := range overrides {
		if set {
			add("%s is a test seam and is refused", name)
		}
	}
	if len(problems) == 0 {
		return nil
	}
	return errors.New("bootstrap: real acceptance profile refused: " + joinSorted(problems))
}

func joinSorted(items []string) string {
	out := ""
	for i, item := range sortedCopy(items) {
		if i > 0 {
			out += "; "
		}
		out += item
	}
	return out
}

func sortedCopy(items []string) []string {
	out := append([]string(nil), items...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// ValidateRealProfile exposes the real acceptance profile guard to the
// operator tools that build the same wiring (the independent safety net), so
// they cannot run with a seam the product itself would refuse.
func ValidateRealProfile(opts Options) error { return validateRealProfile(opts) }
