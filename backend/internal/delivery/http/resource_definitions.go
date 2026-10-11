package http

import (
	"bytes"
	"encoding/json"
	"errors"
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
		writeManagementError(w, err)
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
		writeManagementError(w, err)
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
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, result)
	case errors.Is(err, catalog.ErrPreviewNotFound):
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "application or environment not found"})
	case errors.Is(err, catalog.ErrTargetUnavailable):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "environment execution target is unavailable"})
	case errors.Is(err, catalog.ErrInvalid):
		// Fixed envelope: only the failing field path, never submitted values.
		body := map[string]any{"error": "invalid matching preview document"}
		var invalid *catalog.PreviewInvalidError
		if errors.As(err, &invalid) {
			body["field"] = invalid.Field
		}
		writeJSON(w, http.StatusBadRequest, body)
	default:
		writeManagementError(w, err)
	}
}

// decodePreviewBody is strict like decodeStrict but never echoes decoder
// text: an unknown field name is caller data and could be a secret.
func decodePreviewBody(w http.ResponseWriter, r *http.Request, dst *catalog.MatchingPreviewQuery) bool {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, managementBodyLimit))
	if err != nil {
		if !writeTooLarge(w, err) {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid matching preview document"})
		}
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if trimmed := bytes.TrimSpace(raw); len(trimmed) == 0 || trimmed[0] != '{' || decoder.Decode(dst) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid matching preview document"})
		return false
	}
	return true
}
