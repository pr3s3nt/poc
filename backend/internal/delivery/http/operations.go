package http

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	appsvc "orchestrator/internal/application/deployment"
	"orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
)

func (s *Server) handleListOperations(w http.ResponseWriter, r *http.Request) {
	_, appKey, envKey, ok := s.scopedEnvironment(w, r)
	if !ok {
		return
	}
	if requestID := r.URL.Query().Get("reviewRequestId"); requestID != "" {
		if !ids.ValidUUID(requestID) {
			writeCode(w, codeInvalidRequest, "reviewRequestId")
			return
		}
		op, err := s.store.GetReviewOperation(r.Context(), appKey, envKey, strings.ToLower(requestID))
		if err != nil && !errors.Is(err, persistence.ErrNotFound) {
			s.writeError(w, err)
			return
		}
		views := []operationView{}
		if err == nil {
			views = append(views, operationViewOf(op))
		}
		writeJSON(w, http.StatusOK, map[string]any{"operations": views})
		return
	}
	if s.operations != nil {
		_, _ = s.operations.Sweep(r.Context())
	}
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

// Recovery only settles interrupted workload delivery. Cleanup evidence that
// cannot be reconstructed is held for an operator; it is never reported clean.
func (s *Server) handleRecoverOperation(w http.ResponseWriter, r *http.Request) {
	_, appKey, envKey, ok := s.scopedEnvironment(w, r)
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
		writeCode(w, codeInvalidRequest, "priorExecutionStopped")
		return
	}
	if s.operations == nil {
		s.writeError(w, persistence.ErrNotFound)
		return
	}
	op, err := s.store.GetOperation(r.Context(), r.PathValue("opid"))
	if err != nil || op.ApplicationKey != appKey || op.EnvironmentKey != envKey {
		s.writeError(w, persistence.ErrNotFound)
		return
	}
	if op.Status != environment.OpInterrupted {
		s.writeError(w, persistence.ErrVersionConflict)
		return
	}
	app, err := s.store.GetApplication(r.Context(), appKey)
	if err != nil || app.OrganizationKey != identity.OrganizationKey {
		s.writeError(w, persistence.ErrNotFound)
		return
	}
	_, _ = s.operations.Sweep(r.Context())
	op, err = s.store.BeginRecovery(r.Context(), op.ID, s.operations.InstanceID()+"-recovery-"+ids.New()[:8], strings.TrimSpace(identity.Username))
	if err != nil {
		s.writeError(w, err)
		return
	}
	lease := s.operations.Adopt(r.Context(), op)
	result := map[string]any{"operationId": op.ID, "kind": string(op.Kind)}
	if op.Kind != environment.OpDeploy && op.Kind != environment.OpRemove && op.Kind != environment.OpRoutes {
		reason := "recovery cannot prove cleanup state; the Environment remains held"
		_ = lease.Suspend(reason)
		writeFailure(w, failure{code: codeRecoveryIncomplete, extra: map[string]any{"recovery": map[string]any{"operationId": op.ID, "outcome": reason}}})
		return
	}
	list, err := s.store.ListDeployments(lease.Ctx, app.Key, envKey)
	if err == nil {
		for _, item := range list {
			if item.Status == deployment.StatusSucceeded || item.Status == deployment.StatusFailed {
				continue
			}
			now := time.Now().UTC()
			item.Status, item.FailureReason, item.FinishedAt = deployment.StatusFailed, appsvc.FailureRuntime, &now
			if err = s.store.SaveDeployment(lease.Ctx, item); err != nil {
				break
			}
		}
	}
	if err == nil {
		var instances []deployment.WorkloadInstance
		instances, err = s.store.ListWorkloadInstances(lease.Ctx, app.Key+"/"+envKey)
		for _, item := range instances {
			if item.Status != deployment.InstanceApplying && item.Status != deployment.InstanceRemoving {
				continue
			}
			item.Status = deployment.InstanceFailed
			if err = s.store.UpsertWorkloadInstance(lease.Ctx, item); err != nil {
				break
			}
		}
	}
	if err == nil {
		err = s.store.SetPublicRoutesPending(lease.Ctx, app.Key, envKey, true)
	}
	if err != nil {
		reason := fmt.Sprintf("recovery could not complete (%T); the Environment remains held", err)
		_ = lease.Suspend(reason)
		writeFailure(w, failure{code: codeRecoveryIncomplete, extra: map[string]any{"recovery": map[string]any{"operationId": op.ID, "outcome": reason}}})
		return
	}
	if err := lease.End(environment.OpRecovered, "interrupted delivery records were marked failed; preview the desired state before retrying"); err != nil {
		s.writeError(w, err)
		return
	}
	result["outcome"] = "interrupted deliveries were marked failed; preview the desired state before retrying"
	writeJSON(w, http.StatusOK, map[string]any{"recovery": result})
}
