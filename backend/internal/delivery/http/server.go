// Package http exposes the orchestrator JSON API under /api/v1/ and serves the
// Orchestrator Web Console production bundle under /ui/.
package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"orchestrator/internal/adapters/configmemory"
	"orchestrator/internal/adapters/terraform"
	appcreate "orchestrator/internal/application/application"
	"orchestrator/internal/application/authentication"
	"orchestrator/internal/application/catalog"
	appconfig "orchestrator/internal/application/configuration"
	connectionapp "orchestrator/internal/application/connection"
	appsvc "orchestrator/internal/application/deployment"
	"orchestrator/internal/application/pending"
	"orchestrator/internal/application/preview"
	"orchestrator/internal/application/target"
	workloadconfig "orchestrator/internal/application/workloadconfig"
	domain "orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning"
	"orchestrator/internal/ports/credentials"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/seed"
)

// Server wires the HTTP handlers to the application services.
type Server struct {
	deployments    *appsvc.Service
	queries        *appsvc.QueryService
	auth           *authentication.Service
	applications   *appcreate.Service
	catalog        *catalog.Service
	connections    *connectionapp.Service
	configurations *appconfig.Service
	workloads      *workloadconfig.Service
	pending        *pending.Service
	previews       *preview.Service
	store          persistence.Store
	seedOptions    seed.Options
	uiDir          string
	mux            *http.ServeMux
}

// Config configures the HTTP server.
type Config struct {
	RenderBundles      map[string]resource.RenderBundle
	Deployments        *appsvc.Service
	Queries            *appsvc.QueryService
	Authentication     *authentication.Service
	Applications       *appcreate.Service
	ConnectionVerifier connectionapp.KubernetesVerifier
	// KubeconfigVerifier and ConnectionCredentials enable UC-04 upload
	// registration. Without a credential store it answers 503.
	KubeconfigVerifier    connectionapp.KubeconfigVerifier
	ConnectionCredentials credentials.Store
	Configurations        *appconfig.Service
	Workloads             *workloadconfig.Service
	Pending               *pending.Service
	Previews              *preview.Service
	Store                 persistence.Store
	SeedOptions           seed.Options
	UIDir                 string
}

