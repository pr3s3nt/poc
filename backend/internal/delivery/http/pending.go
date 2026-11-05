package http

import (
	"log"
	"net/http"
	"orchestrator/internal/platform/ids"
	"strings"
)

type previewPendingRequest struct{}

type deployPendingRequest struct {
	Token           *string `json:"token"`
	ReviewRequestID string  `json:"reviewRequestId,omitempty"`
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
		reason := "UNCLASSIFIED"
		for _, marker := range []string{"cannot be rebuilt", "cannot be updated", "legacy literal binding", "catalog update of database or persistent storage", "image must come from the configured Harbor registry", "Service reference cycle", "apply or discard workload changes", "template"} {
			if strings.Contains(err.Error(), marker) {
				reason = marker
				break
			}
		}
		log.Printf("pending preview rejected: reason=%s type=%T", reason, err)
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
	if req.ReviewRequestID != "" && !ids.ValidUUID(req.ReviewRequestID) {
		writeCode(w, codeInvalidRequest, "reviewRequestId")
		return
	}
	report, err := s.pending.DeployWithReview(r.Context(), app, env, identity.UserID, *req.Token, req.ReviewRequestID)
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}
