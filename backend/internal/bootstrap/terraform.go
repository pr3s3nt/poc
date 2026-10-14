package bootstrap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"orchestrator/internal/adapters/awsaccess"
	pgstore "orchestrator/internal/adapters/postgres"
	"orchestrator/internal/adapters/terraform/providers"
	"orchestrator/internal/adapters/terraform/readiness"
	"orchestrator/internal/adapters/terraform/recovery"
	moduleregistry "orchestrator/internal/adapters/terraform/registry"
	"orchestrator/internal/adapters/terraform/runner"
	"orchestrator/internal/application/connection"
	"orchestrator/internal/application/statebackend"
	"orchestrator/internal/application/tfexec"
	deliveryhttp "orchestrator/internal/delivery/http"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/ports/terraformstate"
)

// terraformRuntime is the durable state backend the v2 executor runs on.
type terraformRuntime struct {
	svc    *statebackend.Service
	repo   *pgstore.StateStore
	cipher terraformstate.Cipher
	// executor is the Terraform v2 executor once the real adapters built it;
	// it is also the cleanup engine and the UNREFERENCED instance marker.
	executor *tfexec.Service
}

// volatileTags are per-run values frozen into an instance at its creation:
// the run ID is provenance only and an expiry must not rewrite every deploy.
var volatileTags = map[string]bool{"run-id": true, "expires-at": true}

// newTerraformExecutor wires the Terraform v2 executor: the pinned verified
// module registry, the real Terraform CLI runner, the scoped provider binder
// (no host credential chain), the real readiness check, the encrypted
// recovery store and the internal loopback state backend. It never falls back
// to a local state, a host credential or an always-ready check.
func newTerraformExecutor(opts Options, tf *terraformRuntime, resolver *connection.CredentialResolver, secrets execution.SecretStore, modules *moduleregistry.Service, closers *[]func()) (*tfexec.Service, error) {
	if opts.TerraformRecoveryDir == "" {
		return nil, fmt.Errorf("bootstrap: the Terraform executor needs a private -terraform-recovery-dir for encrypted recovery artifacts")
	}
	recoveryStore, err := recovery.New(opts.TerraformRecoveryDir, tf.cipher)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: %w", err)
	}
	work, err := os.MkdirTemp(opts.TerraformRoot, "terraform-work-")
	if err != nil {
		return nil, fmt.Errorf("bootstrap: create the private Terraform work directory: %w", err)
	}
	*closers = append(*closers, func() { _ = os.RemoveAll(work) })

	listener, err := deliveryhttp.ServeTerraformStateLoopback(deliveryhttp.NewTerraformStateHandler(tf.svc))
	if err != nil {
		return nil, fmt.Errorf("bootstrap: %w", err)
	}
	*closers = append(*closers, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = listener.Close(ctx)
	})

	tags, creation := map[string]string{}, map[string]string{}
	for k, v := range opts.Tags {
		if volatileTags[k] {
			creation[k] = v
		} else {
			tags[k] = v
		}
	}
	aws := awsaccess.New()
	kubectl := opts.KubectlPath
	if kubectl == "" {
		kubectl = "kubectl"
	}
	checker := &readiness.Checker{Kubectl: kubectl, AWS: aws, Dir: work}
	return tfexec.New(tfexec.Config{BackendBase: listener.Address, Tags: tags, CreationTags: creation}, tfexec.Deps{
		State: tf.repo, Tokens: tf.svc, Modules: modules,
		Runner:    runner.New(runner.Options{PluginCacheDir: opts.TerraformPluginCache, PassThroughEnv: []string{"HTTPS_PROXY", "HTTP_PROXY", "NO_PROXY", "https_proxy", "http_proxy", "no_proxy"}}),
		Binder:    &providers.Binder{AWS: resolver, Kube: resolver, API: aws, Dir: filepath.Join(work)},
		Readiness: checker, Namespaces: checker,
		Recovery: recoveryStore, Verification: recoveryStore, Secrets: secrets,
	})
}
