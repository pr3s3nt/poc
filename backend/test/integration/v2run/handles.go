package v2run

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CleanupHandles are the private handles the independent safety net needs to
// rebuild the SAME product wiring (scoped logins, state key, Vault tokens,
// pinned binaries) after the runner, the browser and the product server were
// killed. Secrets are file paths, never values. The file is 0600.
type CleanupHandles struct {
	RunID           string `json:"runId"`
	OrganizationKey string `json:"organizationKey"`
	ApplicationKey  string `json:"applicationKey"`
	EnvironmentKey  string `json:"environmentKey"`
	// NamespaceIdentity is the Environment's seeded identity (the exact
	// physical namespace comes from the Environment, never from this).
	NamespaceIdentity string `json:"namespaceIdentity"`
	Owner             string `json:"owner"`
	// Adapters is the product adapter mode the safety net rebuilds
	// ("kubernetes" or "aws"); empty means kubernetes.
	Adapters string `json:"adapters,omitempty"`

	Terraform string `json:"terraform"`
	Kubectl   string `json:"kubectl"`
	ScoreK8s  string `json:"scoreK8s"`
	Git       string `json:"git"`

	DatabaseURLFile      string `json:"databaseUrlFile"`
	StateDatabaseURLFile string `json:"stateDatabaseUrlFile"`
	StateKeyFile         string `json:"stateKeyFile"`
	StateKeyID           string `json:"stateKeyId"`
	RecoveryDir          string `json:"recoveryDir"`
	TerraformRoot        string `json:"terraformRoot"`
	PluginCacheDir       string `json:"pluginCacheDir"`
	ModuleCacheDir       string `json:"moduleCacheDir"`

	VaultAddress        string `json:"vaultAddress"`
	VaultMount          string `json:"vaultMount"`
	ConnectionTokenFile string `json:"connectionTokenFile"`
	OutputTokenFile     string `json:"outputTokenFile"`

	// CleanupBudget bounds the whole safety net: the v2cleanup context and the
	// RUN_CLEANUP claim (at most MaxCleanup, never unlimited).
	CleanupBudget     Duration `json:"cleanupBudget"`
	InvocationTimeout Duration `json:"invocationTimeout"`
	ReadinessTimeout  Duration `json:"readinessTimeout"`
}

// LoadCleanupHandles reads a private handles file strictly.
func LoadCleanupHandles(path string) (CleanupHandles, error) {
	var h CleanupHandles
	if err := requirePrivateFile(path); err != nil {
		return h, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return h, err
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&h); err != nil {
		return h, fmt.Errorf("invalid cleanup handles: %w", err)
	}
	return h, h.Validate()
}

// Validate checks that every handle is present and absolute.
func (h CleanupHandles) Validate() error {
	var errs []string
	for name, v := range map[string]string{"runId": h.RunID, "organizationKey": h.OrganizationKey, "applicationKey": h.ApplicationKey, "environmentKey": h.EnvironmentKey,
		"namespaceIdentity": h.NamespaceIdentity, "owner": h.Owner, "stateKeyId": h.StateKeyID, "vaultMount": h.VaultMount} {
		if v == "" {
			errs = append(errs, name+" is required")
		}
	}
	for name, v := range map[string]string{"terraform": h.Terraform, "kubectl": h.Kubectl, "scoreK8s": h.ScoreK8s, "git": h.Git, "databaseUrlFile": h.DatabaseURLFile,
		"stateDatabaseUrlFile": h.StateDatabaseURLFile, "stateKeyFile": h.StateKeyFile, "recoveryDir": h.RecoveryDir, "terraformRoot": h.TerraformRoot,
		"pluginCacheDir": h.PluginCacheDir, "moduleCacheDir": h.ModuleCacheDir, "connectionTokenFile": h.ConnectionTokenFile, "outputTokenFile": h.OutputTokenFile} {
		if !filepath.IsAbs(v) {
			errs = append(errs, name+" must be an absolute path")
		}
	}
	if !strings.HasPrefix(h.VaultAddress, "http://127.0.0.1:") && !strings.HasPrefix(h.VaultAddress, "https://") {
		errs = append(errs, "vaultAddress must be the run-owned loopback Vault or an https address")
	}
	if h.CleanupBudget.Duration <= 0 || h.CleanupBudget.Duration > MaxCleanup {
		errs = append(errs, "cleanupBudget must be positive and at most "+MaxCleanup.String())
	}
	if h.Adapters != "" && h.Adapters != "kubernetes" && h.Adapters != "aws" {
		errs = append(errs, "adapters must be kubernetes or aws")
	}
	if h.ConnectionTokenFile == h.OutputTokenFile {
		errs = append(errs, "the Connection and output-secret tokens must be separate files")
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.New("invalid cleanup handles: " + strings.Join(errs, "; "))
}
