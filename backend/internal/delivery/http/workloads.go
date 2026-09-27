package http

import (
	"encoding/json"
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
		writeError(w, err)
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
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

type saveWorkloadRequest struct {
	Version int64          `json:"version"`
	Score   map[string]any `json:"score"`
}

func (s *Server) handleSaveWorkloadDraft(w http.ResponseWriter, r *http.Request) {
	app, env, ok := s.authorizedConfiguration(w, r)
	if !ok {
		return
	}
	var req saveWorkloadRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20)).Decode(&req); err != nil || req.Score == nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "valid Score JSON is required"})
		return
	}
	view, err := s.workloads.Save(r.Context(), app, env, r.PathValue("workload"), req.Score, req.Version)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

type workloadVersionRequest struct {
	Version int64 `json:"version"`
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
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
		return
	}
	var view any
	var err error
	if undo {
		view, err = s.workloads.Undo(r.Context(), app, env, r.PathValue("workload"), req.Version)
	} else {
		view, err = s.workloads.Delete(r.Context(), app, env, r.PathValue("workload"), req.Version)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}
