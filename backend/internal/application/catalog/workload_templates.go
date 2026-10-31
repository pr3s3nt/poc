package catalog

import (
	"context"
	catalogapp "orchestrator/internal/application/catalogrelease"
	"orchestrator/internal/domain/renderbundle"
	"orchestrator/internal/ports/persistence"
)

type WorkloadTemplateView struct {
	renderbundle.WorkloadTemplate
	SHA256 string `json:"sha256"`
}

// ListWorkloadTemplates reads only the authenticated Organization's immutable
// verified metadata. It never fetches source or accesses a provider.
func (s *Service) ListWorkloadTemplates(ctx context.Context, org string) ([]WorkloadTemplateView, error) {
	records, err := persistence.RenderBundlesOf(ctx, s.store, org)
	if err != nil {
		return nil, err
	}
	templates, err := renderbundle.TemplateCatalog(records)
	if err != nil {
		return nil, ErrInvalid
	}
	out := make([]WorkloadTemplateView, 0, len(templates))
	for _, t := range templates {
		out = append(out, WorkloadTemplateView{WorkloadTemplate: t, SHA256: t.SHA256()})
	}
	return out, nil
}

func (s *Service) ListEnvironmentWorkloadTemplates(ctx context.Context, org, app, env string) ([]WorkloadTemplateView, error) {
	records, err := persistence.RenderBundlesOf(ctx, s.store, org)
	if err != nil {
		return nil, err
	}
	records, err = catalogapp.RenderBundles(ctx, s.store, org, app, env, records)
	if err != nil {
		return nil, err
	}
	templates, err := renderbundle.TemplateCatalog(records)
	if err != nil {
		return nil, ErrInvalid
	}
	out := make([]WorkloadTemplateView, 0, len(templates))
	for _, t := range templates {
		out = append(out, WorkloadTemplateView{WorkloadTemplate: t, SHA256: t.SHA256()})
	}
	return out, nil
}
