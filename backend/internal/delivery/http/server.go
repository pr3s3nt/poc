// Package http exposes the orchestrator JSON API under /api/v1/ and serves the
// Orchestrator Web Console production bundle under /ui/.
package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	effectivenessapp "orchestrator/internal/application/effectiveness"
	observabilityapp "orchestrator/internal/application/observability"
	"orchestrator/internal/ports/renderregistry"
	"os"
	"path/filepath"
	"strings"

	appcreate "orchestrator/internal/application/application"
	"orchestrator/internal/application/authentication"
	"orchestrator/internal/application/catalog"
	appconfig "orchestrator/internal/application/configuration"
	connectionapp "orchestrator/internal/application/connection"
	appsvc "orchestrator/internal/application/deployment"
	"orchestrator/internal/application/envops"
	"orchestrator/internal/application/pending"
	"orchestrator/internal/application/preview"
	"orchestrator/internal/application/resourcecleanup"
	"orchestrator/internal/application/secretstores"
	workloadconfig "orchestrator/internal/application/workloadconfig"
	domain "orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/ports/credentials"
	"orchestrator/internal/ports/moduleregistry"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/seed"
)

// Server wires the HTTP handlers to the application services.
type Server struct {
	observability   *observabilityapp.Service
	deployments     *appsvc.Service
	queries         *appsvc.QueryService
	auth            *authentication.Service
	applications    *appcreate.Service
	catalog         *catalog.Service
	connections     *connectionapp.Service
	secretStores    *secretstores.Service
	operations      *envops.Manager
	configurations  *appconfig.Service
	workloads       *workloadconfig.Service
	pending         *pending.Service
	previews        *preview.Service
	resourceCleanup *resourcecleanup.Service
	effectiveness   *effectivenessapp.Service
	store           persistence.Store
	seedOptions     seed.Options
	uiDir           string
	mux             *http.ServeMux
}

// Config configures the HTTP server.
type Config struct {
	Observability *observabilityapp.Service
	RenderBundles map[string]resource.RenderBundle
	// RenderRegistry inspects Git render bundles and reports availability
	// (ADR-021); nil disables Git bundle inspection.
	RenderRegistry renderregistry.Registry
	Deployments    *appsvc.Service
	Queries        *appsvc.QueryService
	Authentication *authentication.Service
	Applications   *appcreate.Service
	// KubeconfigVerifier and ConnectionCredentials enable UC-04 upload
	// registration. Without a credential store it answers 503.
	KubeconfigVerifier    connectionapp.KubeconfigVerifier
	AWSVerifier           connectionapp.AWSVerifier
	EC2RoleProvider       connectionapp.EC2RoleProvider
	ConnectionCredentials credentials.Store
	Configurations        *appconfig.Service
	SecretStores          *secretstores.Service
	Operations            *envops.Manager
	Workloads             *workloadconfig.Service
	Pending               *pending.Service
	Previews              *preview.Service
	// ResourceCleanup serves the scoped resource detail, the deletion preview,
	// the confirmed deletion and its progress (UC-07 DEL-*, UC-09 RES-*).
	ResourceCleanup *resourcecleanup.Service
	Store           persistence.Store
	// ModuleRegistry is the verified Terraform module registry used by
	// Definition registration (verify) and capability metadata (read-only).
	ModuleRegistry moduleregistry.Registry
	SeedOptions    seed.Options
	UIDir          string
}

