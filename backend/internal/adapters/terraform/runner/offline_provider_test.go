//go:build offlineprovider

package runner_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/adapters/terraform/registry"
	"orchestrator/internal/adapters/terraform/runner"
	"orchestrator/internal/adapters/terraform/runner/runnertest"
	"orchestrator/internal/ports/terraformexec"
)

// Explicit image-only check: docker run --network none. Use the production
// registry, artifacts, per-module release locks and runner. No plan or apply.
func TestOfflinePackagedModules(t *testing.T) {
	config := os.Getenv("ORCHESTRATOR_TERRAFORM_CLI_CONFIG_FILE")
	if config == "" {
		t.Fatal("offline image CLI configuration is required")
	}
	ctx := context.Background()
	cache := t.TempDir()
	if err := os.Chmod(cache, 0o700); err != nil {
		t.Fatal(err)
	}
	modules, err := registry.Open(ctx, registry.Options{
		CacheDir: cache, Store: store.New(), TerraformBinary: runnertest.HostTerraform(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	refs, err := registry.PackagedRefs()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"vpc", "eks", "aurora", "elasticache", "k8s-namespace", "redis-k8s", "postgres-k8s"} {
		t.Run(id, func(t *testing.T) {
			art, err := modules.VerifyForExecution(ctx, refs[id])
			if err != nil {
				t.Fatal(err)
			}
			defer art.Close()
			r := runner.New(runner.Options{CLIConfigFile: config})
			s, err := r.Open(ctx, terraformexec.SessionRequest{Artifact: &art, LocalState: []byte{}, Deadline: time.Now().Add(2 * time.Minute)})
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			if err := s.Init(ctx); err != nil {
				t.Fatalf("offline init: %v", err)
			}
			got, err := os.ReadFile(filepath.Join(art.ModuleDir, ".terraform.lock.hcl"))
			if err != nil || string(got) != string(art.LockFile) {
				t.Fatalf("readonly release lock changed: %v", err)
			}
		})
	}
}
