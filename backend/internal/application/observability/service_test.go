package observability

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/telemetry"
	obs "orchestrator/internal/ports/observability"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/persistence/persistencetest"
)

type receivedReader struct {
	calls  int
	fail   bool
	source obs.Source
}

func (r *receivedReader) Collect(_ context.Context, source obs.Source, state telemetry.State) (telemetry.Batch, error) {
	r.calls++
	r.source = source
	if r.fail {
		return telemetry.Batch{}, errors.New("private-provider-diagnostic")
	}
	now := time.Now().UTC()
	state.ObservedAt = &now
	state.LogsCode = telemetry.OK
	state.MetricsCode = telemetry.Unavailable
	return telemetry.Batch{State: state, Logs: []telemetry.Log{{ID: telemetry.Hash(source.Key, "received"), WorkloadID: source.WorkloadID, Timestamp: now, PodUID: "replacement-pod", PodName: "backend-abc", Container: "main", Level: "ERROR", Message: "ERROR received via collector"}}}, nil
}
func TestSweepUsesPinnedTargetsAndQueryOnlyReadsHistory(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	f := persistencetest.Telemetry(t, st)
	dep := deployment.Deployment{ID: "deployment-one", OrganizationKey: f.Scope.OrganizationKey, ApplicationKey: f.Scope.ApplicationKey, EnvironmentKey: "staging", WorkloadID: "backend", Status: deployment.StatusFailed, StartedAt: f.Now}
	if err := st.SaveDeployment(ctx, dep); err != nil {
		t.Fatal(err)
	}
	instance := deployment.WorkloadInstance{ID: "instance-one", EnvironmentKey: f.Scope.ApplicationKey + "/staging", WorkloadID: "backend", LastDeploymentID: dep.ID, Status: deployment.InstanceReady, TargetRef: map[string]any{"organization": f.Scope.OrganizationKey, "connection": "account-one", "namespace": "scoped-staging", "context": "selected-context"}, ObservedAt: f.Now}
	if err := st.UpsertWorkloadInstance(ctx, instance); err != nil {
		t.Fatal(err)
	}
	reader := &receivedReader{}
	service := New(st, reader)
	if err := service.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if reader.calls != 1 || reader.source.Target.Namespace != "scoped-staging" || reader.source.Target.Connection != "account-one" || reader.source.Target.Kubeconfig != "" {
		t.Fatalf("wrong target: %+v", reader.source)
	}
	view, err := service.Query(ctx, f.Scope, "overview", Params{})
	if err != nil || view.LogCount != 4 || view.State != "METRICS_UNAVAILABLE" {
		t.Fatalf("received view: %+v %v", view, err)
	}
	calls := reader.calls
	for _, kind := range []string{"overview", "logs", "metrics"} {
		if _, err := service.Query(ctx, f.Scope, kind, Params{}); err != nil {
			t.Fatal(err)
		}
	}
	if reader.calls != calls {
		t.Fatal("GET collected from provider")
	}
	raw, _ := json.Marshal(view)
	for _, private := range []string{"leaseUntil", "checkpoints", "targetHash", "selected-context", "account-one"} {
		if strings.Contains(string(raw), private) {
			t.Fatalf("private source state escaped: %s", private)
		}
	}
	reader.fail = true
	if err := service.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	view, err = service.Query(ctx, f.Scope, "logs", Params{Level: "ERROR", Search: "collector"})
	if err != nil || view.LogCount != 1 || view.State != "PARTIAL" {
		t.Fatalf("read failure lost history: %+v %v", view, err)
	}
	raw, _ = json.Marshal(view)
	if strings.Contains(string(raw), "private-provider") {
		t.Fatal("provider text escaped")
	}
	instance.TargetRef["organization"] = "foreign"
	if err := st.UpsertWorkloadInstance(ctx, instance); err != nil {
		t.Fatal(err)
	}
	before := reader.calls
	if err := service.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if reader.calls != before {
		t.Fatal("foreign target read")
	}
}
func TestQueryRejectsForeignScopeAndInvalidBounds(t *testing.T) {
	st := store.New()
	f := persistencetest.Telemetry(t, st)
	svc := New(st, nil)
	svc.SetClock(func() time.Time { return f.Now })
	foreign := f.Scope
	foreign.OrganizationKey = "foreign"
	if _, err := svc.Query(context.Background(), foreign, "logs", Params{From: "bad"}); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("auth before parse: %v", err)
	}
	for _, p := range []Params{{From: "bad"}, {Limit: 201}, {Search: strings.Repeat("x", 201)}, {Cursor: "bad"}, {Level: "stdout"}, {Workload: "../foreign"}, {From: f.Now.Add(-8 * 24 * time.Hour).Format(time.RFC3339Nano)}} {
		var field *FieldError
		if _, err := svc.Query(context.Background(), f.Scope, "logs", p); !errors.As(err, &field) {
			t.Fatalf("invalid query accepted: %+v %v", p, err)
		}
	}
	view, err := svc.Query(context.Background(), f.Scope, "metrics", Params{From: f.Now.Add(-15 * 24 * time.Hour).Format(time.RFC3339Nano)})
	if err == nil && (view.To.Sub(telemetry.BucketTime(view.From, time.Duration(view.StepSeconds)*time.Second))+time.Duration(view.StepSeconds)*time.Second-1)/(time.Duration(view.StepSeconds)*time.Second) > 240 {
		t.Fatal("chart exceeded 240 buckets")
	}
	if err != nil || view.State != "UNAVAILABLE" || !view.HasHistory {
		t.Fatalf("stored history without reader: %+v %v", view, err)
	}
}

type waitingReader struct{ entered chan struct{} }

func (r waitingReader) Collect(ctx context.Context, _ obs.Source, _ telemetry.State) (telemetry.Batch, error) {
	close(r.entered)
	<-ctx.Done()
	return telemetry.Batch{}, ctx.Err()
}
func TestCollectorStartsImmediatelyAndStopsBeforeStoreClose(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	f := persistencetest.Telemetry(t, st)
	dep := deployment.Deployment{ID: "auto-deployment", OrganizationKey: f.Scope.OrganizationKey, ApplicationKey: f.Scope.ApplicationKey, EnvironmentKey: "staging", WorkloadID: "backend", Status: deployment.StatusFailed}
	if err := st.SaveDeployment(ctx, dep); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertWorkloadInstance(ctx, deployment.WorkloadInstance{ID: "auto-workload", EnvironmentKey: f.Scope.ApplicationKey + "/staging", WorkloadID: "backend", LastDeploymentID: dep.ID, Status: deployment.InstanceReady, TargetRef: map[string]any{"organization": f.Scope.OrganizationKey, "connection": "scoped-account", "namespace": "app-staging"}}); err != nil {
		t.Fatal(err)
	}
	reader := waitingReader{entered: make(chan struct{})}
	service := New(st, reader)
	stop := service.Start(ctx)
	select {
	case <-reader.entered:
	case <-time.After(2 * time.Second):
		stop()
		t.Fatal("collector did not start immediately")
	}
	stopped := make(chan struct{})
	go func() { stop(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("collector did not stop with cancellation")
	}
	// Reacquiring immediately proves the cancelled read committed/released its
	// collection lease before the stop function returned.
	sources, err := service.discover(ctx)
	if err != nil || len(sources) != 1 {
		t.Fatal("source disappeared")
	}
	_, acquired, err := st.LeaseTelemetry(ctx, sources[0].Source, "next-process", time.Now().UTC())
	if err != nil || !acquired {
		t.Fatalf("stop left a collection lease: %v", err)
	}
}
