package http

import (
	"encoding/json"
	"net/http"

	identity2 "orchestrator/internal/domain/identity"
	"orchestrator/internal/domain/resource"
)

func (s *Server) handleResourceTypes(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.sessionIdentity(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	types, err := s.store.ListResourceTypes(r.Context(), identity.OrganizationKey)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"resourceTypes": types})
}

func (s *Server) handleRegisterResourceType(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.sessionIdentity(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	if identity.Role != identity2.RolePlatformEngineer && identity.Role != identity2.RoleAdmin {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "platform engineer access required"})
		return
	}
	var typ resource.Type
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&typ); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid resource type JSON"})
		return
	}
	created, err := s.catalog.RegisterResourceType(r.Context(), identity.OrganizationKey, typ)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}
