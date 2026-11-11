package envops_test

import (
	"context"
	"testing"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/application/envops"
	"orchestrator/internal/domain/environment"
)

// Issue #68: executor observations are persisted on the owning operation and
// survive a later Stage that replaces the stage detail.
func TestAnnotatePersistsAcrossStage(t *testing.T) {
	st := store.New()
	appKey := seedEnvironment(t, st)
	ctx := context.Background()
	if err := envops.Annotate(ctx, "ignored", 1); err != nil {
		t.Fatal("outside a lease:", err)
	}
	lease, err := envops.NewManager(st).Begin(ctx, envops.Claim{ApplicationKey: appKey, EnvironmentKey: "staging", Kind: environment.OpDeploy, Detail: map[string]any{"changes": 1}})
	if err != nil {
		t.Fatal(err)
	}
	if err := envops.Annotate(lease.Ctx, "stuckPods", "pg-0: ImagePullBackOff"); err != nil {
		t.Fatal(err)
	}
	if err := lease.Stage("RELEASE_READY", map[string]any{"releaseSnapshot": "x"}); err != nil {
		t.Fatal(err)
	}
	if err := lease.End(environment.OpFailed, "not ready"); err != nil {
		t.Fatal(err)
	}
	op, err := st.GetOperation(ctx, lease.Operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if op.Detail["stuckPods"] != "pg-0: ImagePullBackOff" || op.Detail["releaseSnapshot"] != "x" {
		t.Fatalf("detail = %+v", op.Detail)
	}
}
