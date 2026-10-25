package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"orchestrator/internal/domain/telemetry"
	"orchestrator/internal/ports/persistence/persistencetest"
)

func TestTelemetryHistorySurvivesSnapshotReopenAndExpires(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	st, err := NewWithSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	fixture := persistencetest.Telemetry(t, st)
	reopened, err := NewWithSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	persistencetest.AssertTelemetry(t, reopened, fixture)
	persistencetest.TelemetryQuota(t, reopened, fixture)
	if err := reopened.PruneTelemetry(context.Background(), fixture.Now.Add(16*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	data, err := reopened.ReadTelemetry(context.Background(), telemetry.Query{Scope: fixture.Scope, Kind: "overview", From: fixture.Now.Add(-time.Hour), To: fixture.Now.Add(time.Second), Limit: 100, Step: telemetry.Interval})
	if err != nil || data.LogCount != 0 || len(data.Points) != 0 || data.EventCount != 0 {
		t.Fatalf("expired history: %+v %v", data, err)
	}
}
