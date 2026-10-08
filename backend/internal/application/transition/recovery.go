package transition

import (
	"context"
	"fmt"
	"strings"
	"time"

	appsvc "orchestrator/internal/application/deployment"
	appdomain "orchestrator/internal/domain/application"
	"orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
)

// RecoveryResult is the visible outcome of a recovery.
type RecoveryResult struct {
	OperationID        string   `json:"operationId"`
	Kind               string   `json:"kind"`
	Outcome            string   `json:"outcome"`
	Compensation       []string `json:"compensation,omitempty"`
	CompensationFailed []string `json:"compensationFailed,omitempty"`
}

// Recover takes over an INTERRUPTED operation (heartbeat stale; never merely
// time-expired) and cleans up what its dead owner left. Stale heartbeat is only
// an interruption signal: the caller must explicitly confirm the prior
// execution has stopped (confirmedBy is recorded in the operation). The claim
// moves to RECOVERING with a higher fence, so a paused previous owner can no
// longer release it or write under it, even before its next heartbeat.
func (s *Service) Recover(ctx context.Context, org, appKey, envKey, operationID, confirmedBy string) (RecoveryResult, error) {
	if strings.TrimSpace(confirmedBy) == "" {
		return RecoveryResult{}, persistence.ErrRecoveryUnconfirmed
	}
	app, err := s.store.GetApplication(ctx, appKey)
	if err != nil || app.OrganizationKey != org {
		return RecoveryResult{}, fmt.Errorf("%w: application %q", persistence.ErrNotFound, appKey)
	}
	_, _ = s.ops.Sweep(ctx)
	op, err := s.store.GetOperation(ctx, operationID)
	if err != nil {
		return RecoveryResult{}, err
	}
	if op.ApplicationKey != appKey || op.EnvironmentKey != envKey {
		return RecoveryResult{}, fmt.Errorf("%w: operation %q", persistence.ErrNotFound, operationID)
	}
	if op.Status != environment.OpInterrupted {
		return RecoveryResult{}, ErrNotRecoverable
	}
	op, err = s.store.BeginRecovery(ctx, operationID, s.ops.InstanceID()+"-recovery-"+ids.New()[:8], confirmedBy)
	if err != nil {
		if err == persistence.ErrVersionConflict {
			return RecoveryResult{}, ErrNotRecoverable
		}
		return RecoveryResult{}, err
	}
	lease := s.ops.Adopt(ctx, op)
	result := RecoveryResult{OperationID: op.ID, Kind: string(op.Kind)}
	rctx := lease.Ctx
	var recoverErr error
	switch op.Kind {
	case environment.OpTransition:
		result.Outcome, recoverErr = s.recoverTransition(rctx, app, op, &result)
	case environment.OpDeploy, environment.OpRemove, environment.OpRoutes:
		result.Outcome, recoverErr = s.recoverDeploy(rctx, app, envKey, &result)
	case environment.OpCleanup:
		result.Outcome, recoverErr = s.recoverCleanup(rctx, appKey, envKey, &result)
	default:
		result.Outcome = "nothing was committed; the claim was released"
	}
	if recoverErr != nil || len(result.CompensationFailed) > 0 {
		// Missing evidence or failed compensation keeps the Environment held and
		// INTERRUPTED; recovery can be repeated once the cause is fixed.
		reason := "recovery incomplete"
		if recoverErr != nil {
			reason = recoverErr.Error()
		}
		result.Outcome = "recovery did not complete: " + reason + "; the environment stays held until recovery succeeds"
		if err := lease.Suspend(result.Outcome); err != nil {
			return result, err
		}
		return result, ErrRecoveryIncomplete
	}
	if err := lease.End(environment.OpRecovered, result.Outcome); err != nil {
		return result, err
	}
	return result, nil
}

func (s *Service) recoverTransition(ctx context.Context, app appdomain.Application, op environment.Operation, result *RecoveryResult) (string, error) {
	list, err := s.store.ListTransitions(ctx, op.ApplicationKey, op.EnvironmentKey)
	if err != nil {
		return "", fmt.Errorf("the transition records could not be read")
	}
	var t *environment.Transition
	for i := range list {
		if list[i].OperationID == op.ID {
			t = &list[i]
		}
	}
	if t == nil {
		return "", fmt.Errorf("no transition record exists for this operation, so what it changed is unknown")
	}
	env, err := s.store.GetEnvironment(ctx, op.ApplicationKey, op.EnvironmentKey)
	if err != nil {
		return "", fmt.Errorf("the environment could not be read")
	}
	committed := env.ConnectionKey == t.Destination.ConnectionKey && env.TargetGeneration == t.Destination.Generation
	if committed {
		t.Status, t.Stage, t.SourceState = environment.TransitionSucceeded, environment.StageSucceeded, environment.SourceQuiesced
		t.Failure = "the process stopped after the cutover committed; the destination is authoritative"
		t.Compensation, t.CompensationFailed = nil, nil
		s.compensateSourceAfterCommit(ctx, t, app, t.Source, env)
		if len(t.CompensationFailed) > 0 {
			t.SourceState = environment.SourceNeedsAttention
		}
		if err := s.store.SaveTransition(ctx, *t); err != nil {
			return "", fmt.Errorf("the recovered transition could not be saved")
		}
		result.Compensation, result.CompensationFailed = t.Compensation, t.CompensationFailed
		return "the cutover had committed; the destination stays authoritative and the source remains quiesced", nil
	}
	set, err := s.store.GetDeploymentSet(ctx, t.OldSetID)
	if err != nil {
		return "", fmt.Errorf("the previous deployment set could not be read")
	}
	s.compensate(ctx, t, compensationState{app: app, source: env, set: set})
	t.Status = environment.TransitionFailed
	t.Failure = "the process stopped before the cutover; recovery stopped the destination and restored the source"
	if len(t.CompensationFailed) > 0 {
		t.Failure = "the process stopped before the cutover; some recovery steps failed and need operator attention"
	}
	if err := s.store.SaveTransition(ctx, *t); err != nil {
		return "", fmt.Errorf("the recovered transition could not be saved")
	}
	result.Compensation, result.CompensationFailed = t.Compensation, t.CompensationFailed
	return t.Failure, nil
}

