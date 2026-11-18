package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/renderregistry"
)

// SetupDocument is a strict review draft. Template is authoring provenance for
// a template Definition, never a stored template_source ID from another install.
type SetupDocument struct {
	File       string                     `json:"file"`
	Type       *resource.Type             `json:"type,omitempty"`
	Definition *DefinitionDraft           `json:"definition,omitempty"`
	Template   *renderregistry.Inspection `json:"template,omitempty"`
}
type SetupReview struct {
	SetupDocument
	Status string   `json:"status"`
	Fields []string `json:"fields,omitempty"`
}
type setupStore struct {
	persistence.Store
	types []resource.Type
}

type setupTemplateStore struct {
	*setupStore
	persistence.TemplateSourceRepository
}

func (o setupStore) ListResourceTypes(context.Context, string) ([]resource.Type, error) {
	return o.types, nil
}

// PreviewSetup validates the selected documents against an overlay of prospective
// Types. Only verified source metadata/cache may be written, never business data.
func (s *Service) PreviewSetup(ctx context.Context, org string, docs []SetupDocument) ([]SetupReview, error) {
	types, err := s.store.ListResourceTypes(ctx, org)
	if err != nil {
		return nil, err
	}
	repo, _ := s.store.(persistence.TemplateSourceRepository)
	copyService := *s
	overlay := &setupStore{Store: s.store, types: append([]resource.Type{}, types...)}
	copyService.store = overlay
	if repo != nil {
		copyService.store = &setupTemplateStore{setupStore: overlay, TemplateSourceRepository: repo}
	}
	out := make([]SetupReview, len(docs))
	seen := map[string]bool{}
	for i, d := range docs {
		out[i] = SetupReview{SetupDocument: d, Status: "NEW"}
		if (d.Type == nil) == (d.Definition == nil) {
			out[i].Status = "INVALID"
			continue
		}
		kind, key := "definition", ""
		if d.Type != nil {
			kind, key = "type", d.Type.Key
		} else {
			key = d.Definition.Key
		}
		if seen[kind+":"+key] {
			out[i].Status = "INVALID"
			continue
		}
		seen[kind+":"+key] = true
		if d.Type == nil {
			continue
		}
		if validatePublicID("resource type", key) != nil || reservedTypeKeys[key] || d.Type.Validate() != nil || d.Template != nil {
			out[i].Status = "INVALID"
			continue
		}
		found := false
		for _, existing := range types {
			if existing.Key == key {
				comparison := importComparison(existing, *d.Type)
				out[i].Status, out[i].Fields = comparison.Status, comparison.Fields
				found = true
				break
			}
		}
		if !found {
			overlay.types = append(overlay.types, *d.Type)
		}
	}
	defs, err := s.store.ListResourceDefinitions(ctx, org)
	if err != nil {
		return nil, err
	}
	for i, d := range docs {
		if d.Definition == nil || out[i].Status == "INVALID" {
			continue
		}
		draft := *d.Definition
		if d.Template != nil {
			if draft.DriverType != resource.DriverTemplate {
				out[i].Status = "INVALID"
				continue
			}
			source, err := s.InspectTemplateSource(ctx, org, *d.Template)
			if err != nil {
				out[i].Status = "INVALID"
				out[i].Fields = []string{templateSourcePath}
				continue
			}
			// Detach nested maps before substituting the installation-local source ID.
			draft = draftCopy(draft)
			values := draft.Definition().DriverValues()
			variables, _ := values["variables"].(map[string]any)
			if variables == nil {
				variables = map[string]any{}
				values["variables"] = variables
			}
			variables[resource.VariableTemplateSource] = source.ID
		}
		prepared, err := copyService.PrepareDefinitionDocument(ctx, org, draft)
		if err != nil {
			out[i].Status = "INVALID"
			var field *FieldError
			var render *RenderError
			if errors.As(err, &field) {
				out[i].Fields = []string{field.Field}
			} else if errors.As(err, &render) {
				out[i].Fields = []string{render.Field}
			}
			continue
		}
		out[i].Definition = &prepared.Definition
		for _, existing := range defs {
			if existing.Key != draft.Key {
				continue
			}
			left, err := copyService.normalizedDefinitionDocument(ctx, org, existing, nil)
			if err != nil {
				return nil, err
			}
			prospective := prepared.Definition.Definition()
			prospective.Version = 1
			right, err := copyService.normalizedDefinitionDocument(ctx, org, prospective, nil)
			if err != nil {
				return nil, err
			}
			comparison := importComparison(left, right)
			out[i].Status, out[i].Fields = comparison.Status, comparison.Fields
		}
	}
	return out, nil
}

func draftCopy(d DefinitionDraft) DefinitionDraft {
	data, _ := json.Marshal(d)
	var copy DefinitionDraft
	_ = json.Unmarshal(data, &copy)
	return copy
}
