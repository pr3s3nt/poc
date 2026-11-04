package catalog

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/templatesource"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/renderregistry"
)

// Field paths of render bundle validation errors.
const (
	templateSourcePath = "definition.driverInputs.values.variables.template_source"
	renderInputsPath   = "definition.driverInputs.values.variables.template_inputs"
)

// RenderError carries a stable render registry/input class through Definition
// validation and inspection; delivery maps it to its public code.
type RenderError struct {
	Field string
	err   error
}

func (e *RenderError) Error() string { return e.err.Error() }
func (e *RenderError) Unwrap() error { return e.err }

func renderError(field string, err error) error { return &RenderError{Field: field, err: err} }

// SetRenderRegistry enables Git render bundle inspection and availability.
func (s *Service) SetRenderRegistry(registry renderregistry.Registry) { s.renderRegistry = registry }

// RenderSourceView is the public Git source pin.
type RenderSourceView struct {
	Kind          string `json:"kind"`
	URL           string `json:"url"`
	Path          string `json:"path"`
	Revision      string `json:"revision"`
	ContentSHA256 string `json:"contentSha256"`
}

// RendererView names the renderer of a Git bundle.
type RendererView struct {
	Type    string `json:"type"`
	Version string `json:"version"`
}

// TemplateSourceView is the safe catalog projection (contract RB-C07). An
// installed bundle has only id, version and status; a Git bundle adds its
// name, source pin, contracts, renderer and inputs schema. No binary digest,
// cache path or source body is exposed.
type TemplateSourceView struct {
	ID             string            `json:"id"`
	Name           string            `json:"name,omitempty"`
	Version        string            `json:"version"`
	Status         string            `json:"status"`
	Source         *RenderSourceView `json:"source,omitempty"`
	InputContract  string            `json:"inputContract,omitempty"`
	EngineContract string            `json:"engineContract,omitempty"`
	Renderer       *RendererView     `json:"renderer,omitempty"`
	InputsSchema   map[string]any    `json:"inputsSchema,omitempty"`
}

func (s *Service) gitView(ctx context.Context, b templatesource.Bundle) TemplateSourceView {
	status := "AVAILABLE"
	if s.renderRegistry == nil || s.renderRegistry.Available(ctx, b) != nil {
		// The record stays immutable and verified; only the runtime cannot serve it.
		status = "UNAVAILABLE"
	}
	schema, _ := templatesource.DeepCopy(b.Contract.InputsSchema).(map[string]any)
	return TemplateSourceView{
		ID: b.ID, Name: b.Name(), Version: b.ShortRevision(), Status: status,
		Source:        &RenderSourceView{Kind: b.Source.Kind, URL: b.Source.URL, Path: b.Source.Path, Revision: b.Source.Revision, ContentSHA256: b.Source.ContentSHA256},
		InputContract: b.Contract.InputContract, EngineContract: b.EngineContract,
		Renderer:     &RendererView{Type: templatesource.RendererType, Version: b.RendererVersion},
		InputsSchema: schema,
	}
}

func (s *Service) renderRepository() (persistence.TemplateSourceRepository, bool) {
	repo, ok := s.store.(persistence.TemplateSourceRepository)
	return repo, ok
}

// InspectTemplateSource inspects one Git source for the Organization (RB-C07).
// It records the verified pin only; no Definition, provider or cluster changes.
func (s *Service) InspectTemplateSource(ctx context.Context, organizationKey string, req renderregistry.Inspection) (TemplateSourceView, error) {
	if s.renderRegistry == nil {
		return TemplateSourceView{}, renderError("", renderregistry.ErrBundleUnavailable)
	}
	if _, err := s.store.GetOrganization(ctx, organizationKey); err != nil {
		return TemplateSourceView{}, err
	}
	record, err := s.renderRegistry.Inspect(ctx, organizationKey, req)
	if err != nil {
		if isRenderClass(err) {
			return TemplateSourceView{}, renderError("", err)
		}
		return TemplateSourceView{}, err
	}
	return s.gitView(ctx, record), nil
}

func isRenderClass(err error) bool {
	for _, class := range []error{renderregistry.ErrSourceInvalid, renderregistry.ErrBundleInvalid, renderregistry.ErrContractUnsupported,
		renderregistry.ErrSourceUnavailable, renderregistry.ErrBundleUnavailable, renderregistry.ErrDrift, renderregistry.ErrRenderFailed} {
		if errors.Is(err, class) {
			return true
		}
	}
	return false
}

// GetTemplateSource returns one same-Organization Git bundle; a foreign or
// missing ID is not found.
func (s *Service) GetTemplateSource(ctx context.Context, organizationKey, id string) (TemplateSourceView, error) {
	repo, ok := s.renderRepository()
	if !ok || !templatesource.ValidID(id) {
		return TemplateSourceView{}, fmt.Errorf("%w: render bundle", persistence.ErrNotFound)
	}
	record, err := repo.GetTemplateSource(ctx, organizationKey, id)
	if err != nil {
		return TemplateSourceView{}, err
	}
	return s.gitView(ctx, record), nil
}

