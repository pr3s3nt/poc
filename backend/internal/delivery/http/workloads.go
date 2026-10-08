package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"orchestrator/internal/application/workloadconfig"
	"orchestrator/internal/ports/persistence"

	"gopkg.in/yaml.v3"
)

type parseWorkloadRequest struct {
	Content string `json:"content"`
}

func (s *Server) handleParseWorkloadScore(w http.ResponseWriter, r *http.Request) {
	app, env, ok := s.authorizedConfiguration(w, r)
	if !ok {
		return
	}
	var req parseWorkloadRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 3<<20)).Decode(&req); err != nil || strings.TrimSpace(req.Content) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "Score file is required"})
		return
	}
	decoder := yaml.NewDecoder(strings.NewReader(req.Content))
	var score map[string]any
	if err := decoder.Decode(&score); err != nil || score == nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid Score YAML/JSON"})
		return
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "Score file must contain exactly one document"})
		return
	}
	if err := s.workloads.ValidateImport(r.Context(), app, env, score); err != nil {
		s.writeDraftError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"score": score})
}

func (s *Server) handleListWorkloadDrafts(w http.ResponseWriter, r *http.Request) {
	app, env, ok := s.authorizedConfiguration(w, r)
	if !ok {
		return
	}
	view, err := s.workloads.List(r.Context(), app, env)
	if err != nil {
		s.writeDraftError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

// decodeStrict reads exactly one bounded JSON object with known fields
// (UC-07 API mapping). It writes the 400/413 response itself.
func decodeStrict(w http.ResponseWriter, r *http.Request, limit int64, dst any) bool {
	body := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	var raw json.RawMessage
	if err := body.Decode(&raw); err != nil {
		writeDecodeError(w, err)
		return false
	}
	if trimmed := bytes.TrimSpace(raw); len(trimmed) == 0 || trimmed[0] != '{' {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "request body must be one JSON object"})
		return false
	}
	if err := body.Decode(&struct{}{}); err != io.EOF {
		if !writeTooLarge(w, err) {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "request body must contain exactly one JSON object"})
		}
		return false
	}
	fields := json.NewDecoder(bytes.NewReader(raw))
	fields.DisallowUnknownFields()
	if err := fields.Decode(dst); err != nil {
		writeDecodeError(w, err)
		return false
	}
	return true
}

// writeTooLarge answers 413 when err comes from the body size limit.
func writeTooLarge(w http.ResponseWriter, err error) bool {
	var tooLarge *http.MaxBytesError
	if !errors.As(err, &tooLarge) {
		return false
	}
	writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"error": "request body is too large"})
	return true
}

func writeDecodeError(w http.ResponseWriter, err error) {
	if !writeTooLarge(w, err) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON body: " + safeDecodeMessage(err)})
	}
}

// writeDraftError maps UC-16/UC-07 draft errors: validation messages come
// from the caller's own Score or fixed workloadconfig text; store and other
// unknown errors are a generic retryable 500 without cause.
func (s *Server) writeDraftError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, persistence.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
	case errors.Is(err, persistence.ErrEnvironmentBusy):
		s.writeBusy(w, err)
	case errors.Is(err, persistence.ErrVersionConflict):
		writeJSON(w, http.StatusConflict, map[string]any{"error": "workloads changed since they were loaded; reload and try again"})
	case errors.Is(err, workloadconfig.ErrInvalid), strings.HasPrefix(err.Error(), "score:"), strings.HasPrefix(err.Error(), "workloadconfig:"):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
	default:
		log.Printf("workload draft request failed: internal error (%T)", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "the request failed; retry"})
	}
}

// requiredVersion rejects an omitted, null or negative draft version.
func requiredVersion(w http.ResponseWriter, version *int64) bool {
	if version == nil || *version < 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "version is required and must be a nonnegative integer"})
		return false
	}
	return true
}

type saveWorkloadRequest struct {
	Version *int64         `json:"version"`
	Score   map[string]any `json:"score"`
}

func (s *Server) handleSaveWorkloadDraft(w http.ResponseWriter, r *http.Request) {
	app, env, ok := s.authorizedConfiguration(w, r)
	if !ok {
		return
	}
	var req saveWorkloadRequest
	if !decodeStrict(w, r, 2<<20, &req) || !requiredVersion(w, req.Version) {
		return
	}
	if req.Score == nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "valid Score JSON is required"})
		return
	}
	view, err := s.workloads.Save(r.Context(), app, env, r.PathValue("workload"), req.Score, *req.Version)
	if err != nil {
		s.writeDraftError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

type workloadVersionRequest struct {
	Version *int64 `json:"version"`
}

func (s *Server) handleDeleteWorkloadDraft(w http.ResponseWriter, r *http.Request) {
	s.handleWorkloadDraftAction(w, r, false)
}

func (s *Server) handleUndoWorkloadDraft(w http.ResponseWriter, r *http.Request) {
	s.handleWorkloadDraftAction(w, r, true)
}

func (s *Server) handleWorkloadDraftAction(w http.ResponseWriter, r *http.Request, undo bool) {
	app, env, ok := s.authorizedConfiguration(w, r)
	if !ok {
		return
	}
	var req workloadVersionRequest
	if !decodeStrict(w, r, 1<<10, &req) || !requiredVersion(w, req.Version) {
		return
	}
	var view any
	var err error
	if undo {
		view, err = s.workloads.Undo(r.Context(), app, env, r.PathValue("workload"), *req.Version)
	} else {
		view, err = s.workloads.Delete(r.Context(), app, env, r.PathValue("workload"), *req.Version)
	}
	if err != nil {
		s.writeDraftError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}
