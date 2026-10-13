package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"orchestrator/internal/ports/moduleregistry"
	"orchestrator/internal/seed"
)

// Bootstrap opens the module registry before seeding: packaged release
// modules are prepared and persisted, an Admin policy requires a durable
// private cache, and an invalid policy stops startup. In fake mode no
// Terraform binary is configured, so execution verification fails closed.
func TestModuleRegistryBootstrap(t *testing.T) {
	ctx := context.Background()
	app, err := Build(ctx, Options{Seed: seed.Defaults()})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	bundles, err := app.Store.ListModuleBundles(ctx)
	if err != nil || len(bundles) != 3 {
		t.Fatalf("prepared packaged bundles: %d %v", len(bundles), err)
	}
	if _, err := app.Modules.VerifyForExecution(ctx, bundles[0].Ref); err != moduleregistry.ErrBinaryUnverified {
		t.Fatalf("fake mode must not produce runnable artifacts: %v", err)
	}

	dir := t.TempDir()
	policy := filepath.Join(dir, "policy.json")
	if err := os.WriteFile(policy, []byte(`{"sources":[{"id":"acme","kind":"git","url":"https://git.example.com/acme/modules.git","modules":[{"id":"redis","subdirectory":"modules/redis","resourceTypes":["redis"]}]}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(ctx, Options{Seed: seed.Defaults(), TerraformModulePolicyFile: policy}); err == nil || !strings.Contains(err.Error(), "cache") {
		t.Fatalf("a Git policy without a durable cache must fail: %v", err)
	}
	cache := filepath.Join(dir, "cache")
	withPolicy, err := Build(ctx, Options{Seed: seed.Defaults(), TerraformModulePolicyFile: policy, TerraformModuleCacheDir: cache})
	if err != nil {
		t.Fatal(err)
	}
	withPolicy.Close()
	if info, err := os.Stat(cache); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("cache directory: %v %v", info, err)
	}
	if err := os.WriteFile(policy, []byte(`{"sources":[{"id":"acme","kind":"git","url":"http://git.example.com/a.git","modules":[]}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(ctx, Options{Seed: seed.Defaults(), TerraformModulePolicyFile: policy, TerraformModuleCacheDir: cache}); err == nil {
		t.Fatal("an invalid Admin policy must stop startup")
	}
}
