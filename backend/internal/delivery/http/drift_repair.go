package http

import (
	"net/http"

	driftapp "orchestrator/internal/application/drift"
	"orchestrator/internal/platform/ids"
)

func (s *Server) handlePrepareDriftRepair(w http.ResponseWriter, r *http.Request) {
	org, app, env, ok := s.scopedEnvironment(w, r)
	if !ok {
		return
	}
	if _, ok := s.platformActor(w, r); !ok {
		return
	}
	if s.drift == nil {
		writeCode(w, codeServiceUnavailable, "")
		return
	}
	var body struct {
		ScanID                     string   `json:"scanId"`
		InstanceIDs                []string `json:"instanceIds"`
		RequestID                  string   `json:"requestId"`
		ExpectedEnvironmentVersion int64    `json:"expectedEnvironmentVersion"`
	}
	if !decodeStrict(w, r, 16<<10, &body) {
		return
	}
	actor, _ := s.sessionIdentity(r)
	view, err := s.drift.PrepareRepair(r.Context(), driftapp.PrepareRepairCommand{OrganizationKey: org, ApplicationKey: app, EnvironmentKey: env, ActorID: actor.UserID, ScanID: body.ScanID, InstanceIDs: body.InstanceIDs, RequestID: body.RequestID, ExpectedEnvironmentVersion: body.ExpectedEnvironmentVersion})
	if err != nil {
		s.writeDriftError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, view)
}
func (s *Server) handleGetDriftRepair(w http.ResponseWriter, r *http.Request) {
	org, app, env, ok := s.scopedEnvironment(w, r)
	if !ok {
		return
	}
	if _, ok := s.platformActor(w, r); !ok {
		return
	}
	if s.drift == nil {
		writeCode(w, codeServiceUnavailable, "")
		return
	}
	id := r.PathValue("review")
	if !ids.ValidUUID(id) {
		writeCode(w, codeInvalidRequest, "")
		return
	}
	actor, _ := s.sessionIdentity(r)
	view, err := s.drift.GetRepair(r.Context(), org, app, env, actor.UserID, id)
	if err != nil {
		s.writeDriftError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}
func (s *Server) handleConfirmDriftRepair(w http.ResponseWriter, r *http.Request) {
	org, app, env, ok := s.scopedEnvironment(w, r)
	if !ok {
		return
	}
	if _, ok := s.platformActor(w, r); !ok {
		return
	}
	if s.drift == nil {
		writeCode(w, codeServiceUnavailable, "")
		return
	}
	var body struct {
		ReviewID    string `json:"reviewId"`
		ReviewToken string `json:"reviewToken"`
		RequestID   string `json:"requestId"`
	}
	if !decodeStrict(w, r, 2<<10, &body) {
		return
	}
	actor, _ := s.sessionIdentity(r)
	op, err := s.drift.ConfirmRepair(r.Context(), driftapp.ConfirmRepairCommand{OrganizationKey: org, ApplicationKey: app, EnvironmentKey: env, ActorID: actor.UserID, ReviewID: body.ReviewID, ReviewToken: body.ReviewToken, RequestID: body.RequestID})
	if err != nil {
		s.writeDriftError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"operationId": op})
}