// ListTemplateSources returns the available installed bundles sorted by ID,
// then the Organization's verified Git bundles in registration order.
func (s *Service) ListTemplateSources(ctx context.Context, organizationKey string) ([]TemplateSourceView, error) {
	out := []TemplateSourceView{}
	installed := []TemplateSourceView{}
	for id, b := range s.bundles {
		if resource.TemplateSourceAvailable(s.bundles, id) {
			installed = append(installed, TemplateSourceView{ID: b.ID, Version: b.Version, Status: "AVAILABLE"})
		}
	}
	sort.Slice(installed, func(i, j int) bool { return installed[i].ID < installed[j].ID })
	out = append(out, installed...)
	git, err := persistence.TemplateSourcesOf(ctx, s.store, organizationKey)
	if err != nil {
		return nil, err
	}
	records := make([]templatesource.Bundle, 0, len(git))
	for _, b := range git {
		records = append(records, b)
	}
	sort.Slice(records, func(i, j int) bool {
		if !records[i].CreatedAt.Equal(records[j].CreatedAt) {
			return records[i].CreatedAt.Before(records[j].CreatedAt)
		}
		return records[i].ID < records[j].ID
	})
	for _, b := range records {
		out = append(out, s.gitView(ctx, b))
	}
	return out, nil
}

// gitTemplateSources returns the Organization's record a workload Definition
// selects, if template_source names one, keyed by ID.
func (s *Service) gitTemplateSources(ctx context.Context, organizationKey string, def resource.Definition) (map[string]templatesource.Bundle, error) {
	id, _ := def.Variables()[resource.VariableTemplateSource].(string)
	out := map[string]templatesource.Bundle{}
	if !templatesource.ValidID(id) {
		return out, nil
	}
	repo, ok := s.renderRepository()
	if !ok {
		return nil, invalidField(templateSourcePath, fmt.Errorf("%w: Git render bundles are not available", ErrInvalid))
	}
	record, err := repo.GetTemplateSource(ctx, organizationKey, id)
	if errors.Is(err, persistence.ErrNotFound) {
		return nil, invalidField(templateSourcePath, fmt.Errorf("%w: template_source is not a verified bundle of this Organization", ErrInvalid))
	}
	if err != nil {
		return nil, &internalError{err: err}
	}
	out[id] = record
	return out, nil
}

// validateRenderSelection is the template branch of validateDriver: legacy
// installed ID or same-Organization verified Git pin with typed inputs.
func (s *Service) validateRenderSelection(ctx context.Context, organizationKey string, def *resource.Definition, typ resource.Type) error {
	git, err := s.gitTemplateSources(ctx, organizationKey, *def)
	if err != nil {
		return err
	}
	pin, _, err := resource.ResolveRenderDefinition(*def, typ, s.bundles, git, true)
	if err != nil {
		var selection *resource.RenderSelectionError
		if errors.As(err, &selection) {
			if errors.Is(err, resource.ErrRenderInputsInvalid) {
				return renderError(renderInputsPath, templatesource.ErrInputInvalid)
			}
			return invalidField(templateSourcePath, fmt.Errorf("%w: %v", ErrInvalid, err))
		}
		return err
	}
	if pin.Git() {
		record := git[pin.ID]
		if s.renderRegistry == nil {
			return renderError(templateSourcePath, renderregistry.ErrBundleUnavailable)
		}
		if err := s.renderRegistry.Available(ctx, record); err != nil {
			return renderError(templateSourcePath, renderregistry.ErrBundleUnavailable)
		}
	}
	def.SourceFingerpr = pin.Digest
	return nil
}

// verifyRenderAssociation re-reads a selected Git record inside the write
// transaction, so the association is checked in the same scope and snapshot.
func (s *Service) verifyRenderAssociation(ctx context.Context, organizationKey string, def resource.Definition) error {
	if def.DriverType != resource.DriverTemplate {
		return nil
	}
	id, _ := def.Variables()[resource.VariableTemplateSource].(string)
	if !templatesource.ValidID(id) {
		return nil
	}
	repo, ok := s.renderRepository()
	if !ok {
		return invalidField(templateSourcePath, fmt.Errorf("%w: Git render bundles are not available", ErrInvalid))
	}
	if _, err := repo.GetTemplateSource(ctx, organizationKey, id); errors.Is(err, persistence.ErrNotFound) {
		return invalidField(templateSourcePath, fmt.Errorf("%w: template_source is not a verified bundle of this Organization", ErrInvalid))
	} else if err != nil {
		return err
	}
	return nil
}

// UploadTemplateSource validates uploaded content for this Organization.
func (s *Service) UploadTemplateSource(ctx context.Context, org string, req renderregistry.ZIPInspection) (TemplateSourceView, error) {
	inspector, ok := s.renderRegistry.(renderregistry.ZIPInspector)
	if !ok {
		return TemplateSourceView{}, renderError("", renderregistry.ErrBundleUnavailable)
	}
	if _, err := s.store.GetOrganization(ctx, org); err != nil {
		return TemplateSourceView{}, err
	}
	record, err := inspector.InspectZIP(ctx, org, req)
	if err != nil {
		if isRenderClass(err) {
			return TemplateSourceView{}, renderError("", err)
		}
		return TemplateSourceView{}, err
	}
	return s.gitView(ctx, record), nil
}

func (s *Service) PrepareInlineTemplateSource(ctx context.Context, org string, req renderregistry.InlineInspection) (TemplateSourceView, error) {
	inspector, ok := s.renderRegistry.(renderregistry.InlineInspector)
	if !ok {
		return TemplateSourceView{}, renderError("", renderregistry.ErrBundleUnavailable)
	}
	if _, err := s.store.GetOrganization(ctx, org); err != nil {
		return TemplateSourceView{}, err
	}
	record, err := inspector.InspectInline(ctx, org, req)
	if err != nil {
		return TemplateSourceView{}, renderError("", err)
	}
	return s.gitView(ctx, record), nil
}
