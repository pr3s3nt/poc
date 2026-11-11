package delivery

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "orchestrator/internal/domain/delivery"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/progress"
	"orchestrator/internal/platform/ids"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/persistence"
)

// rolloutProvider reports a configurable health for the Rollout phase and
// supports receipted submission recovery without resubmitting.
type rolloutProvider struct {
	*neutralProvider
	rolloutHealth domain.Health
	observeErr    error
	recoveries    int
	expected      map[domain.Phase][]string
}

func (p *rolloutProvider) Submit(ctx context.Context, i port.Intent, prepared port.Prepared) (port.Receipt, error) {
	if p.expected == nil {
		p.expected = map[domain.Phase][]string{}
	}
	p.expected[i.Revision.Phase] = append(p.expected[i.Revision.Phase], i.ExpectedRevision)
	return p.neutralProvider.Submit(ctx, i, prepared)
}

func (p *rolloutProvider) Observe(ctx context.Context, i port.Intent, r port.Receipt) (domain.Observation, error) {
	if p.observeErr != nil {
		return domain.Observation{}, p.observeErr
	}
	observed, err := p.neutralProvider.Observe(ctx, i, r)
	if i.Revision.Phase == domain.Rollout {
		observed.Health = p.rolloutHealth
	}
	return observed, err
}
func (p *rolloutProvider) RecoverSubmission(_ context.Context, _ port.Intent, _ port.Prepared, r port.Receipt, _ port.TransferSource) (port.Receipt, error) {
	p.recoveries++
	return r, nil
}

func rolloutFixture(t *testing.T, detail map[string]any) (*Coordinator, *rolloutProvider, context.Context, DeployRequest) {
	t.Helper()
	c, original, ctx, req, _ := coordinatorFixture(t)
	p := &rolloutProvider{neutralProvider: original, rolloutHealth: domain.Healthy}
	c.Registry = port.NewRegistry()
	if err := c.Registry.Register("fleet", p); err != nil {
		t.Fatal(err)
	}
	c.PollInterval = 5 * time.Millisecond
	c.Deadline = 60 * time.Millisecond
	if detail != nil {
		owner, _ := persistence.OwnerFrom(ctx)
		if err := c.Store.ReleaseOperation(ctx, owner, environment.OpSucceeded, ""); err != nil {
			t.Fatal(err)
		}
		ctx = claimDeploy(t, c, req, detail)
	}
	return c, p, persistence.WithHold(ctx), req
}

