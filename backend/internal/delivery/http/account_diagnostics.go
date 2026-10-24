package http

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"orchestrator/internal/application/accountdiag"
	"orchestrator/internal/application/authentication"
	domain "orchestrator/internal/domain/diagnostics"
	"orchestrator/internal/domain/identity"
	ports "orchestrator/internal/ports/diagnostics"
	"orchestrator/internal/ports/persistence"
)

// Account diagnostics API (ADR-022, contract account-diagnostics.md). Reports
// are a separate aggregate; nothing here changes the Connection record. The
// public DTOs below are explicit projections: lease, owner, recovery and
// storage fields never leave the server.

func (s *Server) registerAccountDiagnosticRoutes() {
	s.mux.HandleFunc("POST /api/v1/driver-accounts/{key}/diagnostics", s.handleStartDiagnostic)
	s.mux.HandleFunc("GET /api/v1/driver-accounts/{key}/diagnostics", s.handleListDiagnostics)
	s.mux.HandleFunc("GET /api/v1/driver-accounts/{key}/diagnostics/{id}", s.handleGetDiagnostic)
	s.mux.HandleFunc("POST /api/v1/driver-accounts/{key}/diagnostics/{id}/cancel", s.handleCancelDiagnostic)
}

type diagContextView struct {
	ApplicationKey  string `json:"applicationKey"`
	ApplicationName string `json:"applicationName,omitempty"`
	EnvironmentKey  string `json:"environmentKey,omitempty"`
	EnvironmentType string `json:"environmentType,omitempty"`
}

type diagScopeView struct {
	Purpose        string            `json:"purpose"`
	Mode           string            `json:"mode"`
	Region         string            `json:"region,omitempty"`
	UseCases       []string          `json:"useCases"`
	Context        *diagContextView  `json:"context,omitempty"`
	AccountKey     string            `json:"accountKey"`
	AccountName    string            `json:"accountName"`
	AccountType    string            `json:"accountType"`
	AccountVersion int64             `json:"accountVersion"`
	Identity       map[string]string `json:"identity,omitempty"`
}

type diagFindingView struct {
	ID               string            `json:"id"`
	Group            string            `json:"group"`
	Verdict          string            `json:"verdict"`
	Reason           string            `json:"reason"`
	Facts            map[string]string `json:"facts"`
	Remediation      string            `json:"remediation,omitempty"`
	AffectedUseCases []string          `json:"affectedUseCases"`
	DependsOn        []string          `json:"dependsOn"`
	ObservedAt       time.Time         `json:"observedAt"`
	DurationMillis   int64             `json:"durationMs"`
}

type diagResourceView struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
	UID       string `json:"uid,omitempty"`
	State     string `json:"state"`
}

type diagCountsView struct {
	Pass    int `json:"pass"`
	Fail    int `json:"fail"`
	Blocked int `json:"blocked"`
	Unknown int `json:"unknown"`
	NA      int `json:"na"`
	Pending int `json:"pending"`
}

type diagSummaryView struct {
	ReportID      string         `json:"reportId"`
	Scope         diagScopeView  `json:"scope"`
	Status        string         `json:"status"`
	Counts        diagCountsView `json:"counts"`
	Ready         bool           `json:"ready"`
	CleanupStatus string         `json:"cleanupStatus"`
	Stale         bool           `json:"stale"`
	CreatedAt     time.Time      `json:"createdAt"`
	FinishedAt    *time.Time     `json:"finishedAt,omitempty"`
}

type diagReportView struct {
	ID           string            `json:"id"`
	Scope        diagScopeView     `json:"scope"`
	Status       string            `json:"status"`
	Progress     map[string]int    `json:"progress"`
	Counts       diagCountsView    `json:"counts"`
	Ready        bool              `json:"ready"`
	Findings     []diagFindingView `json:"findings"`
	Cleanup      diagCleanupView   `json:"cleanup"`
	Limits       []string          `json:"limits"`
	Stale        bool              `json:"stale"`
	StaleReasons []string          `json:"staleReasons"`
	CreatedAt    time.Time         `json:"createdAt"`
	StartedAt    *time.Time        `json:"startedAt,omitempty"`
	FinishedAt   *time.Time        `json:"finishedAt,omitempty"`
	UpdatedAt    time.Time         `json:"updatedAt"`
}

type diagCleanupView struct {
	Status    string             `json:"status"`
	Resources []diagResourceView `json:"resources"`
}

