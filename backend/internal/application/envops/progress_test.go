package envops_test

import (
	"context"
	"orchestrator/internal/adapters/store"
	"orchestrator/internal/application/envops"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/progress"
	"sync"
	"testing"
	"time"
)

func TestProgressSurvivesHeartbeatStageAndRecovery(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	app := seedEnvironment(t, st)
	m := envops.NewManager(st)
	m.SetTiming(time.Millisecond, time.Second)
	lease, err := m.Begin(ctx, envops.Claim{ApplicationKey: app, EnvironmentKey: "staging", Kind: environment.OpDeploy, Detail: map[string]any{"deliveryPolicyOnly": false}})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			progress.Report(lease.Ctx, progress.Step{ID: "wait", Kind: "READINESS", Status: "RUNNING", Pods: []progress.Pod{{Name: "api-1", Reason: "ImagePullBackOff"}}})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			_ = lease.Stage("DEPLOY", map[string]any{"releaseSnapshot": "kept"})
		}
	}()
	wg.Wait()
	if err := lease.Suspend("test interruption"); err != nil {
		t.Fatal(err)
	}
	op, err := st.GetOperation(ctx, lease.Operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	r := progress.Read(op.Detail["progress"])
	if len(r.Steps) != 2 || r.Steps[1].Status != "INTERRUPTED" || len(r.Steps[1].Pods) != 1 || op.Detail["releaseSnapshot"] != "kept" {
		t.Fatalf("lost progress/detail: %+v %+v", r, op.Detail)
	}
	// A fresh process adopts persisted facts, including the old worker heartbeat.
	m2 := envops.NewManager(st)
	op, err = st.BeginRecovery(ctx, op.ID, m2.InstanceID(), "tester")
	if err != nil {
		t.Fatal(err)
	}
	adopted := m2.Adopt(ctx, op)
	adopted.PriorHeartbeat(lease.Operation.HeartbeatAt)
	progress.Check(adopted.Ctx, progress.Step{ID: "ownership", Kind: "RECOVERY_OBJECT", Status: "FAILED", Reason: "OWNERSHIP", ResourceKind: "Deployment", ResourceName: "api", Namespace: "test"})
	if err := adopted.Suspend("held"); err != nil {
		t.Fatal(err)
	}
	op, _ = st.GetOperation(ctx, op.ID)
	r = progress.Read(op.Detail["progress"])
	if len(r.Steps) != 2 || len(r.Checks) != 1 || r.Checks[0].Reason != "OWNERSHIP" || r.PriorHeartbeat == nil {
		t.Fatalf("recovery lost facts: %+v", r)
	}
}

// Integration of #68 wait annotations and #69 progress: either writer must
// persist both projections immediately, including after Stage replacement.
func TestProgressAndWaitAnnotationsPersistTogether(t *testing.T) {
	st := store.New()
	app := seedEnvironment(t, st)
	lease, err := envops.NewManager(st).Begin(context.Background(), envops.Claim{ApplicationKey: app, EnvironmentKey: "staging", Kind: environment.OpDeploy})
	if err != nil {
		t.Fatal(err)
	}
	defer lease.End(environment.OpFailed, "test complete")
	assertBoth := func() {
		t.Helper()
		op, err := st.GetOperation(context.Background(), lease.Operation.ID)
		if err != nil {
			t.Fatal(err)
		}
		if op.Detail["rollout:app"] == nil || len(progress.Read(op.Detail["progress"]).Steps) < 2 {
			t.Fatalf("lost combined observations: %+v", op.Detail)
		}
	}
	if err := envops.Annotate(lease.Ctx, "rollout:app", map[string]any{"waiting": "ImagePullBackOff"}); err != nil {
		t.Fatal(err)
	}
	progress.Report(lease.Ctx, progress.Step{ID: "wait", Kind: "READINESS", Status: "RUNNING", Reason: "WAITING_READINESS"})
	assertBoth()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			progress.Report(lease.Ctx, progress.Step{ID: "wait", Kind: "READINESS", Status: "RUNNING", Reason: "WAITING_READINESS"})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			if err := envops.Annotate(lease.Ctx, "rollout:app", map[string]any{"waiting": "ImagePullBackOff"}); err != nil {
				t.Error(err)
			}
		}
	}()
	wg.Wait()
	assertBoth()
	if err := lease.Stage("READINESS", map[string]any{"phase": "retained"}); err != nil {
		t.Fatal(err)
	}
	assertBoth()
	progress.Report(lease.Ctx, progress.Step{ID: "wait", Kind: "READINESS", Status: "SUCCEEDED"})
	assertBoth()
}
