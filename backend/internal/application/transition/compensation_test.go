package transition_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"orchestrator/internal/application/transition"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/ports/persistence"
)

func (h *harness) destNamespace() string {
	return environment.NamespaceFor(h.env().NamespaceIdentity, 1)
}

func indexOf(lines []string, contains string, from int) int {
	for i := from; i < len(lines); i++ {
		if strings.Contains(lines[i], contains) {
			return i
		}
	}
	return -1
}

// Destination writers that this attempt started are stopped and their route is
// removed BEFORE the source gets its route and writers back.
func TestCompensationStopsDestinationBeforeRestoringSource(t *testing.T) {
	h := newHarness(t)
	h.seedJobs(3)
	p := h.preview(environment.ModeMigratePostgres)
	h.cluster.InjectOnce("route", errors.New("destination route failed"), 2)
	detail, err := h.execute(p, environment.ModeMigratePostgres, true)
	if err != nil || detail.Status != environment.TransitionFailed || len(detail.CompensationFailed) > 0 {
		t.Fatalf("%v %+v", err, detail)
	}
	dest, source := h.destNamespace(), h.env().Namespace()
	log := h.cluster.Calls()
	removeDest := indexOf(log, "route "+dest+" paths=0", 0)
	stopDest := indexOf(log, "scale "+dest+"/", 0)
	restoreRoute := indexOf(log, "route "+source+" paths=", indexOf(log, "route "+dest+" paths=0", 0))
	restoreWriter := indexOf(log, "scale-up "+source+"/", 0)
	if removeDest < 0 || stopDest < 0 || restoreRoute < 0 || restoreWriter < 0 || stopDest > restoreWriter || removeDest > restoreRoute {
		t.Fatalf("order: removeDest=%d stopDest=%d restoreRoute=%d restoreWriter=%d\n%v", removeDest, stopDest, restoreRoute, restoreWriter, log)
	}
	for _, workload := range []string{"backend", "worker", "frontend"} {
		if n, _ := h.cluster.ReplicasOf(h.destTarget(), workload); n != 0 {
			t.Fatalf("destination %s still running: %d", workload, n)
		}
	}
	stored, _ := h.app.Store.GetTransition(context.Background(), detail.ID)
	if len(stored.DestinationWorkloads) == 0 {
		t.Fatal("destination writer identity must be persisted for crash recovery")
	}
}

// If the destination cannot be stopped the source is NOT restored (two writers
// would diverge), the claim stays held and recovery completes it later.
func TestFailedDestinationStopLeavesSourceUnrestoredAndClaimHeldUntilRecovery(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.seedJobs(3)
	p := h.preview(environment.ModeDeployNew)
	h.cluster.InjectOnce("route", errors.New("destination route failed"), 2)
	h.cluster.Inject("scale", errors.New("cannot stop"))
	detail, err := h.execute(p, environment.ModeDeployNew, false)
	if err != nil || detail.Status != environment.TransitionFailed {
		t.Fatalf("%v %+v", err, detail)
	}
	joined := strings.Join(detail.CompensationFailed, ";")
	if !strings.Contains(joined, "stop destination workload") || !strings.Contains(joined, "source NOT restored") || detail.SourceState != environment.SourceNeedsAttention {
		t.Fatalf("unsafe rollback must be reported: %+v", detail)
	}
	if strings.Contains(strings.Join(detail.Compensation, ";"), "restore source routes") {
		t.Fatalf("source routes must not be restored while the destination may still write: %v", detail.Compensation)
	}
	env := h.env()
	op, held, _ := h.app.Store.ActiveOperation(ctx, h.opts.ApplicationKey, h.opts.EnvironmentKey)
	if !env.Busy() || !held || op.Status != environment.OpInterrupted || !strings.Contains(op.Failure, "operator action") {
		t.Fatalf("the claim must stay held and visible: busy=%v %+v", env.Busy(), op)
	}
	// While held, nothing else may run.
	if _, err := h.service.Preview(ctx, h.request(environment.ModeDeployNew)); !errors.Is(err, persistence.ErrEnvironmentBusy) {
		t.Fatalf("held environment: %v", err)
	}
	// Recovery needs the explicit confirmation, then completes the rollback.
	if _, err := h.service.Recover(ctx, h.opts.OrganizationKey, h.opts.ApplicationKey, h.opts.EnvironmentKey, op.ID, ""); !errors.Is(err, persistence.ErrRecoveryUnconfirmed) {
		t.Fatalf("unconfirmed recovery: %v", err)
	}
	h.cluster.Inject("scale", nil)
	result, err := h.service.Recover(ctx, h.opts.OrganizationKey, h.opts.ApplicationKey, h.opts.EnvironmentKey, op.ID, "operator")
	if err != nil || len(result.CompensationFailed) > 0 {
		t.Fatalf("recovery: %+v %v", result, err)
	}
	if h.env().Busy() {
		t.Fatal("successful recovery must release the claim")
	}
	for _, workload := range []string{"backend", "worker", "frontend"} {
		if n, _ := h.cluster.ReplicasOf(h.destTarget(), workload); n != 0 {
			t.Fatalf("destination %s still running after recovery: %d", workload, n)
		}
		if n, _ := h.cluster.ReplicasOf(h.sourceTarget(h.env()), workload); n < 1 {
			t.Fatalf("source %s not authoritative after recovery: %d", workload, n)
		}
	}
	if !h.hasRoute(h.env()) {
		t.Fatal("source route not restored by recovery")
	}
}

