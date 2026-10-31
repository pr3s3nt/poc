package http

import "net/http"

func (s *Server) handleWorkloadTemplates(w http.ResponseWriter, r *http.Request) {
	app, env, ok := s.authorizedConfiguration(w, r)
	if !ok {
		return
	}
	application, err := s.store.GetApplication(r.Context(), app)
	if err != nil {
		s.writeError(w, err)
		return
	}
	templates, err := s.catalog.ListEnvironmentWorkloadTemplates(r.Context(), application.OrganizationKey, app, env)
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"templates": templates})
}
