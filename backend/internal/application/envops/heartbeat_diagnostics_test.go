package envops_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/application/envops"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/ports/persistence"
)

type heartbeatErrorStore struct {
	persistence.Store
	heartbeatErr error
	suspendErr   error
}

func (s heartbeatErrorStore) HeartbeatOperation(context.Context, persistence.Owner, string, map[string]any) error {
	return s.heartbeatErr
}

func (s heartbeatErrorStore) SuspendOperation(ctx context.Context, owner persistence.Owner, failure string) error {
	if s.suspendErr != nil {
		return s.suspendErr
	}
	return s.Store.SuspendOperation(ctx, owner, failure)
}

func TestHeartbeatLossPersistsOnlySafeDiagnosticsAndKeepsHold(t *testing.T) {
	tests := []struct {
		name, cause, trigger string
		err                  error
	}{
		{name: "owner fence lost", cause: "operation_lost", trigger: "operation_lost", err: persistence.ErrOperationLost},
		{name: "heartbeat timeout", cause: "heartbeat_io_timeout", trigger: "stale_after_exceeded", err: fmt.Errorf("private adapter detail: %w", context.DeadlineExceeded)},
		{name: "transient storage errors cross stale threshold", cause: "heartbeat_storage_error", trigger: "stale_after_exceeded", err: errors.New("password=must-not-persist; SQL=private")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			previousLogger := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
			t.Cleanup(func() { slog.SetDefault(previousLogger) })

			ctx := context.Background()
			base := store.New()
			appKey := seedEnvironment(t, base)
			st := heartbeatErrorStore{Store: base, heartbeatErr: tt.err}
			manager := envops.NewManager(st)
			manager.SetTiming(5*time.Millisecond, 40*time.Millisecond)
			lease, err := manager.Begin(ctx, envops.Claim{ApplicationKey: appKey, EnvironmentKey: "staging", Kind: environment.OpDeploy})
			if err != nil {
				t.Fatal(err)
			}
			select {
			case <-lease.Ctx.Done():
			case <-time.After(time.Second):
				t.Fatal("heartbeat failure did not cancel the held lease")
			}
			if !lease.Lost() {
				t.Fatal("heartbeat failure did not mark the lease lost")
			}
			if err := lease.Suspend("deploy failed"); err != nil {
				t.Fatalf("lost lease should suspend safely: %v", err)
			}
			op, held, err := base.ActiveOperation(ctx, appKey, "staging")
			if err != nil || !held || op.Status != environment.OpInterrupted {
				t.Fatalf("heartbeat loss must retain the held operation: op=%+v held=%v err=%v", op, held, err)
			}
			for _, part := range []string{
				"heartbeat_loss cause=" + tt.cause,
				"trigger=" + tt.trigger,
				"operation_id=" + lease.Operation.ID,
				fmt.Sprintf("fence=%d", lease.Operation.Fence),
			} {
				if !strings.Contains(op.Failure, part) {
					t.Errorf("failure is missing safe diagnostic %q: %q", part, op.Failure)
				}
			}
			if strings.Contains(op.Failure, "private adapter detail") || strings.Contains(op.Failure, "password=") || strings.Contains(op.Failure, "SQL=") {
				t.Fatalf("raw heartbeat error leaked into persisted failure: %q", op.Failure)
			}
			assertHeartbeatLossLog(t, logs.String(), tt.cause, tt.trigger, lease.Operation.ID, lease.Operation.Fence)
			if _, err := manager.Begin(ctx, envops.Claim{ApplicationKey: appKey, EnvironmentKey: "staging", Kind: environment.OpDeploy}); !errors.Is(err, persistence.ErrEnvironmentBusy) {
				t.Fatalf("diagnostics must not release the claim: %v", err)
			}
		})
	}
}

func TestHeartbeatLossLogSurvivesFencedSuspensionAndKeepsFirstReason(t *testing.T) {
	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	ctx := context.Background()
	base := store.New()
	appKey := seedEnvironment(t, base)
	denied := heartbeatErrorStore{Store: base, heartbeatErr: persistence.ErrOperationLost, suspendErr: persistence.ErrOperationLost}
	manager := envops.NewManager(denied)
	manager.SetTiming(5*time.Millisecond, 40*time.Millisecond)
	lease, err := manager.Begin(ctx, envops.Claim{ApplicationKey: appKey, EnvironmentKey: "staging", Kind: environment.OpDeploy})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-lease.Ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("owner loss did not cancel the lease")
	}
	if err := lease.Stage("late-stage", nil); !errors.Is(err, persistence.ErrOperationLost) {
		t.Fatalf("late stage should remain fenced: %v", err)
	}
	if err := lease.Suspend("deploy failed"); !errors.Is(err, persistence.ErrOperationLost) {
		t.Fatalf("stale owner must not persist suspension: %v", err)
	}
	assertHeartbeatLossLog(t, logs.String(), "operation_lost", "operation_lost", lease.Operation.ID, lease.Operation.Fence)
	if strings.Contains(logs.String(), "private") || strings.Contains(logs.String(), "SQL") || strings.Contains(logs.String(), "password") {
		t.Fatalf("raw error leaked into heartbeat log: %q", logs.String())
	}
	if strings.Count(logs.String(), "environment operation heartbeat lost") != 1 {
		t.Fatalf("first loss reason must be logged once: %q", logs.String())
	}
}

func assertHeartbeatLossLog(t *testing.T, output, cause, trigger, operationID string, fence int64) {
	t.Helper()
	for _, part := range []string{
		"environment operation heartbeat lost",
		"cause=" + cause,
		"trigger=" + trigger,
		"operation_id=" + operationID,
		fmt.Sprintf("fence=%d", fence),
	} {
		if !strings.Contains(output, part) {
			t.Errorf("heartbeat loss log is missing %q: %q", part, output)
		}
	}
}
