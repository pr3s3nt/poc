package http

import (
	"net/http"
	"orchestrator/internal/application/catalog"
	"orchestrator/internal/domain/resource"
)

func (s *Server) handleImportResourceType(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	var doc resource.Type
	if !decodeStrict(w, r, managementBodyLimit, &doc) {
		return
	}
	result, err := s.catalog.ImportResourceType(r.Context(), org, doc)
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (s *Server) handleImportResourceDefinition(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	var doc catalog.DefinitionDraft
	if !decodeStrict(w, r, managementBodyLimit, &doc) {
		return
	}
	result, err := s.catalog.ImportResourceDefinition(r.Context(), org, doc.Definition())
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
