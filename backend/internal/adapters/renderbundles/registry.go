package renderbundles

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"orchestrator/internal/adapters/terraform/registry"
	"orchestrator/internal/domain/renderbundle"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/moduleregistry"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/renderregistry"
	"orchestrator/internal/renderengine"
)

// Fetcher is the hardened Git transport (registry.GitCLI).
type Fetcher interface {
	Resolve(ctx context.Context, source registry.GitSource, ref string) (string, error)
	Open(ctx context.Context, source registry.GitSource, commit string) (registry.Snapshot, error)
}

// Runtime is the verified score-k8s binary that records pin.
type Runtime interface {
	BinaryDigest() string
	CheckBinary() error
}

// Registry implements renderregistry.Registry and the renderer's verified
// content loader.
type Registry struct {
	Cache   *Cache
	Git     Fetcher
	Store   persistence.RenderBundleRepository
	Runtime Runtime
	Now     func() time.Time
}

var _ renderregistry.Registry = (*Registry)(nil)

const maxURLLength = 2048

// Inspect implements renderregistry.Registry.
func (r *Registry) Inspect(ctx context.Context, organizationKey string, req renderregistry.Inspection) (renderbundle.Bundle, error) {
	url := strings.TrimSpace(req.URL)
	path := strings.TrimSuffix(strings.TrimSpace(req.Path), "/")
	ref := strings.TrimSpace(req.Ref)
	if path == "" {
		path = "."
	}
	if len(url) > maxURLLength || registry.ValidateGitURL(url) != nil || !renderbundle.ValidDirectory(path) || !tfmodule.ValidSubdirectory(path) ||
		!(tfmodule.FullCommit(ref) || registry.ValidGitReference(ref)) {
		return renderbundle.Bundle{}, renderregistry.ErrSourceInvalid
	}
	if r.Runtime == nil || r.Runtime.BinaryDigest() == "" || r.Runtime.CheckBinary() != nil || r.Cache == nil || r.Git == nil || r.Store == nil {
		return renderbundle.Bundle{}, fmt.Errorf("%w: Git render bundles are not enabled on this runtime", renderregistry.ErrBundleUnavailable)
	}
	source := registry.GitSource{ID: "render-bundle", Kind: tfmodule.SourceKindGit, URL: url}
	commit := ref
	if !tfmodule.FullCommit(commit) {
		resolved, err := r.Git.Resolve(ctx, source, ref)
		if err != nil {
			return renderbundle.Bundle{}, renderregistry.ErrSourceUnavailable
		}
		commit = resolved
	}
	snapshot, err := r.Git.Open(ctx, source, commit)
	if err != nil {
		return renderbundle.Bundle{}, renderregistry.ErrSourceUnavailable
	}
	tree, err := snapshot.Tree(path)
	snapshot.Close()
	if err != nil {
		if errors.Is(err, moduleregistry.ErrRejected) {
			// Symlinks, submodules, executables, unsafe paths or oversize trees.
			return renderbundle.Bundle{}, renderregistry.ErrBundleInvalid
		}
		return renderbundle.Bundle{}, renderregistry.ErrSourceUnavailable
	}
	return r.prepare(ctx, organizationKey, renderbundle.Source{Kind: renderbundle.SourceKindGit, URL: url, Path: path, Revision: commit}, tree)
}

func (r *Registry) prepare(ctx context.Context, organizationKey string, source renderbundle.Source, tree registry.Tree) (renderbundle.Bundle, error) {
	files := map[string][]byte(tree)
	bundle, err := renderengine.Validate(files)
	if err != nil {
		return renderbundle.Bundle{}, classifyInvalid(err)
	}
	digest := Digest(files)
	// Persist metadata only after the content is durably cached and verified.
	if err := r.Cache.Store(digest, files); err != nil {
		return renderbundle.Bundle{}, err
	}
	now := time.Now
	if r.Now != nil {
		now = r.Now
	}
	source.ContentSHA256 = digest
	record := renderbundle.Bundle{
		ID: ids.New(), OrganizationKey: organizationKey,
		Source:          source,
		EngineContract:  renderbundle.EngineContract,
		RendererVersion: renderbundle.RendererVersion,
		RendererBinary:  r.Runtime.BinaryDigest(),
		Contract:        bundle.Contract,
		CreatedAt:       now().UTC(),
	}
	stored, err := r.Store.InsertRenderBundle(ctx, record)
	if err != nil {
		return renderbundle.Bundle{}, err
	}
	if !stored.SameVerifiedContent(record) {
		return renderbundle.Bundle{}, renderregistry.ErrDrift
	}
	return stored, nil
}

func classifyInvalid(err error) error {
	if errors.Is(err, renderbundle.ErrUnsupported) {
		return renderregistry.ErrContractUnsupported
	}
	var invalid *renderbundle.InvalidError
	if errors.As(err, &invalid) {
		return &BundleInvalidError{File: invalid.File}
	}
	return renderregistry.ErrBundleInvalid
}

// BundleInvalidError is ErrBundleInvalid at a safe bundle-relative file.
type BundleInvalidError struct{ File string }

func (e *BundleInvalidError) Error() string {
	return renderregistry.ErrBundleInvalid.Error() + ": " + e.File
}
func (e *BundleInvalidError) Is(target error) bool { return target == renderregistry.ErrBundleInvalid }

// Available implements renderregistry.Registry.
func (r *Registry) Available(_ context.Context, b renderbundle.Bundle) error {
	_, err := r.load(resource.GitRenderPin(b))
	return err
}

// Load implements the renderer's verified content loader: engine and binary
// compatibility, then the digest-verified cache. It never fetches Git.
func (r *Registry) Load(ctx context.Context, pin resource.RenderBundle) (renderengine.Bundle, error) {
	if err := ctx.Err(); err != nil {
		return renderengine.Bundle{}, err
	}
	return r.load(pin)
}

func (r *Registry) load(pin resource.RenderBundle) (renderengine.Bundle, error) {
	if pin.Source == nil || pin.EngineContract != renderbundle.EngineContract || pin.RendererVersion != renderbundle.RendererVersion {
		return renderengine.Bundle{}, fmt.Errorf("%w: unsupported engine contract", renderregistry.ErrBundleUnavailable)
	}
	if r.Runtime == nil || r.Cache == nil || pin.BinaryDigest != r.Runtime.BinaryDigest() || r.Runtime.CheckBinary() != nil {
		return renderengine.Bundle{}, fmt.Errorf("%w: the runtime renderer does not match the pin", renderregistry.ErrBundleUnavailable)
	}
	files, err := r.Cache.Load(pin.Source.ContentSHA256)
	if err != nil {
		return renderengine.Bundle{}, err
	}
	bundle, err := renderengine.Validate(files)
	if err != nil {
		return renderengine.Bundle{}, fmt.Errorf("%w: cached content no longer validates", renderregistry.ErrBundleUnavailable)
	}
	return bundle, nil
}