func TestRouteRemovalFailureIsRecordedAndSourceTrafficIsNotClaimedRestored(t *testing.T) {
	h := newHarness(t)
	h.seedJobs(1)
	p := h.preview(environment.ModeDeployNew)
	h.cluster.InjectOnce("route", errors.New("fail apply"), 2)   // destination route apply
	h.cluster.InjectOnce("route", errors.New("fail removal"), 3) // compensation: destination route removal
	detail, err := h.execute(p, environment.ModeDeployNew, false)
	if err != nil || detail.Status != environment.TransitionFailed {
		t.Fatalf("%v %+v", err, detail)
	}
	joined := strings.Join(detail.CompensationFailed, ";")
	if !strings.Contains(joined, "remove destination route") || !strings.Contains(joined, "source NOT restored") || detail.SourceState != environment.SourceNeedsAttention {
		t.Fatalf("route removal failure must be recorded: %+v", detail)
	}
	if strings.Contains(strings.Join(detail.Compensation, ";"), "restore source routes") {
		t.Fatalf("source traffic must not be claimed restored: %v", detail.Compensation)
	}
	if !h.env().Busy() {
		t.Fatal("the claim must stay held for operator action")
	}
}

func TestRecoveryWithoutEvidenceKeepsTheClaim(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	ops := h.app.Operations
	ops.SetTiming(time.Hour, 10*time.Millisecond)
	// An orphaned TRANSITION claim whose record does not exist: nothing proves
	// what it changed, so recovery must refuse to release it.
	op, err := h.app.Store.ClaimEnvironment(ctx, persistence.OperationClaim{ApplicationKey: h.opts.ApplicationKey, EnvironmentKey: h.opts.EnvironmentKey, Owner: "dead", Kind: environment.OpTransition, Deadline: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(30 * time.Millisecond)
	if n, _ := ops.Sweep(ctx); n != 1 {
		t.Fatalf("sweep: %d", n)
	}
	result, err := h.service.Recover(ctx, h.opts.OrganizationKey, h.opts.ApplicationKey, h.opts.EnvironmentKey, op.ID, "operator")
	if !errors.Is(err, transition.ErrRecoveryIncomplete) || !strings.Contains(result.Outcome, "did not complete") {
		t.Fatalf("recovery without evidence: %+v %v", result, err)
	}
	after, _ := h.app.Store.GetOperation(ctx, op.ID)
	if after.Status != environment.OpInterrupted || !h.env().Busy() {
		t.Fatalf("the claim must remain held: %+v", after)
	}
}

// After a committed cutover the destination is authoritative; a source writer
// that cannot be stopped is reported as needing attention, never as quiesced,
// and the source cannot be cleaned up in that state.
func TestSourceWriterThatCannotBeStoppedIsReportedNotQuiesced(t *testing.T) {
	h := newHarness(t)
	h.seedJobs(1)
	p := h.preview(environment.ModeDeployNew)
	h.cluster.Inject("scale", errors.New("cannot stop"))
	detail, err := h.execute(p, environment.ModeDeployNew, false)
	if err != nil || detail.Status != environment.TransitionSucceeded {
		t.Fatalf("%v %+v", err, detail)
	}
	if detail.SourceState != environment.SourceNeedsAttention || detail.Authority != "DESTINATION" || detail.CanCleanupSource || len(detail.CompensationFailed) == 0 {
		t.Fatalf("source state must be truthful: %+v", detail)
	}
	if _, err := h.service.CleanupSource(context.Background(), h.opts.OrganizationKey, h.opts.ApplicationKey, h.opts.EnvironmentKey, detail.ID); !errors.Is(err, transition.ErrCleanupRefused) {
		t.Fatalf("cleanup must match the actual source state: %v", err)
	}
}

// A transition record left RUNNING by a stopped process reads INTERRUPTED, and
// its authority follows the persisted binding.
func TestInterruptedRecordReportsAuthorityFromThePersistedBinding(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	op, err := h.app.Store.ClaimEnvironment(ctx, persistence.OperationClaim{ApplicationKey: h.opts.ApplicationKey, EnvironmentKey: h.opts.EnvironmentKey, Owner: "dead", Kind: environment.OpTransition, Deadline: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	env := h.env()
	record := environment.Transition{ID: "11111111-1111-4111-8111-111111111111", OperationID: op.ID, ApplicationKey: h.opts.ApplicationKey, EnvironmentKey: h.opts.EnvironmentKey, Mode: environment.ModeDeployNew,
		Stage: environment.StageCutover, Status: environment.TransitionRunning, Source: env.Binding(), Destination: env.Binding(), SourceState: environment.SourceAuthoritative, CreatedAt: time.Now()}
	record.Destination.ConnectionKey, record.Destination.Generation = destKey, 1
	if err := h.app.Store.SaveTransition(ctx, record); err != nil {
		t.Fatal(err)
	}
	h.app.Operations.SetTiming(time.Hour, time.Nanosecond)
	time.Sleep(5 * time.Millisecond)
	_, _ = h.app.Operations.Sweep(ctx)
	got, err := h.service.Get(ctx, h.opts.OrganizationKey, h.opts.ApplicationKey, h.opts.EnvironmentKey, record.ID)
	if err != nil || got.Status != environment.TransitionInterrupted || got.Authority != "SOURCE" {
		t.Fatalf("before the commit the source is authoritative: %+v %v", got, err)
	}
}
