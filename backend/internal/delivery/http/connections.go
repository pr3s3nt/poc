package http

import (
	"encoding/json"
	"net/http"

	connectionapp "orchestrator/internal/application/connection"
	"orchestrator/internal/domain/identity"
)

func (s *Server) handleConnections(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.sessionIdentity(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	if actor.Role != identity.RolePlatformEngineer && actor.Role != identity.RoleAdmin {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "platform engineer access required"})
		return
	}
	connections, err := s.store.ListConnections(r.Context(), actor.OrganizationKey)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"connections": connections})
}

func (s *Server) handleRegisterKubernetesConnection(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.sessionIdentity(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	if actor.Role != identity.RolePlatformEngineer && actor.Role != identity.RoleAdmin {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "platform engineer access required"})
		return
	}
	var cmd connectionapp.RegisterKubernetesCommand
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cmd); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid Kubernetes connection JSON"})
		return
	}
	created, err := s.connections.RegisterKubernetesCluster(r.Context(), actor.OrganizationKey, cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}
