package http

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
)

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
