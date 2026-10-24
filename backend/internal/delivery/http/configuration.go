package http

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	appconfig "orchestrator/internal/application/configuration"
	"orchestrator/internal/domain/configuration"
)

func (s *Server) authorizedConfiguration(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	identity, ok := s.sessionIdentity(r)
	if !ok {
		writeCode(w, codeUnauthorized, "")
		return "", "", false
	}
	appKey, envKey := r.PathValue("id"), r.PathValue("env")
	app, err := s.store.GetApplication(r.Context(), appKey)
	if err != nil || app.OrganizationKey != identity.OrganizationKey || (envKey != "staging" && envKey != "production") {
		writeCode(w, codeNotFound, "")
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
		s.writeError(w, err)
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
		writeCode(w, codeInvalidRequest, "")
		return
	}
	view, err := s.configurations.Put(r.Context(), app, env, r.PathValue("key"), req.Kind, req.Value, req.Version)
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

// createConfigurationKeyRequest is the create-only body of OC-12 DEVEX-ENV.
// Every field is required; a pointer or raw value distinguishes missing and
// null from zero values, so neither is read as an empty name, kind or version.
type createConfigurationKeyRequest struct {
	Name    *string         `json:"name"`
	Kind    *string         `json:"kind"`
	Value   *string         `json:"value"`
	Version json.RawMessage `json:"version"`
}

func (s *Server) handleCreateConfigurationKey(w http.ResponseWriter, r *http.Request) {
	app, env, ok := s.authorizedConfiguration(w, r)
	if !ok {
		return
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	var req *createConfigurationKeyRequest
	if err := decoder.Decode(&req); err != nil || req == nil {
		writeCode(w, codeInvalidRequest, "")
		return
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		writeCode(w, codeInvalidRequest, "")
		return
	}
	version, versionOK := strictVersion(req.Version)
	if req.Name == nil || req.Kind == nil || req.Value == nil || !versionOK {
		writeCode(w, codeInvalidRequest, "")
		return
	}
	view, err := s.configurations.CreateKey(r.Context(), app, env, *req.Name, configuration.Kind(*req.Kind), *req.Value, version)
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, view)
}

type renameConfigurationKeyRequest struct {
	NewName            string          `json:"newName"`
	Version            int64           `json:"version"`
	UpdateReferences   json.RawMessage `json:"updateReferences"`
	EnvironmentVersion json.RawMessage `json:"environmentVersion"`
	DraftVersion       json.RawMessage `json:"draftVersion"`
}

// referenceGuards strictly reads the optional T30 opt-in. updateReferences must
// be a JSON boolean when present; when true both guards must be non-negative
// JSON integers. A missing, null or malformed guard is never read as zero.
func (r renameConfigurationKeyRequest) referenceGuards() (appconfig.ReferenceGuards, bool) {
	if r.UpdateReferences == nil {
		return appconfig.ReferenceGuards{}, true
	}
	switch strings.TrimSpace(string(r.UpdateReferences)) {
	case "false":
		return appconfig.ReferenceGuards{}, true
	case "true":
	default:
		return appconfig.ReferenceGuards{}, false
	}
	environmentVersion, ok1 := strictVersion(r.EnvironmentVersion)
	draftVersion, ok2 := strictVersion(r.DraftVersion)
	if !ok1 || !ok2 {
		return appconfig.ReferenceGuards{}, false
	}
	return appconfig.ReferenceGuards{UpdateReferences: true, EnvironmentVersion: environmentVersion, DraftVersion: draftVersion}, true
}

// decodeRenameRequest is the strict bounded decoder of the T30 PATCH: one JSON
// object, no unknown fields, no trailing data, not null.
func decodeRenameRequest(w http.ResponseWriter, r *http.Request) (renameConfigurationKeyRequest, bool) {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	var req *renameConfigurationKeyRequest
	if err := decoder.Decode(&req); err != nil || req == nil {
		return renameConfigurationKeyRequest{}, false
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		return renameConfigurationKeyRequest{}, false
	}
	return *req, true
}

func strictVersion(raw json.RawMessage) (int64, bool) {
	text := strings.TrimSpace(string(raw))
	if text == "" || text[0] < '0' || text[0] > '9' {
		return 0, false
	}
	value, err := strconv.ParseInt(text, 10, 64)
	return value, err == nil
}

func (s *Server) handleRenameConfigurationKey(w http.ResponseWriter, r *http.Request) {
	app, env, ok := s.authorizedConfiguration(w, r)
	if !ok {
		return
	}
	req, ok := decodeRenameRequest(w, r)
	if !ok {
		writeCode(w, codeInvalidRequest, "")
		return
	}
	guards, valid := req.referenceGuards()
	if !valid {
		writeCode(w, codeInvalidRequest, "")
		return
	}
	view, err := s.configurations.RenameKey(r.Context(), app, env, r.PathValue("key"), req.NewName, req.Version, guards)
	if err != nil {
		s.writeError(w, err)
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
		writeCode(w, codeInvalidRequest, "")
		return
	}
	view, err := s.configurations.Delete(r.Context(), app, env, r.PathValue("key"), req.Version)
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}
