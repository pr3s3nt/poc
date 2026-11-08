package tfexec_test

import (
	"context"
	"errors"
	"testing"

	"orchestrator/internal/application/envops"
	"orchestrator/internal/application/resourcecleanup"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/platform/canon"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
)

func TestEmptyRollbackHoldsOneClaimThroughRemovalAndRealTerraformCleanup(t *testing.T) {
	for _, unexpectedChange := range []bool{false, true} {
		name := "success"
		if unexpectedChange {
			name = "changed-version-blocks"
		}
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			ctx := context.Background()
			created := h.createInstance("first-deploy", defaultDescriptor, defaultScope(), nil)
			setup, op := h.claim("setup-empty-set")
			env, err := h.store.GetEnvironment(setup, h.appKey, "staging")
			if err != nil {
				t.Fatal(err)
			}
			doc := environment.NewDocument()
			hash, _ := canon.Hash(doc)
			set := environment.DeploymentSet{ID: ids.New(), EnvironmentID: env.ID, EnvironmentKey: h.appKey + "/staging", Document: doc, DocumentHash: hash}
			if err := h.store.SaveDeploymentSet(setup, set); err != nil {
				t.Fatal(err)
			}
			if err := h.store.CompareVersionAndSetCurrent(setup, h.appKey, "staging", env.Version, set.ID); err != nil {
				t.Fatal(err)
			}
			h.release(op, environment.OpSucceeded)
			manager := envops.NewManager(h.store)
			cleanup := resourcecleanup.New(h.store, manager)
			cleanup.SetState(h.state)
			cleanup.SetEngine(h.svc)
			scope := resourcecleanup.Scope{OrganizationKey: h.orgKey, ApplicationKey: h.appKey, EnvironmentKey: "staging"}
			inv, err := cleanup.RollbackInventory(ctx, scope)
			if err != nil || len(inv.IDs) != 1 {
				t.Fatalf("inventory: %+v %v", inv, err)
			}
			env, _ = h.store.GetEnvironment(ctx, h.appKey, "staging")
			lease, err := cleanup.ClaimRollback(ctx, scope, "test-admin", inv, env)
			if err != nil {
				t.Fatal(err)
			}
			defer lease.End(environment.OpFailed, "test cleanup")
			if _, err := manager.Begin(ctx, envops.Claim{ApplicationKey: h.appKey, EnvironmentKey: "staging", Kind: environment.OpDeploy}); !errors.Is(err, persistence.ErrEnvironmentBusy) {
				t.Fatalf("concurrent deploy admitted: %v", err)
			}
			// This is precisely the metadata change performed by workload removal.
			inst, err := h.state.GetInstance(lease.Ctx, created.InstanceID)
			if err != nil {
				t.Fatal(err)
			}
			inst, err = h.state.SetInstanceStatus(lease.Ctx, inst.ID, inst.Version, tfresource.InstanceUnreferenced)
			if err != nil {
				t.Fatal(err)
			}
			if unexpectedChange {
				_, err = h.state.SetInstanceStatus(lease.Ctx, inst.ID, inst.Version, tfresource.InstanceReady)
				if err != nil {
					t.Fatal(err)
				}
			}
			err = cleanup.FinishRollbackCleanup(lease, scope, inv)
			if unexpectedChange {
				if err == nil {
					t.Fatal("unreviewed version/status change destroyed resource")
				}
				if got := h.status(inst.ID); got == tfresource.InstanceDestroyed {
					t.Fatal("blocked resource destroyed")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := h.status(inst.ID); got != tfresource.InstanceDestroyed {
				t.Fatalf("not destroyed: %s", got)
			}
			current, err := h.store.GetEnvironment(ctx, h.appKey, "staging")
			if err != nil || current.ActiveOperationID != lease.Operation.ID {
				t.Fatalf("claim lost between phases: %+v %v", current, err)
			}
			if err := lease.End(environment.OpSucceeded, ""); err != nil {
				t.Fatal(err)
			}
		})
	}
}
