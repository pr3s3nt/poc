package http

import "net/http"

func (s *Server) handleCatalogModules(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	modules, err := s.catalog.CatalogModules(r.Context(), org, r.PathValue("source"), r.PathValue("tag"), r.URL.Query().Get("resourceType"))
	if err != nil {
		catalogReleaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"modules": modules})
}
