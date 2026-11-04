package http

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"orchestrator/internal/ports/renderregistry"
)

// inspectBodyLimit bounds one render bundle inspection request (RB-C02).
const inspectBodyLimit = 64 << 10

// inspectDeadline bounds resolve, fetch, validation and caching (RB-C02).
const inspectDeadline = 120 * time.Second

// handleInspectTemplateSource is POST /template-sources/inspect (RB-C07): PE/Admin,
// Organization scoped, strict {url,path,ref}. It records a verified pin only
// and never saves a Definition.
func (s *Server) handleInspectTemplateSource(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, inspectBodyLimit))
	if err != nil {
		if !writeTooLarge(w, err) {
			writeCode(w, codeInvalidRequest, "")
		}
		return
	}
	var req renderregistry.Inspection
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if trimmed := bytes.TrimSpace(raw); len(trimmed) == 0 || trimmed[0] != '{' || decoder.Decode(&req) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		// Decoder text may echo a submitted value; it is never returned.
		writeCode(w, codeInvalidRequest, "")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), inspectDeadline)
	defer cancel()
	bundle, err := s.catalog.InspectTemplateSource(ctx, org, req)
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"templateSource": bundle})
}

// handleGetTemplateSource is GET /template-sources/{id}: one same-Organization
// verified Git record; a foreign or missing ID is 404.
func (s *Server) handleGetTemplateSource(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	bundle, err := s.catalog.GetTemplateSource(r.Context(), org, r.PathValue("id"))
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"templateSource": bundle})
}

func (s *Server) handleInlineTemplateSource(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 5<<20))
	decoder.DisallowUnknownFields()
	var req renderregistry.InlineInspection
	if err := decoder.Decode(&req); err != nil {
		if !writeTooLarge(w, err) {
			writeCode(w, codeInvalidRequest, "")
		}
		return
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		writeCode(w, codeInvalidRequest, "")
		return
	}
	source, err := s.catalog.PrepareInlineTemplateSource(r.Context(), org, req)
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"templateSource": source})
}
