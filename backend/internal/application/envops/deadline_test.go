package envops_test

import (
	"context"
	"testing"
	"time"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/application/envops"
	"orchestrator/internal/domain/environment"
)

// A configured Deploy deadline is the finite hold time that is RECORDED on the
// claim and bounds the lease context (so it cancels every executor call under
// it); it never becomes unlimited and the other kinds keep their defaults.
func TestConfiguredDeployDeadlineIsRecordedBoundsTheLeaseAndStaysFinite(t *testing.T) {
	st := store.New()
	appKey := seedEnvironment(t, st)
	manager := envops.NewManager(st)
	if got := manager.DeadlineOf(environment.OpDeploy); got != 45*time.Minute {
		t.Fatalf("the default Deploy hold changed: %s", got)
	}
	for _, bad := range []time.Duration{0, -time.Minute, envops.MaxDeadline + time.Second} {
		if err := manager.SetDeadline(environment.OpDeploy, bad); err == nil {
			t.Fatalf("%s must be refused", bad)
		}
	}
	if err := manager.SetDeadline(environment.OpDeploy, 170*time.Minute); err != nil {
		t.Fatal(err)
	}
	if manager.DeadlineOf(environment.OpRemove) != envops.DeadlineFor(environment.OpRemove) {
		t.Fatal("another kind lost its default")
	}
	start := time.Now()
	lease, err := manager.Begin(context.Background(), envops.Claim{ApplicationKey: appKey, EnvironmentKey: "staging", Kind: environment.OpDeploy})
	if err != nil {
		t.Fatal(err)
	}
	defer lease.End(environment.OpFailed, "test")
	recorded := lease.Operation.Deadline.Sub(start)
	if recorded < 169*time.Minute || recorded > 171*time.Minute {
		t.Fatalf("the recorded hold is %s, not the configured 170m", recorded)
	}
	deadline, ok := lease.Ctx.Deadline()
	if !ok || !deadline.Equal(lease.Operation.Deadline) {
		t.Fatalf("the lease context deadline %v must equal the recorded deadline %v", deadline, lease.Operation.Deadline)
	}
}

func TestAnExpiredLeaseCancelsTheExecutorContext(t *testing.T) {
	st := store.New()
	appKey := seedEnvironment(t, st)
	manager := envops.NewManager(st)
	lease, err := manager.Begin(context.Background(), envops.Claim{ApplicationKey: appKey, EnvironmentKey: "staging", Kind: environment.OpDeploy, Deadline: 150 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer lease.End(environment.OpFailed, "test")
	select {
	case <-lease.Ctx.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("the lease context outlived its recorded deadline")
	}
	if lease.Ctx.Err() != context.DeadlineExceeded {
		t.Fatalf("cancellation reason %v", lease.Ctx.Err())
	}
}