// NewServer builds the HTTP handler tree.
func NewServer(cfg Config) *Server {
	s := &Server{
		observability:   cfg.Observability,
		deployments:     cfg.Deployments,
		queries:         cfg.Queries,
		auth:            cfg.Authentication,
		applications:    cfg.Applications,
		catalog:         catalog.NewService(cfg.Store, cfg.ModuleRegistry),
		connections:     connectionapp.NewService(cfg.Store),
		secretStores:    cfg.SecretStores,
		operations:      cfg.Operations,
		configurations:  cfg.Configurations,
		workloads:       cfg.Workloads,
		pending:         cfg.Pending,
		previews:        cfg.Previews,
		resourceCleanup: cfg.ResourceCleanup,
		effectiveness:   effectivenessapp.New(cfg.Store, effectivenessapp.OriginOf(cfg.SeedOptions.Profile)),
		store:           cfg.Store,
		seedOptions:     cfg.SeedOptions,
		uiDir:           cfg.UIDir,
		mux:             http.NewServeMux(),
	}
	s.connections.SetKubeconfigRegistration(cfg.KubeconfigVerifier, cfg.ConnectionCredentials)
	s.connections.SetAWSRegistration(cfg.AWSVerifier)
	s.connections.SetEC2RoleProvider(cfg.EC2RoleProvider)

	s.catalog.SetRenderBundles(cfg.RenderBundles)
	if cfg.RenderRegistry != nil {
		s.catalog.SetRenderRegistry(cfg.RenderRegistry)
	}
	s.routes()
	return s
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/v1/healthz", s.handleHealth)
	s.effectivenessRoutes()
	s.observabilityRoutes()
	s.mux.HandleFunc("POST /api/v1/auth/sign-in", s.handleSignIn)
	s.mux.HandleFunc("GET /api/v1/auth/session", s.handleSession)
	s.mux.HandleFunc("POST /api/v1/auth/sign-out", s.handleSignOut)
	s.mux.HandleFunc("GET /api/v1/applications", s.handleApplications)
	s.mux.HandleFunc("POST /api/v1/applications", s.handleCreateApplication)
	s.mux.HandleFunc("GET /api/v1/applications/{id}", s.handleGetApplication)
	s.mux.HandleFunc("GET /api/v1/secret-stores", s.handleListSecretStores)
	s.mux.HandleFunc("POST /api/v1/secret-stores", s.handleRegisterSecretStore)
	s.mux.HandleFunc("GET /api/v1/secret-store-choices", s.handleSecretStoreChoices)
	s.mux.HandleFunc("PUT /api/v1/applications/{id}/environments/{env}/secret-store", s.handleSetSecretStore)
	s.mux.HandleFunc("GET /api/v1/applications/{id}/environments/{env}/operations", s.handleListOperations)
	s.mux.HandleFunc("POST /api/v1/applications/{id}/environments/{env}/operations/{opid}/recover", s.handleRecoverOperation)
	s.mux.HandleFunc("GET /api/v1/applications/{id}/environments/{env}/configuration", s.handleGetConfiguration)
	s.mux.HandleFunc("POST /api/v1/applications/{id}/environments/{env}/configuration/keys", s.handleCreateConfigurationKey)
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
	s.mux.HandleFunc("PUT /api/v1/resource-definitions/{key}", s.handleUpdateResourceDefinition)
	s.mux.HandleFunc("POST /api/v1/resource-definitions/matching-preview", s.handleMatchingPreview)
	s.mux.HandleFunc("GET /api/v1/resource-definition-capabilities", s.handleDefinitionCapabilities)
	s.mux.HandleFunc("POST /api/v1/terraform-modules/inspect", s.handleInspectTerraformModule)
	s.mux.HandleFunc("GET /api/v1/render-bundles", s.handleRenderBundles)
	s.mux.HandleFunc("POST /api/v1/render-bundles/inspect", s.handleInspectRenderBundle)
	s.mux.HandleFunc("GET /api/v1/render-bundles/{id}", s.handleGetRenderBundle)
	s.mux.HandleFunc("GET /api/v1/connections", s.handleConnections)
	s.mux.HandleFunc("POST /api/v1/connections/kubernetes", s.handleRegisterKubernetesConnection)
	s.mux.HandleFunc("POST /api/v1/connections/aws", s.handleRegisterAWSConnection)
	s.mux.HandleFunc("POST /api/v1/connections/kubernetes/inspect", s.handleInspectKubeconfig)
	s.registerDriverAccountRoutes()
	s.mux.HandleFunc("POST /api/v1/deployments", s.handleCreateDeployment)
	s.mux.HandleFunc("GET /api/v1/applications/{id}/environments/{env}/resources", s.handleListResources)
	s.mux.HandleFunc("GET /api/v1/applications/{id}/environments/{env}/resources/{resourceId}", s.handleGetResource)
	s.mux.HandleFunc("POST /api/v1/applications/{id}/environments/{env}/resources/{resourceId}/deletion-preview", s.handleDeletionPreview)
	s.mux.HandleFunc("POST /api/v1/applications/{id}/environments/{env}/resources/{resourceId}/deletions", s.handleDeletions)
	s.mux.HandleFunc("GET /api/v1/applications/{id}/environments/{env}/resource-cleanups/{operationId}", s.handleGetResourceCleanup)
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
	// An Environment carries no deployment Connection, execution profile or region (ADR-017).
	InfrastructureScope string `json:"infrastructureScope"`
	// ADR-012: independent secret-store selection, target generation and the
	// operation currently owning the Environment (nonsecret).
	SecretStoreKey   string         `json:"secretStoreKey"`
	SecretStoreName  string         `json:"secretStoreName,omitempty"`
	TargetGeneration int64          `json:"targetGeneration"`
	DraftVersion     int64          `json:"draftVersion"`
	ActiveOperation  *operationView `json:"activeOperation,omitempty"`
}

type applicationView struct {
	Key          string            `json:"key"`
	Name         string            `json:"name"`
	Subdomain    string            `json:"subdomain"`
	Environments []environmentView `json:"environments"`
}

