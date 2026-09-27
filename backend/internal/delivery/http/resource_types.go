package http

import "net/http"

func (s *Server) handleResourceTypes(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.sessionIdentity(r); !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	types, err := s.store.ListResourceTypes(r.Context())
	if err != nil { writeError(w, err); return }
	writeJSON(w, http.StatusOK, map[string]any{"resourceTypes": types})
}
