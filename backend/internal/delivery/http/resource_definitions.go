package http

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"orchestrator/internal/application/catalog"
	"orchestrator/internal/domain/resource"
)

func (s *Server) handleResourceDefinitions(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	definitions, err := s.store.ListResourceDefinitions(r.Context(), org)
	if err != nil {
		s.writeError(w, err)
		return
	}
	if definitions == nil {
		definitions = []resource.Definition{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"resourceDefinitions": definitions})
}

func (s *Server) handleRegisterResourceDefinition(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	// The strict authoring DTO has no version, fingerprint, executionProfile
	// or connectionKey: unknown and retired fields are rejected.
	var draft catalog.DefinitionDraft
	if !decodeStrict(w, r, managementBodyLimit, &draft) {
		return
	}
	created, err := s.catalog.RegisterResourceDefinition(r.Context(), org, draft.Definition())
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

// handleUpdateResourceDefinition is the UC-03 V02-06 edit: the same strict
// authoring fields plus the expected version, as one compare-and-set.
func (s *Server) handleUpdateResourceDefinition(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	var update catalog.DefinitionUpdate
	if !decodeStrict(w, r, managementBodyLimit, &update) {
		return
	}
	updated, err := s.catalog.UpdateResourceDefinition(r.Context(), org, r.PathValue("key"), update.Version, update.Definition())
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleRenderBundles(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.platformActor(w, r); !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"renderBundles": s.catalog.ListRenderBundles()})
}

func (s *Server) handleDefinitionCapabilities(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	capabilities, err := s.catalog.ListDefinitionCapabilities(r.Context(), org)
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"capabilities": capabilities})
}

func (s *Server) handleMatchingPreview(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	var query catalog.MatchingPreviewQuery
	if !decodePreviewBody(w, r, &query) {
		return
	}
	result, err := s.catalog.PreviewMatching(r.Context(), org, query)
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// decodePreviewBody is strict like decodeStrict but never echoes decoder
// text: an unknown field name is caller data and could be a secret.
func decodePreviewBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, managementBodyLimit))
	if err != nil {
		if !writeTooLarge(w, err) {
			writeCode(w, codeInvalidRequest, "")
		}
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if trimmed := bytes.TrimSpace(raw); len(trimmed) == 0 || trimmed[0] != '{' || decoder.Decode(dst) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		writeCode(w, codeInvalidRequest, "")
		return false
	}
	return true
}
