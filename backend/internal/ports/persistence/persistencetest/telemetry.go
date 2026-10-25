package persistencetest

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/telemetry"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
)

type TelemetryFixture struct {
	Scope telemetry.Scope
	Now   time.Time
}

// Telemetry checks the same received-history contract on memory and real SQL:
// fencing, duplicate timestamps, retry/reopen, scope, nulls, peaks and expiry.
func Telemetry(t *testing.T, st persistence.Store) TelemetryFixture {
	t.Helper()
	ctx := context.Background()
	repo := st.(persistence.TelemetryRepository)
	if err := st.SaveOrganization(ctx, application.Organization{ID: ids.New(), Key: "telemetry-org", Name: "Telemetry"}); err != nil {
		t.Fatal(err)
	}
	app := newOperationApp(t, st, "telemetry-org")
	scope := telemetry.Scope{OrganizationKey: "telemetry-org", ApplicationKey: app, EnvironmentKey: "staging"}
	now := time.Now().UTC().Truncate(time.Microsecond)
	source := telemetry.Source{Scope: scope, Key: telemetry.Hash(app, "source"), WorkloadID: "backend", TargetHash: telemetry.Hash("target")}
	lease, ok, err := repo.LeaseTelemetry(ctx, source, "worker-a", now)
	if err != nil || !ok {
		t.Fatalf("lease: %v %v", ok, err)
	}
	if _, ok, err := repo.LeaseTelemetry(ctx, source, "worker-b", now); err != nil || ok {
		t.Fatalf("competing lease: %v %v", ok, err)
	}
	if _, ok, err := repo.LeaseTelemetry(ctx, source, "worker-a", now); err != nil || ok {
		t.Fatalf("overlapping same-owner lease: %v %v", ok, err)
	}
	stamp := now.Add(-time.Minute)
	cpu, limit, low := 99.0, 100.0, 20.0
	memory, memoryLimit, lowMemory := 90.0*1048576, 100.0*1048576, 20.0*1048576
	logs := []telemetry.Log{}
	for i, message := range []string{"ERROR duplicate", "ERROR duplicate", "WARN reconnect"} {
		logs = append(logs, telemetry.Log{ID: telemetry.Hash(source.Key, "log", string(rune('a'+i))), WorkloadID: "backend", Timestamp: stamp, PodUID: "pod-one", PodName: "backend-abc", Container: "main", Level: telemetry.Level(message), Message: message})
	}
	lease.ObservedAt = &now
	lease.LastLogAt = &stamp
	lease.LastMetricAt = &now
	lease.LogsCode = telemetry.OK
	lease.MetricsCode = telemetry.OK
	lease.Checkpoints = map[string]telemetry.Cursor{"stream": {Timestamp: stamp}}
	b := telemetry.Batch{At: now, State: lease, Logs: logs, Samples: []telemetry.Sample{
		{ID: telemetry.Hash(source.Key, "peak"), WorkloadID: "backend", ObservedAt: now.Add(-2 * time.Minute), CPU: &cpu, Memory: &memory, CPULimit: &limit, MemoryLimit: &memoryLimit, TotalPods: 1, ReadyPods: 1},
		{ID: telemetry.Hash(source.Key, "gap"), WorkloadID: "backend", ObservedAt: now.Add(-time.Minute), TotalPods: 1},
		{ID: telemetry.Hash(source.Key, "current"), WorkloadID: "backend", ObservedAt: now, CPU: &low, Memory: &lowMemory, CPULimit: &limit, MemoryLimit: &memoryLimit, TotalPods: 1, ReadyPods: 1},
	}, Events: []telemetry.Event{{ID: telemetry.Hash(source.Key, "restart"), WorkloadID: "backend", Timestamp: stamp, Kind: "RESTART", Reason: "OOMKilled", Occurrences: 1}}}
	if err := repo.CommitTelemetry(ctx, b); err != nil {
		t.Fatal(err)
	}
	if err := repo.CommitTelemetry(ctx, b); !errors.Is(err, persistence.ErrOperationLost) {
		t.Fatalf("stale commit: %v", err)
	}
	lease2, ok, err := repo.LeaseTelemetry(ctx, source, "worker-b", now.Add(time.Second))
	if err != nil || !ok || lease2.Fence <= lease.Fence || lease2.Checkpoints["stream"].Timestamp.IsZero() {
		t.Fatalf("checkpoint/reacquire: %+v %v", lease2, err)
	}
	b.State = lease2
	b.State.LogsCode = telemetry.OK
	b.State.MetricsCode = telemetry.OK
	b.At = now.Add(time.Second)
	if err := repo.CommitTelemetry(ctx, b); err != nil {
		t.Fatal(err)
	}
	fixture := TelemetryFixture{Scope: scope, Now: now}
	AssertTelemetry(t, st, fixture)
	foreign := scope
	foreign.OrganizationKey = "foreign"
	if _, err := repo.ReadTelemetry(ctx, telemetry.Query{Scope: foreign, Kind: "overview", From: now.Add(-time.Hour), To: now.Add(time.Second), Limit: 100, Step: telemetry.Interval}); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("foreign scope: %v", err)
	}
	other := scope
	other.EnvironmentKey = "production"
	data, err := repo.ReadTelemetry(ctx, telemetry.Query{Scope: other, Kind: "overview", From: now.Add(-time.Hour), To: now.Add(time.Second), Limit: 100, Step: telemetry.Interval})
	if err != nil || data.LogCount != 0 || len(data.Sources) != 0 {
		t.Fatalf("environment isolation: %+v %v", data, err)
	}
	return fixture
}

