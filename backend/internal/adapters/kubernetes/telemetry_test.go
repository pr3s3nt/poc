package kubernetes

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"orchestrator/internal/domain/telemetry"
	"orchestrator/internal/ports/execution"
	obs "orchestrator/internal/ports/observability"
)

type telemetryCredentials struct{}

func (telemetryCredentials) ResolveKubeconfig(context.Context, execution.Target) ([]byte, error) {
	return []byte("apiVersion: v1\nkind: Config\n# synthetic-private-credential-sentinel\n"), nil
}

func telemetryReaderFixture(t *testing.T, missingMetrics bool) (*TelemetryReader, obs.Source, telemetry.State) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Microsecond)
	pod := map[string]any{"metadata": map[string]any{"name": "backend-6db7f8c95d-m8r2k", "namespace": "app-staging", "uid": "pod-one", "creationTimestamp": now.Add(-time.Hour), "labels": map[string]string{"app.kubernetes.io/name": "backend", "app.kubernetes.io/managed-by": "orchestrator", "orchestrator.io/application": "app", "orchestrator.io/environment": "staging"}}, "spec": map[string]any{"containers": []any{map[string]any{"name": "main", "resources": map[string]any{"limits": map[string]string{"cpu": "500m", "memory": "128Mi"}}}}}, "status": map[string]any{"phase": "Running", "containerStatuses": []any{map[string]any{"name": "main", "ready": true, "restartCount": 0}}}}
	pods, _ := json.Marshal(map[string]any{"items": []any{pod}})
	metrics, _ := json.Marshal(map[string]any{"items": []any{map[string]any{"metadata": map[string]string{"name": "backend-6db7f8c95d-m8r2k", "namespace": "app-staging"}, "timestamp": now.Add(-15 * time.Second), "window": "30s", "containers": []any{map[string]any{"name": "main", "usage": map[string]string{"cpu": "125000000n", "memory": "102400Ki"}}}}}})
	logs := now.Add(-time.Second).Format(time.RFC3339Nano) + " ERROR repeated\n" + now.Add(-time.Second).Format(time.RFC3339Nano) + " ERROR repeated\n"
	metricCommand := "cat <<'METRICS'\n" + string(metrics) + "\nMETRICS\n"
	if missingMetrics {
		metricCommand = "echo 'Error from server (NotFound): synthetic-private-credential-sentinel' >&2\nexit 1\n"
	}
	script := "#!/bin/sh\ncase \"$*\" in\n *metrics.k8s.io*)\n" + metricCommand + ";;\n *'/api/v1/namespaces/app-staging/pods?'*)\ncat <<'PODS'\n" + string(pods) + "\nPODS\n;;\n *'logs backend-'*)\ncat <<'LOGS'\n" + logs + "LOGS\n;;\n *) echo 'synthetic-private-credential-sentinel' >&2; exit 1;;\nesac\n"
	path := filepath.Join(t.TempDir(), "kubectl")
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	source := obs.Source{Source: telemetry.Source{Scope: telemetry.Scope{OrganizationKey: "org", ApplicationKey: "app", EnvironmentKey: "staging"}, Key: telemetry.Hash("source"), WorkloadID: "backend", TargetHash: telemetry.Hash("target")}, Target: execution.Target{Organization: "org", Connection: "account", Namespace: "app-staging", Context: "context"}}
	state := telemetry.State{Source: source.Source, Owner: "worker", Fence: 1, LeaseUntil: now.Add(time.Minute), Checkpoints: map[string]telemetry.Cursor{}}
	return &TelemetryReader{KubectlPath: path, Credentials: telemetryCredentials{}, Now: func() time.Time { return now }}, source, state
}

func TestTelemetryReaderReceivesHistoryAndKeepsDuplicateTimestampLines(t *testing.T) {
	r, source, state := telemetryReaderFixture(t, false)
	batch, err := r.Collect(context.Background(), source, state)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Logs) != 2 || batch.Logs[0].ID == batch.Logs[1].ID || len(batch.Samples) != 1 {
		t.Fatalf("received facts: %+v", batch)
	}
	sample := batch.Samples[0]
	if sample.CPU == nil || *sample.CPU != 125 || sample.Memory == nil || *sample.Memory != 100*1024*1024 || sample.CPULimit == nil || *sample.CPULimit != 500 || sample.MetricsAt == nil || !sample.ObservedAt.Equal(*sample.MetricsAt) {
		t.Fatalf("metric facts: %+v", sample)
	}
	if sample.ReadyPods != 1 || sample.TotalPods != 1 || batch.State.LogsCode != telemetry.OK || batch.State.MetricsCode != telemetry.OK {
		t.Fatalf("coverage: %+v", batch.State)
	}
	if err := batch.Validate(); err != nil {
		t.Fatal(err)
	}
	second, err := r.Collect(context.Background(), source, batch.State)
	if err != nil {
		t.Fatal(err)
	}
	if second.Logs[0].ID != batch.Logs[0].ID || second.Logs[1].ID != batch.Logs[1].ID {
		t.Fatal("overlap changed duplicate identity")
	}
	if len(second.State.Checkpoints) != 1 {
		t.Fatalf("checkpoint: %+v", second.State.Checkpoints)
	}
}

