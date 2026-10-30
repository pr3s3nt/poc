package deployment

import (
	"context"
	"orchestrator/internal/application/envops"
	domain "orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/ports/execution"
)

// RemoveRetiredAuxiliary retries direct auxiliary cleanup for retired delivery
// records before application namespace destruction. The original persisted
// target/owner remains authoritative, including after a Direct/Fleet handoff.
func (s *Service) RemoveRetiredAuxiliary(ctx context.Context, appKey, envKey string) error {
	app, err := s.store.GetApplication(ctx, appKey)
	if err != nil {
		return err
	}
	env, err := s.store.GetEnvironment(ctx, appKey, envKey)
	if err != nil {
		return err
	}
	instances, err := s.store.ListWorkloadInstancesFor(ctx, appKey+"/"+envKey, env.TargetGeneration)
	if err != nil {
		return err
	}
	retired := []domain.WorkloadInstance{}
	for _, w := range instances {
		if w.Status == domain.InstanceRemoved && (w.TargetRef["deliveryMode"] == "CD" || w.TargetRef["deliveryMode"] == "DIRECT") {
			retired = append(retired, w)
		}
	}
	if len(retired) == 0 {
		return nil
	}
	lease, err := s.operations().Begin(ctx, envops.Claim{ApplicationKey: appKey, EnvironmentKey: envKey, Kind: environment.OpRemove, Pins: environment.OperationPins{CheckEnvVersion: true, EnvVersion: env.Version}, Detail: map[string]any{"retiredAuxiliary": true}})
	if err != nil {
		return err
	}
	for _, w := range retired {
		target := execution.Target{Namespace: env.Namespace(), Extra: map[string]string{"application": appKey, "environment": envKey, "deployment": workloadOwnerDeploymentID(w)}}
		if err = restoreEnvironmentTarget(&target, w.TargetRef, app.OrganizationKey, env); err != nil {
			break
		}
		if s.delivery == nil {
			err = ErrGenerationMismatch
			break
		}
		if err = s.delivery.RemoveWorkload(lease.Ctx, app.OrganizationKey, env.ID, w.WorkloadID); err != nil {
			break
		}
		if err = s.deployer.Remove(lease.Ctx, target, w.WorkloadID); err != nil {
			break
		}
	}
	if err != nil {
		_ = lease.Suspend("AUXILIARY_REMOVE_UNVERIFIED")
		return err
	}
	return lease.End(environment.OpSucceeded, "")
}
