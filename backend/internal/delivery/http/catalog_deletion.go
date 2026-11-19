package http

import (
	"errors"
	"net/http"

	"orchestrator/internal/application/catalog"
)

func (s *Server) handleDeleteCatalogEntry(definition bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		org, ok := s.platformActor(w, r)
		if !ok {
			return
		}
		err := s.catalog.DeleteEntry(r.Context(), org, r.PathValue("key"), definition)
		var blocked *catalog.DeletionBlocked
		if errors.As(err, &blocked) {
			writeFailure(w, failure{code: codeCatalogDeletionBlocked, extra: map[string]any{"references": blocked.References}})
			return
		}
		if err != nil {
			s.writeError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
