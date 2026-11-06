package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"orchestrator/internal/application/observability"
	"orchestrator/internal/domain/monitoring"
	"orchestrator/internal/domain/telemetry"
	"orchestrator/internal/ports/persistence"
)

func (s *Server) monitoringRoutes() {
	s.mux.HandleFunc("GET /api/v1/monitoring/connections", s.handleMonitoringConnections)
	s.mux.HandleFunc("POST /api/v1/monitoring/connections", s.handleMonitoringConnections)
	s.mux.HandleFunc("PUT /api/v1/monitoring/connections/{connection}", s.handleMonitoringConnections)
	s.mux.HandleFunc("DELETE /api/v1/monitoring/connections/{connection}", s.handleMonitoringConnections)
	s.mux.HandleFunc("POST /api/v1/monitoring/connections/{connection}/test", s.handleMonitoringTest)
	s.mux.HandleFunc("GET /api/v1/applications/{id}/environments/{env}/monitoring", s.handleMonitoringRead)
	s.mux.HandleFunc("GET /api/v1/applications/{id}/environments/{env}/monitoring/binding", s.handleMonitoringBinding)
	s.mux.HandleFunc("PUT /api/v1/applications/{id}/environments/{env}/monitoring/binding", s.handleMonitoringBinding)
}
func publicMonitoring(c monitoring.Connection) map[string]any {
	return map[string]any{"id": c.ID, "key": c.Key, "name": c.Name, "revision": c.Revision, "prometheusUrl": c.PrometheusURL, "alertmanagerUrl": c.AlertmanagerURL, "grafanaUrl": c.GrafanaURL, "dashboardUid": c.DashboardUID, "clusterUid": c.ClusterUID, "ruleLabels": c.RuleLabels, "rules": c.Rules, "verifiedAt": c.VerifiedAt, "credentialConfigured": c.CredentialRef != ""}
}
func monitoringBody(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(v) != nil || d.Decode(&struct{}{}) != io.EOF {
		writeCode(w, codeInvalidRequest, "")
		return false
	}
	return true
}
func (s *Server) monitoringError(w http.ResponseWriter, e error) {
	var field *observability.FieldError
	switch {
	case errors.Is(e, monitoring.ErrInvalid), errors.As(e, &field):
		writeCode(w, codeInvalidRequest, "")
	case errors.Is(e, persistence.ErrVersionConflict):
		s.writeError(w, e)
	case errors.Is(e, persistence.ErrNotFound):
		s.writeError(w, e)
	default:
		writeCode(w, codeServiceUnavailable, "")
	}
}
func (s *Server) handleMonitoringConnections(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.platformIdentity(w, r)
	if !ok {
		return
	}
	if s.monitoring == nil || s.monitoring.Repo == nil {
		writeCode(w, codeServiceUnavailable, "")
		return
	}
	if r.Method == http.MethodGet {
		connections, e := s.monitoring.Connections(r.Context(), actor.OrganizationKey)
		if e != nil {
			s.monitoringError(w, e)
			return
		}
		out := []map[string]any{}
		for _, c := range connections {
			out = append(out, publicMonitoring(c))
		}
		writeJSON(w, 200, map[string]any{"connections": out})
		return
	}
	var body struct {
		Connection       monitoring.Connection `json:"connection"`
		ExpectedRevision int64                 `json:"expectedRevision"`
		Token            string                `json:"token"`
	}
	if !monitoringBody(w, r, &body) {
		return
	}
	body.Connection.ID = r.PathValue("connection")
	body.Connection.CredentialRef = ""
	if r.Method == http.MethodDelete {
		e := s.store.Transact(r.Context(), func(ctx context.Context) error {
			return s.monitoring.Repo.DeleteMonitoringConnection(ctx, actor.OrganizationKey, body.Connection.ID, body.ExpectedRevision)
		})
		if e != nil {
			s.monitoringError(w, e)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	c, e := s.monitoring.Save(r.Context(), actor.OrganizationKey, body.Connection, body.ExpectedRevision, body.Token)
	if e != nil {
		s.monitoringError(w, e)
		return
	}
	writeJSON(w, 200, publicMonitoring(c))
}
func (s *Server) handleMonitoringTest(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.platformIdentity(w, r)
	if !ok {
		return
	}
	out, e := s.monitoring.Test(r.Context(), actor.OrganizationKey, r.PathValue("connection"))
	if e != nil {
		s.monitoringError(w, e)
		return
	}
	writeJSON(w, 200, out)
}
func (s *Server) handleMonitoringRead(w http.ResponseWriter, r *http.Request) {
	org, app, env, ok := s.scopedEnvironment(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	for key, values := range q {
		if (key != "from" && key != "to" && key != "workload") || len(values) != 1 {
			writeCode(w, codeInvalidRequest, "")
			return
		}
	}
	out, e := s.monitoring.Read(r.Context(), telemetry.Scope{OrganizationKey: org, ApplicationKey: app, EnvironmentKey: env}, observability.Params{From: q.Get("from"), To: q.Get("to"), Workload: q.Get("workload")})
	if e != nil {
		s.monitoringError(w, e)
		return
	}
	writeJSON(w, 200, out)
}
func (s *Server) handleMonitoringBinding(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.platformIdentity(w, r)
	if !ok {
		return
	}
	org, app, env, ok := s.scopedEnvironment(w, r)
	if !ok {
		return
	}
	if s.monitoring.Repo == nil {
		writeCode(w, codeServiceUnavailable, "")
		return
	}
	if r.Method == http.MethodGet {
		out, e := s.monitoring.Repo.GetMonitoringBinding(r.Context(), org, app, env)
		if errors.Is(e, persistence.ErrNotFound) {
			writeJSON(w, 200, map[string]any{"binding": nil})
			return
		}
		if e != nil {
			s.monitoringError(w, e)
			return
		}
		writeJSON(w, 200, map[string]any{"binding": out})
		return
	}
	var body struct {
		ConnectionID     string `json:"connectionId"`
		ExpectedRevision int64  `json:"expectedRevision"`
	}
	if !monitoringBody(w, r, &body) {
		return
	}
	out, e := s.monitoring.Bind(r.Context(), telemetry.Scope{OrganizationKey: actor.OrganizationKey, ApplicationKey: app, EnvironmentKey: env}, body.ConnectionID, body.ExpectedRevision)
	if e != nil {
		s.monitoringError(w, e)
		return
	}
	writeJSON(w, 200, out)
}
