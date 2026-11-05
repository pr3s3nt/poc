package http

import (
	"net/http"
	"strings"
)

// applicationAccessGuard runs before lifecycle and route dispatch so every
// nested Application endpoint is protected, including future routes and deletion
// progress. Return true when the request has been answered.
func (s *Server) applicationAccessGuard(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path == "/api/v1/applications" {
		w.Header().Set("Cache-Control", "no-store")
		return false
	}
	rest, applicationRoute := strings.CutPrefix(r.URL.Path, "/api/v1/applications/")
	if !applicationRoute || rest == "" {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	actor, signedIn := s.sessionIdentity(r)
	if !signedIn {
		writeCode(w, codeUnauthorized, "")
		return true
	}
	key, _, _ := strings.Cut(rest, "/")
	app, err := s.store.GetApplication(r.Context(), key)
	if err != nil {
		s.writeError(w, err)
		return true
	}
	if !app.AccessibleBy(actor.OrganizationKey, actor.UserID, actor.Role) {
		writeCode(w, codeNotFound, "")
		return true
	}
	return false
}
