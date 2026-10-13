package envops_test

import (
	"context"
	"errors"
	"testing"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/application/envops"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/ports/persistence"
)

// An executor that marks an unknown outcome makes End suspend the claim: the
// Environment stays held (INTERRUPTED) whatever status the caller records, so
// no other operation and no duplicate CREATE can start.
func TestLeaseEndSuspendsWhenAnExecutorMarkedAHold(t *testing.T) {
	st := store.New()
	appKey := seedEnvironment(t, st)
	manager := envops.NewManager(st)
	ctx := context.Background()

	released, err := manager.Begin(ctx, envops.Claim{ApplicationKey: appKey, EnvironmentKey: "staging", Kind: environment.OpDeploy})
	if err != nil {
		t.Fatal(err)
	}
	if err := released.End(environment.OpFailed, "an ordinary failure"); err != nil {
		t.Fatal(err)
	}

	lease, err := manager.Begin(ctx, envops.Claim{ApplicationKey: appKey, EnvironmentKey: "staging", Kind: environment.OpDeploy})
	if err != nil {
		t.Fatal(err)
	}
	if persistence.Held(lease.Ctx) {
		t.Fatal("a fresh claim is already held")
	}
	persistence.MarkHold(lease.Ctx)
	if err := lease.End(environment.OpSucceeded, ""); err != nil {
		t.Fatalf("end: %v", err)
	}
	if _, err := manager.Begin(ctx, envops.Claim{ApplicationKey: appKey, EnvironmentKey: "staging", Kind: environment.OpDeploy}); !errors.Is(err, persistence.ErrEnvironmentBusy) {
		t.Fatalf("a held Environment was released: %v", err)
	}
	op, ok, err := st.ActiveOperation(ctx, appKey, "staging")
	if err != nil || !ok || op.Status != environment.OpInterrupted {
		t.Fatalf("held operation %+v %v %v", op, ok, err)
	}
}
