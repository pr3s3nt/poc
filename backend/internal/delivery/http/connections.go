package http

import (
	"net/http"

	connectionapp "orchestrator/internal/application/connection"
	"orchestrator/internal/domain/application"
)

func (s *Server) handleConnections(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	connections, err := s.store.ListConnections(r.Context(), org)
	if err != nil {
		writeManagementError(w, err)
		return
	}
	if connections == nil {
		connections = []application.Connection{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"connections": connections})
}

func (s *Server) handleRegisterKubernetesConnection(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	var cmd connectionapp.RegisterKubernetesCommand
	if !decodeStrict(w, r, managementBodyLimit, &cmd) {
		return
	}
	created, err := s.connections.RegisterKubernetesCluster(r.Context(), org, cmd)
	if err != nil {
		writeManagementError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}
