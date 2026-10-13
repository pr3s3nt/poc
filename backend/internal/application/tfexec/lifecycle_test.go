package tfexec_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"orchestrator/internal/adapters/postgres"
	"orchestrator/internal/adapters/terraform/runner/runnertest"
	"orchestrator/internal/application/statebackend"
	"orchestrator/internal/application/tfexec"
	deliveryhttp "orchestrator/internal/delivery/http"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/tfresource"
)

// FAKE CLI (script): cancelling the caller stops the whole process group
// before the executor returns; the outcome is unknown and holds.
func TestCancellationStopsProcessGroupThenHolds(t *testing.T) {
	h := newHarness(t)
	marker := filepath.Join(t.TempDir(), "child.pid")
	h.cli = runnertest.Script(t, "/bin/sleep 300 &\necho $! > "+marker+"\nwait\n")
	ctx, op := h.claim("proc-a")
	cctx, cancel := context.WithCancel(ctx)
	go func() {
		for i := 0; i < 500; i++ {
			if _, err := os.Stat(marker); err == nil {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		cancel()
	}()
	_, err := h.svc.Execute(cctx, h.request("run-1", h.mod, inputs("db1", "k")))
	if !errors.Is(err, tfexec.ErrNeedsAttention) {
		t.Fatalf("cancelled execution: %v", err)
	}
	pid, _ := os.ReadFile(marker)
	out, _ := exec.Command("sh", "-c", "ps -o stat= -p "+strings.TrimSpace(string(pid))).Output()
	if s := strings.TrimSpace(string(out)); s != "" && !strings.HasPrefix(s, "Z") {
		t.Fatalf("child %s survived cancellation (%s)", pid, s)
	}
	inst, _ := h.instance(ctx, defaultDescriptor, defaultScope())
	attempts := h.attempts(inst.ID)
	if inst.Status != tfresource.InstanceNeedsAttention || attempts[0].SafeErrorCode != "CANCELLED" {
		t.Fatalf("held: %+v %+v", inst, attempts)
	}
	_ = op
}

// State persists across a restart of the state repository, backend service
// and listener: a new process reads the encrypted state and updates in place.
func TestStatePersistsAcrossBackendReopen(t *testing.T) {
	h := newHarness(t)
	ctx, op := h.claim("proc-a")
	first := mustExecute(t, h, ctx, h.request("run-1", h.mod, inputs("db1", "k")))
	h.release(op, environment.OpSucceeded)

	h.state.Close()
	state, err := postgres.OpenState(context.Background(), h.db.State)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(state.Close)
	backend, err := statebackend.New(state, h.cipher)
	if err != nil {
		t.Fatal(err)
	}
	l, err := deliveryhttp.ServeTerraformStateLoopback(deliveryhttp.NewTerraformStateHandler(backend))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = l.Close(c)
	})
	h.state, h.stateRepo, h.backend, h.base = state, state, backend, l.Address
	svc := h.newService(nil)
	ctx2, _ := h.claim("proc-b")
	second, err := svc.Execute(ctx2, h.request("run-2", h.mod, inputs("db2", "k")))
	if err != nil || second.InstanceID != first.InstanceID || second.Action != tfresource.ActionUpdate || second.Outputs["database"] != "db2" || second.Outputs["password"] != first.Outputs["password"] {
		t.Fatalf("update after reopen: %+v %v", second, err)
	}
}
