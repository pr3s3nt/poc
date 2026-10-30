package http

import (
	"context"
	"errors"
	"net/http"
	"time"

	"orchestrator/internal/application/gitaccess"
	access "orchestrator/internal/ports/gitaccess"
	"orchestrator/internal/ports/persistence"
)

// GitTester performs only a bounded source read; it never creates catalog data.
type GitTester interface {
	TestGit(context.Context, string, string, access.Access) error
}

func (s *Server) gitSettingsError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, gitaccess.ErrInvalid):
		writeCode(w, codeInvalidRequest, "")
	case errors.Is(err, persistence.ErrVersionConflict):
		writeCode(w, codeStaleVersion, "")
	default:
		writeCode(w, codeServiceUnavailable, "")
	}
}
func (s *Server) handleGetGitSettings(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	v, err := s.gitAccess.Get(r.Context(), org)
	if err != nil {
		s.gitSettingsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": v})
}
func (s *Server) handleSaveGitSettings(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
	var d gitaccess.Draft
	if !decodePreviewBody(w, r, &d) {
		return
	}
	who, _ := s.sessionIdentity(r)
	v, err := s.gitAccess.Save(r.Context(), org, who.UserID, d)
	if err != nil {
		s.gitSettingsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": v})
}
func (s *Server) handleTestGitSettings(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if s.gitTester == nil || s.gitAccess == nil {
		writeCode(w, codeServiceUnavailable, "")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
	var d struct {
		gitaccess.Draft
		RepositoryURL string `json:"repositoryUrl"`
		Ref           string `json:"ref"`
	}
	if !decodePreviewBody(w, r, &d) {
		return
	}
	value, err := s.gitAccess.DraftAccess(r.Context(), org, d.RepositoryURL, d.Draft)
	if err != nil {
		s.gitSettingsError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err = s.gitTester.TestGit(ctx, d.RepositoryURL, d.Ref, value); err != nil {
		writeCode(w, codeVerificationFailed, "")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"reachable": true})
}
