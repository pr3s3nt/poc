package postgres

import (
	"context"
	"testing"
	"time"

	"orchestrator/internal/domain/telemetry"
	"orchestrator/internal/ports/persistence/persistencetest"
)

func TestPostgresTelemetryHistoryLeaseAndReopen(t *testing.T) {
	db := freshDatabase(t)
	st, err := Open(context.Background(), db.Console)
	if err != nil {
		t.Fatal(err)
	}
	fixture := persistencetest.Telemetry(t, st)
	st.Close()
	reopened, err := Open(context.Background(), db.Console)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	persistencetest.AssertTelemetry(t, reopened, fixture)
	persistencetest.TelemetryQuota(t, reopened, fixture)
	state := connectAs(t, db.State)
	denied(t, state, "State cannot read application logs", `SELECT message FROM telemetry_logs`)
	denied(t, state, "State cannot collect telemetry", `SELECT checkpoints FROM telemetry_sources`)
	if err := reopened.PruneTelemetry(context.Background(), fixture.Now.Add(16*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	data, err := reopened.ReadTelemetry(context.Background(), telemetry.Query{Scope: fixture.Scope, Kind: "overview", From: fixture.Now.Add(-time.Hour), To: fixture.Now.Add(time.Second), Limit: 100, Step: telemetry.Interval})
	if err != nil || data.LogCount != 0 || len(data.Points) != 0 || data.EventCount != 0 {
		t.Fatalf("expired telemetry: %+v %v", data, err)
	}
}
