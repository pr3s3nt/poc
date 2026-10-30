package delivery

import (
	"context"
	domain "orchestrator/internal/domain/delivery"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/ports/persistence"
	"testing"
	"time"
)

func TestReadOnlyObservationUsesAdmittedReceiptAndStoredReadsDoNotPoll(t *testing.T) {
	c, p, ctx, req, _ := coordinatorFixture(t)
	if err := c.Deploy(ctx, req); err != nil {
		t.Fatal(err)
	}
	owner, _ := persistence.OwnerFrom(ctx)
	if err := c.Store.ReleaseOperation(ctx, owner, environment.OpSucceeded, ""); err != nil {
		t.Fatal(err)
	}
	before := len(p.order)
	repo := c.Store.(persistence.DeliveryObservationRepository)
	c.collectObservations(context.Background(), repo, "observer")
	rows, _, err := c.StoredObservations(context.Background(), req.OrganizationKey, req.ApplicationKey, req.Environment.Key, "", 50)
	if err != nil || len(rows) != 3 {
		t.Fatal("observations missing", len(rows), err)
	}
	for _, row := range rows {
		if row.Observation.Sync != domain.InSync || row.Sequence != 1 {
			t.Fatal("invalid admitted observation")
		}
		if row.Observation.Effective(time.Now().Add(4*ObservationInterval), ObservationInterval).Sync != domain.SyncUnknown {
			t.Fatal("stale observation stayed green")
		}
	}
	if len(p.order) != before {
		t.Fatal("observer submitted workload")
	}
	c.collectObservations(context.Background(), repo, "competing-observer")
	again, _, err := c.StoredObservations(context.Background(), req.OrganizationKey, req.ApplicationKey, req.Environment.Key, "", 50)
	if err != nil || len(again) != len(rows) || again[0].Sequence != 1 {
		t.Fatal("poll cadence was bypassed", err)
	}
	if _, _, err := c.StoredObservations(context.Background(), "foreign", req.ApplicationKey, req.Environment.Key, "", 50); err == nil {
		t.Fatal("foreign observations exposed")
	}
}
