package http

import (
	"net/http"

	identity2 "orchestrator/internal/domain/identity"
	"orchestrator/internal/domain/resource"
)

// managementBodyLimit bounds one UC-02/03/04 registration document.
const managementBodyLimit = 1 << 20

// platformActor returns the session identity of a Platform Engineer/Admin.
func (s *Server) platformActor(w http.ResponseWriter, r *http.Request) (organizationKey string, ok bool) {
	actor, ok := s.sessionIdentity(r)
	if !ok {
		writeCode(w, codeUnauthorized, "")
		return "", false
	}
	if actor.Role != identity2.RolePlatformEngineer && actor.Role != identity2.RoleAdmin {
		writeCode(w, codeForbidden, "")
		return "", false
	}
	return actor.OrganizationKey, true
}

func (s *Server) handleResourceTypes(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.sessionIdentity(r)
	if !ok {
		writeCode(w, codeUnauthorized, "")
		return
	}
	types, err := s.catalog.ListResourceTypeViews(r.Context(), identity.OrganizationKey)
	if err != nil {
		s.writeError(w, err)
		return
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
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

// addResourceTypeOutputsRequest is the strict body of UC-02 EXT. Required is
// deliberately absent: an added output is always optional.
type addResourceTypeOutputsRequest struct {
	Outputs []struct {
		Name   string `json:"name"`
		Type   string `json:"type"`
		Secret bool   `json:"secret"`
	} `json:"outputs"`
}

func (s *Server) handleAddResourceTypeOutputs(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	var body addResourceTypeOutputsRequest
	if !decodeStrict(w, r, managementBodyLimit, &body) {
		return
	}
	outputs := make([]resource.OutputField, 0, len(body.Outputs))
	for _, field := range body.Outputs {
		outputs = append(outputs, resource.OutputField{Name: field.Name, Type: field.Type, Secret: field.Secret})
	}
	updated, err := s.catalog.AddResourceTypeOutputs(r.Context(), org, r.PathValue("key"), outputs)
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}
