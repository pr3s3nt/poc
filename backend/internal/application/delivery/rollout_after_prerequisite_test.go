package delivery

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "orchestrator/internal/domain/delivery"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/platform/ids"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/persistence"
)

// rolloutInterruptedProvider loses the first Rollout submission before
// publication and proves only that Rollout absent, as the Fleet adapter does.
type rolloutInterruptedProvider struct {
	*neutralProvider
	recoveryGuard func(context.Context, port.Intent) error
	failRollout   bool
	absenceReads  []domain.Phase
	recoveries    int
}

func (p *rolloutInterruptedProvider) Submit(ctx context.Context, i port.Intent, prepared port.Prepared) (port.Receipt, error) {
	if i.Revision.Phase == domain.Rollout && p.failRollout {
		p.order = append(p.order, i.Revision.Phase)
		return port.Receipt{}, errors.New("worker stopped before publication")
	}
	return p.neutralProvider.Submit(ctx, i, prepared)
}

func (p *rolloutInterruptedProvider) ObserveSubmissionAbsent(ctx context.Context, i port.Intent, _ port.Prepared) (bool, error) {
	if err := p.recoveryGuard(ctx, i); err != nil {
		return false, err
	}
	p.absenceReads = append(p.absenceReads, i.Revision.Phase)
	return i.Revision.Phase == domain.Rollout, nil
}

func (p *rolloutInterruptedProvider) RecoverSubmission(context.Context, port.Intent, port.Prepared, port.Receipt, port.TransferSource) (port.Receipt, error) {
	p.recoveries++
	return port.Receipt{}, domain.ErrUnsupported
}

// Issue #48: a first Rollout interrupted after its Prerequisite completed is
// settled FAILED by recovery once proven absent; the Prerequisite stays
// acknowledged and untouched, nothing is replayed, and redeploy is the user's.
func TestRecoveryEndsUnpublishedRolloutAndKeepsCompletedPrerequisite(t *testing.T) {
	c, original, ctx, req, _ := coordinatorFixture(t)
	p := &rolloutInterruptedProvider{neutralProvider: original, recoveryGuard: c.GuardRecovery, failRollout: true}
	c.Registry = port.NewRegistry()
	if err := c.Registry.Register("fleet", p); err != nil {
		t.Fatal(err)
	}
	ctx = persistence.WithHold(ctx)
	if err := c.Deploy(ctx, req); err == nil {
		t.Fatal("missing interrupted Rollout")
	}
	if !persistence.Held(ctx) {
		t.Fatal("an unknown Rollout outcome must hold the Environment")
	}
	org, _ := c.Store.GetOrganization(ctx, req.OrganizationKey)
	before, _ := c.Store.ListDeliveryExecutions(ctx, org.ID, req.Environment.ID)
	prerequisite := map[string]persistence.DeliveryExecution{}
	for _, r := range before {
		if r.Revision.Phase == domain.Prerequisite {
			if r.Mutation.Status != domain.IntentAcknowledged {
				t.Fatal("Prerequisite did not complete before the Rollout", r.Mutation.Status)
			}
			prerequisite[r.Mutation.ID] = r
		}
	}
	if len(prerequisite) == 0 {
		t.Fatal("fixture has no Prerequisite")
	}
	submitted := len(p.order)

	owner, _ := persistence.OwnerFrom(ctx)
	if err := c.Store.SuspendOperation(ctx, owner, "worker stopped"); err != nil {
		t.Fatal(err)
	}
	op, err := c.Store.BeginRecovery(context.Background(), owner.OperationID, "rollout-after-prerequisite", "operator")
	if err != nil {
		t.Fatal(err)
	}
	recovery := persistence.WithOwner(context.Background(), persistence.Owner{OperationID: op.ID, Owner: op.Owner, Fence: op.Fence})
	if err := c.RecoverSubmissions(recovery, req.OrganizationKey, req.Environment); err != nil {
		t.Fatal("recovery of an unpublished Rollout", err)
	}
	if len(p.absenceReads) != 1 || p.absenceReads[0] != domain.Rollout || p.recoveries != 0 || len(p.order) != submitted {
		t.Fatal("recovery observed or replayed beyond the Rollout", p.absenceReads, p.recoveries, p.order)
	}
	after, _ := c.Store.ListDeliveryExecutions(recovery, org.ID, req.Environment.ID)
	for _, r := range after {
		switch r.Revision.Phase {
		case domain.Rollout:
			if r.Mutation.Status != domain.IntentFailed {
				t.Fatal("unpublished Rollout not settled FAILED", r.Mutation.Status)
			}
		case domain.Prerequisite:
			old, ok := prerequisite[r.Mutation.ID]
			if !ok || r.Mutation.Status != domain.IntentAcknowledged || r.Mutation.Version != old.Mutation.Version || r.Receipt.RevisionID != old.Receipt.RevisionID || len(r.Inventory) != len(old.Inventory) {
				t.Fatal("completed Prerequisite changed by recovery", r.Mutation, old.Mutation)
			}
		}
	}
	owner, _ = persistence.OwnerFrom(recovery)
	if err := c.Store.ReleaseOperation(recovery, owner, environment.OpRecovered, ""); err != nil {
		t.Fatal("Environment not released after recovery", err)
	}

	// Redeploy is a separate user Deploy; it completes the Rollout.
	claim, err := c.Store.ClaimEnvironment(context.Background(), persistence.OperationClaim{ID: ids.New(), ApplicationKey: req.ApplicationKey, EnvironmentKey: req.Environment.Key, Owner: "redeploy", Kind: environment.OpDeploy, Deadline: time.Now().Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	ctx = persistence.WithOwner(context.Background(), persistence.Owner{OperationID: claim.ID, Owner: claim.Owner, Fence: claim.Fence})
	p.failRollout = false
	req.DeploymentID = ids.New()
	if err := c.Deploy(ctx, req); err != nil {
		t.Fatal("redeploy after recovery", err)
	}
	redeployed := p.order[submitted:]
	if len(redeployed) == 0 || redeployed[len(redeployed)-1] != domain.Rollout {
		t.Fatal("redeploy did not submit the Rollout", redeployed)
	}
}