func orEmpty(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

func diagScopeOf(sc domain.Scope) diagScopeView {
	v := diagScopeView{Purpose: string(sc.Purpose), Mode: string(sc.Mode), Region: sc.Region, UseCases: orEmpty(sc.UseCases), AccountKey: sc.AccountKey, AccountName: sc.AccountName,
		AccountType: sc.AccountType, AccountVersion: sc.AccountVersion, Identity: sc.Identity}
	if c := sc.Context; c != nil {
		v.Context = &diagContextView{ApplicationKey: c.ApplicationKey, ApplicationName: c.ApplicationName, EnvironmentKey: c.EnvironmentKey, EnvironmentType: c.EnvironmentType}
	}
	return v
}

func diagCountsOf(c domain.Counts) diagCountsView {
	return diagCountsView{Pass: c.Pass, Fail: c.Fail, Blocked: c.Blocked, Unknown: c.Unknown, NA: c.NA, Pending: c.Pending}
}

// limitsOf lists what this report cannot prove, derived from its scope.
func limitsOf(sc domain.Scope) []string {
	limits := []string{}
	if sc.Mode == domain.ModeConditions {
		limits = append(limits, "CONDITIONS_ONLY")
	}
	if sc.AccountType == "AWS" {
		limits = append(limits, "POLICY_SIMULATION_ONLY", "QUOTA_NOT_COMPARED")
		if sc.Mode == domain.ModeExecution {
			limits = append(limits, "EXECUTION_UNSUPPORTED_AWS")
		}
	}
	if sc.Purpose == domain.PurposeUseCase && sc.Context == nil {
		limits = append(limits, "CONTEXT_NOT_PROVIDED")
	}
	return limits
}

// staleReasons compares the pinned versions with what is current now.
func (s *Server) staleReasons(ctx context.Context, org string, sc domain.Scope, accountVersion int64) []string {
	reasons := []string{}
	if accountVersion > 0 && accountVersion != sc.AccountVersion {
		reasons = append(reasons, "ACCOUNT_CHANGED")
	}
	if c := sc.Context; c != nil {
		if app, err := s.store.GetApplication(ctx, c.ApplicationKey); err == nil && app.OrganizationKey == org && app.Version != c.ApplicationVersion {
			reasons = append(reasons, "APPLICATION_CHANGED")
		}
		if c.EnvironmentKey != "" {
			if env, err := s.store.GetEnvironment(ctx, c.ApplicationKey, c.EnvironmentKey); err == nil && env.Version != c.EnvironmentVersion {
				reasons = append(reasons, "ENVIRONMENT_CHANGED")
			}
		}
	}
	return reasons
}

func (s *Server) diagReportOf(ctx context.Context, org string, r domain.Report, accountVersion int64) diagReportView {
	findings := make([]diagFindingView, 0, len(r.Findings))
	for _, f := range r.Findings {
		facts := map[string]string{}
		for k, v := range f.Facts {
			facts[k] = v
		}
		findings = append(findings, diagFindingView{ID: f.ID, Group: f.Group, Verdict: string(f.Verdict), Reason: f.Reason, Facts: facts, Remediation: f.Remediation,
			AffectedUseCases: orEmpty(f.AffectedUseCases), DependsOn: orEmpty(f.DependsOn), ObservedAt: f.ObservedAt, DurationMillis: f.DurationMillis})
	}
	resources := make([]diagResourceView, 0, len(r.ProbeResources))
	for _, p := range r.ProbeResources {
		resources = append(resources, diagResourceView{Kind: p.Kind, Namespace: p.Namespace, Name: p.Name, UID: p.UID, State: p.State})
	}
	reasons := s.staleReasons(ctx, org, r.Scope, accountVersion)
	return diagReportView{
		ID: r.ID, Scope: diagScopeOf(r.Scope), Status: string(r.Status), Progress: map[string]int{"completed": r.CompletedChecks, "total": r.TotalChecks},
		Counts: diagCountsOf(r.Counts()), Ready: r.Ready(), Findings: findings, Cleanup: diagCleanupView{Status: string(r.CleanupStatus), Resources: resources},
		Limits: limitsOf(r.Scope), Stale: len(reasons) > 0, StaleReasons: reasons, CreatedAt: r.CreatedAt, StartedAt: r.StartedAt, FinishedAt: r.FinishedAt, UpdatedAt: r.UpdatedAt,
	}
}

func (s *Server) diagSummaryOf(ctx context.Context, org string, r domain.Report, accountVersion int64) diagSummaryView {
	reasons := s.staleReasons(ctx, org, r.Scope, accountVersion)
	return diagSummaryView{ReportID: r.ID, Scope: diagScopeOf(r.Scope), Status: string(r.Status), Counts: diagCountsOf(r.Counts()), Ready: r.Ready(), CleanupStatus: string(r.CleanupStatus),
		Stale: len(reasons) > 0, CreatedAt: r.CreatedAt, FinishedAt: r.FinishedAt}
}

// platformIdentity is the PE/Admin gate that also returns the actor.
func (s *Server) platformIdentity(w http.ResponseWriter, r *http.Request) (authentication.Identity, bool) {
	actor, ok := s.sessionIdentity(r)
	if !ok {
		writeCode(w, codeUnauthorized, "")
		return authentication.Identity{}, false
	}
	if actor.Role != identity.RolePlatformEngineer && actor.Role != identity.RoleAdmin {
		writeCode(w, codeForbidden, "")
		return authentication.Identity{}, false
	}
	return actor, true
}

func (s *Server) diagnosticsReady(w http.ResponseWriter) bool {
	if s.diagnostics == nil {
		writeCode(w, codeServiceUnavailable, "")
		return false
	}
	return true
}

func (s *Server) handleStartDiagnostic(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.platformIdentity(w, r)
	if !ok || !s.diagnosticsReady(w) {
		return
	}
	body, ok := decodeStrictBody(w, r, accountSmallEnvelopeLimit, map[string]memberKind{
		"version": kindInteger, "purpose": kindString, "mode": kindString, "region": kindString, "useCases": kindStringList, "application": kindString, "environment": kindString,
	})
	if !ok {
		return
	}
	created, err := s.diagnostics.Start(r.Context(), accountdiag.StartCommand{
		Organization: actor.OrganizationKey, UserID: actor.UserID, AccountKey: r.PathValue("key"),
		Request: domain.Request{Version: body.number("version"), Purpose: domain.Purpose(body.text("purpose")), Mode: domain.Mode(body.text("mode")), Region: body.text("region"),
			UseCases: body.list("useCases"), Application: body.text("application"), Environment: body.text("environment")},
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, s.diagReportOf(r.Context(), actor.OrganizationKey, created, created.AccountVersion))
}

func (s *Server) currentAccountVersion(ctx context.Context, org, key string) int64 {
	if conn, err := s.connections.GetAccount(ctx, org, key); err == nil {
		return conn.Version
	}
	return 0
}

func (s *Server) handleGetDiagnostic(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.platformIdentity(w, r)
	if !ok || !s.diagnosticsReady(w) {
		return
	}
	report, err := s.diagnostics.Get(r.Context(), actor.OrganizationKey, r.PathValue("key"), r.PathValue("id"))
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.diagReportOf(r.Context(), actor.OrganizationKey, report, s.currentAccountVersion(r.Context(), actor.OrganizationKey, report.AccountKey)))
}

func (s *Server) handleCancelDiagnostic(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.platformIdentity(w, r)
	if !ok || !s.diagnosticsReady(w) {
		return
	}
	report, err := s.diagnostics.Cancel(r.Context(), actor.OrganizationKey, r.PathValue("key"), r.PathValue("id"))
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.diagReportOf(r.Context(), actor.OrganizationKey, report, s.currentAccountVersion(r.Context(), actor.OrganizationKey, report.AccountKey)))
}

