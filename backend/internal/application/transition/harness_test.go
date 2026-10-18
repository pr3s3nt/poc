package transition_test

import (
	"context"
	"orchestrator/internal/seed/seedtest"
	"testing"

	"orchestrator/internal/adapters/fake"
	appsvc "orchestrator/internal/application/deployment"
	"orchestrator/internal/application/transition"
	"orchestrator/internal/bootstrap"
	"orchestrator/internal/bootstrap/bootstraptest"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/seed"
)

type harness struct {
	t       *testing.T
	app     *bootstrap.App
	opts    seed.Options
	cluster *fake.Cluster
	scores  map[string]map[string]any
	service *transition.Service
}

const destKey = "lab2"

// newHarness deploys the acceptance workloads on the seeded Kubernetes target
// and registers a second logical Kubernetes Connection with its own matching
// cluster Definition. Both Connections resolve to the same fake physical
// cluster (the same limitation as the live kind run), so only generations
// separate their namespaces.
func newHarness(t *testing.T, mutate ...func(*bootstrap.Options)) *harness {
	t.Helper()
	opts := seedtest.Defaults()
	opts.RunID, opts.Region, opts.AccountID = "transition-test", "us-east-1", "000000000000"
	options := bootstrap.Options{Seed: opts, Adapters: bootstrap.AdapterFake, ConnectionCredentialStore: "memory"}
	for _, m := range mutate {
		m(&options)
	}
	app, err := bootstraptest.Build(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	app.Transitions.SetAsync(false)
	h := &harness{t: t, app: app, opts: opts, cluster: app.FakeCluster, service: app.Transitions, scores: seed.AcceptanceScores(opts)}
	ctx := context.Background()
	// The acceptance frontend gets a public route so cutover has an Ingress to move.
	service := h.scores["frontend"]["service"].(map[string]any)
	service["publicRoutes"] = []any{map[string]any{"path": "/", "port": "http"}}
	for _, id := range seed.AcceptanceOrder() {
		if _, err := app.Deployments.DeployWorkload(ctx, appsvc.DeployCommand{
			OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey,
			WorkloadID: id, ScoreAfter: h.scores[id], Actor: "test", RunID: "run-source",
		}); err != nil {
			t.Fatalf("deploy %s: %v", id, err)
		}
	}
	return h
}

func (h *harness) env() environment.Environment {
	h.t.Helper()
	env, err := h.app.Store.GetEnvironment(context.Background(), h.opts.ApplicationKey, h.opts.EnvironmentKey)
	if err != nil {
		h.t.Fatal(err)
	}
	return env
}

func (h *harness) request(mode environment.TransitionMode, mappings ...environment.ResourceMapping) transition.Request {
	return transition.Request{OrganizationKey: h.opts.OrganizationKey, ApplicationKey: h.opts.ApplicationKey, EnvironmentKey: h.opts.EnvironmentKey, DestinationKey: destKey, Mode: mode, Mappings: mappings}
}
