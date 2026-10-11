package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

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
		writeCode(w, codeInvalidRequest, "")
		return
	}
	decoder := yaml.NewDecoder(strings.NewReader(req.Content))
	var score map[string]any
	if err := decoder.Decode(&score); err != nil || score == nil {
		writeCode(w, codeInvalidRequest, "")
		return
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		writeCode(w, codeInvalidRequest, "")
		return
	}
	if err := s.workloads.ValidateImport(r.Context(), app, env, score); err != nil {
		s.writeError(w, err)
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
		s.writeError(w, err)
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
		writeCode(w, codeInvalidRequest, "")
		return false
	}
	if err := body.Decode(&struct{}{}); err != io.EOF {
		if !writeTooLarge(w, err) {
			writeCode(w, codeInvalidRequest, "")
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
	writeCode(w, codeRequestTooLarge, "")
	return true
}

func writeDecodeError(w http.ResponseWriter, err error) {
	if !writeTooLarge(w, err) {
		writeCode(w, codeInvalidRequest, "")
	}
}

// requiredVersion rejects an omitted, null or negative draft version.
func requiredVersion(w http.ResponseWriter, version *int64) bool {
	if version == nil || *version < 0 {
		writeCode(w, codeInvalidRequest, "")
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
		writeCode(w, codeInvalidRequest, "")
		return
	}
	view, err := s.workloads.Save(r.Context(), app, env, r.PathValue("workload"), req.Score, *req.Version)
	if err != nil {
		s.writeError(w, err)
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
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}
