package registry

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/ports/moduleregistry"
)

func gitSourceID(url string) string { return fmt.Sprintf("git-%x", sha256.Sum256([]byte(url)))[:60] }
func gitModuleID(typ, path string) string {
	return fmt.Sprintf("module-%x", sha256.Sum256([]byte(typ+"\x00"+path)))[:40]
}

// InspectGit accepts a human Git reference only at preparation time. It stores
// the exact commit and content, so registration and execution never resolve it.
func (s *Service) InspectGit(ctx context.Context, request moduleregistry.GitInspection) (tfmodule.Bundle, error) {
	if validateGitURL(request.URL) != nil || !tfmodule.ValidIdentifier(request.ResourceType) {
		return tfmodule.Bundle{}, moduleregistry.ErrNotAllowed
	}
	if request.Path == "" {
		request.Path = "."
	}
	if !tfmodule.ValidSubdirectory(request.Path) || !validGitReference(request.Ref) {
		return tfmodule.Bundle{}, moduleregistry.ErrNotAllowed
	}
	source := GitSource{ID: gitSourceID(request.URL), Kind: tfmodule.SourceKindGit, URL: request.URL}
	for _, configured := range s.cfg.Policy.Sources {
		if configured.URL == request.URL {
			source.CAFile = configured.CAFile
		}
	}
	commit := request.Ref
	if !tfmodule.FullCommit(commit) {
		resolver, ok := s.cfg.Git.(interface {
			Resolve(context.Context, GitSource, string) (string, error)
		})
		if !ok {
			return tfmodule.Bundle{}, moduleregistry.ErrUnavailable
		}
		var err error
		commit, err = resolver.Resolve(ctx, source, request.Ref)
		if err != nil {
			return tfmodule.Bundle{}, err
		}
	}
	if !tfmodule.FullCommit(commit) || s.cfg.Git == nil {
		return tfmodule.Bundle{}, moduleregistry.ErrUnavailable
	}
	snapshot, err := s.cfg.Git.Open(ctx, source, commit)
	if err != nil {
		return tfmodule.Bundle{}, err
	}
	ref := tfmodule.SourceRef{SourceID: source.ID, ModuleID: gitModuleID(request.ResourceType, request.Path), Revision: commit, Subdirectory: request.Path}
	sources := s.resolver(ctx, ref, snapshot)
	sources.direct = map[string]GitSource{source.ID: source}
	defer sources.close()
	tree, err := sources.tree(rootDir(ref))
	if err != nil {
		return tfmodule.Bundle{}, err
	}
	ref.ContentSHA256 = tree.Digest()
	// Content identity, not a Git path or submitted name, carries trusted
	// release semantics (EKS executor outputs and namespace lifecycle hooks).
	var executorOutputs []string
	for _, module := range s.cfg.Release.Packaged.Modules {
		if module.ContentSHA256 == ref.ContentSHA256 && slices.Contains(module.ResourceTypes, request.ResourceType) {
			ref.ModuleID = module.ID
			executorOutputs = module.ExecutorOutputs
			break
		}
	}
	bundle, trees, err := s.build(sources, ref, tfmodule.SourceKindGit, []string{request.ResourceType}, executorOutputs, "")
	if err != nil {
		return tfmodule.Bundle{}, err
	}
	bundle.Contract.SourceURL = request.URL
	// Treat a previously recorded pin as immutable; cache drift is not repaired
	// by another inspection. Cache.store and InsertModuleBundle compare content.
	if err := s.cfg.Cache.store(ref, bundle.Contract, trees); err != nil {
		return tfmodule.Bundle{}, err
	}
	stored, err := s.cfg.Store.InsertModuleBundle(ctx, bundle)
	if err != nil {
		return tfmodule.Bundle{}, err
	}
	if !stored.SameVerifiedContent(bundle) {
		return tfmodule.Bundle{}, moduleregistry.ErrDrift
	}
	return stored, nil
}

// remoteRepository extracts only a credential-free HTTPS origin. The existing
// remote parser then enforces full commit and clean module directory syntax.
func remoteRepository(source string) (string, error) {
	rest, ok := strings.CutPrefix(source, "git::")
	if !ok {
		return "", contractError("remote child modules require pinned HTTPS Git sources")
	}
	u, err := url.Parse(rest)
	if err != nil || u.User != nil || u.Fragment != "" {
		return "", contractError("invalid remote module source")
	}
	if i := strings.Index(u.Path, "//"); i >= 0 {
		u.Path = u.Path[:i]
		u.RawPath = ""
	}
	u.RawQuery = ""
	repo := u.String()
	if validateGitURL(repo) != nil {
		return "", contractError("remote child modules require HTTPS")
	}
	return repo, nil
}

var _ moduleregistry.GitInspector = (*Service)(nil)
