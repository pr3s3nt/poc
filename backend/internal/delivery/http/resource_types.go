package http

import (
	"errors"
	"log"
	"net/http"

	"orchestrator/internal/application/catalog"
	connectionapp "orchestrator/internal/application/connection"
	identity2 "orchestrator/internal/domain/identity"
	"orchestrator/internal/domain/resource"
)

// managementBodyLimit bounds one UC-02/03/04 registration document.
const managementBodyLimit = 1 << 20

// writeManagementError maps UC-02/03/04 registration and list errors.
// Validation of the caller's own document stays actionable; verification
// failures carry only fixed guidance; store, inspector and other unknown
// failures are a generic 500 whose cause is neither returned nor logged.
func writeManagementError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, catalog.ErrDuplicate), errors.Is(err, connectionapp.ErrDuplicate):
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error()})
	case errors.Is(err, catalog.ErrInvalid), errors.Is(err, connectionapp.ErrInvalid):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
	case errors.Is(err, connectionapp.ErrVerification):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": err.Error()})
	default:
		log.Printf("catalog or connection request failed: internal error (%T)", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "the request failed; retry"})
	}
}

// platformActor returns the session identity of a Platform Engineer/Admin.
func (s *Server) platformActor(w http.ResponseWriter, r *http.Request) (organizationKey string, ok bool) {
	actor, ok := s.sessionIdentity(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return "", false
	}
	if actor.Role != identity2.RolePlatformEngineer && actor.Role != identity2.RoleAdmin {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "platform engineer access required"})
		return "", false
	}
	return actor.OrganizationKey, true
}

func (s *Server) handleResourceTypes(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.sessionIdentity(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	types, err := s.store.ListResourceTypes(r.Context(), identity.OrganizationKey)
	if err != nil {
		writeManagementError(w, err)
		return
	}
	if types == nil {
		types = []resource.Type{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"resourceTypes": types})
}

func (s *Server) handleRegisterResourceType(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	var typ resource.Type
	if !decodeStrict(w, r, managementBodyLimit, &typ) {
		return
	}
	created, err := s.catalog.RegisterResourceType(r.Context(), org, typ)
	if err != nil {
		writeManagementError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}
