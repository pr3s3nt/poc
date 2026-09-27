package http

import (
	"encoding/json"
	"net/http"

	"orchestrator/internal/domain/configuration"
)

func (s *Server) authorizedConfiguration(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	identity, ok := s.sessionIdentity(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return "", "", false
	}
	appKey, envKey := r.PathValue("id"), r.PathValue("env")
	app, err := s.store.GetApplication(r.Context(), appKey)
	if err != nil || app.OrganizationKey != identity.OrganizationKey || (envKey != "staging" && envKey != "production") {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return "", "", false
	}
	return appKey, envKey, true
}

func (s *Server) handleGetConfiguration(w http.ResponseWriter, r *http.Request) {
	app, env, ok := s.authorizedConfiguration(w, r)
	if !ok {
		return
	}
	view, err := s.configurations.List(r.Context(), app, env)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

type putConfigurationKeyRequest struct {
	Kind    configuration.Kind `json:"kind"`
	Value   string             `json:"value"`
	Version int64              `json:"version"`
}

func (s *Server) handlePutConfigurationKey(w http.ResponseWriter, r *http.Request) {
	app, env, ok := s.authorizedConfiguration(w, r)
	if !ok {
		return
	}
	var req putConfigurationKeyRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
		return
	}
	view, err := s.configurations.Put(r.Context(), app, env, r.PathValue("key"), req.Kind, req.Value, req.Version)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

type renameConfigurationKeyRequest struct {
	NewName string `json:"newName"`
	Version int64  `json:"version"`
}

func (s *Server) handleRenameConfigurationKey(w http.ResponseWriter, r *http.Request) {
	app, env, ok := s.authorizedConfiguration(w, r)
	if !ok {
		return
	}
	var req renameConfigurationKeyRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
		return
	}
	view, err := s.configurations.Rename(r.Context(), app, env, r.PathValue("key"), req.NewName, req.Version)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

type deleteConfigurationKeyRequest struct {
	Version int64 `json:"version"`
}

func (s *Server) handleDeleteConfigurationKey(w http.ResponseWriter, r *http.Request) {
	app, env, ok := s.authorizedConfiguration(w, r)
	if !ok {
		return
	}
	var req deleteConfigurationKeyRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
		return
	}
	view, err := s.configurations.Delete(r.Context(), app, env, r.PathValue("key"), req.Version)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}