// NewServer builds the HTTP handler tree.
func NewServer(cfg Config) *Server {
	s := &Server{
		deployments:    cfg.Deployments,
		queries:        cfg.Queries,
		auth:           cfg.Authentication,
		applications:   cfg.Applications,
		catalog:        catalog.NewService(cfg.Store, terraform.NewInspector()),
		connections:    connectionapp.NewService(cfg.Store, cfg.ConnectionVerifier),
		configurations: cfg.Configurations,
		workloads:      cfg.Workloads,
		pending:        cfg.Pending,
		previews:       cfg.Previews,
		store:          cfg.Store,
		seedOptions:    cfg.SeedOptions,
		uiDir:          cfg.UIDir,
		mux:            http.NewServeMux(),
	}
	s.connections.SetKubeconfigRegistration(cfg.KubeconfigVerifier, cfg.ConnectionCredentials)

	s.catalog.SetRenderBundles(cfg.RenderBundles)
	s.routes()
	return s
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/v1/healthz", s.handleHealth)
	s.mux.HandleFunc("POST /api/v1/auth/sign-in", s.handleSignIn)
	s.mux.HandleFunc("GET /api/v1/auth/session", s.handleSession)
	s.mux.HandleFunc("POST /api/v1/auth/sign-out", s.handleSignOut)
	s.mux.HandleFunc("GET /api/v1/applications", s.handleApplications)
	s.mux.HandleFunc("POST /api/v1/applications", s.handleCreateApplication)
	s.mux.HandleFunc("GET /api/v1/applications/{id}", s.handleGetApplication)
	s.mux.HandleFunc("PUT /api/v1/applications/{id}/environments/{env}/connection", s.handleSetEnvironmentConnection)
	s.mux.HandleFunc("GET /api/v1/applications/{id}/environments/{env}/configuration", s.handleGetConfiguration)
	s.mux.HandleFunc("PUT /api/v1/applications/{id}/environments/{env}/configuration/keys/{key}", s.handlePutConfigurationKey)
	s.mux.HandleFunc("PATCH /api/v1/applications/{id}/environments/{env}/configuration/keys/{key}", s.handleRenameConfigurationKey)
	s.mux.HandleFunc("DELETE /api/v1/applications/{id}/environments/{env}/configuration/keys/{key}", s.handleDeleteConfigurationKey)
	s.mux.HandleFunc("GET /api/v1/applications/{id}/environments/{env}/workloads", s.handleListWorkloadDrafts)
	s.mux.HandleFunc("POST /api/v1/applications/{id}/environments/{env}/workloads/parse", s.handleParseWorkloadScore)
	s.mux.HandleFunc("PUT /api/v1/applications/{id}/environments/{env}/workloads/{workload}", s.handleSaveWorkloadDraft)
	s.mux.HandleFunc("DELETE /api/v1/applications/{id}/environments/{env}/workloads/{workload}", s.handleDeleteWorkloadDraft)
	s.mux.HandleFunc("POST /api/v1/applications/{id}/environments/{env}/workloads/{workload}/undo", s.handleUndoWorkloadDraft)
	s.mux.HandleFunc("POST /api/v1/applications/{id}/environments/{env}/preview", s.handlePreviewPending)
	s.mux.HandleFunc("POST /api/v1/applications/{id}/environments/{env}/deploy", s.handleDeployPending)
	s.mux.HandleFunc("POST /api/v1/applications/{id}/environments/{env}/score-preview", s.handleScorePreview)
	s.mux.HandleFunc("GET /api/v1/score-samples", s.handleScoreSamples)
	s.mux.HandleFunc("GET /api/v1/resource-types", s.handleResourceTypes)
	s.mux.HandleFunc("POST /api/v1/resource-types", s.handleRegisterResourceType)
	s.mux.HandleFunc("GET /api/v1/resource-definitions", s.handleResourceDefinitions)
	s.mux.HandleFunc("POST /api/v1/resource-definitions", s.handleRegisterResourceDefinition)
	s.mux.HandleFunc("GET /api/v1/application-connections", s.handleApplicationConnections)
	s.mux.HandleFunc("GET /api/v1/connections", s.handleConnections)
	s.mux.HandleFunc("POST /api/v1/connections/kubernetes", s.handleRegisterKubernetesConnection)
	s.mux.HandleFunc("POST /api/v1/connections/kubernetes/inspect", s.handleInspectKubeconfig)
	s.mux.HandleFunc("POST /api/v1/deployments", s.handleCreateDeployment)
	s.mux.HandleFunc("GET /api/v1/applications/{id}/environments/{env}/deployments", s.handleListDeployments)
	s.mux.HandleFunc("GET /api/v1/applications/{id}/environments/{env}/deployments/{deployment}", s.handleGetDeployment)
	s.mux.HandleFunc("/ui/", s.handleUI)
	s.mux.HandleFunc("/", s.handleRoot)
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		http.Redirect(w, r, "/ui/", http.StatusFound)
		return
	}
	http.NotFound(w, r)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

// environmentView is the Deploy page selector payload.
type environmentView struct {
	Key                    string `json:"key"`
	Name                   string `json:"name"`
	Type                   string `json:"environmentType"`
	NamespaceIdentity      string `json:"namespaceIdentity"`
	CurrentDeploymentSetID string `json:"currentDeploymentSetId"`
	Version                int64  `json:"version"`
	// Target (ADR-011): safe nonsecret binding of this Environment.
	Configured          bool   `json:"configured"`
	ConnectionKey       string `json:"connectionKey"`
	ConnectionName      string `json:"connectionName,omitempty"`
	ConnectionKind      string `json:"connectionKind,omitempty"`
	Profile             string `json:"executionProfile"`
	Region              string `json:"region,omitempty"`
	RuntimeStatus       string `json:"runtimeStatus"`
	InfrastructureScope string `json:"infrastructureScope"`
}

type applicationView struct {
	Key          string            `json:"key"`
	Name         string            `json:"name"`
	Subdomain    string            `json:"subdomain"`
	Environments []environmentView `json:"environments"`
}

// environmentViewOf projects an Environment with its safe Connection labels.
func (s *Server) environmentViewOf(ctx context.Context, organizationKey string, e environment.Environment) environmentView {
	v := environmentView{
		Key: e.Key, Name: e.Name, Type: e.Type, NamespaceIdentity: e.NamespaceIdentity,
		CurrentDeploymentSetID: e.CurrentDeploymentSetID, Version: e.Version,
		Configured: e.Configured(), ConnectionKey: e.ConnectionKey, Profile: string(e.Profile), Region: e.Region,
		RuntimeStatus: string(e.Status()), InfrastructureScope: string(e.Scope()),
	}
	if e.Configured() {
		if conn, err := s.store.GetConnection(ctx, organizationKey, e.ConnectionKey); err == nil {
			v.ConnectionName, v.ConnectionKind = conn.Name, string(conn.Kind)
			if v.ConnectionName == "" {
				v.ConnectionName = conn.Key
			}
		}
	}
	return v
}

