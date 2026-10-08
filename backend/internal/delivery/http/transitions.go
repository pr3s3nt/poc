package http

import (
	"errors"
	"net/http"

	"orchestrator/internal/application/transition"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/ports/persistence"
)

type transitionBody struct {
	DestinationKey      string                        `json:"destinationKey"`
	Mode                string                        `json:"mode"`
	Mappings            []environment.ResourceMapping `json:"mappings"`
	Token               string                        `json:"token"`
	AcknowledgeDowntime bool                          `json:"acknowledgeDowntime"`
}

// scopedEnvironment authenticates and scopes the Application Environment of a
// request; it writes the response and reports false on failure.
func (s *Server) scopedEnvironment(w http.ResponseWriter, r *http.Request) (org, appKey, envKey string, ok bool) {
	identity, ok := s.sessionIdentity(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return "", "", "", false
	}
	appKey, envKey = r.PathValue("id"), r.PathValue("env")
	app, err := s.store.GetApplication(r.Context(), appKey)
	if err != nil || app.OrganizationKey != identity.OrganizationKey || (envKey != "staging" && envKey != "production") {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return "", "", "", false
	}
	return identity.OrganizationKey, appKey, envKey, true
}

func (s *Server) transitionRequest(org, appKey, envKey string, body transitionBody) transition.Request {
	return transition.Request{
		OrganizationKey: org, ApplicationKey: appKey, EnvironmentKey: envKey, DestinationKey: body.DestinationKey,
		Mode: environment.TransitionMode(body.Mode), Mappings: body.Mappings,
	}
}

func (s *Server) handlePreviewTransition(w http.ResponseWriter, r *http.Request) {
	org, appKey, envKey, ok := s.scopedEnvironment(w, r)
	if !ok {
		return
	}
	var body transitionBody
	if !decodeStrict(w, r, 1<<20, &body) {
		return
	}
	preview, err := s.transitions.Preview(r.Context(), s.transitionRequest(org, appKey, envKey, body))
	if err != nil {
		s.writeTransitionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"preview": preview})
}

func (s *Server) handleExecuteTransition(w http.ResponseWriter, r *http.Request) {
	org, appKey, envKey, ok := s.scopedEnvironment(w, r)
	if !ok {
		return
	}
	identity, _ := s.sessionIdentity(r)
	var body transitionBody
	if !decodeStrict(w, r, 1<<20, &body) {
		return
	}
	detail, err := s.transitions.Execute(r.Context(), transition.ExecuteRequest{
		Request: s.transitionRequest(org, appKey, envKey, body), Token: body.Token, AcknowledgeDowntime: body.AcknowledgeDowntime, Actor: identity.Username,
	})
	if err != nil {
		s.writeTransitionError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"transition": detail})
}

func (s *Server) handleListTransitions(w http.ResponseWriter, r *http.Request) {
	org, appKey, envKey, ok := s.scopedEnvironment(w, r)
	if !ok {
		return
	}
	list, err := s.transitions.List(r.Context(), org, appKey, envKey)
	if err != nil {
		s.writeTransitionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"transitions": list})
}

func (s *Server) handleGetTransition(w http.ResponseWriter, r *http.Request) {
	org, appKey, envKey, ok := s.scopedEnvironment(w, r)
	if !ok {
		return
	}
	detail, err := s.transitions.Get(r.Context(), org, appKey, envKey, r.PathValue("tid"))
	if err != nil {
		s.writeTransitionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"transition": detail})
}

func (s *Server) handleCleanupSource(w http.ResponseWriter, r *http.Request) {
	org, appKey, envKey, ok := s.scopedEnvironment(w, r)
	if !ok {
		return
	}
	detail, err := s.transitions.CleanupSource(r.Context(), org, appKey, envKey, r.PathValue("tid"))
	if err != nil {
		s.writeTransitionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"transition": detail})
}

func (s *Server) handleListOperations(w http.ResponseWriter, r *http.Request) {
	_, appKey, envKey, ok := s.scopedEnvironment(w, r)
	if !ok {
		return
	}
	_, _ = s.operations.Sweep(r.Context())
	ops, err := s.store.ListOperations(r.Context(), appKey, envKey, 20)
	if err != nil {
		s.writeError(w, err)
		return
	}
	views := make([]operationView, 0, len(ops))
	for _, op := range ops {
		views = append(views, operationViewOf(op))
	}
	writeJSON(w, http.StatusOK, map[string]any{"operations": views})
}

func (s *Server) handleRecoverOperation(w http.ResponseWriter, r *http.Request) {
	org, appKey, envKey, ok := s.scopedEnvironment(w, r)
	if !ok {
		return
	}
	identity, _ := s.sessionIdentity(r)
	var body struct {
		PriorExecutionStopped bool `json:"priorExecutionStopped"`
	}
	if !decodeStrict(w, r, 1<<10, &body) {
		return
	}
	if !body.PriorExecutionStopped {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "confirm that the interrupted operation has stopped before recovery", "field": "priorExecutionStopped"})
		return
	}
	result, err := s.transitions.Recover(r.Context(), org, appKey, envKey, r.PathValue("opid"), identity.Username)
	if errors.Is(err, transition.ErrRecoveryIncomplete) {
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "code": "RECOVERY_INCOMPLETE", "recovery": result})
		return
	}
	if err != nil {
		s.writeTransitionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"recovery": result})
}

// writeTransitionError maps transition failures to fixed, safe responses.
func (s *Server) writeTransitionError(w http.ResponseWriter, err error) {
	var unsupported *transition.UnsupportedError
	switch {
	case errors.As(err, &unsupported):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": err.Error(), "code": "TRANSITION_UNSUPPORTED"})
	case errors.Is(err, transition.ErrNoRuntime):
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "code": "NO_RUNTIME"})
	case errors.Is(err, transition.ErrSameDestination), errors.Is(err, transition.ErrDestinationFailed):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": err.Error(), "field": "destinationKey"})
	case errors.Is(err, transition.ErrAcknowledge):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": err.Error(), "field": "acknowledgeDowntime"})
	case errors.Is(err, transition.ErrStaleToken):
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "code": "STALE_PREVIEW"})
	case errors.Is(err, transition.ErrCleanupRefused):
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "code": "CLEANUP_REFUSED"})
	case errors.Is(err, persistence.ErrRecoveryUnconfirmed):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": err.Error(), "field": "priorExecutionStopped"})
	case errors.Is(err, transition.ErrNotRecoverable):
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "code": "NOT_RECOVERABLE"})
	case transition.IsInvalid(err):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
	case errors.Is(err, persistence.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
	default:
		if !s.writeEnvironmentError(w, err) {
			s.writeError(w, err)
		}
	}
}
