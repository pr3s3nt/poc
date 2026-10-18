package transition_test

import (
	"context"
	"testing"

	"orchestrator/internal/domain/environment"
)

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
