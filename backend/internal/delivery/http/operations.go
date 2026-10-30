package http

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	appsvc "orchestrator/internal/application/deployment"
	deliverydomain "orchestrator/internal/domain/delivery"
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
	policyRecovery, _ := op.Detail["deliveryPolicyOnly"].(bool)
	// Stage is retained for attempts admitted before the explicit detail flag.
	policyRecovery = policyRecovery || op.Stage == "DELIVERY_POLICY"
	if op.Kind == environment.OpDriftScan {
		if _, ok := s.platformActor(w, r); !ok {
			return
		}
	}
	if transfer, _ := op.Detail["deliveryTransfer"].(bool); transfer || policyRecovery {
		if _, ok := s.platformActor(w, r); !ok {
			return
		}
	}
	app, err := s.store.GetApplication(r.Context(), appKey)
	if err != nil || app.OrganizationKey != identity.OrganizationKey {
		s.writeError(w, persistence.ErrNotFound)
		return
	}
	// Generic workload recovery cannot release an unresolved CD mutation. A
	// transfer uses its dedicated forward protocol below; ordinary submissions
	// remain held until exact provider outcome recovery is implemented.
	if transfer, _ := op.Detail["deliveryTransfer"].(bool); !transfer && !policyRecovery {
		if repo, ok := s.store.(persistence.DeliveryExecutionRepository); ok {
			organization, lookupErr := s.store.GetOrganization(r.Context(), app.OrganizationKey)
			if lookupErr != nil {
				s.writeError(w, lookupErr)
				return
			}
			currentEnv, lookupErr := s.store.GetEnvironment(r.Context(), appKey, envKey)
			if lookupErr != nil {
				s.writeError(w, lookupErr)
				return
			}
			records, lookupErr := repo.ListDeliveryExecutions(r.Context(), organization.ID, currentEnv.ID)
			if lookupErr != nil {
				s.writeError(w, lookupErr)
				return
			}
			for _, record := range records {
				if record.Mutation.OperationID == op.ID && (record.Mutation.Status == deliverydomain.IntentRunning || record.Mutation.Status == deliverydomain.IntentUnknown) {
					submission := record.Mutation.Action == deliverydomain.Submit && record.Receipt.IntentID == record.Mutation.ID
					removal := record.Mutation.Action == deliverydomain.Withdraw && record.Receipt.IntentID != "" && len(record.Inventory) > 0
					if !submission && !removal {
						writeCode(w, codeRecoveryIncomplete, "")
						return
					}
				}
			}
		}
	}
	_, _ = s.operations.Sweep(r.Context())
	op, err = s.store.BeginRecovery(r.Context(), op.ID, s.operations.InstanceID()+"-recovery-"+ids.New()[:8], strings.TrimSpace(identity.Username))
	if err != nil {
		s.writeError(w, err)
		return
	}
	lease := s.operations.Adopt(r.Context(), op)
	result := map[string]any{"operationId": op.ID, "kind": string(op.Kind)}
	if op.Kind == environment.OpDriftScan {
		scanID, _ := op.Detail["driftScanId"].(string)
		if s.drift == nil || scanID == "" {
			_ = lease.Suspend("drift scan recovery unavailable")
			writeCode(w, codeRecoveryIncomplete, "")
			return
		}
		if err := s.drift.RecoverStoppedScan(lease.Ctx, app.OrganizationKey, appKey, envKey, scanID); err != nil {
			_ = lease.Suspend("drift scan outcome requires further recovery")
			writeCode(w, codeRecoveryIncomplete, "")
			return
		}
		if err := lease.End(environment.OpRecovered, "interrupted scan remains UNKNOWN; request a fresh scan"); err != nil {
			s.writeError(w, err)
			return
		}
		result["outcome"] = "RECOVERED"
		writeJSON(w, http.StatusOK, map[string]any{"recovery": result})
		return
	}
	if op.Kind != environment.OpDeploy && op.Kind != environment.OpRemove && op.Kind != environment.OpRoutes {
		reason := "recovery cannot prove cleanup state; the Environment remains held"
		_ = lease.Suspend(reason)
		writeFailure(w, failure{code: codeRecoveryIncomplete, extra: map[string]any{"recovery": map[string]any{"operationId": op.ID, "outcome": reason}}})
		return
	}
	if policyRecovery {
		env, err := s.store.GetEnvironment(lease.Ctx, appKey, envKey)
		if err == nil {
			err = s.deployments.ResumeDeliveryPolicy(lease.Ctx, app.OrganizationKey, env, lease.Stage)
		}
		if err != nil {
			_ = lease.Suspend("delivery policy requires further recovery")
			writeCode(w, codeRecoveryIncomplete, "")
			return
		}
		if err := lease.End(environment.OpRecovered, ""); err != nil {
			s.writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"recovery": map[string]any{"operationId": op.ID, "kind": string(op.Kind), "outcome": "RECOVERED"}})
		return
	}
	if transfer, _ := op.Detail["deliveryTransfer"].(bool); transfer {
		env, err := s.store.GetEnvironment(lease.Ctx, appKey, envKey)
		if err == nil {
			err = s.deployments.ResumeDeliveryTransfer(lease.Ctx, app.OrganizationKey, env, lease.Stage)
		}
		if err != nil {
			_ = lease.Suspend("delivery transfer requires further recovery")
			writeCode(w, codeRecoveryIncomplete, "")
			return
		}
		if err := lease.End(environment.OpRecovered, ""); err != nil {
			s.writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"recovery": map[string]any{"operationId": op.ID, "kind": string(op.Kind), "outcome": "RECOVERED"}})
		return
	}
	if s.reconciliation != nil {
		env, err := s.store.GetEnvironment(lease.Ctx, appKey, envKey)
		if err == nil {
			err = s.reconciliation.RecoverRemovals(lease.Ctx, app.OrganizationKey, env)
		}
		if err == nil {
			err = s.reconciliation.RecoverSubmissions(lease.Ctx, app.OrganizationKey, env)
		}
		if err != nil {
			_ = lease.Suspend("delivery submission requires further recovery")
			writeCode(w, codeRecoveryIncomplete, "")
			return
		}
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
