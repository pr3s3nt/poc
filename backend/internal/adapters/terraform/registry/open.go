package registry

import (
	"context"

	"orchestrator/internal/adapters/terraform"
)

// Options configures Open.
type Options struct {
	// PolicyFile is the Admin source allowlist; empty allows only the
	// packaged release modules.
	PolicyFile string
	// CacheDir is the private, absolute module cache directory.
	CacheDir string
	// GitBinary is the git CLI used for allowlisted HTTPS sources.
	GitBinary string
	// TerraformBinary is the CLI execution verification hashes; empty
	// makes VerifyForExecution fail closed.
	TerraformBinary string
	Store           BundleStore
}

// Open builds the registry from the embedded release, the Admin policy and
// the private cache, then runs platform preparation for packaged modules.
func Open(ctx context.Context, opts Options) (*Service, error) {
	release, err := DefaultRelease()
	if err != nil {
		return nil, err
	}
	policy, err := LoadPolicy(opts.PolicyFile, release)
	if err != nil {
		return nil, err
	}
	cache, err := OpenCache(opts.CacheDir)
	if err != nil {
		return nil, err
	}
	var git GitFetcher
	if len(policy.Sources) > 0 {
		cli, err := NewGitCLI(opts.GitBinary, cache.TempDir())
		if err != nil {
			return nil, err
		}
		git = cli
	}
	svc, err := New(Config{Release: release, Policy: policy, Packaged: terraform.PackagedModules(), Git: git, Cache: cache, Store: opts.Store, TerraformBinary: opts.TerraformBinary})
	if err != nil {
		return nil, err
	}
	if err := svc.Prepare(ctx); err != nil {
		return nil, err
	}
	return svc, nil
}
