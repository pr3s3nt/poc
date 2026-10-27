package bootstrap

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTerraformCLIConfigIsExplicitAndValidated(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "offline.tfrc")
	if err := os.WriteFile(config, []byte("provider_installation {}"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TF_CLI_CONFIG_FILE", "/untrusted/host.tfrc")
	for _, path := range []string{"", config} {
		if err := validateTerraformCLIConfig(path); err != nil {
			t.Fatal(err)
		}
		if got := terraformRunnerOptions(Options{TerraformCLIConfigFile: path}).CLIConfigFile; got != path {
			t.Fatalf("runner configuration %q, want %q", got, path)
		}
	}
	for _, path := range []string{"relative.tfrc", dir, filepath.Join(dir, "missing")} {
		if err := validateTerraformCLIConfig(path); err == nil {
			t.Fatalf("accepted invalid CLI configuration %q", path)
		}
	}
}
