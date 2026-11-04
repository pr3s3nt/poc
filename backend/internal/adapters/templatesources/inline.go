package templatesources

import (
	"context"
	"orchestrator/internal/adapters/terraform/registry"
	"orchestrator/internal/domain/templatesource"
	"orchestrator/internal/ports/renderregistry"
	standardtemplates "orchestrator/template-sources"
)

func (r *Registry) InspectInline(ctx context.Context, org string, req renderregistry.InlineInspection) (templatesource.Bundle, error) {
	if r.Runtime == nil || r.Cache == nil || r.Store == nil {
		return templatesource.Bundle{}, renderregistry.ErrBundleUnavailable
	}
	tree := registry.Tree{}
	for path, content := range req.Files {
		tree[path] = []byte(content)
	}
	digest := tree.Digest()
	return r.prepare(ctx, org, templatesource.Source{Kind: "inline", URL: "inline:sha256:" + digest, Path: ".", Revision: digest}, tree)
}

// PrepareStandard registers content, not an installed renderer compatibility path.
func (r *Registry) PrepareStandard(ctx context.Context, org string) (templatesource.Bundle, error) {
	tree := registry.Tree{}
	for name, content := range standardtemplates.Files() {
		tree[name] = []byte(content)
	}
	digest := tree.Digest()
	return r.prepareWithID(ctx, org, templatesource.Source{Kind: "inline", URL: "inline:sha256:" + digest, Path: ".", Revision: digest}, tree, standardtemplates.ID(org))
}
