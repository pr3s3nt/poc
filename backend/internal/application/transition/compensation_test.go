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
