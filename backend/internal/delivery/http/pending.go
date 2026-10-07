package http

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"orchestrator/internal/application/pending"
	"orchestrator/internal/application/workloadconfig"
	"orchestrator/internal/ports/persistence"
)

type previewPendingRequest struct{}

type deployPendingRequest struct {
	Token *string `json:"token"`
}

func (s *Server) handlePreviewPending(w http.ResponseWriter, r *http.Request) {
	app, env, ok := s.authorizedConfiguration(w, r)
	if !ok {
		return
	}
	var req previewPendingRequest
	if !decodeStrict(w, r, 1<<10, &req) {
		return
	}
	preview, err := s.pending.Preview(r.Context(), app, env)
	if err != nil {
		writePendingError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (s *Server) handleDeployPending(w http.ResponseWriter, r *http.Request) {
	app, env, ok := s.authorizedConfiguration(w, r)
	if !ok {
		return
	}
	identity, _ := s.sessionIdentity(r)
	var req deployPendingRequest
	if !decodeStrict(w, r, 1<<10, &req) {
		return
	}
	if req.Token == nil || *req.Token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "preview token is required"})
		return
	}
	report, err := s.pending.Deploy(r.Context(), app, env, identity.UserID, *req.Token)
	if err != nil {
		writePendingError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

// writePendingError maps the UC-16/UC-07 pending flow. Validation and stale
// messages are user-derived or fixed; anything else is a generic retryable
// error whose cause is not returned or logged.
func writePendingError(w http.ResponseWriter, err error) {
	if writeUnconfigured(w, err) {
		return
	}
	switch {
	case errors.Is(err, persistence.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
	case errors.Is(err, pending.ErrStalePreview), errors.Is(err, persistence.ErrVersionConflict):
		writeJSON(w, http.StatusConflict, map[string]any{"error": pending.ErrStalePreview.Error()})
	case errors.Is(err, pending.ErrRouteReconcile):
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": pending.ErrRouteReconcile.Error()})
	case errors.Is(err, pending.ErrInvalid), errors.Is(err, workloadconfig.ErrInvalid), strings.HasPrefix(err.Error(), "score:"):
		// score: errors come from parsing the user's own saved draft.
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
	default:
		log.Printf("pending changes failed: internal error (%T)", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "the request failed; retry"})
	}
}
