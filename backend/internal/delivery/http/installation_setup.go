package http

import (
	"errors"
	"net/http"
	"orchestrator/internal/application/installationsetup"
)

func (s *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	available, err := s.setup.Available(r.Context())
	if err != nil {
		writeCode(w, codeServiceUnavailable, "")
		return
	}
	writeJSON(w, 200, map[string]bool{"available": available})
}
func (s *Server) handleSetupClaim(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	// A cross-site browser cannot submit operator authorization to this endpoint.
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		writeCode(w, codeForbidden, "")
		return
	}
	var d installationsetup.Claim
	if !decodeStrict(w, r, 16<<10, &d) {
		return
	}
	err := s.setup.Initialize(r.Context(), d)
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, installationsetup.ErrClosed):
		writeCode(w, codeForbidden, "")
	case errors.Is(err, installationsetup.ErrToken):
		writeCode(w, codeForbidden, "")
	case errors.Is(err, installationsetup.ErrInvalid):
		writeCode(w, codeInvalidRequest, "")
	default:
		writeCode(w, codeServiceUnavailable, "")
	}
}