func AssertTelemetry(t *testing.T, st persistence.Store, f TelemetryFixture) {
	t.Helper()
	ctx := context.Background()
	repo := st.(persistence.TelemetryRepository)
	q := telemetry.Query{Scope: f.Scope, Kind: "overview", From: f.Now.Add(-time.Hour), To: f.Now.Add(time.Second), Limit: 1, Step: telemetry.Interval}
	data, err := repo.ReadTelemetry(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	if data.LogCount != 3 || data.ErrorCount != 2 || len(data.Logs) != 1 || data.NextCursor == "" || data.EventCount != 1 || len(data.Latest) != 1 || data.Latest[0].CPU == nil || *data.Latest[0].CPU != 20 {
		t.Fatalf("history: %+v", data)
	}
	if len(data.Peaks) != 2 {
		t.Fatalf("peaks: %+v", data.Peaks)
	}
	for _, p := range data.Peaks {
		if p.Metric == "cpu" && p.Percent != 99 {
			t.Fatalf("peak confused with current: %+v", p)
		}
	}
	null := false
	for _, p := range data.Points {
		if p.CPU == nil {
			null = true
		}
	}
	if !null {
		t.Fatal("missing sample was replaced with zero")
	}
	seen := map[string]bool{data.Logs[0].ID: true}
	cursor := data.NextCursor
	for cursor != "" {
		q.Before, err = telemetry.DecodeCursor(cursor)
		if err != nil {
			t.Fatal(err)
		}
		next, err := repo.ReadTelemetry(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		for _, l := range next.Logs {
			if seen[l.ID] {
				t.Fatal("duplicate cursor row")
			}
			seen[l.ID] = true
		}
		cursor = next.NextCursor
	}
	q.Before = nil
	q.Step = 7 * telemetry.Interval
	aligned, err := repo.ReadTelemetry(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	for _, point := range aligned.Points {
		if point.ObservedAt.UnixNano()%int64(q.Step) != 0 {
			t.Fatal("chart bucket is not aligned to Unix UTC")
		}
	}
	if len(seen) != 3 {
		t.Fatalf("duplicate timestamps lost a log: %d", len(seen))
	}
}

// A refused log batch must preserve history and the committed cursor. The same
// bounded payload crosses the 64 MiB cap without requiring a huge stored DB.
func TelemetryQuota(t *testing.T, st persistence.Store, f TelemetryFixture) {
	t.Helper()
	ctx := context.Background()
	repo := st.(persistence.TelemetryRepository)
	q := telemetry.Query{Scope: f.Scope, Kind: "overview", From: f.Now.Add(-time.Hour), To: f.Now.Add(time.Minute), Limit: 100, Step: telemetry.Interval}
	before, err := repo.ReadTelemetry(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	source := before.Sources[0].Source
	at := f.Now.Add(2 * time.Second)
	lease, ok, err := repo.LeaseTelemetry(ctx, source, "quota-worker", at)
	if err != nil || !ok {
		t.Fatalf("quota lease: %v", err)
	}
	previous := lease.Checkpoints["stream"]
	lease.Checkpoints["stream"] = telemetry.Cursor{Timestamp: at}
	lease.LastLogAt = &at
	lease.LogsCode = telemetry.OK
	logs := make([]telemetry.Log, 4096)
	message := strings.Repeat("x", telemetry.MaxMessage)
	for i := range logs {
		logs[i] = telemetry.Log{ID: telemetry.Hash(source.Key, "quota", strconv.Itoa(i)), WorkloadID: source.WorkloadID, Timestamp: at, PodUID: "quota-pod", PodName: "backend-quota", Container: "main", Level: "UNKNOWN", Message: message}
	}
	if err := repo.CommitTelemetry(ctx, telemetry.Batch{At: at, State: lease, Logs: logs}); err != nil {
		t.Fatal(err)
	}
	after, err := repo.ReadTelemetry(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	state := after.Sources[0]
	if after.LogCount != before.LogCount || !state.Limited || state.LogsCode != telemetry.StorageLimit || state.Checkpoints["stream"] != previous || state.LastLogAt == nil || !state.LastLogAt.Equal(*before.Sources[0].LastLogAt) {
		t.Fatal("quota discarded history or advanced rejected log checkpoint")
	}
	old, ok, err := repo.LeaseTelemetry(ctx, source, "expired-worker", at.Add(time.Second))
	if err != nil || !ok {
		t.Fatal("expired lease setup")
	}
	_, ok, err = repo.LeaseTelemetry(ctx, source, "replacement-worker", at.Add(2*time.Minute))
	if err != nil || !ok {
		t.Fatal("expired lease not reclaimed")
	}
	if err := repo.CommitTelemetry(ctx, telemetry.Batch{At: at.Add(2 * time.Minute), State: old}); !errors.Is(err, persistence.ErrOperationLost) {
		t.Fatalf("expired owner committed: %v", err)
	}
}
