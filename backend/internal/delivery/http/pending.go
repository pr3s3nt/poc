package http

import (
	"net/http"
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
		s.writeError(w, err)
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
		writeCode(w, codeInvalidRequest, "")
		return
	}
	report, err := s.pending.Deploy(r.Context(), app, env, identity.UserID, *req.Token)
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}
