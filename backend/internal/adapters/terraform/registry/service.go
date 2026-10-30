package registry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	access "orchestrator/internal/ports/gitaccess"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
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
	// Retained lists trusted historical platform releases kept for cleanup:
	// the original Terraform binary and pins of bundles verified before an
	// upgrade. They are platform-reviewed configuration, used only by
	// VerifyHistoricalForExecution; nothing is downloaded or re-matched.
	Retained []RetainedRelease
	Now      func() time.Time
}

// RetainedRelease is one retained historical release and the local path of
// its pinned Terraform binary.
type RetainedRelease struct {
	Release         Release
	TerraformBinary string
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
	retained := make([]RetainedRelease, len(cfg.Retained))
	for i, r := range cfg.Retained {
		r.Release = r.Release.Clone()
		if err := r.Release.validate(); err != nil || !filepath.IsAbs(r.TerraformBinary) {
			return nil, errors.New("registry: a retained release needs a valid manifest and an absolute Terraform binary path")
		}
		retained[i] = r
	}
	cfg.Retained = retained
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
		bundle, trees, err := s.build(sources, ref, tfmodule.SourceKindPackaged, module.ResourceTypes, module.ExecutorOutputs, "")
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
	ctx    context.Context
	svc    *Service
	root   moduleDir
	snaps  map[string]Snapshot
	direct map[string]GitSource
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
	if dynamic, found := r.direct[sourceID]; found {
		source, ok = dynamic, true
	}
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
	if r.direct == nil {
		return parseRemoteSource(source, r.svc.cfg.Policy)
	}
	repo, err := remoteRepository(source)
	if err != nil {
		return moduleDir{}, err
	}
	id := gitSourceID(repo)
	entry := GitSource{ID: id, Kind: tfmodule.SourceKindGit, URL: repo}
	if configured, ok := r.svc.cfg.Policy.source(id); ok && configured.URL == repo {
		entry = configured
	}
	// Reuse a platform CA for a matching repository without requiring allowlisting.
	for _, configured := range r.svc.cfg.Policy.Sources {
		if configured.URL == repo {
			entry.CAFile = configured.CAFile
		}
	}
	r.direct[id] = entry
	return parseRemoteSource(source, Policy{Sources: []GitSource{entry}})
}

func (r *resolver) close() {
	for _, snap := range r.snaps {
		snap.Close()
	}
}

