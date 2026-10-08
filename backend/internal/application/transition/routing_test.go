package transition_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"orchestrator/internal/application/transition"
	"orchestrator/internal/domain/environment"
)

// Public traffic can only move between Connections that reach the same physical
// cluster; anything else fails before a single writer stops.
func TestDistinctClusterRoutingFailsBeforeAnyWriterStops(t *testing.T) {
	h := newHarness(t)
	h.seedJobs(2)
	h.cluster.SetIdentity("fake-second-context", "another-physical-cluster")
	calls := len(h.cluster.Calls())
	_, err := h.service.Preview(context.Background(), h.request(environment.ModeMigratePostgres))
	var unsupported *transition.UnsupportedError
	if !errors.As(err, &unsupported) || !strings.Contains(err.Error(), "different cluster") {
		t.Fatalf("distinct clusters need an external cutover: %v", err)
	}
	for _, line := range h.cluster.Calls()[calls:] {
		if strings.HasPrefix(line, "scale") || strings.HasPrefix(line, "backup") {
			t.Fatalf("a writer stopped before routing was proven: %s", line)
		}
	}
	// Execute is refused the same way, claim untouched.
	p, _ := h.service.Preview(context.Background(), h.request(environment.ModeDeployNew))
	_ = p
	if _, err := h.service.Execute(context.Background(), transition.ExecuteRequest{Request: h.request(environment.ModeDeployNew), Token: "x", AcknowledgeDowntime: true}); !errors.As(err, &unsupported) {
		t.Fatalf("execute: %v", err)
	}
	if h.env().Busy() {
		t.Fatal("a refused transition must not hold the environment")
	}
	// An unreadable identity also fails safely.
	h.cluster.SetIdentity("fake-second-context", "")
	h.cluster.Inject("identity", errors.New("no access"))
	if _, err := h.service.Preview(context.Background(), h.request(environment.ModeDeployNew)); !errors.As(err, &unsupported) {
		t.Fatalf("unreadable identity: %v", err)
	}
}

// A desired delete of the only routed workload does not hide that the source
// owns a route that must be proven movable.
func TestRoutingCheckStillAppliesWhenOnlyTheSourceOwnedARoute(t *testing.T) {
	h := newHarness(t)
	h.cluster.SetIdentity("fake-second-context", "another-physical-cluster")
	draft := environment.WorkloadDraft{ApplicationKey: h.opts.ApplicationKey, EnvironmentKey: h.opts.EnvironmentKey, WorkloadID: "frontend", State: environment.DraftDelete}
	if err := h.app.Store.SaveWorkloadDraft(context.Background(), h.env().DraftVersion, draft); err != nil {
		t.Fatal(err)
	}
	// frontend (the only routed workload) is removed from the desired set and the
	// current set's route is its only one: routing still moves (source had routes).
	if _, err := h.service.Preview(context.Background(), h.request(environment.ModeDeployNew)); err == nil {
		t.Fatal("the source still owns a route, so the distinct cluster must be refused")
	}
}