// environmentViewOf projects surviving Environment metadata; deployment targets are pinned per resource.
func (s *Server) environmentViewOf(ctx context.Context, organizationKey string, e environment.Environment) environmentView {
	v := environmentView{
		Key: e.Key, Name: e.Name, Type: e.Type, NamespaceIdentity: e.NamespaceIdentity,
		CurrentDeploymentSetID: e.CurrentDeploymentSetID, Version: e.Version,
		InfrastructureScope: string(e.Scope()),
		SecretStoreKey:      e.SecretStoreKey, TargetGeneration: e.TargetGeneration, DraftVersion: e.DraftVersion,
	}
	if e.SecretStoreKey != "" {
		v.SecretStoreName = e.SecretStoreKey
		if store, err := s.store.GetSecretStore(ctx, organizationKey, e.SecretStoreKey); err == nil && store.Name != "" {
			v.SecretStoreName = store.Name
		}
	}
	if e.Busy() {
		if op, err := s.store.GetOperation(ctx, e.ActiveOperationID); err == nil {
			view := operationViewOf(op)
			v.ActiveOperation = &view
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
		writeCode(w, codeInvalidRequest, "")
		return
	}
	token, identity, err := s.auth.SignIn(r.Context(), req.Username, req.Password)
	if err != nil {
		writeCode(w, codeInvalidCredentials, "")
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
		writeCode(w, codeUnauthorized, "")
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
		writeCode(w, codeUnauthorized, "")
		return
	}
	apps, err := s.store.ListApplications(r.Context())
	if err != nil {
		s.writeError(w, err)
		return
	}
	out := make([]applicationView, 0, len(apps))
	for _, a := range apps {
		if a.OrganizationKey != identity.OrganizationKey {
			continue
		}
		envs, err := s.store.ListEnvironments(r.Context(), a.Key)
		if err != nil {
			s.writeError(w, err)
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

func (s *Server) handleCreateApplication(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.sessionIdentity(r)
	if !ok {
		writeCode(w, codeUnauthorized, "")
		return
	}
	// Only Name and Subdomain are accepted; Organization and role come from the
	// session. Placement comes from Resource Definitions, never from an Environment
	// (ADR-017). Everything else, including connectionKey, is rejected.
	var req createApplicationRequest
	if err := decodeSingleObject(r, &req); err != nil {
		writeCode(w, codeInvalidRequest, "")
		return
	}
	cmd := appcreate.CreateCommand{OrganizationKey: identity.OrganizationKey, Name: req.Name, Subdomain: req.Subdomain, BaseDomain: s.seedOptions.BaseDomain}
	result, err := s.applications.Create(r.Context(), cmd)
	if err != nil {
		s.writeError(w, err)
		return
	}
	view := applicationView{Key: result.Application.Key, Name: result.Application.Name, Subdomain: result.Application.Subdomain}
	for _, env := range result.Environments {
		view.Environments = append(view.Environments, s.environmentViewOf(r.Context(), identity.OrganizationKey, env))
	}
	writeJSON(w, http.StatusCreated, map[string]any{"application": view})
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

func (s *Server) handleGetApplication(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.sessionIdentity(r)
	if !ok {
		writeCode(w, codeUnauthorized, "")
		return
	}
	app, err := s.store.GetApplication(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeError(w, err)
		return
	}
	if app.OrganizationKey != identity.OrganizationKey {
		writeCode(w, codeNotFound, "")
		return
	}
	envs, err := s.store.ListEnvironments(r.Context(), app.Key)
	if err != nil {
		s.writeError(w, err)
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
		writeCode(w, codeInvalidRequest, "")
		return
	}
	if req.ApplicationKey == "" || req.EnvironmentKey == "" {
		writeCode(w, codeInvalidRequest, "")
		return
	}
	if req.Score == nil {
		writeCode(w, codeInvalidRequest, "")
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
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) handleListDeployments(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.sessionIdentity(r)
	if !ok {
		writeCode(w, codeUnauthorized, "")
		return
	}
	list, err := s.queries.ListDeployments(r.Context(), appsvc.ListDeploymentsQuery{
		OrganizationKey: identity.OrganizationKey, ApplicationKey: r.PathValue("id"),
		EnvironmentKey: r.PathValue("env"), Status: domain.Status(r.URL.Query().Get("status")),
		ReviewRequestID: r.URL.Query().Get("reviewRequestId"),
	})
	if err != nil {
		if errors.Is(err, appsvc.ErrInvalidReviewRequestID) {
			writeCode(w, codeInvalidRequest, "reviewRequestId")
			return
		}
		if errors.Is(err, appsvc.ErrInvalidStatus) {
			writeCode(w, codeInvalidRequest, "status")
			return
		}
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deployments": list})
}

func (s *Server) handleGetDeployment(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.sessionIdentity(r)
	if !ok {
		writeCode(w, codeUnauthorized, "")
		return
	}
	view, err := s.queries.GetDeployment(r.Context(), appsvc.GetDeploymentQuery{
		OrganizationKey: identity.OrganizationKey, ApplicationKey: r.PathValue("id"),
		EnvironmentKey: r.PathValue("env"), DeploymentID: r.PathValue("deployment"),
	})
	if err != nil {
		s.writeError(w, err)
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
