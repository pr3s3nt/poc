package http

import (
	"errors"
	"net/http"
	"strings"

	"orchestrator/internal/application/appdeletion"
	appdomain "orchestrator/internal/domain/application"
	"orchestrator/internal/domain/identity"
)

// Application deletion (UC-18, ADR-025). Every role of the Organization may
// delete an Application; the typed confirmation and the backend guards are
// the same for all of them.

type applicationDeletionBody struct {
	Token            string `json:"token"`
	ConfirmationName string `json:"confirmationName"`
}

func (s *Server) applicationDeletionActor(w http.ResponseWriter, r *http.Request) (string, appdeletion.Actor, bool) {
	who, ok := s.sessionIdentity(r)
	if !ok {
		writeCode(w, codeUnauthorized, "")
		return "", appdeletion.Actor{}, false
	}
	if s.appDeletion == nil {
		writeCode(w, codeServiceUnavailable, "")
		return "", appdeletion.Actor{}, false
	}
	switch who.Role {
	case identity.RoleAdmin, identity.RolePlatformEngineer, identity.RoleDeveloper:
		return who.OrganizationKey, appdeletion.Actor{Username: who.Username, MayDelete: true}, true
	}
	return who.OrganizationKey, appdeletion.Actor{Username: who.Username}, true
}

func (s *Server) writeDeletionError(w http.ResponseWriter, err error) {
	var blocked *appdeletion.BlockedError
	switch {
	case errors.As(err, &blocked):
		writeFailure(w, failure{code: codeApplicationDeletionBlocked, extra: map[string]any{"blockers": blocked.Codes}})
	case errors.Is(err, appdeletion.ErrForbidden):
		writeCode(w, codeForbidden, "")
	case errors.Is(err, appdeletion.ErrUnavailable):
		writeCode(w, codeServiceUnavailable, "")
	case errors.Is(err, appdeletion.ErrInvalid):
		writeCode(w, codeInvalidRequest, "token")
	case errors.Is(err, appdeletion.ErrConfirmationMismatch):
		writeCode(w, codeApplicationConfirmationMismatch, "confirmationName")
	case errors.Is(err, appdeletion.ErrStalePreview):
		writeCode(w, codeStalePreview, "token")
	case errors.Is(err, appdeletion.ErrDeleting):
		writeCode(w, codeApplicationDeleting, "")
	default:
		s.writeError(w, err)
	}
}

func (s *Server) handleApplicationDeletionPreview(w http.ResponseWriter, r *http.Request) {
	org, actor, ok := s.applicationDeletionActor(w, r)
	if !ok {
		return
	}
	var body struct{}
	if !decodeStrict(w, r, 1<<10, &body) {
		return
	}
	view, err := s.appDeletion.Preview(r.Context(), org, r.PathValue("id"), actor)
	if err != nil {
		s.writeDeletionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"preview": view})
}

func (s *Server) handleApplicationDeletion(w http.ResponseWriter, r *http.Request) {
	org, actor, ok := s.applicationDeletionActor(w, r)
	if !ok {
		return
	}
	var body applicationDeletionBody
	if !decodeStrict(w, r, 16<<10, &body) {
		return
	}
	view, err := s.appDeletion.Delete(r.Context(), org, r.PathValue("id"), actor, body.Token, body.ConfirmationName)
	if err != nil {
		s.writeDeletionError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"deletion": view})
}

func (s *Server) handleGetApplicationDeletion(w http.ResponseWriter, r *http.Request) {
	org, _, ok := s.applicationDeletionActor(w, r)
	if !ok {
		return
	}
	view, err := s.appDeletion.Get(r.Context(), org, r.PathValue("id"), r.PathValue("operationId"))
	if err != nil {
		s.writeDeletionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deletion": view})
}

// lifecycleGuard freezes an Application being deleted (UC-18 BR-04): every
// mutation under /api/v1/applications/{id}/ answers 409 APPLICATION_DELETING,
// and a DELETED Application is not found, except for the deletion endpoints
// themselves. Only signed-in callers of the owning Organization are answered
// here; everyone else reaches the handler and its own scope checks.
func (s *Server) lifecycleGuard(w http.ResponseWriter, r *http.Request) bool {
	rest, ok := strings.CutPrefix(r.URL.Path, "/api/v1/applications/")
	if !ok || rest == "" {
		return false
	}
	id, sub, _ := strings.Cut(rest, "/")
	if id == "" || sub == "deletion-preview" || sub == "deletions" || strings.HasPrefix(sub, "deletions/") {
		return false
	}
	who, signedIn := s.sessionIdentity(r)
	if !signedIn {
		return false
	}
	app, err := s.store.GetApplication(r.Context(), id)
	if err != nil || app.OrganizationKey != who.OrganizationKey {
		return false
	}
	switch app.Lifecycle() {
	case appdomain.StatusDeleted:
		writeCode(w, codeNotFound, "")
		return true
	case appdomain.StatusDeleting:
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeCode(w, codeApplicationDeleting, "")
			return true
		}
	}
	return false
}

// activeApplication reports false only for an existing Application that is
// being deleted or deleted; an unknown key is left to the handler.
func (s *Server) activeApplication(r *http.Request, key string) bool {
	app, err := s.store.GetApplication(r.Context(), key)
	return err != nil || app.Lifecycle() == appdomain.StatusActive
}
