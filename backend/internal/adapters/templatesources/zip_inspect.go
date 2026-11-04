package templatesources

import (
	"context"

	"orchestrator/internal/adapters/terraform/registry"
	"orchestrator/internal/domain/templatesource"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/platform/sourcezip"
	"orchestrator/internal/ports/renderregistry"
)

func (r *Registry) InspectZIP(ctx context.Context, org string, req renderregistry.ZIPInspection) (templatesource.Bundle, error) {
	if req.Path == "" {
		req.Path = "."
	}
	if !templatesource.ValidDirectory(req.Path) || !tfmodule.ValidSubdirectory(req.Path) {
		return templatesource.Bundle{}, renderregistry.ErrSourceInvalid
	}
	if r.Runtime == nil || r.Runtime.BinaryDigest() == "" || r.Runtime.CheckBinary() != nil || r.Cache == nil || r.Store == nil {
		return templatesource.Bundle{}, renderregistry.ErrBundleUnavailable
	}
	files, err := sourcezip.Read(ctx, req.Data)
	if err != nil {
		return templatesource.Bundle{}, renderregistry.ErrBundleInvalid
	}
	snapshot := registry.ZIPSnapshot{Files: registry.Tree(files)}
	tree, err := snapshot.Tree(req.Path)
	if err != nil {
		return templatesource.Bundle{}, renderregistry.ErrBundleInvalid
	}
	revision := snapshot.Files.Digest()
	return r.prepare(ctx, org, templatesource.Source{Kind: templatesource.SourceKindZIP, URL: "zip:sha256:" + revision, Path: req.Path, Revision: revision}, tree)
}