func (s *Server) handleListDiagnostics(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.platformIdentity(w, r)
	if !ok || !s.diagnosticsReady(w) {
		return
	}
	query := ports.ListQuery{Cursor: r.URL.Query().Get("cursor")}
	switch purpose := domain.Purpose(r.URL.Query().Get("purpose")); purpose {
	case "", domain.PurposeGeneral, domain.PurposeUseCase:
		query.Purpose = purpose
	default:
		writeCode(w, codeDiagnosticScope, "purpose")
		return
	}
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100 {
			writeCode(w, codeDiagnosticScope, "")
			return
		}
		query.Limit = n
	}
	key := r.PathValue("key")
	page, err := s.diagnostics.List(r.Context(), actor.OrganizationKey, key, query)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidScope) {
			writeCode(w, codeDiagnosticScope, "")
			return
		}
		s.writeError(w, err)
		return
	}
	version := s.currentAccountVersion(r.Context(), actor.OrganizationKey, key)
	items := make([]diagSummaryView, 0, len(page.Reports))
	for _, report := range page.Reports {
		items = append(items, s.diagSummaryOf(r.Context(), actor.OrganizationKey, report, version))
	}
	writeJSON(w, http.StatusOK, map[string]any{"reports": items, "nextCursor": page.NextCursor})
}

// latestSummaries is the safe projection added to the account list/detail.
func (s *Server) latestSummaries(ctx context.Context, org string, versions map[string]int64) map[string]diagSummaryView {
	out := map[string]diagSummaryView{}
	if s.diagnostics == nil {
		return out
	}
	latest, err := s.diagnostics.Latest(ctx, org)
	if err != nil {
		return out
	}
	for key, report := range latest {
		out[key] = s.diagSummaryOf(ctx, org, report, versions[key])
	}
	return out
}

var _ = persistence.ErrNotFound
