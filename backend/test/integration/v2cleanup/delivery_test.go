package main

import (
	"context"
	"encoding/json"
	"testing"

	appsvc "orchestrator/internal/application/deployment"
	"orchestrator/internal/bootstrap"
	"orchestrator/internal/bootstrap/bootstraptest"
	domain "orchestrator/internal/domain/deployment"
	"orchestrator/internal/seed"
	"orchestrator/internal/seed/seedtest"
	"orchestrator/test/integration/v2run"
)

// Fake adapter regression for CLI admission, not real-cluster acceptance.
func TestDeliveryCleansAppliedTargetsWithoutLegacyEnvironmentBinding(t *testing.T) {
	ctx := context.Background()
	opts := seedtest.Defaults()
	app, err := bootstraptest.Build(ctx, bootstrap.Options{Seed: opts, Adapters: bootstrap.AdapterFake})
	if err != nil {
		t.Fatal(err)
	}
	scores := seed.AcceptanceScores(opts)
	for _, id := range []string{"worker", "frontend"} {
		if _, err := app.Deployments.DeployWorkload(ctx, appsvc.DeployCommand{OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey, WorkloadID: id, ScoreAfter: scores[id], Actor: "test"}); err != nil {
			t.Fatal(err)
		}
	}
	env, err := app.Store.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}
	wire, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	var metadata map[string]any
	if err := json.Unmarshal(wire, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata["connectionKey"] != nil && metadata["connectionKey"] != "" {
		t.Fatal("fixture unexpectedly has legacy binding")
	}
	h := v2run.CleanupHandles{OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey}
	if code := delivery(ctx, app, h, v2run.Manifest{Generation: env.TargetGeneration}); code != exitDone {
		t.Fatalf("delivery exit=%d", code)
	}
	instances, err := app.Store.ListWorkloadInstances(ctx, opts.ApplicationKey+"/"+opts.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(instances) != 2 {
		t.Fatalf("instances=%d", len(instances))
	}
	for _, instance := range instances {
		if instance.Status != domain.InstanceRemoved {
			t.Fatalf("workload %s retained status %s", instance.WorkloadID, instance.Status)
		}
	}
}
