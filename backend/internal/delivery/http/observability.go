package http

import (
	"errors"
	"net/http"
	"strconv"

	obs "orchestrator/internal/application/observability"
	"orchestrator/internal/domain/telemetry"
)

func (s *Server) observabilityRoutes() {
	for _, kind := range []string{"overview", "logs", "metrics"} {
		s.mux.HandleFunc("GET /api/v1/applications/{id}/environments/{env}/observability/"+kind, func(w http.ResponseWriter, r *http.Request) { s.handleObservability(w, r, kind) })
	}
}

func (s *Server) handleObservability(w http.ResponseWriter, r *http.Request, kind string) {
	org, app, env, ok := s.scopedEnvironment(w, r)
	if !ok {
		return
	}
	if s.observability == nil {
		writeCode(w, codeServiceUnavailable, "")
		return
	}
	q := r.URL.Query()
	allowed := map[string]bool{"from": true, "to": true, "workload": true, "q": true, "level": true, "limit": true, "cursor": true}
	for key, values := range q {
		if !allowed[key] || len(values) != 1 {
			writeCode(w, codeInvalidRequest, "")
			return
		}
	}
	p := obs.Params{From: q.Get("from"), To: q.Get("to"), Workload: q.Get("workload"), Search: q.Get("q"), Level: q.Get("level"), Cursor: q.Get("cursor")}
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			writeCode(w, codeInvalidRequest, "")
			return
		}
		p.Limit = n
	}
	view, err := s.observability.Query(r.Context(), telemetry.Scope{OrganizationKey: org, ApplicationKey: app, EnvironmentKey: env}, kind, p)
	if err != nil {
		var field *obs.FieldError
		switch {
		case errors.As(err, &field):
			writeCode(w, codeInvalidRequest, "")
		case errors.Is(err, obs.ErrUnavailable):
			writeCode(w, codeServiceUnavailable, "")
		default:
			s.writeError(w, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, view)
}
