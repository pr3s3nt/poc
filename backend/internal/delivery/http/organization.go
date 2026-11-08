package http

import "net/http"

func (s *Server) handleOrganization(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.sessionIdentity(r)
	if !ok {
		writeCode(w, codeUnauthorized, "")
		return
	}
	org, err := s.store.GetOrganization(r.Context(), identity.OrganizationKey)
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"organization": org})
}