const sessionCookie = "orchestrator_session"

type signInRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) handleSignIn(w http.ResponseWriter, r *http.Request) {
	var req signInRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
		return
	}
	token, identity, err := s.auth.SignIn(r.Context(), req.Username, req.Password)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid credentials"})
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 8 * 60 * 60})
	writeJSON(w, http.StatusOK, map[string]any{"user": identity})
}
func (s *Server) sessionIdentity(r *http.Request) (authentication.Identity, bool) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return authentication.Identity{}, false
	}
	identity, err := s.auth.IdentityForToken(r.Context(), cookie.Value)
	return identity, err == nil
}
func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.sessionIdentity(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": identity})
}
func (s *Server) handleSignOut(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookie); err == nil {
		_ = s.auth.SignOut(r.Context(), cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, MaxAge: -1})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleApplications(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.sessionIdentity(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	apps, err := s.store.ListApplications(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]applicationView, 0, len(apps))
	for _, a := range apps {
		if a.OrganizationKey != identity.OrganizationKey {
			continue
		}
		envs, err := s.store.ListEnvironments(r.Context(), a.Key)
		if err != nil {
			writeError(w, err)
			return
		}
		view := applicationView{
			Key: a.Key, Name: a.Name, Subdomain: a.Subdomain,
			Environments: make([]environmentView, 0, len(envs)),
		}
		for _, e := range envs {
			view.Environments = append(view.Environments, s.environmentViewOf(r.Context(), a.OrganizationKey, e))
		}
		out = append(out, view)
	}
	writeJSON(w, http.StatusOK, map[string]any{"applications": out})
}

type createApplicationRequest struct {
	Name      string `json:"name"`
	Subdomain string `json:"subdomain"`
}

// handleApplicationConnections lists the READY Connection choices of the
// session Organization. Any authenticated member may read it; the payload is
// minimal and unrelated to UC-04 management (platform-only).
func (s *Server) handleApplicationConnections(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.sessionIdentity(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	choices, err := s.applications.ListChoices(r.Context(), identity.OrganizationKey)
	if err != nil {
		writeError(w, err)
		return
	}
	type choiceView struct {
		Key    string `json:"key"`
		Name   string `json:"name"`
		Kind   string `json:"kind"`
		Status string `json:"status"`
	}
	views := make([]choiceView, 0, len(choices.Connections))
	for _, c := range choices.Connections {
		views = append(views, choiceView{Key: c.Key, Name: c.Name, Kind: string(c.Kind), Status: string(c.Status)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"connections": views, "defaultConnectionKey": choices.DefaultConnectionKey})
}

func (s *Server) handleCreateApplication(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.sessionIdentity(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	// Only Name and Subdomain are accepted; Organization and role come from the
	// session. The execution target is set later, once per Environment
	// (UC-01 BR-13). Everything else, including connectionKey, is rejected.
	var req createApplicationRequest
	if err := decodeSingleObject(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "request must be one JSON object with only name and subdomain"})
		return
	}
	cmd := appcreate.CreateCommand{OrganizationKey: identity.OrganizationKey, Name: req.Name, Subdomain: req.Subdomain, BaseDomain: s.seedOptions.BaseDomain}
	result, err := s.applications.Create(r.Context(), cmd)
	if err != nil {
		writeCreateApplicationError(w, err)
		return
	}
	view := applicationView{Key: result.Application.Key, Name: result.Application.Name, Subdomain: result.Application.Subdomain}
	for _, env := range result.Environments {
		view.Environments = append(view.Environments, s.environmentViewOf(r.Context(), identity.OrganizationKey, env))
	}
	writeJSON(w, http.StatusCreated, map[string]any{"application": view})
}

type setConnectionRequest struct {
	ConnectionKey   json.RawMessage `json:"connectionKey"`
	ExpectedVersion json.RawMessage `json:"expectedVersion"`
}

// Fixed 409 sentences; the UI distinguishes them to reload the Environment.
const (
	conflictAlreadyConfigured = "this environment already has a connection; it cannot be changed"
	conflictStaleVersion      = "the environment changed since it was loaded; reload and try again"
)

// handleSetEnvironmentConnection sets the Environment target exactly once
// (UC-01 ES-03..06). Session Organization scopes the Application.
func (s *Server) handleSetEnvironmentConnection(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.sessionIdentity(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	var req setConnectionRequest
	if err := decodeSingleObject(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "request must be one JSON object with only connectionKey and expectedVersion"})
		return
	}
	var key string
	if req.ConnectionKey == nil || json.Unmarshal(req.ConnectionKey, &key) != nil || strings.TrimSpace(key) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "connectionKey must be a non-blank string", "field": "connectionKey"})
		return
	}
	var version int64
	if req.ExpectedVersion == nil || json.Unmarshal(req.ExpectedVersion, &version) != nil || version <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "expectedVersion must be a positive integer", "field": "expectedVersion"})
		return
	}
	env, err := s.applications.SetConnection(r.Context(), appcreate.SetConnectionCommand{
		OrganizationKey: identity.OrganizationKey, ApplicationKey: r.PathValue("id"), EnvironmentKey: r.PathValue("env"),
		ConnectionKey: key, ExpectedVersion: version,
	})
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, map[string]any{"environment": s.environmentViewOf(r.Context(), identity.OrganizationKey, env)})
	case errors.Is(err, appcreate.ErrAlreadyConfigured):
		writeJSON(w, http.StatusConflict, map[string]any{"error": conflictAlreadyConfigured, "code": "ALREADY_CONFIGURED"})
	case errors.Is(err, appcreate.ErrStaleVersion):
		writeJSON(w, http.StatusConflict, map[string]any{"error": conflictStaleVersion, "code": "STALE_VERSION"})
	case errors.Is(err, persistence.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
	default:
		writeCreateApplicationError(w, err)
	}
}

