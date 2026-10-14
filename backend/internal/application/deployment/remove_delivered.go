package deployment

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"orchestrator/internal/application/envops"
	domain "orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/ports/persistence"
)

// ErrGenerationMismatch means the Environment is no longer on the target
// generation the caller recorded; nothing was touched.
var ErrGenerationMismatch = errors.New("deployment: the Environment is not on the recorded target generation")

// RemovedDelivery reports one RemoveDelivered call.
type RemovedDelivery struct {
	Removed []string
	// Protected are Workload instances of OTHER target generations; they are
	// never touched.
	Protected []string
	Failed    []string
	// SecretObjects are the VSO synchronization objects removed by exact name.
	SecretObjects []string
}

// RemoveDelivered is the independent safety net's delivery layer: it removes
// the delivered Workloads of ONE recorded target generation through the same
// per-instance removal the product uses, each with its ORIGINAL persisted
// target. The Environment is claimed first (REMOVE): a held or interrupted
// Environment reports persistence.ErrEnvironmentBusy before ANY effect, and
// every removal and progress write runs under the claim's lease, so a lost
// lease fences it. A removal that fails or returns an unknown outcome keeps
// the claim INTERRUPTED (never released, never unlocked here) and its instance
// FAILED, so the later Terraform layer is blocked and the evidence stays.
func (s *Service) RemoveDelivered(ctx context.Context, organizationKey, applicationKey, environmentKey string, generation int64) (RemovedDelivery, error) {
	var report RemovedDelivery
	env, err := s.store.GetEnvironment(ctx, applicationKey, environmentKey)
	if err != nil {
		return report, err
	}
	if env.TargetGeneration != generation {
		return report, ErrGenerationMismatch
	}
	scope := applicationKey + "/" + environmentKey
	instances, err := s.store.ListWorkloadInstancesFor(ctx, scope, generation)
	if err != nil {
		return report, err
	}
	for other := int64(0); other <= env.TargetGeneration+1; other++ {
		if other == generation {
			continue
		}
		foreign, err := s.store.ListWorkloadInstancesFor(ctx, scope, other)
		if err != nil {
			return report, err
		}
		for _, w := range foreign {
			if w.Status != domain.InstanceRemoved {
				report.Protected = append(report.Protected, fmt.Sprintf("%s@%d", w.WorkloadID, other))
			}
		}
	}
	var candidates []domain.WorkloadInstance
	for _, w := range instances {
		if w.Status != domain.InstanceRemoved {
			candidates = append(candidates, w)
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].WorkloadID < candidates[j].WorkloadID })
	_, hasRemover := s.configSync.(execution.ConfigSecretRemover)
	if len(candidates) == 0 && (!hasRemover || !env.Configured()) {
		return report, nil
	}
	// Admission BEFORE any effect: Begin fails with ErrBusy while another operation (or an interrupted one) holds the Environment.
	lease, err := s.operations().Begin(ctx, envops.Claim{
		ApplicationKey: applicationKey, EnvironmentKey: environmentKey, Kind: environment.OpRemove,
		Pins:   environment.OperationPins{CheckEnvVersion: true, EnvVersion: env.Version},
		Detail: map[string]any{"origin": "acceptance-safety-net", "generation": generation},
	})
	if err != nil {
		return report, err
	}
	var lastTarget execution.Target
	for _, w := range candidates {
		target := execution.Target{Namespace: env.Namespace(), Extra: map[string]string{"application": applicationKey, "environment": environmentKey, "deployment": workloadOwnerDeploymentID(w)}}
		if err := restoreEnvironmentTarget(&target, w.TargetRef, organizationKey, env.ConnectionKey); err != nil {
			report.Failed = append(report.Failed, w.WorkloadID)
			continue
		}
		lastTarget = target
		w.Status = domain.InstanceRemoving
		w.ObservedAt = s.clock.Now()
		if err := s.store.UpsertWorkloadInstance(lease.Ctx, w); err != nil {
			report.Failed = append(report.Failed, w.WorkloadID)
			continue
		}
		if err := s.deployer.Remove(lease.Ctx, target, w.WorkloadID); err != nil {
			w.Status = domain.InstanceFailed
			_ = s.store.UpsertWorkloadInstance(lease.Ctx, w)
			report.Failed = append(report.Failed, w.WorkloadID)
			continue
		}
		w.Status = domain.InstanceRemoved
		w.AppliedConfigRevisionID = ""
		w.ObservedAt = s.clock.Now()
		if err := s.store.UpsertWorkloadInstance(lease.Ctx, w); err != nil {
			report.Failed = append(report.Failed, w.WorkloadID)
			continue
		}
		report.Removed = append(report.Removed, w.WorkloadID)
	}
	// The VSO objects the secret delivery created for this Application Environment (proven by their exact Vault value path) are
	// run-owned non-Terraform artifacts: without them the namespace would hold unaccounted objects.
	if _, ok := s.configSync.(execution.ConfigSecretRemover); ok && len(report.Failed) == 0 {
		if lastTarget.Namespace == "" {
			app, err := s.store.GetApplication(lease.Ctx, applicationKey)
			if err == nil {
				lastTarget, err = s.GenerationTarget(lease.Ctx, app, env)
			}
			if err != nil {
				report.Failed = append(report.Failed, "secret-delivery-target")
			}
		}
	}
	if remover, ok := s.configSync.(execution.ConfigSecretRemover); ok && len(report.Failed) == 0 {
		names, err := remover.RemoveSynchronized(lease.Ctx, lastTarget, applicationKey, environmentKey)
		report.SecretObjects = names
		if err != nil {
			report.Failed = append(report.Failed, "secret-delivery-objects")
		}
	}
	if len(report.Failed) > 0 {
		_ = lease.Suspend("DELIVERY_REMOVE_UNVERIFIED")
		return report, fmt.Errorf("deployment: %d Workload removals are unverified; the Environment stays held", len(report.Failed))
	}
	if err := lease.End(environment.OpSucceeded, ""); err != nil {
		return report, err
	}
	return report, nil
}

// RemoveInputs groups what a caller (and the tests) needs around RemoveDelivered.
type RemoveInputs struct {
	Service                                *Service
	Store                                  persistence.Store
	Ops                                    *envops.Manager
	Organization, Application, Environment string
}