// compensateSourceAfterCommit makes sure a committed transition's source
// writers are stopped (a crash between commit and quiesce leaves them running).
func (s *Service) compensateSourceAfterCommit(ctx context.Context, t *environment.Transition, app appdomain.Application, sourceBinding environment.Binding, current environment.Environment) {
	if s.scaler == nil {
		return
	}
	source := current.WithBinding(sourceBinding, appdomain.RuntimeReady)
	instances, err := s.store.ListWorkloadInstancesFor(ctx, app.Key+"/"+source.Key, source.TargetGeneration)
	if err != nil {
		return
	}
	for _, instance := range instances {
		target := executionTarget(source, app)
		if err := appsvc.RestoreTarget(&target, instance.TargetRef, app.OrganizationKey); err != nil {
			continue
		}
		replicas, exists, err := s.scaler.Replicas(ctx, target, instance.WorkloadID)
		if err != nil || !exists || replicas == 0 {
			continue
		}
		if err := s.scaler.Scale(ctx, target, instance.WorkloadID, 0); err != nil {
			t.CompensationFailed = append(t.CompensationFailed, "quiesce source workload "+instance.WorkloadID)
		} else {
			t.Compensation = append(t.Compensation, "quiesce source workload "+instance.WorkloadID)
		}
	}
}

// recoverDeploy fails the Deployments the dead owner left in flight, marks the
// instances it was applying and asks the next Preview to retry routes. Nothing
// was committed to the Set pointer by an interrupted workload. Every read or
// write failure is reported; none is assumed safe.
func (s *Service) recoverDeploy(ctx context.Context, app appdomain.Application, envKey string, result *RecoveryResult) (string, error) {
	list, err := s.store.ListDeployments(ctx, app.Key, envKey)
	if err != nil {
		return "", fmt.Errorf("deployments could not be read")
	}
	failed := 0
	for _, d := range list {
		if d.Status == deployment.StatusSucceeded || d.Status == deployment.StatusFailed {
			continue
		}
		now := time.Now().UTC()
		d.Status, d.FailureReason, d.FinishedAt = deployment.StatusFailed, appsvc.FailureRuntime, &now
		if err := s.store.SaveDeployment(ctx, d); err != nil {
			result.CompensationFailed = append(result.CompensationFailed, "mark interrupted deployment failed")
			continue
		}
		failed++
		result.Compensation = append(result.Compensation, "failed interrupted deployment")
	}
	instances, err := s.store.ListWorkloadInstances(ctx, app.Key+"/"+envKey)
	if err != nil {
		return "", fmt.Errorf("workload instances could not be read")
	}
	for _, instance := range instances {
		if instance.Status == deployment.InstanceApplying || instance.Status == deployment.InstanceRemoving {
			instance.Status = deployment.InstanceFailed
			if err := s.store.UpsertWorkloadInstance(ctx, instance); err != nil {
				result.CompensationFailed = append(result.CompensationFailed, "mark workload "+instance.WorkloadID+" failed")
			}
		}
	}
	if err := s.store.SetPublicRoutesPending(ctx, app.Key, envKey, true); err != nil {
		result.CompensationFailed = append(result.CompensationFailed, "flag public routes for retry")
	}
	return fmt.Sprintf("%d interrupted deployment(s) were marked failed; preview changes again to retry", failed), nil
}

func (s *Service) recoverCleanup(ctx context.Context, appKey, envKey string, result *RecoveryResult) (string, error) {
	list, err := s.store.ListTransitions(ctx, appKey, envKey)
	if err != nil {
		return "", fmt.Errorf("the transition records could not be read")
	}
	for i := range list {
		if list[i].SourceState == environment.SourceQuiesced || list[i].SourceState == environment.SourceCleanupFailed {
			// A half-finished namespace deletion is safe to repeat.
			list[i].SourceState = environment.SourceCleanupFailed
			if err := s.store.SaveTransition(ctx, list[i]); err != nil {
				return "", fmt.Errorf("the transition record could not be saved")
			}
		}
	}
	return "the cleanup was interrupted; run the source cleanup again", nil
}
