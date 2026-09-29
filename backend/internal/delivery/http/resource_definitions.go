package http

import (
	"encoding/json"
	"net/http"

	"orchestrator/internal/domain/identity"
	"orchestrator/internal/domain/resource"
)

func (s *Server) handleResourceDefinitions(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.sessionIdentity(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	if actor.Role != identity.RolePlatformEngineer && actor.Role != identity.RoleAdmin {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "platform engineer access required"})
		return
	}
	definitions, err := s.store.ListResourceDefinitions(r.Context(), actor.OrganizationKey)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"resourceDefinitions": definitions})
}

func (s *Server) handleRegisterResourceDefinition(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.sessionIdentity(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	if actor.Role != identity.RolePlatformEngineer && actor.Role != identity.RoleAdmin {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "platform engineer access required"})
		return
	}
	var def resource.Definition
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&def); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid resource definition JSON"})
		return
	}
	created, err := s.catalog.RegisterResourceDefinition(r.Context(), actor.OrganizationKey, def)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}
