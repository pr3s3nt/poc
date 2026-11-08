package delivery

import (
	"context"
	"errors"
	domain "orchestrator/internal/domain/delivery"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/platform/ids"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/ports/persistence"
	"testing"
	"time"
)

type terminalJobFake struct {
	*receiptRecoveryFake
	proof bool
	empty bool
}

func (p *terminalJobFake) Observe(ctx context.Context, i port.Intent, r port.Receipt) (domain.Observation, error) {
	if i.Revision.Phase == domain.Job {
		return domain.Observation{}, errors.New("terminal Job failure")
	}
	return p.receiptRecoveryFake.Observe(ctx, i, r)
}
func (p *terminalJobFake) ObserveTerminalJobFailure(ctx context.Context, i port.Intent, _ port.Prepared, _ port.Receipt) ([]port.AppliedObject, bool, error) {
	if err := p.recoveryGuard(ctx, i); err != nil {
		return nil, false, err
	}
	if p.empty {
		return nil, true, nil
	}
	return []port.AppliedObject{{ObjectRef: execution.ObjectRef{APIVersion: "batch/v1", Kind: "Job", Name: "app"}, UID: "job-original"}}, p.proof, nil
}
func TestTerminalJobRecoveryRetainsFailureForRemovalAndNeverReplays(t *testing.T) {
	for _, known := range []bool{false, true} {
		t.Run(map[bool]string{false: "unknown-keeps-hold", true: "terminal-settles-failure"}[known], func(t *testing.T) {
			c, original, ctx, req, _ := coordinatorFixture(t)
			p := &terminalJobFake{receiptRecoveryFake: &receiptRecoveryFake{transferFake: &transferFake{neutralProvider: original, live: map[string]string{}}, recoveryGuard: c.GuardRecovery}, proof: known}
			c.Registry = port.NewRegistry()
			if err := c.Registry.Register("fleet", p); err != nil {
				t.Fatal(err)
			}
			req.Manifests = []execution.Manifest{{APIVersion: "batch/v1", Kind: "Job", Name: "app", Namespace: "app", Object: map[string]any{"apiVersion": "batch/v1", "kind": "Job", "metadata": map[string]any{"name": "app", "namespace": "app"}}}}
			if err := c.Deploy(ctx, req); err == nil {
				t.Fatal("failed Job committed successfully")
			}
			calls := len(p.order)
			owner, _ := persistence.OwnerFrom(ctx)
			if err := c.Store.SuspendOperation(ctx, owner, "executor stopped"); err != nil {
				t.Fatal(err)
			}
			op, err := c.Store.BeginRecovery(context.Background(), owner.OperationID, "failed-job-recovery", "operator")
			if err != nil {
				t.Fatal(err)
			}
			ctx = persistence.WithOwner(context.Background(), persistence.Owner{OperationID: op.ID, Owner: op.Owner, Fence: op.Fence})
			err = c.RecoverSubmissions(ctx, req.OrganizationKey, req.Environment)
			if !known {
				if err == nil {
					t.Fatal("unknown failure released")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			org, _ := c.Store.GetOrganization(ctx, req.OrganizationKey)
			records, err := c.Store.ListDeliveryExecutions(ctx, org.ID, req.Environment.ID)
			if err != nil || len(records) != 1 || !records[0].TerminalJobFailure() {
				t.Fatal("failed publication/inventory not retained", err)
			}
			if records[0].Inventory[0].UID != "job-original" || len(p.order) != calls || p.recoveries != 0 {
				t.Fatal("recovery replayed/woke submission or changed UID")
			}
			active, err := activeExecutions(records, req.WorkloadID)
			if err != nil || len(active) != 1 {
				t.Fatal("failed Job not removable", err)
			}
			tombstone := records[0]
			tombstone.Mutation.Status = domain.IntentAcknowledged
			tombstone.Mutation.Action = domain.Withdraw
			tombstone.CreatedAt = tombstone.CreatedAt.Add(time.Second)
			active, err = activeExecutions(append(records, tombstone), req.WorkloadID)
			if err != nil || len(active) != 0 {
				t.Fatal("withdrawal resurrected failure", err)
			}
			owner, _ = persistence.OwnerFrom(ctx)
			if err = c.Store.ReleaseOperation(ctx, owner, environment.OpRecovered, ""); err != nil {
				t.Fatal(err)
			}
			next, err := c.Store.ClaimEnvironment(context.Background(), persistence.OperationClaim{ID: ids.New(), ApplicationKey: req.ApplicationKey, EnvironmentKey: req.Environment.Key, Owner: "next-deploy", Kind: environment.OpDeploy, Deadline: time.Now().Add(time.Minute)})
			if err != nil {
				t.Fatal(err)
			}
			ctx = persistence.WithOwner(context.Background(), persistence.Owner{OperationID: next.ID, Owner: next.Owner, Fence: next.Fence})
			req.DeploymentID = ids.New()
			if err = c.Deploy(ctx, req); !errors.Is(err, domain.ErrJobFailed) {
				t.Fatal("failed Job reused as completion", err)
			}
			if len(p.order) != calls {
				t.Fatal("failed Job replayed")
			}
		})
	}
}
