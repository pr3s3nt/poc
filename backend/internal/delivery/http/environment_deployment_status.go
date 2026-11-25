package http

import (
	"context"
	"time"

	appdelivery "orchestrator/internal/application/delivery"
	deliverydomain "orchestrator/internal/domain/delivery"
	"orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/progress"
)

// The latest operation can fail before a per-workload Deployment exists.
func (s *Server) environmentDeploymentStatus(ctx context.Context, organizationKey string, env environment.Environment) (string, bool) {
	ops, err := s.store.ListOperations(ctx, env.ApplicationKey, env.Key, 20)
	if err != nil {
		return "", false
	}
	history, err := s.store.ListDeployments(ctx, env.ApplicationKey, env.Key)
	if err != nil {
		return "", false
	}
	byID := make(map[string]deployment.Deployment, len(history))
	var latest *deployment.Deployment
	for i := range history {
		d := &history[i]
		if d.TargetGeneration != env.TargetGeneration || d.EnvironmentID != env.ID {
			continue
		}
		byID[d.ID] = *d
		if latest == nil {
			latest = d
		}
	}
	for _, op := range ops {
		if op.Kind != environment.OpDeploy && op.Kind != environment.OpRemove {
			continue
		}
		if latest != nil && op.ReviewRequestID != latest.ReviewRequestID && op.StartedAt.Before(latest.StartedAt) {
			break
		}
		switch op.Status {
		case environment.OpActive, environment.OpRecovering:
			return "DEPLOYING", false
		case environment.OpInterrupted:
			return "", false
		case environment.OpFailed:
			if env.CurrentDeploymentSetID == "" || !op.Pins.CheckSet || env.CurrentDeploymentSetID != op.Pins.CurrentSetID {
				return "FAILED", false
			}
			instances, err := s.store.ListWorkloadInstances(ctx, env.ApplicationKey+"/"+env.Key)
			if err != nil {
				return "FAILED", false
			}
			for _, instance := range instances {
				prior, ok := byID[instance.LastDeploymentID]
				if ok && prior.Status == deployment.StatusSucceeded && prior.StartedAt.Before(op.StartedAt) &&
					instance.Status == deployment.InstanceReady && instance.ObservedAt.After(op.StartedAt) &&
					time.Since(instance.ObservedAt) >= 0 && time.Since(instance.ObservedAt) <= 3*appdelivery.ObservationInterval {
					return "FAILED", true
				}
			}
			return "FAILED", s.priorDeliveryStillServing(ctx, organizationKey, env, op)
		case environment.OpSucceeded:
			return s.appliedDeploymentStatus(ctx, env), false
		default:
			return "", false
		}
	}
	if latest != nil {
		switch latest.Status {
		case deployment.StatusPlanning, deployment.StatusProvisioning, deployment.StatusDeploying:
			return "DEPLOYING", false
		case deployment.StatusFailed:
			return "FAILED", false
		case deployment.StatusSucceeded:
			return s.appliedDeploymentStatus(ctx, env), false
		}
	}
	if len(history) > 0 || len(ops) >= 20 {
		return "", false
	}
	if s.appliedDeploymentStatus(ctx, env) == "NOT_DEPLOYED" {
		return "NOT_DEPLOYED", false
	}
	return "", false
}

func (s *Server) appliedDeploymentStatus(ctx context.Context, env environment.Environment) string {
	if env.CurrentDeploymentSetID == "" {
		return "NOT_DEPLOYED"
	}
	set, err := s.store.GetDeploymentSet(ctx, env.CurrentDeploymentSetID)
	if err != nil || set.EnvironmentID != env.ID {
		return ""
	}
	if len(set.Document.Modules) == 0 {
		return "NOT_DEPLOYED"
	}
	return "DEPLOYED"
}

// A failed plan that did not start publication leaves the prior CD revision
// selected. Require a fresh positive observation of that exact revision.
func (s *Server) priorDeliveryStillServing(ctx context.Context, organizationKey string, env environment.Environment, op environment.Operation) bool {
	if s.reconciliation == nil || s.appliedDeploymentStatus(ctx, env) != "DEPLOYED" {
		return false
	}
	planFailed := false
	for _, step := range progress.Read(op.Detail["progress"]).Steps {
		if step.Kind == "PUBLISH" || step.Kind == "APPLY" || step.Kind == "READINESS" ||
			(step.Kind == "TERRAFORM" && step.Stage != "PLANNING" && step.Stage != "INIT" && step.Stage != "INTENT") {
			return false
		}
		if step.Kind == "TERRAFORM" && step.Stage == "PLANNING" && step.Status == "FAILED" {
			planFailed = true
		}
	}
	if !planFailed {
		return false
	}
	rows, current, err := s.reconciliation.StoredObservations(ctx, organizationKey, env.ApplicationKey, env.Key, "", 200)
	if err != nil || current.ID != env.ID || current.Version != env.Version || current.ActiveOperationID != "" {
		return false
	}
	now := time.Now().UTC()
	for _, row := range rows {
		observed := row.Observation.Effective(now, appdelivery.ObservationInterval)
		if observed.Complete && observed.Health == deliverydomain.Healthy && observed.Sync == deliverydomain.InSync &&
			observed.ObservedRevision != "" && observed.ObservedRevision == observed.IntendedRevision &&
			observed.ObservedAt.After(op.StartedAt) {
			return true
		}
	}
	return false
}
