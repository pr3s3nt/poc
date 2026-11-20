package http

import (
	"mime"
	"net/http"

	"orchestrator/internal/domain/identity"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
)

func mayPurgeApplication(role identity.Role) bool {
	return role == identity.RoleAdmin || role == identity.RolePlatformEngineer
}

func (s *Server) webPurgeAvailable(r *http.Request, role identity.Role) bool {
	store, ok := s.store.(persistence.WebApplicationPurgeStore)
	if !ok || !mayPurgeApplication(role) {
		return false
	}
	enabled, err := store.WebApplicationPurgeEnabled(r.Context())
	return err == nil && enabled
}

func (s *Server) handleWebApplicationPurge(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	who, ok := s.sessionIdentity(r)
	if !ok {
		writeCode(w, codeUnauthorized, "")
		return
	}
	if !mayPurgeApplication(who.Role) {
		writeCode(w, codeForbidden, "")
		return
	}
	if !s.webPurgeAvailable(r, who.Role) {
		writeCode(w, codeNotFound, "")
		return
	}
	store := s.store.(persistence.WebApplicationPurgeStore)
	app := r.PathValue("id")
	if r.Method == http.MethodGet {
		preview, err := store.PreviewWebApplicationPurge(r.Context(), who.OrganizationKey, app)
		if err != nil {
			s.writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"preview": preview})
		return
	}

	// Browser cross-origin simple requests cannot carry this content type.
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" {
		writeCode(w, codeInvalidRequest, "")
		return
	}
	var body struct {
		ApplicationID    string `json:"applicationId"`
		ConfirmationName string `json:"confirmationName"`
		PriorStopped     bool   `json:"priorExecutionStopped"`
	}
	if !decodeStrict(w, r, 2048, &body) {
		return
	}
	if !ids.ValidUUID(body.ApplicationID) || body.ConfirmationName == "" || len(body.ConfirmationName) > 512 {
		writeCode(w, codeInvalidRequest, "")
		return
	}

	outcome, err := store.ExecuteWebApplicationPurge(r.Context(), who.OrganizationKey, app, body.ApplicationID, body.ConfirmationName, body.PriorStopped)
	if err != nil {
		writeCode(w, codePurgeFailed, "")
		return
	}
	switch outcome {
	case "PURGED":
		w.WriteHeader(http.StatusNoContent)
	case "DISABLED", "NOT_FOUND":
		writeCode(w, codeNotFound, "")
	case "MISMATCH":
		writeCode(w, codeApplicationConfirmationMismatch, "confirmationName")
	case "BUSY":
		writeCode(w, codeEnvironmentBusy, "")
	case "RECOVERY_REQUIRED":
		writeCode(w, codePurgeNeedsStopped, "")
	default:
		writeCode(w, codePurgeFailed, "")
	}
}
