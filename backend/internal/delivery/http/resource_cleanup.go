package http

import (
	"net/http"

	"orchestrator/internal/application/resourcecleanup"
	identitydomain "orchestrator/internal/domain/identity"
)

// resourceRequest authenticates the caller and scopes the Application/
// Environment of a resource route. A missing session is 401 and a foreign or
// unknown scope 404, both before the role matters; the role only decides the
// actor's right to delete.
func (s *Server) resourceRequest(w http.ResponseWriter, r *http.Request) (resourcecleanup.Scope, resourcecleanup.Actor, bool) {
	org, appKey, envKey, ok := s.scopedEnvironment(w, r)
	if !ok {
		return resourcecleanup.Scope{}, resourcecleanup.Actor{}, false
	}
	identity, _ := s.sessionIdentity(r)
	actor := resourcecleanup.Actor{
		Username:  identity.Username,
		MayDelete: identity.Role == identitydomain.RolePlatformEngineer || identity.Role == identitydomain.RoleAdmin,
	}
	return resourcecleanup.Scope{OrganizationKey: org, ApplicationKey: appKey, EnvironmentKey: envKey}, actor, true
}

// resourceServiceReady answers 503 when the resource service is not wired (a
// server built without it). There is no in-memory or State fallback: reads and
// mutations are simply unavailable. It runs after the session scope and the role
// gate, so a Developer is still denied first and a foreign scope is still 404.
func (s *Server) resourceServiceReady(w http.ResponseWriter) bool {
	if s.resourceCleanup != nil {
		return true
	}
	s.writeError(w, resourcecleanup.ErrUnavailable)
	return false
}

func (s *Server) handleListResources(w http.ResponseWriter, r *http.Request) {
	scope, _, ok := s.resourceRequest(w, r)
	if !ok || !s.resourceServiceReady(w) {
		return
	}
	list, err := s.resourceCleanup.ListResources(r.Context(), scope)
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"resources": list})
}

func (s *Server) handleGetResource(w http.ResponseWriter, r *http.Request) {
	scope, _, ok := s.resourceRequest(w, r)
	if !ok || !s.resourceServiceReady(w) {
		return
	}
	view, err := s.resourceCleanup.GetResource(r.Context(), scope, r.PathValue("resourceId"))
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"resource": view})
}

func (s *Server) handleDeletionPreview(w http.ResponseWriter, r *http.Request) {
	scope, actor, ok := s.resourceRequest(w, r)
	if !ok {
		return
	}
	// The role gate comes straight after the session scope: a Developer is
	// denied before the body is read and before any planning (UC-07).
	if !actor.MayDelete {
		writeCode(w, codeForbidden, "")
		return
	}
	if !s.resourceServiceReady(w) {
		return
	}
	var body struct{}
	if !decodeStrict(w, r, 1<<10, &body) {
		return
	}
	view, err := s.resourceCleanup.PrepareDeletion(r.Context(), scope, actor, r.PathValue("resourceId"))
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"preview": view})
}

type deletionBody struct {
	Token               string   `json:"token"`
	ConfirmationName    string   `json:"confirmationName"`
	SelectedResourceIDs []string `json:"selectedResourceIds"`
}

func (s *Server) handleDeletions(w http.ResponseWriter, r *http.Request) {
	scope, actor, ok := s.resourceRequest(w, r)
	if !ok {
		return
	}
	if !actor.MayDelete {
		writeCode(w, codeForbidden, "")
		return
	}
	if !s.resourceServiceReady(w) {
		return
	}
	var body deletionBody
	if !decodeStrict(w, r, 16<<10, &body) {
		return
	}
	view, err := s.resourceCleanup.CleanupResources(r.Context(), scope, actor, r.PathValue("resourceId"), resourcecleanup.Input{
		Token: body.Token, ConfirmationName: body.ConfirmationName, Selected: body.SelectedResourceIDs,
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"cleanup": view})
}

func (s *Server) handleGetResourceCleanup(w http.ResponseWriter, r *http.Request) {
	scope, _, ok := s.resourceRequest(w, r)
	if !ok || !s.resourceServiceReady(w) {
		return
	}
	view, err := s.resourceCleanup.GetCleanup(r.Context(), scope, r.PathValue("operationId"))
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cleanup": view})
}
