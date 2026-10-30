package http

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	driftapp "orchestrator/internal/application/drift"
	"orchestrator/internal/application/tfexec"
	domain "orchestrator/internal/domain/drift"
	"orchestrator/internal/platform/ids"
)

func (s *Server) handleQueueDriftScan(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.platformActor(w, r); !ok {
		return
	}
	app, env, ok := s.authorizedConfiguration(w, r)
	if !ok {
		return
	}
	if s.drift == nil {
		writeCode(w, codeServiceUnavailable, "")
		return
	}
	var body struct {
		RequestID                  string `json:"requestId"`
		ExpectedEnvironmentVersion int64  `json:"expectedEnvironmentVersion"`
	}
	if !decodeStrict(w, r, 1<<10, &body) {
		return
	}
	if !ids.ValidUUID(body.RequestID) || body.ExpectedEnvironmentVersion < 1 {
		writeCode(w, codeInvalidRequest, "")
		return
	}
	identity, _ := s.sessionIdentity(r)
	scan, err := s.drift.Queue(r.Context(), driftapp.QueueCommand{OrganizationKey: identity.OrganizationKey, ApplicationKey: app, EnvironmentKey: env, RequestID: body.RequestID, ExpectedEnvironmentVersion: body.ExpectedEnvironmentVersion})
	if err != nil {
		s.writeDriftError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"scanId": scan.ID, "scan": scan})
}
func (s *Server) handleListDriftScans(w http.ResponseWriter, r *http.Request) {
	app, env, ok := s.authorizedConfiguration(w, r)
	if !ok {
		return
	}
	if s.drift == nil {
		writeCode(w, codeServiceUnavailable, "")
		return
	}
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 200 {
			writeCode(w, codeInvalidRequest, "")
			return
		}
		limit = n
	}
	cursor := r.URL.Query().Get("cursor")
	if cursor != "" && !ids.ValidUUID(cursor) {
		writeCode(w, codeInvalidRequest, "")
		return
	}
	identity, _ := s.sessionIdentity(r)
	rows, err := s.drift.List(r.Context(), identity.OrganizationKey, app, env, cursor, limit)
	if err != nil {
		s.writeDriftError(w, err)
		return
	}
	next := ""
	if len(rows) == limit {
		next = rows[len(rows)-1].ID
	}
	views := make([]driftScanView, 0, len(rows))
	for _, row := range rows {
		views = append(views, s.driftView(row, time.Now().UTC()))
	}
	writeJSON(w, http.StatusOK, map[string]any{"scans": views, "nextCursor": next})
}
func (s *Server) handleGetDriftScan(w http.ResponseWriter, r *http.Request) {
	app, env, ok := s.authorizedConfiguration(w, r)
	if !ok {
		return
	}
	if s.drift == nil {
		writeCode(w, codeServiceUnavailable, "")
		return
	}
	id := r.PathValue("scan")
	if !ids.ValidUUID(id) {
		writeCode(w, codeInvalidRequest, "")
		return
	}
	identity, _ := s.sessionIdentity(r)
	scan, err := s.drift.Get(r.Context(), identity.OrganizationKey, app, env, id)
	if err != nil {
		s.writeDriftError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"scan": s.driftView(scan, time.Now().UTC())})
}

type driftScanView struct {
	domain.Scan
	Fresh           bool          `json:"fresh"`
	EffectiveResult domain.Result `json:"effectiveResult"`
}

func (s *Server) driftView(scan domain.Scan, now time.Time) driftScanView {
	interval := s.drift.Interval
	if interval <= 0 {
		interval = 15 * time.Minute
	}
	fresh := scan.Status == domain.Succeeded && scan.FinishedAt != nil && !scan.FinishedAt.After(now) && now.Sub(*scan.FinishedAt) <= 3*interval
	result := scan.Result
	if !fresh {
		result = domain.Unknown
	}
	return driftScanView{scan, fresh, result}
}
func (s *Server) writeDriftError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, tfexec.ErrBaselineUnavailable):
		writeCode(w, codeDriftBaselineUnavailable, "")
		return
	case errors.Is(err, tfexec.ErrRepairBlocked):
		writeCode(w, codeDriftRepairBlocked, "")
		return
	case errors.Is(err, tfexec.ErrRepairStale):
		writeCode(w, codeDriftRepairStale, "")
		return
	case errors.Is(err, tfexec.ErrNeedsAttention):
		writeCode(w, codeRecoveryIncomplete, "")
		return
	}
	if errors.Is(err, domain.ErrScan) {
		writeCode(w, codeInvalidRequest, "")
		return
	}
	s.writeError(w, err)
}
