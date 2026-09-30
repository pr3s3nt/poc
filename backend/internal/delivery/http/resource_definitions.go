package http

import (
	"net/http"

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
