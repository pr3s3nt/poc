package catalog

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"orchestrator/internal/domain/renderbundle"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/renderregistry"
)

// Field paths of render bundle validation errors.
const (
	renderBundlePath = "definition.driverInputs.values.variables.render_bundle"
	renderInputsPath = "definition.driverInputs.values.variables.render_inputs"
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

// RenderBundleView is the safe catalog projection (contract RB-C07). An
// installed bundle has only id, version and status; a Git bundle adds its
// name, source pin, contracts, renderer and inputs schema. No binary digest,
// cache path or source body is exposed.
type RenderBundleView struct {
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

func (s *Service) gitView(ctx context.Context, b renderbundle.Bundle) RenderBundleView {
	status := "AVAILABLE"
	if s.renderRegistry == nil || s.renderRegistry.Available(ctx, b) != nil {
		// The record stays immutable and verified; only the runtime cannot serve it.
		status = "UNAVAILABLE"
	}
	schema, _ := renderbundle.DeepCopy(b.Contract.InputsSchema).(map[string]any)
	return RenderBundleView{
		ID: b.ID, Name: b.Name(), Version: b.ShortRevision(), Status: status,
		Source:        &RenderSourceView{Kind: b.Source.Kind, URL: b.Source.URL, Path: b.Source.Path, Revision: b.Source.Revision, ContentSHA256: b.Source.ContentSHA256},
		InputContract: b.Contract.InputContract, EngineContract: b.EngineContract,
		Renderer:     &RendererView{Type: renderbundle.RendererType, Version: b.RendererVersion},
		InputsSchema: schema,
	}
}

func (s *Service) renderRepository() (persistence.RenderBundleRepository, bool) {
	repo, ok := s.store.(persistence.RenderBundleRepository)
	return repo, ok
}

// InspectRenderBundle inspects one Git source for the Organization (RB-C07).
// It records the verified pin only; no Definition, provider or cluster changes.
func (s *Service) InspectRenderBundle(ctx context.Context, organizationKey string, req renderregistry.Inspection) (RenderBundleView, error) {
	if s.renderRegistry == nil {
		return RenderBundleView{}, renderError("", renderregistry.ErrBundleUnavailable)
	}
	if _, err := s.store.GetOrganization(ctx, organizationKey); err != nil {
		return RenderBundleView{}, err
	}
	record, err := s.renderRegistry.Inspect(ctx, organizationKey, req)
	if err != nil {
		if isRenderClass(err) {
			return RenderBundleView{}, renderError("", err)
		}
		return RenderBundleView{}, err
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

// GetRenderBundle returns one same-Organization Git bundle; a foreign or
// missing ID is not found.
func (s *Service) GetRenderBundle(ctx context.Context, organizationKey, id string) (RenderBundleView, error) {
	repo, ok := s.renderRepository()
	if !ok || !renderbundle.ValidID(id) {
		return RenderBundleView{}, fmt.Errorf("%w: render bundle", persistence.ErrNotFound)
	}
	record, err := repo.GetRenderBundle(ctx, organizationKey, id)
	if err != nil {
		return RenderBundleView{}, err
	}
	return s.gitView(ctx, record), nil
}

// ListRenderBundles returns the available installed bundles sorted by ID,
// then the Organization's verified Git bundles in registration order.
func (s *Service) ListRenderBundles(ctx context.Context, organizationKey string) ([]RenderBundleView, error) {
	out := []RenderBundleView{}
	installed := []RenderBundleView{}
	for id, b := range s.bundles {
		if resource.RenderBundleAvailable(s.bundles, id) {
			installed = append(installed, RenderBundleView{ID: b.ID, Version: b.Version, Status: "AVAILABLE"})
		}
	}
	sort.Slice(installed, func(i, j int) bool { return installed[i].ID < installed[j].ID })
	out = append(out, installed...)
	git, err := persistence.RenderBundlesOf(ctx, s.store, organizationKey)
	if err != nil {
		return nil, err
	}
	records := make([]renderbundle.Bundle, 0, len(git))
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

// gitRenderBundles returns the Organization's record a workload Definition
// selects, if render_bundle names one, keyed by ID.
func (s *Service) gitRenderBundles(ctx context.Context, organizationKey string, def resource.Definition) (map[string]renderbundle.Bundle, error) {
	id, _ := def.Variables()[resource.VariableRenderBundle].(string)
	out := map[string]renderbundle.Bundle{}
	if !renderbundle.ValidID(id) {
		return out, nil
	}
	repo, ok := s.renderRepository()
	if !ok {
		return nil, invalidField(renderBundlePath, fmt.Errorf("%w: Git render bundles are not available", ErrInvalid))
	}
	record, err := repo.GetRenderBundle(ctx, organizationKey, id)
	if errors.Is(err, persistence.ErrNotFound) {
		return nil, invalidField(renderBundlePath, fmt.Errorf("%w: render_bundle is not a verified bundle of this Organization", ErrInvalid))
	}
	if err != nil {
		return nil, &internalError{err: err}
	}
	out[id] = record
	return out, nil
}

// validateRenderSelection is the score-k8s branch of validateDriver: legacy
// installed ID or same-Organization verified Git pin with typed inputs.
func (s *Service) validateRenderSelection(ctx context.Context, organizationKey string, def *resource.Definition, typ resource.Type) error {
	git, err := s.gitRenderBundles(ctx, organizationKey, *def)
	if err != nil {
		return err
	}
	pin, _, err := resource.ResolveRenderDefinition(*def, typ, s.bundles, git, true)
	if err != nil {
		var selection *resource.RenderSelectionError
		if errors.As(err, &selection) {
			if errors.Is(err, resource.ErrRenderInputsInvalid) {
				return renderError(renderInputsPath, renderbundle.ErrInputInvalid)
			}
			return invalidField(renderBundlePath, fmt.Errorf("%w: %v", ErrInvalid, err))
		}
		return err
	}
	if pin.Git() {
		record := git[pin.ID]
		if s.renderRegistry == nil {
			return renderError(renderBundlePath, renderregistry.ErrBundleUnavailable)
		}
		if err := s.renderRegistry.Available(ctx, record); err != nil {
			return renderError(renderBundlePath, renderregistry.ErrBundleUnavailable)
		}
	}
	def.SourceFingerpr = pin.Digest
	return nil
}

// verifyRenderAssociation re-reads a selected Git record inside the write
// transaction, so the association is checked in the same scope and snapshot.
func (s *Service) verifyRenderAssociation(ctx context.Context, organizationKey string, def resource.Definition) error {
	if def.DriverType != resource.DriverScoreK8s {
		return nil
	}
	id, _ := def.Variables()[resource.VariableRenderBundle].(string)
	if !renderbundle.ValidID(id) {
		return nil
	}
	repo, ok := s.renderRepository()
	if !ok {
		return invalidField(renderBundlePath, fmt.Errorf("%w: Git render bundles are not available", ErrInvalid))
	}
	if _, err := repo.GetRenderBundle(ctx, organizationKey, id); errors.Is(err, persistence.ErrNotFound) {
		return invalidField(renderBundlePath, fmt.Errorf("%w: render_bundle is not a verified bundle of this Organization", ErrInvalid))
	} else if err != nil {
		return err
	}
	return nil
}

// UploadRenderBundle validates uploaded content for this Organization.
func (s *Service) UploadRenderBundle(ctx context.Context, org string, req renderregistry.ZIPInspection) (RenderBundleView, error) {
	inspector, ok := s.renderRegistry.(renderregistry.ZIPInspector)
	if !ok {
		return RenderBundleView{}, renderError("", renderregistry.ErrBundleUnavailable)
	}
	if _, err := s.store.GetOrganization(ctx, org); err != nil {
		return RenderBundleView{}, err
	}
	record, err := inspector.InspectZIP(ctx, org, req)
	if err != nil {
		if isRenderClass(err) {
			return RenderBundleView{}, renderError("", err)
		}
		return RenderBundleView{}, err
	}
	return s.gitView(ctx, record), nil
}
