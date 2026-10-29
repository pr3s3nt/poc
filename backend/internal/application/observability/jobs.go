package observability

import (
	"context"
	"encoding/json"
	"time"

	deployapp "orchestrator/internal/application/deployment"
	"orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/telemetry"
	"orchestrator/internal/ports/execution"
	obs "orchestrator/internal/ports/observability"
	"orchestrator/internal/ports/persistence"
)

// Jobs inspects only an applied, scoped inventory. Desired drafts never select
// credentials, a controller or an execution target for this live read.
func (s *Service) Jobs(ctx context.Context, scope telemetry.Scope, workload string) (obs.JobHistory, error) {
	if !workloadShape.MatchString(workload) {
		return obs.JobHistory{}, &FieldError{"workload"}
	}
	app, err := s.store.GetApplication(ctx, scope.ApplicationKey)
	if err != nil || app.OrganizationKey != scope.OrganizationKey {
		return obs.JobHistory{}, persistence.ErrNotFound
	}
	env, err := s.store.GetEnvironment(ctx, app.Key, scope.EnvironmentKey)
	if err != nil {
		return obs.JobHistory{}, err
	}
	instances, err := s.store.ListWorkloadInstancesFor(ctx, app.Key+"/"+env.Key, env.TargetGeneration)
	if err != nil {
		return obs.JobHistory{}, err
	}
	reader, ok := s.reader.(obs.JobReader)
	if !ok {
		return obs.JobHistory{}, ErrUnavailable
	}
	for _, instance := range instances {
		if instance.WorkloadID != workload || instance.Status == deployment.InstanceRemoved || instance.Status == deployment.InstanceRemoving {
			continue
		}
		dep, err := s.store.GetDeployment(ctx, instance.LastDeploymentID)
		if err != nil || dep.OrganizationKey != scope.OrganizationKey || dep.ApplicationKey != app.Key || dep.EnvironmentKey != env.Key || dep.WorkloadID != workload || dep.TargetGeneration != env.TargetGeneration {
			return obs.JobHistory{}, persistence.ErrNotFound
		}
		var target execution.Target
		if deployapp.RestoreTarget(&target, instance.TargetRef, app.OrganizationKey) != nil || !target.CredentialBacked() || target.Namespace == "" {
			return obs.JobHistory{}, ErrUnavailable
		}
		var objects []execution.ObjectRef
		raw, err := json.Marshal(instance.TargetRef["objects"])
		if err != nil || json.Unmarshal(raw, &objects) != nil {
			return obs.JobHistory{}, ErrUnavailable
		}
		for _, ref := range objects {
			if ref.APIVersion != "batch/v1" || ref.Kind != "CronJob" || ref.Name != workload || len(ref.Owners) == 0 {
				continue
			}
			scope.Generation = env.TargetGeneration
			source := obs.Source{Source: telemetry.Source{Scope: scope, Key: telemetry.Hash(app.Key, env.Key, workload), WorkloadID: workload, TargetHash: telemetry.Hash(instance.ManifestDigest)}, Target: target}
			bounded, cancel := context.WithTimeout(ctx, 25*time.Second)
			defer cancel()
			return reader.InspectJobs(bounded, source, ref), nil
		}
		return obs.JobHistory{}, persistence.ErrNotFound
	}
	return obs.JobHistory{}, persistence.ErrNotFound
}
