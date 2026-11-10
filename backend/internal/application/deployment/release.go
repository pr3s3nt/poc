package deployment

import (
	"context"
	"errors"
	"fmt"
	"sync"

	deliveryapp "orchestrator/internal/application/delivery"
	deliverydomain "orchestrator/internal/domain/delivery"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/ports/execution"
)

// Release collects the workloads of one Deploy batch so they are published as
// one Environment release (ADR-042). Each workload is planned, provisioned and
// rendered as before, then staged here; Flush publishes once and completes each
// staged workload with the delivery outcome.
type Release struct {
	mu        sync.Mutex
	orgKey    string
	appKey    string
	env       environment.Environment
	target    *execution.Target
	workloads map[string][]execution.Manifest
	removed   []string
	order     []string
	finishers map[string]func(context.Context, error) error
}

type releaseKey struct{}

// NewRelease starts an Environment release for one batch.
func NewRelease(orgKey, appKey string, env environment.Environment) *Release {
	return &Release{orgKey: orgKey, appKey: appKey, env: env, workloads: map[string][]execution.Manifest{}, finishers: map[string]func(context.Context, error) error{}}
}

// WithRelease carries the batch release to DeployWorkload.
func WithRelease(ctx context.Context, r *Release) context.Context {
	return context.WithValue(ctx, releaseKey{}, r)
}

func releaseFrom(ctx context.Context) *Release {
	r, _ := ctx.Value(releaseKey{}).(*Release)
	return r
}

// Empty reports whether nothing was staged.
func (r *Release) Empty() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.order) == 0
}

// Staged lists staged workloads in staging order.
func (r *Release) Staged() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.order...)
}

func (r *Release) setTarget(target execution.Target) error {
	if r.target == nil {
		t := target
		r.target = &t
		return nil
	}
	if r.target.Namespace != target.Namespace || r.target.Connection != target.Connection || r.target.Organization != target.Organization || r.target.Kubeconfig != target.Kubeconfig {
		return fmt.Errorf("%w: every workload of an Environment must run on the same cluster and namespace", deliverydomain.ErrUnsupported)
	}
	return nil
}

func (r *Release) stage(workloadID string, target execution.Target, manifests []execution.Manifest, finish func(context.Context, error) error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.finishers[workloadID]; exists {
		return fmt.Errorf("deployment: workload %q is already part of this release", workloadID)
	}
	if err := r.setTarget(target); err != nil {
		return err
	}
	r.workloads[workloadID] = manifests
	r.order = append(r.order, workloadID)
	r.finishers[workloadID] = finish
	return nil
}

func (r *Release) stageRemoval(workloadID string, target execution.Target, finish func(context.Context, error) error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.finishers[workloadID]; exists {
		return fmt.Errorf("deployment: workload %q is already part of this release", workloadID)
	}
	if r.target == nil {
		t := target
		r.target = &t
	}
	r.removed = append(r.removed, workloadID)
	r.order = append(r.order, workloadID)
	r.finishers[workloadID] = finish
	return nil
}

// ReleaseOutcome is the delivery result of each staged workload.
type ReleaseOutcome map[string]error

// UsesEnvironmentBundle reports whether this Environment deploys through the
// ADR-042 Environment release.
func (s *Service) UsesEnvironmentBundle(ctx context.Context, orgKey string, env environment.Environment) (bool, error) {
	if s.delivery == nil || env.DesiredDelivery.Effective().Mode != deliverydomain.CD {
		return false, nil
	}
	return s.delivery.UsesEnvironmentBundle(ctx, orgKey, env)
}

// extend chains a completion step after the staged workload's finisher; the
// step receives the first error of the delivery or the earlier steps.
func (r *Release) extend(workloadID string, next func(context.Context, error) error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	previous := r.finishers[workloadID]
	if previous == nil {
		return
	}
	r.finishers[workloadID] = func(ctx context.Context, cause error) error {
		if err := previous(ctx, cause); err != nil && cause == nil {
			cause = err
		}
		if err := next(ctx, cause); err != nil && cause == nil {
			cause = err
		}
		return cause
	}
}

func (r *Release) has(workloadID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.finishers[workloadID]
	return ok
}

// FlushRelease publishes the staged workloads as one release and completes
// every staged workload. A rollout failure attributed to named workloads fails
// only those; any other failure fails every staged workload. The second result
// is the release error, also when only an unchanged workload failed.
func (s *Service) FlushRelease(ctx context.Context, r *Release) (ReleaseOutcome, error) {
	outcome := ReleaseOutcome{}
	if r == nil || r.Empty() {
		return outcome, nil
	}
	var err error
	if s.delivery == nil {
		err = deliveryapp.ErrUnavailable
	} else if r.target == nil {
		err = fmt.Errorf("deployment: release has no target")
	} else {
		err = s.delivery.DeployEnvironment(ctx, deliveryapp.EnvironmentRequest{OrganizationKey: r.orgKey, ApplicationKey: r.appKey, Environment: r.env, Target: *r.target, Workloads: r.workloads, Removed: r.removed})
	}
	failed := map[string]bool{}
	var rollout *deliveryapp.RolloutFailure
	attributed := errors.As(err, &rollout) && len(rollout.Workloads) > 0
	if attributed {
		for _, name := range rollout.Workloads {
			failed[name] = true
		}
	}
	for _, workloadID := range r.Staged() {
		var cause error
		if err != nil && (!attributed || failed[workloadID]) {
			cause = err
		}
		if finishErr := r.finishers[workloadID](ctx, cause); finishErr != nil && cause == nil {
			cause = finishErr
		}
		outcome[workloadID] = cause
	}
	return outcome, err
}