func claimDeploy(t *testing.T, c *Coordinator, req DeployRequest, detail map[string]any) context.Context {
	t.Helper()
	op, err := c.Store.ClaimEnvironment(context.Background(), persistence.OperationClaim{ID: ids.New(), ApplicationKey: req.ApplicationKey, EnvironmentKey: req.Environment.Key, Owner: "rollout-test", Kind: environment.OpDeploy, Detail: detail, Deadline: time.Now().Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	return persistence.WithOwner(context.Background(), persistence.Owner{OperationID: op.ID, Owner: op.Owner, Fence: op.Fence})
}

func rolloutStatus(t *testing.T, c *Coordinator, ctx context.Context, req DeployRequest) map[domain.Phase]domain.IntentStatus {
	t.Helper()
	org, err := c.Store.GetOrganization(ctx, req.OrganizationKey)
	if err != nil {
		t.Fatal(err)
	}
	records, err := c.Store.ListDeliveryExecutions(ctx, org.ID, req.Environment.ID)
	if err != nil {
		t.Fatal(err)
	}
	out := map[domain.Phase]domain.IntentStatus{}
	for _, r := range records {
		out[r.Revision.Phase] = r.Mutation.Status
	}
	return out
}

func TestDegradedRolloutFailsWithoutHoldAndAllowsRedeploy(t *testing.T) {
	c, p, ctx, req := rolloutFixture(t, nil)
	p.rolloutHealth = domain.Degraded
	if err := c.Deploy(ctx, req); !errors.Is(err, ErrRolloutDegraded) {
		t.Fatal("expected a determined degraded failure", err)
	}
	if persistence.Held(ctx) {
		t.Fatal("determined failure held the Environment")
	}
	status := rolloutStatus(t, c, ctx, req)
	if status[domain.Rollout] != domain.IntentFailed || status[domain.Prerequisite] != domain.IntentAcknowledged {
		t.Fatal("unexpected intent states", status)
	}
	failedRevision := rolloutRevision(t, c, ctx, req)
	// The released Environment accepts a corrected deployment without replaying
	// the failed revision.
	owner, _ := persistence.OwnerFrom(ctx)
	if err := c.Store.ReleaseOperation(ctx, owner, environment.OpFailed, ""); err != nil {
		t.Fatal(err)
	}
	submitted := len(p.order)
	ctx = persistence.WithHold(claimDeploy(t, c, req, nil))
	p.rolloutHealth = domain.Healthy
	req.DeploymentID = ids.New()
	req.Manifests[0].Object = map[string]any{"kind": "Deployment", "fixed": true}
	if err := c.Deploy(ctx, req); err != nil {
		t.Fatal("redeploy after a determined failure", err)
	}
	if persistence.Held(ctx) || len(p.order) <= submitted {
		t.Fatal("corrected rollout was not submitted", p.order)
	}
	// The published failed revision holds the provider pin; the corrected
	// rollout must name it so the provider accepts the replacement.
	rollouts := p.expected[domain.Rollout]
	if len(rollouts) != 2 || rollouts[0] != "" || rollouts[1] != failedRevision {
		t.Fatal("corrected rollout did not replace the failed revision", rollouts, failedRevision)
	}
}

func rolloutRevision(t *testing.T, c *Coordinator, ctx context.Context, req DeployRequest) string {
	t.Helper()
	org, err := c.Store.GetOrganization(ctx, req.OrganizationKey)
	if err != nil {
		t.Fatal(err)
	}
	records, err := c.Store.ListDeliveryExecutions(ctx, org.ID, req.Environment.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range records {
		if r.Revision.Phase == domain.Rollout && r.Mutation.Status == domain.IntentFailed {
			return r.Revision.ID
		}
	}
	t.Fatal("no failed rollout record")
	return ""
}

func TestRolloutNotReadyAtDeadlineFailsWithoutHold(t *testing.T) {
	c, p, ctx, req := rolloutFixture(t, nil)
	p.rolloutHealth = domain.Progressing
	if err := c.Deploy(ctx, req); !errors.Is(err, ErrRolloutNotReady) {
		t.Fatal("expected a readiness deadline failure", err)
	}
	if persistence.Held(ctx) || rolloutStatus(t, c, ctx, req)[domain.Rollout] != domain.IntentFailed {
		t.Fatal("not-ready rollout was held or not failed")
	}
}

func TestUnknownRolloutObservationStillHolds(t *testing.T) {
	c, p, ctx, req := rolloutFixture(t, nil)
	p.observeErr = errors.New("controller unreachable")
	if err := c.Deploy(ctx, req); err == nil {
		t.Fatal("unobservable rollout reported success")
	}
	if !persistence.Held(ctx) || rolloutStatus(t, c, ctx, req)[domain.Prerequisite] != domain.IntentRunning {
		t.Fatal("unknown observation released the Environment")
	}
}

func TestCancelledOperationAtDeadlineStillHolds(t *testing.T) {
	c, p, ctx, req := rolloutFixture(t, nil)
	p.rolloutHealth = domain.Progressing
	c.Deadline = time.Minute
	cancelled, cancel := context.WithTimeout(ctx, 40*time.Millisecond)
	defer cancel()
	if err := c.Deploy(cancelled, req); err == nil || errors.Is(err, ErrRolloutNotReady) {
		t.Fatal("a lost operation must not be classified as a determined failure", err)
	}
	if !persistence.Held(ctx) {
		t.Fatal("cancelled operation released the Environment")
	}
}

func TestTransferDeliveryKeepsHoldForDegradedRollout(t *testing.T) {
	c, p, ctx, req := rolloutFixture(t, map[string]any{"deliveryTransfer": true})
	p.rolloutHealth = domain.Degraded
	if err := c.Deploy(ctx, req); !errors.Is(err, ErrRolloutDegraded) {
		t.Fatal(err)
	}
	if !persistence.Held(ctx) || rolloutStatus(t, c, ctx, req)[domain.Rollout] != domain.IntentRunning {
		t.Fatal("transfer rollout failure must keep the hold")
	}
}

// heldRollout leaves a receipted Rollout RUNNING and held, as an earlier
// release did for a degraded rollout, then starts a recovery operation.
func heldRollout(t *testing.T, c *Coordinator, p *rolloutProvider, ctx context.Context, req DeployRequest) context.Context {
	t.Helper()
	p.observeErr = errors.New("observation lost")
	if err := c.Deploy(ctx, req); err == nil || !persistence.Held(ctx) {
		t.Fatal("setup: expected a held deployment", err)
	}
	p.observeErr = nil
	owner, _ := persistence.OwnerFrom(ctx)
	if err := c.Store.SuspendOperation(ctx, owner, "prior worker stopped"); err != nil {
		t.Fatal(err)
	}
	op, err := c.Store.BeginRecovery(context.Background(), owner.OperationID, "rollout-recovery", "operator")
	if err != nil {
		t.Fatal(err)
	}
	return persistence.WithOwner(context.Background(), persistence.Owner{OperationID: op.ID, Owner: op.Owner, Fence: op.Fence})
}

func TestRecoveryFailsObservedDegradedRolloutWithoutReplay(t *testing.T) {
	c, p, ctx, req := rolloutFixture(t, nil)
	c.Deadline = time.Minute
	recovery := heldRollout(t, c, p, ctx, req)
	submitted := len(p.order)
	c.Registry = port.NewRegistry()
	if err := c.Registry.Register("fleet", &notReadyProvider{rolloutProvider: p, degraded: true, pods: []progress.Pod{{Name: "backend-1", Reason: "CrashLoopBackOff", Restarts: 4}}}); err != nil {
		t.Fatal(err)
	}
	var observedCheck bool
	recovery = progress.WithReporter(recovery, func(step progress.Step, check bool) {
		if check && step.Kind == "observe" && len(step.Pods) == 1 && step.Pods[0].Restarts == 4 {
			observedCheck = true
		}
	})
	started := time.Now()
	if err := c.RecoverSubmissions(recovery, req.OrganizationKey, req.Environment); err != nil {
		t.Fatal("recovery of a degraded rollout", err)
	}
	if time.Since(started) > 10*time.Second {
		t.Fatal("recovery waited for the full deadline")
	}
	if !observedCheck {
		t.Fatal("recovery lost the latest pod observation")
	}
	status := rolloutStatus(t, c, recovery, req)
	if status[domain.Prerequisite] != domain.IntentFailed || len(p.order) != submitted || p.recoveries != 1 {
		t.Fatal("recovery did not settle the degraded rollout without replay", status, p.order, p.recoveries)
	}
}

func TestRecoveryFailsRolloutStillNotReadyAtDeadline(t *testing.T) {
	c, p, ctx, req := rolloutFixture(t, nil)
	recovery := heldRollout(t, c, p, ctx, req)
	p.rolloutHealth = domain.Progressing
	// The held unit is the first phase; observe it as never ready.
	p.observeErr = nil
	c.Registry = port.NewRegistry()
	never := &notReadyProvider{rolloutProvider: p}
	if err := c.Registry.Register("fleet", never); err != nil {
		t.Fatal(err)
	}
	if err := c.RecoverSubmissions(recovery, req.OrganizationKey, req.Environment); err != nil {
		t.Fatal("recovery of a not-ready rollout", err)
	}
	if rolloutStatus(t, c, recovery, req)[domain.Prerequisite] != domain.IntentFailed {
		t.Fatal("not-ready recovery was not settled")
	}
}

func TestTransferRecoveryKeepsHoldForDegradedRollout(t *testing.T) {
	c, p, ctx, req := rolloutFixture(t, map[string]any{"deliveryTransfer": true})
	recovery := heldRollout(t, c, p, ctx, req)
	c.Registry = port.NewRegistry()
	never := &notReadyProvider{rolloutProvider: p, degraded: true}
	if err := c.Registry.Register("fleet", never); err != nil {
		t.Fatal(err)
	}
	if err := c.RecoverSubmissions(recovery, req.OrganizationKey, req.Environment); !errors.Is(err, domain.ErrOutcomeUnknown) {
		t.Fatal("transfer recovery must keep the hold", err)
	}
	if rolloutStatus(t, c, recovery, req)[domain.Prerequisite] != domain.IntentRunning {
		t.Fatal("transfer recovery settled the unit")
	}
}

// notReadyProvider observes every phase as not ready (or degraded).
type notReadyProvider struct {
	*rolloutProvider
	degraded bool
	pods     []progress.Pod
}

func (p *notReadyProvider) Observe(ctx context.Context, i port.Intent, r port.Receipt) (domain.Observation, error) {
	observed, err := p.neutralProvider.Observe(ctx, i, r)
	observed.Health = domain.Progressing
	observed.Pods = p.pods
	if p.degraded {
		observed.Health = domain.Degraded
	}
	return observed, err
}

func TestPublishedFailureSkipsNeverPublishedAttempts(t *testing.T) {
	at := time.Now().UTC()
	record := func(id string, status domain.IntentStatus, published bool, age int) persistence.DeliveryExecution {
		e := persistence.DeliveryExecution{UnitID: "u", CreatedAt: at.Add(time.Duration(age) * time.Second), Revision: domain.Revision{ID: "rev-" + id}, Mutation: domain.Mutation{ID: "m-" + id, Action: domain.Submit, Status: status}}
		if published {
			e.Receipt = port.Receipt{IntentID: e.Mutation.ID, RevisionID: e.Revision.ID}
		}
		return e
	}
	ack := record("ack", domain.IntentAcknowledged, true, 0)
	failed := record("failed", domain.IntentFailed, true, 1)
	unpublished := record("absent", domain.IntentFailed, false, 2)
	if got := publishedFailure([]persistence.DeliveryExecution{ack, failed, unpublished}, "u", []persistence.DeliveryExecution{ack}); got == nil || got.Revision.ID != "rev-failed" {
		t.Fatal("published failure behind a never-published attempt was not found", got)
	}
	newer := record("newer", domain.IntentAcknowledged, true, 3)
	if got := publishedFailure([]persistence.DeliveryExecution{ack, failed, newer}, "u", []persistence.DeliveryExecution{newer}); got != nil {
		t.Fatal("a newer acknowledged revision owns the pin", got.Revision.ID)
	}
	if got := publishedFailure([]persistence.DeliveryExecution{ack, unpublished}, "u", []persistence.DeliveryExecution{ack}); got != nil {
		t.Fatal("a never-published failure must not become the expected revision", got.Revision.ID)
	}
	if got := publishedFailure(nil, "u", nil); got != nil {
		t.Fatal("no executions")
	}
}

// failedPinProvider proves absence only against the published failed revision
// that holds the provider pin, as the Fleet adapter does.
type failedPinProvider struct {
	*rolloutProvider
	recoveryGuard func(context.Context, port.Intent) error
	seen          []port.Intent
	rolloutError  error
}

func (p *failedPinProvider) Submit(ctx context.Context, i port.Intent, prepared port.Prepared) (port.Receipt, error) {
	if i.Revision.Phase == domain.Rollout && p.rolloutError != nil {
		return port.Receipt{}, p.rolloutError
	}
	return p.rolloutProvider.Submit(ctx, i, prepared)
}

func (p *failedPinProvider) ObserveSubmissionAbsent(ctx context.Context, i port.Intent, _ port.Prepared) (bool, error) {
	if err := p.recoveryGuard(ctx, i); err != nil {
		return false, err
	}
	p.seen = append(p.seen, i)
	return i.PriorSubmission != nil && i.PriorSubmission.Failed && i.ExpectedRevision == i.PriorSubmission.Prepared.Revision.ID, nil
}

// UC-06 CD-10: a replacement interrupted after a published failure is recovered
// against the failed revision's pin and releases the Environment.
func TestRecoveryAfterPublishedFailureObservesTheFailedPinAndReleases(t *testing.T) {
	c, rp, ctx, req := rolloutFixture(t, nil)
	p := &failedPinProvider{rolloutProvider: rp, recoveryGuard: c.GuardRecovery}
	c.Registry = port.NewRegistry()
	if err := c.Registry.Register("fleet", p); err != nil {
		t.Fatal(err)
	}
	rp.rolloutHealth = domain.Degraded
	if err := c.Deploy(ctx, req); !errors.Is(err, ErrRolloutDegraded) {
		t.Fatal("expected a determined degraded failure", err)
	}
	failedRevision := rolloutRevision(t, c, ctx, req)
	owner, _ := persistence.OwnerFrom(ctx)
	if err := c.Store.ReleaseOperation(ctx, owner, environment.OpFailed, ""); err != nil {
		t.Fatal(err)
	}
	// The replacement is interrupted before publication.
	ctx = persistence.WithHold(claimDeploy(t, c, req, nil))
	rp.rolloutHealth = domain.Healthy
	p.rolloutError = errors.New("before publication")
	req.DeploymentID = ids.New()
	req.Manifests[0].Object = map[string]any{"kind": "Deployment", "fixed": true}
	if err := c.Deploy(ctx, req); err == nil {
		t.Fatal("missing interrupted replacement")
	}
	owner, _ = persistence.OwnerFrom(ctx)
	if err := c.Store.SuspendOperation(ctx, owner, "prior worker stopped"); err != nil {
		t.Fatal(err)
	}
	op, err := c.Store.BeginRecovery(context.Background(), owner.OperationID, "failed-pin-recovery", "operator")
	if err != nil {
		t.Fatal(err)
	}
	ctx = persistence.WithOwner(context.Background(), persistence.Owner{OperationID: op.ID, Owner: op.Owner, Fence: op.Fence})
	if err := c.RecoverSubmissions(ctx, req.OrganizationKey, req.Environment); err != nil {
		t.Fatal("recovery after a published failure", err)
	}
	if len(p.seen) != 1 || p.seen[0].ExpectedRevision != failedRevision || p.seen[0].Revision.ID == failedRevision {
		t.Fatal("recovery did not observe against the failed pin", p.seen)
	}
	if rolloutStatus(t, c, ctx, req)[domain.Rollout] != domain.IntentFailed {
		t.Fatal("absent replacement was not settled")
	}
	owner, _ = persistence.OwnerFrom(ctx)
	if err := c.Store.ReleaseOperation(ctx, owner, environment.OpRecovered, ""); err != nil {
		t.Fatal(err)
	}
	// The released Environment deploys the correction against the same pin.
	ctx = persistence.WithHold(claimDeploy(t, c, req, nil))
	p.rolloutError = nil
	req.DeploymentID = ids.New()
	if err := c.Deploy(ctx, req); err != nil {
		t.Fatal("deploy after recovery", err)
	}
	rollouts := rp.expected[domain.Rollout]
	if rollouts[len(rollouts)-1] != failedRevision {
		t.Fatal("corrected rollout did not replace the failed revision", rollouts, failedRevision)
	}
}