// build reads the root tree, checks its digest against the pin, inspects the
// whole call tree and assembles the bundle.
func (s *Service) build(sources *resolver, ref tfmodule.SourceRef, kind string, resourceTypes, executorOutputs []string, organizationKey string) (tfmodule.Bundle, map[moduleDir]Tree, error) {
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
	result.contract.SourceOrganizationKey = organizationKey
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
	_, legacySource := s.cfg.Policy.source(ref.SourceID)
	// Uploaded pins can only be reused from verified metadata and cache.
	if !legacySource && strings.HasPrefix(ref.SourceID, "zip-") {
		existing, err := s.cfg.Store.GetModuleBundle(ctx, ref)
		if err != nil {
			return tfmodule.Bundle{}, moduleregistry.ErrNotVerified
		}
		if existing.Ref != ref || existing.Contract.SourceKind != tfmodule.SourceKindZIP || !tfmodule.ValidDigest(ref.Revision) || existing.Contract.SourceOrganizationKey == "" || ref.SourceID != zipSourceID(existing.Contract.SourceOrganizationKey, ref.Revision) {
			return tfmodule.Bundle{}, moduleregistry.ErrDrift
		}
		if err := s.releaseMatches(existing); err != nil {
			return tfmodule.Bundle{}, err
		}
		if _, err := s.cfg.Cache.verify(ref, existing.Contract); err != nil {
			return tfmodule.Bundle{}, err
		}
		return existing, nil
	}
	// Direct pins must already have been inspected by the authenticated service.
	if !legacySource && strings.HasPrefix(ref.SourceID, "git-") {
		existing, err := s.cfg.Store.GetModuleBundle(ctx, ref)
		if err != nil {
			return tfmodule.Bundle{}, moduleregistry.ErrNotVerified
		}
		if existing.Ref != ref || existing.Contract.SourceURL == "" || gitSourceID(existing.Contract.SourceURL) != ref.SourceID {
			return tfmodule.Bundle{}, moduleregistry.ErrDrift
		}
		if err := s.releaseMatches(existing); err != nil {
			return tfmodule.Bundle{}, err
		}
		if _, err := s.cfg.Cache.verify(ref, existing.Contract); err != nil {
			return tfmodule.Bundle{}, err
		}
		return existing, nil
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
	bundle, trees, err := s.build(sources, ref, kind, module.ResourceTypes, nil, "")
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

// ReadPinned returns only locally cached Terraform source at the exact verified
// pin. It deliberately calls no resolver, fetcher, Terraform CLI or provider.
// The cache re-hashes every transitive tree before any bytes are returned.
func (s *Service) ReadPinned(ctx context.Context, ref tfmodule.SourceRef) (map[string][]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	bundle, err := s.Bundle(ctx, ref)
	if err != nil {
		return nil, err
	}
	root, err := s.cfg.Cache.verify(ref, bundle.Contract)
	if err != nil {
		return nil, err
	}
	out := map[string][]byte{}
	dirs := []moduleDir{rootDir(ref)}
	digests := []string{ref.ContentSHA256}
	for _, child := range bundle.Contract.Transitive {
		dirs = append(dirs, moduleDir{sourceID: child.SourceID, revision: child.Revision, path: child.Path})
		digests = append(digests, child.ContentSHA256)
	}
	for index, dir := range dirs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		tree, err := readTree(sourceDir(root, dir))
		if err != nil || tree.Digest() != digests[index] {
			return nil, ErrArtifact
		}
		for file, data := range tree {
			if !strings.HasSuffix(file, ".tf") {
				continue
			}
			name := file
			if index > 0 {
				name = path.Join("modules", fmt.Sprint(index), dir.path, file)
			}
			out[name] = append([]byte(nil), data...)
		}
	}
	return out, nil
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

// BundleByID returns the persisted verified bundle with the given ID: the
// historical module of an instance. It never fetches.
func (s *Service) BundleByID(ctx context.Context, id string) (tfmodule.Bundle, error) {
	bundle, err := s.cfg.Store.GetModuleBundleByID(ctx, id)
	if errors.Is(err, persistence.ErrNotFound) {
		return tfmodule.Bundle{}, moduleregistry.ErrNotVerified
	}
	return bundle, err
}

// releaseMatches checks the bundle against the current release pins.
func (s *Service) releaseMatches(b tfmodule.Bundle) error { return matchRelease(s.cfg.Release, b) }

// matchRelease checks the bundle's Terraform and provider pins against one
// release manifest.
func matchRelease(release Release, b tfmodule.Bundle) error {
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

// invocationDir creates one private, owner-only directory for an execution.
func (s *Service) invocationDir() (string, error) {
	runs := filepath.Join(s.cfg.Cache.dir, "runs")
	if err := os.MkdirAll(runs, 0o700); err != nil {
		return "", errors.New("registry: prepare the private invocation area")
	}
	dir, err := os.MkdirTemp(runs, "inv-")
	if err != nil {
		return "", errors.New("registry: create a private invocation directory")
	}
	return dir, nil
}

// copyVerifiedBinary copies a Terraform CLI into the private invocation
// directory, hashing the very bytes it writes. The copy is returned only when
// its digest equals the release pin; the source path is never executed.
func copyVerifiedBinary(src, dest, want string) error {
	in, err := os.Open(src)
	if err != nil {
		return moduleregistry.ErrReleaseMismatch
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return moduleregistry.ErrReleaseMismatch
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return errors.New("registry: create the private binary directory")
	}
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o700)
	if err != nil {
		return errors.New("registry: create the private Terraform binary")
	}
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(out, h), in); err != nil {
		out.Close()
		return errors.New("registry: copy the Terraform binary")
	}
	if err := out.Close(); err != nil {
		return errors.New("registry: write the private Terraform binary")
	}
	if hex.EncodeToString(h.Sum(nil)) != want {
		return moduleregistry.ErrReleaseMismatch
	}
	// Read-only for the owner from here on.
	return os.Chmod(dest, 0o500)
}

// VerifyForExecution implements moduleregistry.ExecutionVerifier. It never
// fetches: a missing or drifted artifact stops the run. On success every
// returned path points into a private copy made for this call (Terraform
// binary, root module tree and every transitive module directory), verified
// from the bytes copied; the caller must Close the artifact.
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
	return s.materializeBundle(bundle, s.cfg.TerraformBinary)
}

