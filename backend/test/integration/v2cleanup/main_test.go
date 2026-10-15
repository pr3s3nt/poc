package main

import (
	"path/filepath"
	"testing"
	"time"

	"orchestrator/internal/bootstrap"
	"orchestrator/test/integration/v2run"
)

func TestCleanupOptionsWireVaultDeliveryForEachAdapter(t *testing.T) {
	dir := t.TempDir()
	databaseURL := filepath.Join(dir, "database.url")
	stateURL := filepath.Join(dir, "state.url")
	for path, value := range map[string]string{databaseURL: "postgres://fixture/console", stateURL: "postgres://fixture/state"} {
		if err := v2run.WriteFilePrivate(path, []byte(value)); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		name, adapters, want string
	}{
		{name: "Kubernetes keeps VSO", adapters: "kubernetes", want: "vso"},
		{name: "AWS uses the native delivery path", adapters: "aws", want: "agent"},
		{name: "legacy Kubernetes handle keeps VSO", adapters: "", want: "vso"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := v2run.CleanupHandles{
				RunID: "a02-0001", OrganizationKey: "acme", ApplicationKey: "acceptance", EnvironmentKey: "staging",
				NamespaceIdentity: "orch-v2-test-a02-0001", Owner: "codex", Adapters: tt.adapters,
				Terraform: "/p/terraform", Kubectl: "/p/kubectl", ScoreK8s: "/p/score-k8s", Git: "/p/git",
				DatabaseURLFile: databaseURL, StateDatabaseURLFile: stateURL, StateKeyFile: "/p/state.key", StateKeyID: "v2-a02",
				RecoveryDir: "/p/recovery", TerraformRoot: "/p/tfroot", PluginCacheDir: "/p/plugins", ModuleCacheDir: "/p/modules",
				VaultAddress: "http://127.0.0.1:18200", VaultMount: "secret", ConnectionTokenFile: "/p/connection-token", OutputTokenFile: "/p/output-token",
				CleanupBudget: v2run.Duration{Duration: 10 * time.Minute},
			}
			got, err := options(h)
			if err != nil {
				t.Fatalf("options(%q): %v", tt.adapters, err)
			}
			wantAdapter := bootstrap.AdapterKubernetes
			if tt.adapters == "aws" {
				wantAdapter = bootstrap.AdapterAWS
			}
			if got.Adapters != wantAdapter || got.VaultDelivery != tt.want {
				t.Fatalf("cleanup wiring for %q = adapter %q / Vault %q, want %q / %q", tt.adapters, got.Adapters, got.VaultDelivery, wantAdapter, tt.want)
			}
		})
	}
}
