package http

import (
	"context"
	"net/http"
	"orchestrator/internal/ports/moduleregistry"
	"time"
)

func (s *Server) handleInspectTerraformModule(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	var draft moduleregistry.GitInspection
	if !decodePreviewBody(w, r, &draft) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	capability, err := s.catalog.InspectTerraformModule(ctx, org, draft)
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"capability": capability})
}
