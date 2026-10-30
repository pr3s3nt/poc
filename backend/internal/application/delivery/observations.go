package delivery

import (
	"context"
	"orchestrator/internal/application/reconcilebudget"
	domain "orchestrator/internal/domain/delivery"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/platform/ids"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/persistence"
	"sync"
	"time"
)

const ObservationInterval = 30 * time.Second

// StartObservations reads admitted receipts only. It never renders, publishes,
// applies or invokes Reconcile, and does not acquire a mutation capability.
func (c *Coordinator) StartObservations(parent context.Context) func() {
	repo, ok := c.Store.(persistence.DeliveryObservationRepository)
	if !ok {
		return func() {}
	}
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	worker := ids.New()
	go func() {
		defer close(done)
		ticker := time.NewTicker(ObservationInterval)
		defer ticker.Stop()
		for {
			c.collectObservations(ctx, repo, worker)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return func() { cancel(); <-done }
}
func (c *Coordinator) collectObservations(ctx context.Context, repo persistence.DeliveryObservationRepository, worker string) {
	apps, err := c.Store.ListApplications(ctx)
	if err != nil {
		return
	}
	type job struct {
		orgKey string
		e      persistence.DeliveryExecution
	}
	jobs := []job{}
	for _, app := range apps {
		if app.Lifecycle() != "ACTIVE" {
			continue
		}
		org, err := c.Store.GetOrganization(ctx, app.OrganizationKey)
		if err != nil {
			continue
		}
		envs, err := c.Store.ListEnvironments(ctx, app.Key)
		if err != nil {
			continue
		}
		for _, env := range envs {
			if env.Busy() {
				continue
			}
			records, err := c.Store.ListDeliveryExecutions(ctx, org.ID, env.ID)
			if err != nil {
				continue
			}
			workloads := map[string]bool{}
			scoped := []persistence.DeliveryExecution{}
			for _, e := range records {
				if e.Revision.Scope.TargetGeneration == env.TargetGeneration && e.Revision.Scope.Epoch == env.DeliveryEpoch {
					scoped = append(scoped, e)
					workloads[e.WorkloadID] = true
				}
			}
			for workload := range workloads {
				active, err := activeExecutions(scoped, workload)
				if err != nil {
					continue
				}
				for _, e := range active {
					jobs = append(jobs, job{app.OrganizationKey, e})
				}
			}
		}
	}
	global := make(chan struct{}, 8)
	targets := map[string]chan struct{}{}
	var wg sync.WaitGroup
	for _, j := range jobs {
		key := j.e.Target.Organization + "/" + j.e.Target.Connection
		if targets[key] == nil {
			targets[key] = make(chan struct{}, 2)
		}
		target := targets[key]
		select {
		case global <- struct{}{}:
		case <-ctx.Done():
			wg.Wait()
			return
		}
		select {
		case target <- struct{}{}:
		case <-ctx.Done():
			<-global
			wg.Wait()
			return
		}
		wg.Add(1)
		go func(j job) {
			defer wg.Done()
			defer func() { <-global; <-target }()
			_ = c.observeExecution(ctx, repo, worker, j.orgKey, j.e)
		}(j)
	}
	wg.Wait()
}
func (c *Coordinator) observeExecution(ctx context.Context, repo persistence.DeliveryObservationRepository, worker, orgKey string, e persistence.DeliveryExecution) error {
	release, err := reconcilebudget.Observation(ctx)
	if err != nil {
		return err
	}
	defer release()
	now := time.Now().UTC()
	lease, err := repo.ClaimDeliveryPoll(ctx, persistence.DeliveryPollLease{OrganizationID: e.OrganizationID, UnitID: e.UnitID, IntentID: e.Mutation.ID, Owner: worker, LeaseUntil: now.Add(25 * time.Second), NextPollAt: now.Add(ObservationInterval)}, now)
	if err != nil {
		return err
	}
	i := port.Intent{ID: e.Mutation.ID, OrganizationKey: orgKey, Owner: port.Owner{OperationID: e.Mutation.OperationID, Instance: e.Mutation.Owner, Fence: e.Mutation.Fence}, Revision: e.Revision, Target: e.Target}
	observed := domain.Observation{Scope: e.Revision.Scope, IntendedRevision: e.Revision.ID, Sync: domain.SyncUnknown, Health: domain.HealthUnknown, Drift: domain.DriftUnknown, ObservedAt: now}
	reason := "OBSERVATION_UNAVAILABLE"
	binding, provider, err := c.sourceProvider(ctx, e)
	if err == nil {
		i.Binding = binding
		read, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		result, readErr := provider.Observe(read, i, e.Receipt)
		if readErr == nil && result.Validate(e.Revision) == nil && !result.ObservedAt.After(time.Now().UTC()) {
			observed = result
			reason = ""
		}
	}
	now = time.Now().UTC()
	return repo.SaveDeliveryObservation(ctx, lease, persistence.DeliveryObservation{ID: ids.New(), UnitID: e.UnitID, Sequence: lease.Sequence + 1, Observation: observed, ReasonCode: reason, StoredAt: now}, now)
}

// StoredObservations performs no provider calls. A missing or stale result stays
// explicit and cannot rewrite historical Deployment status.
func (c *Coordinator) StoredObservations(ctx context.Context, orgKey, appKey, envKey, after string, limit int) ([]persistence.DeliveryObservation, environment.Environment, error) {
	app, err := c.Store.GetApplication(ctx, appKey)
	if err != nil || app.OrganizationKey != orgKey {
		return nil, environment.Environment{}, persistence.ErrNotFound
	}
	env, err := c.Store.GetEnvironment(ctx, appKey, envKey)
	if err != nil {
		return nil, env, err
	}
	org, err := c.Store.GetOrganization(ctx, orgKey)
	if err != nil {
		return nil, env, err
	}
	repo, ok := c.Store.(persistence.DeliveryObservationRepository)
	if !ok {
		return nil, env, ErrUnavailable
	}
	rows, err := repo.ListDeliveryObservations(ctx, org.ID, env.ID, env.TargetGeneration, env.DeliveryEpoch, after, limit)
	if err != nil {
		return nil, env, err
	}
	// Keep last-known dimensions alongside effective freshness in the HTTP view.
	return rows, env, nil
}