// writeUnconfigured answers the target failures of Preview and Deploy with a
// safe 422 on connectionKey; it reports whether it did. The messages are fixed
// sentences and never name the Connection.
func writeUnconfigured(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, planning.ErrEnvironmentUnconfigured):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": appsvc.FailureUnconfigured, "field": "connectionKey", "code": "ENVIRONMENT_UNCONFIGURED"})
	case errors.Is(err, target.ErrInconsistent):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": appsvc.FailureInconsistent, "field": "connectionKey", "code": "ENVIRONMENT_TARGET_INCONSISTENT"})
	case errors.Is(err, appsvc.ErrConnectionNotReady):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": appsvc.FailureNotReady, "field": "connectionKey", "code": "CONNECTION_NOT_READY"})
	default:
		return false
	}
	return true
}

// decodeSingleObject decodes exactly one JSON object with no unknown fields
// and nothing after it.
func decodeSingleObject(r *http.Request, into any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return err
	}
	if trimmed := bytes.TrimSpace(body); len(trimmed) == 0 || trimmed[0] != '{' {
		return errors.New("body is not a JSON object")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(into); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("body has trailing data after the JSON object")
	}
	return nil
}

// writeCreateApplicationError maps UC-01 failures to 400 (field validation),
// 409 (duplicate Name/Subdomain) and 422 (selected target unavailable).
func writeCreateApplicationError(w http.ResponseWriter, err error) {
	body := map[string]any{"error": err.Error()}
	var fieldErr *appcreate.FieldError
	if errors.As(err, &fieldErr) {
		body["field"] = fieldErr.Field
	}
	switch {
	case errors.Is(err, appcreate.ErrInvalid):
		writeJSON(w, http.StatusBadRequest, body)
	case errors.Is(err, appcreate.ErrDuplicate):
		writeJSON(w, http.StatusConflict, body)
	case errors.Is(err, appcreate.ErrTargetNotReady):
		writeJSON(w, http.StatusUnprocessableEntity, body)
	default:
		writeError(w, err)
	}
}
func (s *Server) handleGetApplication(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.sessionIdentity(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	app, err := s.store.GetApplication(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	if app.OrganizationKey != identity.OrganizationKey {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	envs, err := s.store.ListEnvironments(r.Context(), app.Key)
	if err != nil {
		writeError(w, err)
		return
	}
	view := applicationView{Key: app.Key, Name: app.Name, Subdomain: app.Subdomain}
	for _, env := range envs {
		view.Environments = append(view.Environments, s.environmentViewOf(r.Context(), app.OrganizationKey, env))
	}
	writeJSON(w, http.StatusOK, map[string]any{"application": view})
}

func (s *Server) handleScoreSamples(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"order":   seed.AcceptanceOrder(),
		"samples": seed.AcceptanceScores(s.seedOptions),
	})
}

type createDeploymentRequest struct {
	ApplicationKey string         `json:"applicationKey"`
	EnvironmentKey string         `json:"environmentKey"`
	WorkloadID     string         `json:"workloadId"`
	Actor          string         `json:"actor"`
	RunID          string         `json:"runId"`
	Score          map[string]any `json:"score"`
	ScoreBefore    map[string]any `json:"scoreBefore"`
}

func (s *Server) handleCreateDeployment(w http.ResponseWriter, r *http.Request) {
	var req createDeploymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
		return
	}
	if req.ApplicationKey == "" || req.EnvironmentKey == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "applicationKey and environmentKey are required"})
		return
	}
	if req.Score == nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "score is required"})
		return
	}
	actor := req.Actor
	if actor == "" {
		actor = "web-console"
	}
	// The console has no run id of its own: the process was started with one.
	runID := req.RunID
	if runID == "" {
		runID = s.seedOptions.RunID
	}
	result, err := s.deployments.DeployWorkload(r.Context(), appsvc.DeployCommand{
		OrganizationKey: s.seedOptions.OrganizationKey,
		ApplicationKey:  req.ApplicationKey,
		EnvironmentKey:  req.EnvironmentKey,
		WorkloadID:      req.WorkloadID,
		ScoreAfter:      req.Score,
		ScoreBefore:     req.ScoreBefore,
		Actor:           actor,
		RunID:           runID,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) handleListDeployments(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.sessionIdentity(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	list, err := s.queries.ListDeployments(r.Context(), appsvc.ListDeploymentsQuery{
		OrganizationKey: identity.OrganizationKey, ApplicationKey: r.PathValue("id"),
		EnvironmentKey: r.PathValue("env"), Status: domain.Status(r.URL.Query().Get("status")),
	})
	if err != nil {
		if errors.Is(err, appsvc.ErrInvalidStatus) {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		writeDeploymentReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deployments": list})
}

func (s *Server) handleGetDeployment(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.sessionIdentity(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	view, err := s.queries.GetDeployment(r.Context(), appsvc.GetDeploymentQuery{
		OrganizationKey: identity.OrganizationKey, ApplicationKey: r.PathValue("id"),
		EnvironmentKey: r.PathValue("env"), DeploymentID: r.PathValue("deployment"),
	})
	if err != nil {
		writeDeploymentReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

// handleUI serves the production bundle and falls back to index.html so browser
// routes such as /ui/deployments/<id> work on reload.
func (s *Server) handleUI(w http.ResponseWriter, r *http.Request) {
	if s.uiDir == "" {
		http.Error(w, "web console bundle is not configured", http.StatusNotFound)
		return
	}
	rel := strings.TrimPrefix(r.URL.Path, "/ui/")
	if rel == "" {
		rel = "index.html"
	}
	clean := filepath.Clean(rel)
	if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	candidate := filepath.Join(s.uiDir, clean)
	if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
		http.ServeFile(w, r, candidate)
		return
	}
	index := filepath.Join(s.uiDir, "index.html")
	if _, err := os.Stat(index); err != nil {
		http.Error(w, "web console bundle is not built", http.StatusNotFound)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, index)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// writeError maps domain errors onto HTTP status codes. The MVP uses sentinel
// errors for persistence and message prefixes for validation failures.
// writeDeploymentReadError maps UC-09 read failures without echoing storage
// details: any scope mismatch is a plain 404, anything else a retryable 500.
func writeDeploymentReadError(w http.ResponseWriter, err error) {
	if errors.Is(err, persistence.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "deployment not found"})
		return
	}
	log.Printf("deployment read failed: %v", err)
	writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "deployment read failed; retry"})
}

func writeError(w http.ResponseWriter, err error) {
	if writeUnconfigured(w, err) {
		return
	}
	switch {
	case errors.Is(err, persistence.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]any{"error": err.Error()})
	case errors.Is(err, persistence.ErrVersionConflict):
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error()})
	case errors.Is(err, pending.ErrStalePreview):
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error()})
	case errors.Is(err, appconfig.ErrConflict):
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error()})
	case errors.Is(err, catalog.ErrDuplicate):
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error()})
	case errors.Is(err, catalog.ErrInvalid):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
	case errors.Is(err, connectionapp.ErrDuplicate):
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error()})
	case errors.Is(err, connectionapp.ErrInvalid):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
	case errors.Is(err, connectionapp.ErrVerification):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": err.Error()})
	case errors.Is(err, configmemory.ErrUnavailable):
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": err.Error()})
	case isValidation(err):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
	}
}

func isValidation(err error) bool {
	msg := err.Error()
	for _, prefix := range []string{"score:", "planning:", "environment:", "resource:", "application:", "configuration:", "workloadconfig:", "pending:"} {
		if strings.HasPrefix(msg, prefix) {
			return true
		}
	}
	return false
}