// VerifyHistoricalForExecution implements moduleregistry.ExecutionVerifier for
// cleanup: it verifies the ORIGINAL persisted bundle against the retained
// trusted release it was verified under (Terraform binary, version and
// provider lock) and copies that release's binary and the bundle's cached
// trees privately. It never matches the current release, fetches or upgrades;
// a bundle with no retained release, or whose cache drifted, fails closed.
func (s *Service) VerifyHistoricalForExecution(ctx context.Context, bundle tfmodule.Bundle) (tfmodule.VerifiedArtifact, error) {
	stored, err := s.Bundle(ctx, bundle.Ref)
	if err != nil {
		return tfmodule.VerifiedArtifact{}, err
	}
	if stored.ID != bundle.ID || !stored.SameVerifiedContent(bundle) {
		return tfmodule.VerifiedArtifact{}, moduleregistry.ErrDrift
	}
	if err := s.releaseMatches(stored); err == nil && s.cfg.TerraformBinary != "" {
		return s.materializeBundle(stored, s.cfg.TerraformBinary)
	}
	for _, retained := range s.cfg.Retained {
		if matchRelease(retained.Release, stored) == nil {
			return s.materializeBundle(stored, retained.TerraformBinary)
		}
	}
	return tfmodule.VerifiedArtifact{}, moduleregistry.ErrReleaseMismatch
}

// materializeBundle copies the pinned binary and the verified cached trees of
// bundle into one private invocation directory.
func (s *Service) materializeBundle(bundle tfmodule.Bundle, binarySource string) (tfmodule.VerifiedArtifact, error) {
	ref := bundle.Ref
	for _, call := range bundle.Contract.ModuleCalls {
		if call.Transitive < 0 || call.Transitive >= len(bundle.Contract.Transitive) {
			return tfmodule.VerifiedArtifact{}, ErrArtifact
		}
	}
	inv, err := s.invocationDir()
	if err != nil {
		return tfmodule.VerifiedArtifact{}, err
	}
	fail := func(err error) (tfmodule.VerifiedArtifact, error) {
		_ = removePrivate(inv)
		return tfmodule.VerifiedArtifact{}, err
	}
	binary := filepath.Join(inv, "bin", "terraform")
	if err := copyVerifiedBinary(binarySource, binary, bundle.TerraformBinarySHA256); err != nil {
		return fail(err)
	}
	if err := s.cfg.Cache.materialize(ref, bundle.Contract, inv); err != nil {
		return fail(err)
	}
	installs := make([]tfmodule.ModuleInstall, 0, len(bundle.Contract.ModuleCalls))
	for _, call := range bundle.Contract.ModuleCalls {
		t := bundle.Contract.Transitive[call.Transitive]
		installs = append(installs, tfmodule.ModuleInstall{Key: call.Key, Source: call.Source, Dir: sourceDir(inv, moduleDir{sourceID: t.SourceID, revision: t.Revision, path: t.Path})})
	}
	return tfmodule.VerifiedArtifact{
		Bundle: bundle, Root: inv, SourceDir: inv, ModuleDir: sourceDir(inv, rootDir(ref)),
		LockFile: LockFile(bundle.Providers), TerraformBinary: binary, Modules: installs,
		Cleanup: func() error { return removePrivate(inv) },
		Retain:  func(handle string) error { return s.recordRetained(handle, inv) },
	}, nil
}

var retainedHandle = regexp.MustCompile(`^retained-[A-Za-z0-9-]{1,80}$`)

// recordRetained writes the private mapping of an opaque handle to the
// invocation directory the executor kept (0600 under the private cache).
func (s *Service) recordRetained(handle, inv string) error {
	if !retainedHandle.MatchString(handle) {
		return errors.New("registry: invalid retained handle")
	}
	dir := filepath.Join(s.cfg.Cache.dir, "runs", "retained")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return errors.New("registry: prepare the retained mapping")
	}
	return os.WriteFile(filepath.Join(dir, handle), []byte(filepath.Base(inv)), 0o600)
}

// ResolveRetained maps a retained handle (from an attempt's recovery handle)
// to the private invocation directory, for operator-driven recovery. It is an
// internal function: no public API or log carries the result.
func (s *Service) ResolveRetained(handle string) (string, error) {
	if !retainedHandle.MatchString(handle) {
		return "", errors.New("registry: invalid retained handle")
	}
	data, err := os.ReadFile(filepath.Join(s.cfg.Cache.dir, "runs", "retained", handle))
	if err != nil || strings.ContainsAny(string(data), "/\\") || len(data) == 0 {
		return "", errors.New("registry: the retained workspace is not recorded")
	}
	return filepath.Join(s.cfg.Cache.dir, "runs", string(data)), nil
}

// removePrivate deletes an invocation directory; it makes read-only entries
// writable first so a 0500 binary directory does not block removal.
func removePrivate(dir string) error {
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			_ = os.Chmod(p, 0o700)
		}
		return nil
	})
	return os.RemoveAll(dir)
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// SetGitAccess is called once at bootstrap before serving requests.
func (s *Service) SetGitAccess(resolver access.Resolver) {
	if git, ok := s.cfg.Git.(*GitCLI); ok {
		git.Access = resolver
	}
}
