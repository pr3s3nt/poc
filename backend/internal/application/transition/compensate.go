package transition

import (
	"context"
	"fmt"
	"strings"

	appsvc "orchestrator/internal/application/deployment"
	appdomain "orchestrator/internal/domain/application"
	"orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/ports/execution"
)

// compensationState is everything compensation needs; it is derivable from the
// store, so recovery after a crash rebuilds it without the dead runner.
type compensationState struct {
	app       appdomain.Application
	source    environment.Environment
	set       environment.DeploymentSet
	committed bool
}

// compensate restores the source generation after a failure or interruption
// before the cutover commit. Order matters and is safety-driven: the
// destination first loses its route and its writers (the ones this attempt
// started), and only then do the source route and writers come back. If any
// destination step fails, restoring the source would let two systems write
// divergent databases, so the source is NOT restored and the record says so.
// Every step is recorded; a failed step is reported, never hidden.
func (s *Service) compensate(ctx context.Context, t *environment.Transition, c compensationState) {
	if c.committed {
		return
	}
	t.Compensation, t.CompensationFailed = nil, nil
	note := func(ok bool, label string) {
		if ok {
			t.Compensation = append(t.Compensation, label)
		} else {
			t.CompensationFailed = append(t.CompensationFailed, label)
		}
	}
	dest := c.source.WithBinding(t.Destination, appdomain.RuntimeReady)
	destinationStopped := true

	routesOK := true
	if s.deployer.RoutesManaged() && t.RoutesMoved {
		err := s.deployer.RemoveRoutesFor(ctx, c.app.Key, dest)
		note(err == nil, "remove destination route")
		routesOK = err == nil
	}
	instances, err := s.store.ListWorkloadInstancesFor(ctx, c.app.Key+"/"+c.source.Key, dest.TargetGeneration)
	if err != nil {
		note(false, "list destination workloads")
		destinationStopped = false
	}
	for _, instance := range instances {
		if instance.Status == deployment.InstanceRemoved {
			continue
		}
		if s.scaler == nil {
			note(false, "stop destination workload "+instance.WorkloadID+" (no scaler)")
			destinationStopped = false
			continue
		}
		target := execution.Target{Namespace: dest.Namespace(), Extra: map[string]string{"application": c.app.Key, "environment": c.source.Key}}
		if err := appsvc.RestoreTarget(&target, instance.TargetRef, c.app.OrganizationKey); err != nil {
			note(false, "stop destination workload "+instance.WorkloadID)
			destinationStopped = false
			continue
		}
		replicas, exists, err := s.scaler.Replicas(ctx, target, instance.WorkloadID)
		if err != nil {
			note(false, "stop destination workload "+instance.WorkloadID)
			destinationStopped = false
			continue
		}
		if !exists || replicas == 0 {
			continue
		}
		if err := s.scaler.Scale(ctx, target, instance.WorkloadID, 0); err != nil {
			note(false, "stop destination workload "+instance.WorkloadID)
			destinationStopped = false
			continue
		}
		note(true, "stopped destination workload "+instance.WorkloadID)
	}

	if !destinationStopped || !routesOK {
		// Restoring the source while the destination may still write or serve is unsafe.
		note(false, "source NOT restored: the destination could not be fully stopped")
		t.SourceState = environment.SourceNeedsAttention
		s.removeBackups(ctx, t, c, note)
		return
	}
	if s.deployer.RoutesManaged() && t.RoutesMoved {
		ok := s.deployer.ReconcileRoutesFor(ctx, c.app.Key, c.source, c.set.Document) == nil
		note(ok, "restore source routes")
		if ok {
			t.RoutesMoved = false
		}
	}
	if s.scaler != nil {
		sourceInstances, err := s.store.ListWorkloadInstancesFor(ctx, c.app.Key+"/"+c.source.Key, c.source.TargetGeneration)
		if err != nil {
			note(false, "restore source workloads")
		}
		for _, instance := range sourceInstances {
			recorded, ok := t.Replicas[instance.WorkloadID]
			if !ok || recorded <= 0 || instance.Status == deployment.InstanceRemoved {
				continue
			}
			target := execution.Target{Namespace: c.source.Namespace(), Extra: map[string]string{"application": c.app.Key, "environment": c.source.Key}}
			if err := appsvc.RestoreTarget(&target, instance.TargetRef, c.app.OrganizationKey); err != nil {
				note(false, "restore source workload "+instance.WorkloadID)
				continue
			}
			current, exists, err := s.scaler.Replicas(ctx, target, instance.WorkloadID)
			if err != nil {
				note(false, "restore source workload "+instance.WorkloadID)
				continue
			}
			if exists && current == recorded {
				continue
			}
			note(s.scaler.Scale(ctx, target, instance.WorkloadID, recorded) == nil, "restore source workload "+instance.WorkloadID+" to "+fmt.Sprint(recorded)+" replicas")
		}
	}
	s.removeBackups(ctx, t, c, note)
	t.SourceState = environment.SourceAuthoritative
	if len(t.CompensationFailed) > 0 {
		t.SourceState = environment.SourceNeedsAttention
	}
}

func filterOut(items []string, prefixes ...string) []string {
	var out []string
	for _, item := range items {
		keep := true
		for _, prefix := range prefixes {
			if strings.HasPrefix(item, prefix) {
				keep = false
			}
		}
		if keep {
			out = append(out, item)
		}
	}
	return out
}

// removeBackups deletes private archives that still exist.
func (s *Service) removeBackups(ctx context.Context, t *environment.Transition, c compensationState, note func(bool, string)) {
	if s.postgres == nil {
		return
	}
	target, err := s.deployer.GenerationTarget(ctx, c.app, c.source)
	for i := range t.Backups {
		record := &t.Backups[i]
		if record.Removed {
			continue
		}
		if err != nil {
			note(false, "remove backup archive "+record.Dir)
			continue
		}
		res := execution.PostgresResource{Target: target, Namespace: record.Namespace, Name: strings.TrimSuffix(record.Pod, "-0")}
		res.Target.Namespace = record.Namespace
		if removeErr := s.postgres.RemoveArchive(ctx, res, execution.PostgresArchive{Pod: record.Pod, Dir: record.Dir, SHA256: record.SHA256}); removeErr != nil {
			note(false, "remove backup archive "+record.Dir)
			continue
		}
		record.Removed = true
		note(true, "remove backup archive")
	}
}