func TestTelemetryReaderMissingMetricsPreservesLogsAndPodStatus(t *testing.T) {
	r, source, state := telemetryReaderFixture(t, true)
	batch, err := r.Collect(context.Background(), source, state)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Logs) != 2 || batch.State.LogsCode != telemetry.OK || batch.State.MetricsCode != telemetry.Unavailable || batch.Samples[0].CPU != nil || batch.Samples[0].Memory != nil || batch.Samples[0].ReadyPods != 1 {
		t.Fatalf("partial collection: %+v", batch)
	}
	jsonData, _ := json.Marshal(batch)
	if strings.Contains(string(jsonData), "synthetic-private-credential-sentinel") {
		t.Fatal("credential/provider diagnostic escaped into facts")
	}
}

func TestTelemetryReaderRefusesHostOrForeignTarget(t *testing.T) {
	r, source, state := telemetryReaderFixture(t, false)
	for _, target := range []execution.Target{{Kubeconfig: "/unread-host-config", Namespace: "app-staging"}, {Connection: "account", Organization: "foreign", Namespace: "app-staging"}} {
		source.Target = target
		if _, err := r.Collect(context.Background(), source, state); !errors.Is(err, ErrCredentialTarget) {
			t.Fatalf("target boundary: %v", err)
		}
	}
}

func TestTelemetryQuantities(t *testing.T) {
	for _, c := range []struct {
		raw  string
		cpu  bool
		want float64
	}{{"12000000n", true, 12}, {"1200u", true, 1.2}, {".5", true, 500}, {"20m", true, 20}, {"128Mi", false, 134217728}, {"1Gi", false, 1073741824}, {"2M", false, 2000000}} {
		v, ok := quantity(c.raw, c.cpu)
		if !ok || v != c.want {
			t.Fatalf("quantity %q: %v %v", c.raw, v, ok)
		}
	}
	for _, raw := range []string{"-1", "NaN", "Infinity", "1evil", "999999999999999999999999999999999999"} {
		if _, ok := quantity(raw, false); ok {
			t.Fatalf("accepted %q", raw)
		}
	}
}

func TestTelemetryRestartRetainsBothStreamsAndReadsPreviousOnce(t *testing.T) {
	r, source, state := telemetryReaderFixture(t, false)
	script, err := os.ReadFile(r.KubectlPath)
	if err != nil {
		t.Fatal(err)
	}
	finished := r.Now().Add(-30 * time.Second).Format(time.RFC3339Nano)
	replacement := `"restartCount":1,"lastState":{"terminated":{"reason":"OOMKilled","finishedAt":"` + finished + `"}}`
	script = []byte(strings.ReplaceAll(string(script), `"restartCount":0`, replacement))
	if err := os.WriteFile(r.KubectlPath, script, 0o700); err != nil {
		t.Fatal(err)
	}
	b, err := r.Collect(context.Background(), source, state)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Logs) != 4 || len(b.Events) != 1 || b.Events[0].Reason != "OOMKilled" || len(b.State.Checkpoints) != 2 {
		t.Fatalf("restart facts: %+v", b)
	}
	if b.Logs[0].ID == b.Logs[2].ID {
		t.Fatal("restart stream identity collision")
	}
	next, err := r.Collect(context.Background(), source, b.State)
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Logs) != 2 || next.Events[0].ID != b.Events[0].ID {
		t.Fatal("previous stream reread or event duplicated")
	}
}
func TestTelemetryDropsByteLimitedPartialRecordAndCapsTransport(t *testing.T) {
	r, source, _ := telemetryReaderFixture(t, false)
	stamp := r.Now().Add(-time.Second).Format(time.RFC3339Nano)
	first := stamp + " INFO complete\n"
	raw := []byte(first + stamp + " ERROR " + strings.Repeat("x", 256<<10))
	rows, last, truncated := parseTelemetryLogs(source, telemetryPod{}, "main", 0, "stream", raw, r.Now())
	if !truncated || len(rows) != 1 || rows[0].Message != "INFO complete" || last.IsZero() {
		t.Fatal("partial log persisted")
	}
	file := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(file, []byte("synthetic"), 0o600); err != nil {
		t.Fatal(err)
	}
	cli := CLI{Path: r.KubectlPath, Kubeconfig: file}
	if _, err := cli.RunBounded(context.Background(), 16, "get", "--raw", "/apis/metrics.k8s.io/v1beta1/namespaces/app-staging/pods"); !errors.Is(err, ErrTelemetryOutputLimit) {
		t.Fatalf("uncapped transport: %v", err)
	}
}
