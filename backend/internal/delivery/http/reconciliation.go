package http

import (
	"net/http"
	appdelivery "orchestrator/internal/application/delivery"
	domain "orchestrator/internal/domain/delivery"
	"orchestrator/internal/platform/ids"
	"strconv"
	"time"
)

func (s *Server) handleReconciliation(w http.ResponseWriter, r *http.Request) {
	app, env, ok := s.authorizedConfiguration(w, r)
	if !ok {
		return
	}
	if s.reconciliation == nil {
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
	rows, current, err := s.reconciliation.StoredObservations(r.Context(), identity.OrganizationKey, app, env, cursor, limit)
	if err != nil {
		s.writeError(w, err)
		return
	}
	type observationView struct {
		UnitID     string             `json:"unitId"`
		WorkloadID string             `json:"workloadId"`
		Sequence   int64              `json:"sequence"`
		LastKnown  domain.Observation `json:"lastKnown"`
		Effective  domain.Observation `json:"effective"`
		Fresh      bool               `json:"fresh"`
		Reason     string             `json:"reasonCode,omitempty"`
	}
	views := []observationView{}
	now := time.Now().UTC()
	for _, row := range rows {
		observed := row.Observation
		observed.PayloadDigest = ""
		effective := observed.Effective(now, appdelivery.ObservationInterval)
		fresh := !observed.ObservedAt.IsZero() && !observed.ObservedAt.After(now) && now.Sub(observed.ObservedAt) <= 3*appdelivery.ObservationInterval
		views = append(views, observationView{row.UnitID, row.WorkloadID, row.Sequence, observed, effective, fresh, row.ReasonCode})
	}
	next := ""
	if len(rows) == limit {
		next = rows[len(rows)-1].UnitID
	}
	writeJSON(w, http.StatusOK, map[string]any{"desired": current.DesiredDelivery.Effective(), "applied": current.AppliedDelivery.Effective(), "units": views, "nextCursor": next, "intervalSeconds": int(appdelivery.ObservationInterval.Seconds())})
}
