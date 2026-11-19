package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/ports/moduleregistry"
)

// DocumentGitSource is authoring provenance, never an execution pin.
type DocumentGitSource struct {
	URL      string `json:"url"`
	Path     string `json:"path"`
	Revision string `json:"revision"`
}

type PreparedDefinitionDocument struct {
	Definition DefinitionDraft    `json:"definition"`
	Source     *DocumentGitSource `json:"source,omitempty"`
}

func definitionDraft(def resource.Definition) DefinitionDraft {
	return DefinitionDraft{Key: def.Key, Name: def.Name, ResourceTypeKey: def.ResourceTypeKey,
		DriverType: def.DriverType, DriverAccount: def.DriverAccount, DriverInputs: def.DriverInputs,
		Provision: def.Provision, Criteria: def.Criteria}
}

// PrepareDefinitionDocument shares form inspection and full registration
// validation. Only verified source/cache records may be prepared; no Definition
// or revision is written. The returned pin is immutable even if a branch moves.
func (s *Service) PrepareDefinitionDocument(ctx context.Context, org string, draft DefinitionDraft) (PreparedDefinitionDocument, error) {
	// Work on a detached draft; replacing source must not modify the editor or
	// any caller-owned maps, including on a subsequent validation error.
	data, err := json.Marshal(draft)
	if err != nil {
		return PreparedDefinitionDocument{}, invalidField(sourcePath, ErrInvalid)
	}
	var copy DefinitionDraft
	if err := json.Unmarshal(data, &copy); err != nil {
		return PreparedDefinitionDocument{}, invalidField(sourcePath, ErrInvalid)
	}
	def := copy.Definition()
	def.Version = 1
	source := def.Source()
	_, urlPresent := source["url"]
	_, refPresent := source["ref"]
	_, pathPresent := source["path"]
	if urlPresent || refPresent || pathPresent {
		if def.DriverType != resource.DriverTerraform || len(source) != 3 || !urlPresent || !refPresent || !pathPresent || def.DriverInputs["catalog"] != nil {
			return PreparedDefinitionDocument{}, invalidField(sourcePath, fmt.Errorf("%w: Git source requires only url, ref and path", ErrInvalid))
		}
		url, urlOK := source["url"].(string)
		ref, refOK := source["ref"].(string)
		path, pathOK := source["path"].(string)
		if !urlOK || !refOK || !pathOK || strings.TrimSpace(url) == "" || strings.TrimSpace(ref) == "" || strings.TrimSpace(path) == "" {
			return PreparedDefinitionDocument{}, invalidField(sourcePath, fmt.Errorf("%w: Git source fields must be nonempty strings", ErrInvalid))
		}
		capability, err := s.InspectTerraformModule(ctx, org, moduleregistry.GitInspection{ResourceType: def.ResourceTypeKey, URL: url, Ref: ref, Path: path})
		if err != nil {
			return PreparedDefinitionDocument{}, err
		}
		pin := capability.Source
		if pin == nil {
			return PreparedDefinitionDocument{}, invalidField(sourcePath, moduleregistry.ErrNotVerified)
		}
		def.DriverValues()["source"] = map[string]any{"source_id": pin.SourceID, "module_id": pin.ModuleID,
			"revision": pin.Revision, "subdirectory": pin.Subdirectory, "content_sha256": pin.ContentSHA256}
	}
	prepared, bundle, err := s.prepareDefinition(ctx, org, def)
	if err != nil {
		return PreparedDefinitionDocument{}, err
	}
	result := PreparedDefinitionDocument{Definition: definitionDraft(prepared)}
	if bundle != nil && bundle.Contract.SourceKind == tfmodule.SourceKindGit {
		result.Source = &DocumentGitSource{URL: bundle.Contract.SourceURL, Path: bundle.Ref.Subdirectory, Revision: bundle.Ref.Revision}
	}
	return result, nil
}

// ExportDefinitionDocument projects one scoped saved revision using metadata
// reads only. Portable Git origin comes from the verified bundle, not IDs or a
// mutable reference resolved at export time.
func (s *Service) ExportDefinitionDocument(ctx context.Context, org, key, revision string, version int64) (DefinitionDraft, error) {
	defs, err := s.store.ListResourceDefinitions(ctx, org)
	if err != nil {
		return DefinitionDraft{}, err
	}
	var def resource.Definition
	foundKey := false
	for _, candidate := range defs {
		if candidate.Key == key {
			def, foundKey = candidate, true
			break
		}
	}
	if !foundKey {
		return DefinitionDraft{}, ErrNotFound
	}
	if revision != "" || version > 0 {
		history, err := s.store.GetDefinitionHistory(ctx, org, key)
		if err != nil {
			return DefinitionDraft{}, err
		}
		found := false
		for _, v := range history.Versions {
			if (revision != "" && v.ID == revision) || (revision == "" && v.Definition.Version == version) {
				def, found = v.Definition, true
				break
			}
		}
		if !found && !(revision == "" && len(history.Versions) == 0 && def.Version == version) {
			return DefinitionDraft{}, ErrNotFound
		}
	}
	data, err := json.Marshal(definitionDraft(def))
	if err != nil {
		return DefinitionDraft{}, err
	}
	var doc DefinitionDraft
	if err := json.Unmarshal(data, &doc); err != nil {
		return DefinitionDraft{}, err
	}
	if def.DriverType != resource.DriverTerraform {
		return doc, nil
	}
	ref, err := tfmodule.ParseSourceRef(def.Source())
	if err != nil {
		return DefinitionDraft{}, invalidField(sourcePath, err)
	}
	if s.modules == nil {
		return DefinitionDraft{}, moduleregistry.ErrUnavailable
	}
	bundle, err := s.modules.Bundle(ctx, ref)
	if err != nil {
		return DefinitionDraft{}, registryError(err)
	}
	if bundle.Contract.SourceKind == tfmodule.SourceKindGit {
		if bundle.Contract.SourceURL == "" {
			return DefinitionDraft{}, registryError(moduleregistry.ErrNotVerified)
		}
		doc.Definition().DriverValues()["source"] = map[string]any{"url": bundle.Contract.SourceURL, "ref": ref.Revision, "path": ref.Subdirectory}
		// Catalog provenance is installation-specific; the exported exact Git
		// module is inspected directly on the destination installation.
		delete(doc.DriverInputs, "catalog")
	}
	return doc, nil
}
