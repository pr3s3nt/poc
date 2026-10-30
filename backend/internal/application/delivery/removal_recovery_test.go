package delivery

import (
	"context"
	"testing"

	domain "orchestrator/internal/domain/delivery"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/persistence"
)

type removalRecoveryFake struct {
	*transferFake
	failAfterDelete bool
}

func (p *removalRecoveryFake) RemoveRetainedObjects(ctx context.Context, i port.Intent, _ port.Receipt, objects []port.AppliedObject) error {
	if err := p.guard(ctx, i); err != nil {
		return err
	}
	for _, o := range objects {
		if uid, present := p.live[o.Key()]; present && uid != o.UID {
			return domain.ErrStale
		}
		delete(p.live, o.Key())
	}
	if p.failAfterDelete {
		p.failAfterDelete = false
		return domain.ErrOutcomeUnknown
	}
	return nil
}
func TestRemovalRecoveryRetainsOriginalUIDProofAndNeverSubmits(t *testing.T) {
	c, original, ctx, req, _ := coordinatorFixture(t)
	p := &removalRecoveryFake{transferFake: &transferFake{neutralProvider: original, live: map[string]string{}}, failAfterDelete: true}
	c.Registry = port.NewRegistry()
	if err := c.Registry.Register("fleet", p); err != nil {
		t.Fatal(err)
	}
	for n := range req.Manifests {
		m := &req.Manifests[n]
		m.APIVersion, m.Name, m.Namespace = "v1", "app", "app"
		if m.Kind == "Job" {
			m.APIVersion = "batch/v1"
		}
		if m.Kind == "Deployment" {
			m.APIVersion = "apps/v1"
		}
		m.Object = map[string]any{"apiVersion": m.APIVersion, "kind": m.Kind, "metadata": map[string]any{"name": m.Name, "namespace": m.Namespace}}
	}
	if err := c.Deploy(ctx, req); err != nil {
		t.Fatal(err)
	}
	submitted := len(p.order)
	if err := c.RemoveWorkload(ctx, req.OrganizationKey, req.Environment.ID, req.WorkloadID); err == nil {
		t.Fatal("lost delete acknowledgement reported success")
	}
	owner, _ := persistence.OwnerFrom(ctx)
	if err := c.Store.SuspendOperation(ctx, owner, "prior worker stopped"); err != nil {
		t.Fatal(err)
	}
	op, err := c.Store.BeginRecovery(context.Background(), owner.OperationID, "removal-recovery", "operator")
	if err != nil {
		t.Fatal(err)
	}
	ctx = persistence.WithOwner(context.Background(), persistence.Owner{OperationID: op.ID, Owner: op.Owner, Fence: op.Fence})
	if err := c.RecoverRemovals(ctx, req.OrganizationKey, req.Environment); err != nil {
		t.Fatal(err)
	}
	if err := c.RemoveWorkload(ctx, req.OrganizationKey, req.Environment.ID, req.WorkloadID); err != nil {
		t.Fatal(err)
	}
	if len(p.live) != 0 || len(p.order) != submitted || p.jobs != 1 {
		t.Fatal("removal recovery replayed Submit/Job or left owned resources")
	}
}
