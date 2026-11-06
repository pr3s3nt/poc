package pending_test

import (
	"context"
	"errors"
	"orchestrator/internal/application/pending"
	"orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/monitoring"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
	"testing"
)

func TestMonitoringProfileOnlyUpdateInvalidatesPreviewAndRecordsAppliedSignature(t *testing.T) {
	ctx := context.Background()
	app, _, svc, opts := fixture(t)
	deploy(t, app, opts, "backend", scoreFor("backend", "example.invalid/backend:v1", false))
	before, e := svc.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if e != nil || len(before.Changes) != 0 {
		t.Fatal("initial noop", e)
	}
	repo := app.Store.(persistence.MonitoringRepository)
	c := monitoring.Connection{ID: ids.New(), Organization: opts.OrganizationKey, Key: "monitoring-unit", Name: "Unit source", Revision: 1, PrometheusURL: "https://prometheus.invalid", ClusterUID: "cluster", Rules: monitoring.DevRules()}
	if e = repo.PutMonitoringConnection(ctx, c, 0); e != nil {
		t.Fatal(e)
	}
	env, e := app.Store.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if e != nil {
		t.Fatal(e)
	}
	b := monitoring.Binding{Organization: opts.OrganizationKey, Application: opts.ApplicationKey, Environment: opts.EnvironmentKey, ConnectionID: c.ID, ConnectionRevision: c.Revision, ClusterUID: c.ClusterUID, Generation: env.TargetGeneration, Revision: 1, MetricsSource: "prometheus", LogsSource: "native"}
	if e = repo.PutMonitoringBinding(ctx, b, 0); e != nil {
		t.Fatal(e)
	}
	first, e := svc.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if e != nil || len(first.Changes) != 1 || first.Changes[0].Action != deployment.ActionUpdate {
		t.Fatalf("binding-only update: %+v %v", first.Changes, e)
	}
	c.Revision = 2
	c.Rules.RestartThreshold = 5
	if e = repo.PutMonitoringConnection(ctx, c, 1); e != nil {
		t.Fatal(e)
	}
	if _, e = svc.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey); !errors.Is(e, monitoring.ErrStaleBinding) {
		t.Fatal("stale source pin accepted", e)
	}
	b.ConnectionRevision = 2
	b.Revision = 2
	if e = repo.PutMonitoringBinding(ctx, b, 1); e != nil {
		t.Fatal(e)
	}
	second, e := svc.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if e != nil || len(second.Changes) != 1 || second.Token == first.Token || second.Changes[0].PlanHash == first.Changes[0].PlanHash {
		t.Fatal("profile pin did not change Preview/plan", e)
	}
	applied := len(app.FakeDeploy.Applied)
	if _, e = svc.Deploy(ctx, opts.ApplicationKey, opts.EnvironmentKey, "test", first.Token); !errors.Is(e, pending.ErrStalePreview) {
		t.Fatal("old Preview admitted", e)
	}
	if len(app.FakeDeploy.Applied) != applied {
		t.Fatal("stale Preview mutated runtime")
	}
	result, e := svc.Deploy(ctx, opts.ApplicationKey, opts.EnvironmentKey, "test", second.Token)
	if e != nil || result.Status != "SUCCEEDED" {
		t.Fatalf("profile-only deploy: %+v %v", result, e)
	}
	after, e := svc.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if e != nil || len(after.Changes) != 0 {
		t.Fatalf("applied monitoring revision repeated: %+v %v", after.Changes, e)
	}
}
