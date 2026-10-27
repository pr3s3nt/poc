package registry

import (
	"context"
	"crypto/sha256"
	"fmt"
	"slices"

	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/platform/sourcezip"
	"orchestrator/internal/ports/moduleregistry"
)

// ZIPSnapshot retains the repository layout for local child module resolution.
type ZIPSnapshot struct{ Files Tree }

func (s ZIPSnapshot) Close() {}
func (s ZIPSnapshot) Tree(dir string) (Tree, error) {
	if !tfmodule.ValidSubdirectory(dir) {
		return nil, moduleregistry.ErrNotAllowed
	}
	selected := s.Files.Sub(dir)
	if len(selected) == 0 {
		return nil, moduleregistry.ErrRejected
	}
	out := Tree{}
	total := 0
	for name, data := range selected {
		if err := out.add(name, data, &total); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Service) InspectZIP(ctx context.Context, request moduleregistry.ZIPInspection) (tfmodule.Bundle, error) {
	if request.OrganizationKey == "" || !tfmodule.ValidIdentifier(request.ResourceType) {
		return tfmodule.Bundle{}, moduleregistry.ErrNotAllowed
	}
	if request.Path == "" {
		request.Path = "."
	}
	if !tfmodule.ValidSubdirectory(request.Path) {
		return tfmodule.Bundle{}, moduleregistry.ErrNotAllowed
	}
	files, err := sourcezip.Read(ctx, request.Data)
	if err != nil {
		return tfmodule.Bundle{}, moduleregistry.ErrRejected
	}
	snapshot := ZIPSnapshot{Files: Tree(files)}
	revision := snapshot.Files.Digest()
	ref := tfmodule.SourceRef{SourceID: zipSourceID(request.OrganizationKey, revision), ModuleID: gitModuleID(request.ResourceType, request.Path), Revision: revision, Subdirectory: request.Path}
	sources := s.resolver(ctx, ref, snapshot)
	// Remote children retain full-commit HTTPS validation, fetched only at inspect.
	sources.direct = map[string]GitSource{}
	defer sources.close()
	tree, err := sources.tree(rootDir(ref))
	if err != nil {
		return tfmodule.Bundle{}, err
	}
	ref.ContentSHA256 = tree.Digest()
	var executorOutputs []string
	for _, module := range s.cfg.Release.Packaged.Modules {
		if module.ContentSHA256 == ref.ContentSHA256 && slices.Contains(module.ResourceTypes, request.ResourceType) {
			ref.ModuleID = module.ID
			executorOutputs = module.ExecutorOutputs
			break
		}
	}
	bundle, trees, err := s.build(sources, ref, tfmodule.SourceKindZIP, []string{request.ResourceType}, executorOutputs, request.OrganizationKey)
	if err != nil {
		return tfmodule.Bundle{}, err
	}
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

func zipSourceID(org, revision string) string {
	return fmt.Sprintf("zip-%x", sha256.Sum256([]byte(org+"\x00"+revision)))[:60]
}
