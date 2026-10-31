package catalogrelease

import (
	"context"
	"errors"
	domain "orchestrator/internal/domain/catalogrelease"
	"orchestrator/internal/domain/renderbundle"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/ports/persistence"
)

// ResolveDefinitions reads only verified snapshots from the current store view.
// Legacy absence preserves existing matching. Pinned scopes have no fallback.
func ResolveDefinitions(ctx context.Context, st persistence.Store, org, app, env string, legacy []resource.Definition) ([]resource.Definition, string, error) {
	binding, err := st.GetCatalogEnvironment(ctx, app, env)
	if err != nil {
		return nil, "", err
	}
	if binding.Desired == nil {
		return legacy, "", nil
	}
	r, err := st.GetCatalogRelease(ctx, org, binding.Desired.SourceID, binding.Desired.Tag)
	if err != nil {
		return nil, "", err
	}
	defs, err := r.Definitions(*binding.Desired)
	return defs, binding.Desired.Digest(), err
}
func (s *Service) Select(ctx context.Context, org, app, env, sourceID, tag string, expectedEnv, expectedRevision int64) (persistence.CatalogEnvironment, error) {
	var out persistence.CatalogEnvironment
	err := s.Store.Transact(ctx, func(ctx context.Context) error {
		a, err := s.Store.GetApplication(ctx, app)
		if err != nil {
			return err
		}
		if a.OrganizationKey != org {
			return persistence.ErrNotFound
		}
		source, err := s.Store.GetCatalogSource(ctx, org, sourceID)
		if err != nil {
			return err
		}
		release, err := s.Store.GetCatalogRelease(ctx, org, sourceID, tag)
		if err != nil {
			return err
		}
		// Binding values are platform-managed, never accepted from the Developer.
		selection := domain.Selection{SourceID: sourceID, Tag: tag, Fingerprint: release.Fingerprint(), Accounts: source.Source.AccountBindings}
		if _, err = release.Definitions(selection); err != nil {
			return err
		}
		out, err = s.Store.SelectCatalog(ctx, persistence.CatalogSelectionCommand{Organization: org, ApplicationKey: app, EnvironmentKey: env, ExpectedEnvironmentVersion: expectedEnv, ExpectedRevision: expectedRevision, Selection: selection})
		return err
	})
	return out, err
}

// RenderBundles filters authoring/template discovery to the selected release.
func RenderBundles(ctx context.Context, st persistence.Store, org, app, env string, all map[string]renderbundle.Bundle) (map[string]renderbundle.Bundle, error) {
	binding, err := st.GetCatalogEnvironment(ctx, app, env)
	if errors.Is(err, persistence.ErrNotFound) {
		return all, nil
	}
	if err != nil {
		return nil, err
	}
	if binding.Desired == nil {
		return all, nil
	}
	r, err := st.GetCatalogRelease(ctx, org, binding.Desired.SourceID, binding.Desired.Tag)
	if err != nil {
		return nil, err
	}
	if r.Fingerprint() != binding.Desired.Fingerprint {
		return nil, domain.ErrIncomplete
	}
	out := map[string]renderbundle.Bundle{}
	for _, component := range r.Components {
		if component.Kind == domain.RenderBundle {
			b, ok := all[component.BundleID]
			if !ok || b.Source.ContentSHA256 != component.SHA256 {
				return nil, domain.ErrIncomplete
			}
			out[component.BundleID] = b
		}
	}
	return out, nil
}
