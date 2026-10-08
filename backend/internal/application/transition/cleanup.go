package transition

import (
	"context"
	"errors"
	"fmt"

	"orchestrator/internal/application/envops"
	appdomain "orchestrator/internal/domain/application"
	"orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/ports/persistence"
)

// CleanupSource deletes the retained source generation of a succeeded
// transition after explicit review. It refuses the current generation, any
// generation still referenced by an in-flight transition, non-Kubernetes
// sources and namespaces whose labels do not show this Environment as owner.
func (s *Service) CleanupSource(ctx context.Context, org, appKey, envKey, transitionID string) (Detail, error) {
	t, err := s.scoped(ctx, org, appKey, envKey, transitionID)
	if err != nil {
		return Detail{}, err
	}
	app, err := s.store.GetApplication(ctx, appKey)
	if err != nil {
		return Detail{}, err
	}
	if s.cleaner == nil {
		return Detail{}, refuse("this backend cannot delete Kubernetes namespaces")
	}
	if t.Status != environment.TransitionSucceeded || (t.SourceState != environment.SourceQuiesced && t.SourceState != environment.SourceCleanupFailed) {
		return Detail{}, refuse("only the retained source of a succeeded transition can be cleaned up")
	}
	if t.Source.Profile != appdomain.ProfileInternalK8s {
		return Detail{}, refuse("cleanup of an AWS source generation is not supported")
	}
	env, err := s.store.GetEnvironment(ctx, appKey, envKey)
	if err != nil {
		return Detail{}, err
	}
	if env.TargetGeneration == t.Source.Generation || env.ConnectionKey == t.Source.ConnectionKey && env.TargetGeneration == t.Source.Generation {
		return Detail{}, refuse("the generation is the current target")
	}
	if err := s.referenced(ctx, appKey, envKey, t); err != nil {
		return Detail{}, err
	}
	conn, err := s.store.GetConnection(ctx, org, t.Source.ConnectionKey)
	if err != nil || conn.Status != appdomain.ConnectionReady || conn.OrganizationKey != org {
		return Detail{}, refuse("the source connection is no longer available to resolve its credentials")
	}
	lease, err := s.ops.Begin(ctx, envops.Claim{
		ApplicationKey: appKey, EnvironmentKey: envKey, Kind: environment.OpCleanup,
		Pins:   environment.OperationPins{CheckEnvVersion: true, EnvVersion: env.Version, CheckBinding: true, Binding: env.Binding()},
		Detail: map[string]any{"transition": t.ID, "generation": t.Source.Generation},
	})
	if err != nil {
		return Detail{}, err
	}
	source := env.WithBinding(t.Source, appdomain.RuntimeReady)
	source.NamespaceIdentity = env.NamespaceIdentity
	target, err := s.deployer.GenerationTarget(lease.Ctx, app, source)
	if err == nil {
		target.Namespace = t.SourceNamespace
		err = s.cleaner.DeleteOwnedNamespace(lease.Ctx, target, t.SourceNamespace, appKey, envKey)
	}
	if err != nil {
		t.SourceState = environment.SourceCleanupFailed
		t.CompensationFailed = append(t.CompensationFailed, "source cleanup failed; review ownership and retry")
		s.saveDetached(lease, t)
		_ = lease.End(environment.OpFailed, "source cleanup failed")
		return s.detailOf(ctx, org, t), errors.Join(ErrCleanupRefused, fmt.Errorf("the source namespace could not be deleted"))
	}
	if err := s.markSourceGone(lease.Ctx, org, app, t); err != nil {
		t.SourceState = environment.SourceCleanupFailed
		s.saveDetached(lease, t)
		_ = lease.End(environment.OpFailed, "source cleanup bookkeeping failed")
		return s.detailOf(ctx, org, t), err
	}
	t.SourceState = environment.SourceCleaned
	t.CompensationFailed = filterOut(t.CompensationFailed, "source cleanup")
	t.Compensation = append(t.Compensation, "deleted source namespace "+t.SourceNamespace)
	detached, cancel := lease.Detached()
	saveErr := s.store.SaveTransition(detached, t)
	cancel()
	if saveErr != nil {
		_ = lease.Suspend("the source was deleted but its record could not be saved; recover to finish bookkeeping")
		return Detail{}, saveErr
	}
	if err := lease.End(environment.OpSucceeded, ""); err != nil {
		return Detail{}, err
	}
	return s.detailOf(ctx, org, t), nil
}

// saveDetached records a cleanup outcome under the lease owner with a bounded,
// non-cancelled context, so the fence still applies.
func (s *Service) saveDetached(lease *envops.Lease, t environment.Transition) {
	ctx, cancel := lease.Detached()
	defer cancel()
	_ = s.store.SaveTransition(ctx, t)
}

func refuse(reason string) error { return fmt.Errorf("%w: %s", ErrCleanupRefused, reason) }

// referenced rejects a generation that an in-flight transition still uses.
func (s *Service) referenced(ctx context.Context, appKey, envKey string, t environment.Transition) error {
	all, err := s.store.ListTransitions(ctx, appKey, envKey)
	if err != nil {
		return err
	}
	for _, other := range all {
		if other.ID == t.ID || other.Status != environment.TransitionRunning {
			continue
		}
		if other.Source.Generation == t.Source.Generation || other.Destination.Generation == t.Source.Generation {
			return refuse("an in-flight transition references the generation")
		}
	}
	return nil
}

// markSourceGone retires the generation-owned records after its namespace is
// deleted: instances read REMOVED and resources UNREFERENCED, in the old
// generation's own rows only.
func (s *Service) markSourceGone(ctx context.Context, org string, app appdomain.Application, t environment.Transition) error {
	instances, err := s.store.ListWorkloadInstancesFor(ctx, app.Key+"/"+t.EnvironmentKey, t.Source.Generation)
	if err != nil {
		return err
	}
	for _, instance := range instances {
		instance.Status = deployment.InstanceRemoved
		if err := s.store.UpsertWorkloadInstance(ctx, instance); err != nil {
			return err
		}
	}
	active, err := s.store.ListActiveResources(ctx, org)
	if err != nil {
		return err
	}
	for _, a := range active {
		if a.Scope.Type == resource.ScopeApplication || !envops.InGeneration(a.Scope, app.Key, t.EnvironmentKey, t.Source.Generation) || a.Status == resource.StatusUnreferenced {
			continue
		}
		a.Status = resource.StatusUnreferenced
		if _, err := s.store.UpsertActiveResource(ctx, a); err != nil {
			return err
		}
	}
	return nil
}

var _ = execution.Target{}
var _ = persistence.ErrNotFound
