package registry

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"sync"
	"time"

	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/moduleregistry"
	"orchestrator/internal/ports/persistence"
)

// BundleStore is the persistence the registry needs.
type BundleStore interface {
	persistence.ModuleBundleRepository
}

// Config wires one registry.
type Config struct {
	Release  Release
	Policy   Policy
	Packaged fs.FS // the trusted packaged module files of the release
	Git      GitFetcher
	Cache    *Cache
	Store    BundleStore
	// TerraformBinary is the Terraform CLI the executor runs. Execution
	// verification hashes it every time and requires the release digest;
	// when it is empty, preparation, registration and metadata still work
	// but VerifyForExecution fails closed (ErrBinaryUnverified).
	TerraformBinary string
	Now             func() time.Time
}

// Service is the module registry. It implements moduleregistry.Registry.
type Service struct {
	cfg Config

	mu       sync.Mutex
	prepared map[string]tfmodule.Bundle // packaged bundles by pin
}

// New validates the configuration.
func New(cfg Config) (*Service, error) {
	// Take ownership before validation: later changes to the caller's
	// release or policy values must not reach the validated copies.
	cfg.Release = cfg.Release.Clone()
	cfg.Policy = cfg.Policy.Clone()
	switch {
	case cfg.Packaged == nil || cfg.Cache == nil || cfg.Store == nil:
		return nil, errors.New("registry: packaged modules, a cache and a bundle store are required")
	case len(cfg.Policy.Sources) > 0 && cfg.Git == nil:
		return nil, errors.New("registry: allowlisted Git sources need a Git fetcher")
	}
	if err := cfg.Release.validate(); err != nil {
		return nil, fmt.Errorf("registry: release manifest: %w", err)
	}
	if err := cfg.Policy.Validate(cfg.Release); err != nil {
		return nil, err
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Service{cfg: cfg, prepared: map[string]tfmodule.Bundle{}}, nil
}

var _ moduleregistry.Registry = (*Service)(nil)

// Release returns a copy of the platform release the registry pins against.
func (s *Service) Release() Release { return s.cfg.Release.Clone() }

// Prepare is the platform preparation step: every packaged module of the
// release is read, checked against its fixed digest, inspected, cached and
// persisted (or, when already persisted, compared). Any drift stops startup.
func (s *Service) Prepare(ctx context.Context) error {
	release := s.cfg.Release
	for _, module := range release.Packaged.Modules {
		ref := release.Ref(module)
		sources := s.resolver(ctx, ref, packagedSnapshot{fsys: s.cfg.Packaged})
		bundle, trees, err := s.build(sources, ref, tfmodule.SourceKindPackaged, module.ResourceTypes, module.ExecutorOutputs)
		sources.close()
		if err != nil {
			return fmt.Errorf("registry: packaged module %q: %w", module.ID, err)
		}
		if err := s.cfg.Cache.store(ref, bundle.Contract, trees); err != nil {
			return fmt.Errorf("registry: packaged module %q: %w", module.ID, err)
		}
		stored, err := s.cfg.Store.InsertModuleBundle(ctx, bundle)
		if err != nil {
			return fmt.Errorf("registry: persist packaged module %q: %w", module.ID, err)
		}
		if !stored.SameVerifiedContent(bundle) {
			return fmt.Errorf("registry: packaged module %q: %w", module.ID, moduleregistry.ErrDrift)
		}
		s.mu.Lock()
		s.prepared[ref.Pin()] = stored.Clone()
		s.mu.Unlock()
	}
	return nil
}

// resolver serves the root snapshot and opens allowlisted Git sources at
// exact commits for remote child modules, each at most once.
type resolver struct {
	ctx   context.Context
	svc   *Service
	root  moduleDir
	snaps map[string]Snapshot
}

func (s *Service) resolver(ctx context.Context, ref tfmodule.SourceRef, root Snapshot) *resolver {
	dir := rootDir(ref)
	return &resolver{ctx: ctx, svc: s, root: dir, snaps: map[string]Snapshot{dir.sourceID + "@" + dir.revision: root}}
}

func (r *resolver) snapshot(sourceID, revision string) (Snapshot, error) {
	key := sourceID + "@" + revision
	if snap, ok := r.snaps[key]; ok {
		return snap, nil
	}
	source, ok := r.svc.cfg.Policy.source(sourceID)
	if !ok || r.svc.cfg.Git == nil || !tfmodule.FullCommit(revision) {
		return nil, moduleregistry.ErrNotAllowed
	}
	snap, err := r.svc.cfg.Git.Open(r.ctx, source, revision)
	if err != nil {
		return nil, err
	}
	r.snaps[key] = snap
	return snap, nil
}

func (r *resolver) tree(dir moduleDir) (Tree, error) {
	snap, err := r.snapshot(dir.sourceID, dir.revision)
	if err != nil {
		return nil, err
	}
	return snap.Tree(dir.path)
}

func (r *resolver) remote(source string) (moduleDir, error) {
	return parseRemoteSource(source, r.svc.cfg.Policy)
}

func (r *resolver) close() {
	for _, snap := range r.snaps {
		snap.Close()
	}
}

// build reads the root tree, checks its digest against the pin, inspects the
// whole call tree and assembles the bundle.
func (s *Service) build(sources *resolver, ref tfmodule.SourceRef, kind string, resourceTypes, executorOutputs []string) (tfmodule.Bundle, map[moduleDir]Tree, error) {
	root, err := sources.tree(rootDir(ref))
	if err != nil {
		return tfmodule.Bundle{}, nil, err
	}
	if root.Digest() != ref.ContentSHA256 {
		return tfmodule.Bundle{}, nil, moduleregistry.ErrDrift
	}
	result, err := inspect(sources, rootDir(ref), s.cfg.Release, kind, resourceTypes, executorOutputs)
	if err != nil {
		return tfmodule.Bundle{}, nil, err
	}
	bundle := tfmodule.Bundle{
		ID:                    ids.New(),
		Ref:                   ref,
		TerraformVersion:      s.cfg.Release.Terraform.Version,
		TerraformBinarySHA256: s.cfg.Release.Terraform.BinarySHA256,
		ProviderLockDigest:    LockDigest(result.providers),
		Contract:              result.contract,
		Providers:             result.providers,
		Status:                tfmodule.StatusVerified,
		CreatedAt:             s.cfg.Now().UTC().Truncate(time.Microsecond),
	}
	if err := bundle.Validate(); err != nil {
		return tfmodule.Bundle{}, nil, fmt.Errorf("%w: %v", ErrContract, err)
	}
	return bundle, result.trees, nil
}

// allowed resolves the policy entry of a pin: a packaged release module or
// an allowlisted Git module at a full commit.
func (s *Service) allowed(ref tfmodule.SourceRef) (kind string, source GitSource, module GitModule, err error) {
	release := s.cfg.Release
	if ref.SourceID == release.Packaged.SourceID {
		for _, m := range release.Packaged.Modules {
			if m.ID != ref.ModuleID {
				continue
			}
			pinned := release.Ref(m)
			if ref.Revision != pinned.Revision || ref.Subdirectory != pinned.Subdirectory {
				return "", GitSource{}, GitModule{}, moduleregistry.ErrNotAllowed
			}
			if ref.ContentSHA256 != pinned.ContentSHA256 {
				return "", GitSource{}, GitModule{}, moduleregistry.ErrDrift
			}
			return tfmodule.SourceKindPackaged, GitSource{}, GitModule{}, nil
		}
		return "", GitSource{}, GitModule{}, moduleregistry.ErrNotAllowed
	}
	source, ok := s.cfg.Policy.source(ref.SourceID)
	if !ok {
		return "", GitSource{}, GitModule{}, moduleregistry.ErrNotAllowed
	}
	module, ok = source.module(ref.ModuleID)
	if !ok || module.Subdirectory != ref.Subdirectory || !tfmodule.FullCommit(ref.Revision) {
		// Branches, tags, short IDs and other paths are mutable or not allowed.
		return "", GitSource{}, GitModule{}, moduleregistry.ErrNotAllowed
	}
	return tfmodule.SourceKindGit, source, module, nil
}

// Verify implements moduleregistry.Registrar.
func (s *Service) Verify(ctx context.Context, ref tfmodule.SourceRef) (tfmodule.Bundle, error) {
	if err := ref.Validate(); err != nil {
		return tfmodule.Bundle{}, moduleregistry.ErrNotAllowed
	}
	kind, source, module, err := s.allowed(ref)
	if err != nil {
		return tfmodule.Bundle{}, err
	}
	if kind == tfmodule.SourceKindPackaged {
		s.mu.Lock()
		bundle, ok := s.prepared[ref.Pin()]
		s.mu.Unlock()
		if !ok {
			return tfmodule.Bundle{}, errors.New("registry: packaged modules are not prepared")
		}
		if _, err := s.cfg.Cache.verify(ref, bundle.Contract); err != nil {
			return tfmodule.Bundle{}, err
		}
		return bundle.Clone(), nil
	}
	existing, err := s.cfg.Store.GetModuleBundle(ctx, ref)
	switch {
	case err == nil:
		if existing.Ref.ContentSHA256 != ref.ContentSHA256 {
			return tfmodule.Bundle{}, moduleregistry.ErrDrift
		}
		if s.releaseMatches(existing) != nil {
			return tfmodule.Bundle{}, moduleregistry.ErrReleaseMismatch
		}
		if _, err := s.cfg.Cache.verify(ref, existing.Contract); err == nil {
			return existing, nil
		} else if exists(s.cfg.Cache.bundleDir(ref)) {
			return tfmodule.Bundle{}, err // drifted cache entry: fail closed
		}
		// The cache entry is missing: registration may fetch it again and
		// must reproduce exactly the recorded bundle.
	case !errors.Is(err, persistence.ErrNotFound):
		return tfmodule.Bundle{}, err
	}
	snapshot, err := s.cfg.Git.Open(ctx, source, ref.Revision)
	if err != nil {
		return tfmodule.Bundle{}, err
	}
	sources := s.resolver(ctx, ref, snapshot)
	defer sources.close()
	bundle, trees, err := s.build(sources, ref, kind, module.ResourceTypes, nil)
	if err != nil {
		return tfmodule.Bundle{}, err
	}
	if existing.ID != "" {
		bundle.ID, bundle.CreatedAt = existing.ID, existing.CreatedAt
		if !bundle.SameVerifiedContent(existing) {
			return tfmodule.Bundle{}, moduleregistry.ErrDrift
		}
	}
	if err := s.cfg.Cache.store(ref, bundle.Contract, trees); err != nil {
		return tfmodule.Bundle{}, err
	}
	return bundle, nil
}

// Bundle implements moduleregistry.Metadata: prepared packaged bundles and
// persisted rows only, never a fetch.
func (s *Service) Bundle(ctx context.Context, ref tfmodule.SourceRef) (tfmodule.Bundle, error) {
	if err := ref.Validate(); err != nil {
		return tfmodule.Bundle{}, moduleregistry.ErrNotVerified
	}
	s.mu.Lock()
	bundle, ok := s.prepared[ref.Pin()]
	s.mu.Unlock()
	bundle = bundle.Clone()
	if !ok {
		var err error
		bundle, err = s.cfg.Store.GetModuleBundle(ctx, ref)
		if errors.Is(err, persistence.ErrNotFound) {
			return tfmodule.Bundle{}, moduleregistry.ErrNotVerified
		}
		if err != nil {
			return tfmodule.Bundle{}, err
		}
	}
	if bundle.Ref != ref {
		return tfmodule.Bundle{}, moduleregistry.ErrDrift
	}
	return bundle, nil
}

// List implements moduleregistry.Metadata.
func (s *Service) List(ctx context.Context) ([]tfmodule.Bundle, error) {
	bundles, err := s.cfg.Store.ListModuleBundles(ctx)
	if err != nil {
		return nil, err
	}
	sort.Slice(bundles, func(i, j int) bool { return bundles[i].Ref.Key() < bundles[j].Ref.Key() })
	return bundles, nil
}

// releaseMatches checks the bundle against the current release pins.
func (s *Service) releaseMatches(b tfmodule.Bundle) error {
	release := s.cfg.Release
	if b.TerraformVersion != release.Terraform.Version || b.TerraformBinarySHA256 != release.Terraform.BinarySHA256 {
		return moduleregistry.ErrReleaseMismatch
	}
	current := map[string]tfmodule.ProviderRequirement{}
	for local, p := range b.Providers {
		pin, ok := release.provider(p.Source)
		if !ok || pin.Version != p.Version || pin.Binding != p.Binding {
			return moduleregistry.ErrReleaseMismatch
		}
		p.Hashes = append([]string(nil), pin.Hashes...)
		current[local] = p
	}
	if LockDigest(current) != b.ProviderLockDigest || LockDigest(b.Providers) != b.ProviderLockDigest {
		return moduleregistry.ErrReleaseMismatch
	}
	return nil
}

// VerifyForExecution implements moduleregistry.ExecutionVerifier. It never
// fetches: a missing or drifted artifact stops the run.
func (s *Service) VerifyForExecution(ctx context.Context, ref tfmodule.SourceRef) (tfmodule.VerifiedArtifact, error) {
	bundle, err := s.Bundle(ctx, ref)
	if err != nil {
		return tfmodule.VerifiedArtifact{}, err
	}
	if err := s.releaseMatches(bundle); err != nil {
		return tfmodule.VerifiedArtifact{}, err
	}
	// A runnable artifact always includes the binary pin check: without a
	// configured Terraform binary nothing is verified for execution.
	if s.cfg.TerraformBinary == "" {
		return tfmodule.VerifiedArtifact{}, moduleregistry.ErrBinaryUnverified
	}
	sum, err := fileSHA256(s.cfg.TerraformBinary)
	if err != nil || sum != bundle.TerraformBinarySHA256 {
		return tfmodule.VerifiedArtifact{}, moduleregistry.ErrReleaseMismatch
	}
	bundleRoot, err := s.cfg.Cache.verify(ref, bundle.Contract)
	if err != nil {
		return tfmodule.VerifiedArtifact{}, err
	}
	installs := make([]tfmodule.ModuleInstall, 0, len(bundle.Contract.ModuleCalls))
	for _, call := range bundle.Contract.ModuleCalls {
		if call.Transitive < 0 || call.Transitive >= len(bundle.Contract.Transitive) {
			return tfmodule.VerifiedArtifact{}, ErrArtifact
		}
		t := bundle.Contract.Transitive[call.Transitive]
		installs = append(installs, tfmodule.ModuleInstall{Key: call.Key, Source: call.Source, Dir: sourceDir(bundleRoot, moduleDir{sourceID: t.SourceID, revision: t.Revision, path: t.Path})})
	}
	return tfmodule.VerifiedArtifact{
		Bundle: bundle, SourceDir: bundleRoot, ModuleDir: sourceDir(bundleRoot, rootDir(ref)),
		LockFile: LockFile(bundle.Providers), TerraformBinary: s.cfg.TerraformBinary, Modules: installs,
	}, nil
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}
