package delivery

import (
	"context"
	domain "orchestrator/internal/domain/delivery"
	"orchestrator/internal/domain/progress"
	"testing"
	"time"
)

func TestPublishedGitDoesNotClaimFleetApplyOrReadinessCompleted(t *testing.T) {
	var steps []progress.Step
	ctx := progress.WithReporter(context.Background(), func(s progress.Step, _ bool) { steps = append(steps, s) })
	observation := domain.Observation{IntendedRevision: "revision", Sync: domain.Pending, ReasonCode: "FLEET_FORBIDDEN"}
	reportReadiness(ctx, "intent", "backend", "RUNNING", "FLEET_FORBIDDEN", time.Now().Add(time.Minute), observation, "commit")
	if len(steps) != 1 || steps[0].Kind != "APPLY" || steps[0].Status != "RUNNING" || steps[0].Reason != "FLEET_FORBIDDEN" {
		t.Fatalf("unapplied revision misrepresented: %+v", steps)
	}
	steps = nil
	observation.Complete = true
	observation.ObservedRevision = "revision"
	observation.Sync = domain.InSync
	reportReadiness(ctx, "intent", "backend", "RUNNING", "WAITING_READINESS", time.Now().Add(time.Minute), observation, "commit")
	if len(steps) != 2 || steps[0].Kind != "APPLY" || steps[0].Status != "SUCCEEDED" || steps[1].Kind != "READINESS" || steps[1].Status != "RUNNING" {
		t.Fatalf("apply and readiness collapsed: %+v", steps)
	}
}
