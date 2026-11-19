package http

import (
	"context"
	"net/http"
	"orchestrator/internal/application/catalog"
	"orchestrator/internal/domain/resource"
	"strconv"
	"time"
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

func (s *Server) handlePrepareDefinitionDocument(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	var draft catalog.DefinitionDraft
	if !decodeStrict(w, r, managementBodyLimit, &draft) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	result, err := s.catalog.PrepareDefinitionDocument(ctx, org, draft)
	if err != nil {
		s.writeError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleExportDefinitionDocument(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	revision := r.URL.Query().Get("revision")
	var version int64
	if value, present := r.URL.Query()["version"]; present {
		var err error
		if len(value) != 1 || revision != "" {
			writeCode(w, codeInvalidRequest, "")
			return
		}
		version, err = strconv.ParseInt(value[0], 10, 64)
		if err != nil || version < 1 {
			writeCode(w, codeInvalidRequest, "")
			return
		}
	}
	doc, err := s.catalog.ExportDefinitionDocument(r.Context(), org, r.PathValue("key"), revision, version)
	if err != nil {
		s.writeError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, doc)
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
