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
	var def resource.Definition
	if !decodeStrict(w, r, managementBodyLimit, &def) {
		return
	}
	created, err := s.catalog.RegisterResourceDefinition(r.Context(), org, def)
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) handleRenderBundles(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.platformActor(w, r); !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"renderBundles": s.catalog.ListRenderBundles()})
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
func decodePreviewBody(w http.ResponseWriter, r *http.Request, dst *catalog.MatchingPreviewQuery) bool {
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
