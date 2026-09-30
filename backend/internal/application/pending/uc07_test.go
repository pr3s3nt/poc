package pending_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	tf "orchestrator/internal/adapters/terraform"
	appsvc "orchestrator/internal/application/deployment"
	"orchestrator/internal/application/pending"
	"orchestrator/internal/application/workloadconfig"
	"orchestrator/internal/bootstrap"
	"orchestrator/internal/planning"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/seed"
)

// consumerOf returns a Score whose variable references provider's http Service.
func consumerOf(name, provider string) map[string]any {
	score := scoreFor(name, "example.invalid/"+name+":v1", false)
	score["containers"].(map[string]any)["main"].(map[string]any)["variables"] = map[string]any{"API_URL": "${resources.api.url}"}
	score["resources"] = map[string]any{"api": map[string]any{"type": "service", "params": map[string]any{"workload": provider, "port": "http"}}}
	return score
}

// TestRemovingReferencedServiceIsBlockedUnlessConsumerGoesToo covers UC-07
// BR-08: the final Environment may not keep a reference to a removed Service,
// while one batch may remove consumer and provider together.
func TestRemovingReferencedServiceIsBlockedUnlessConsumerGoesToo(t *testing.T) {
	ctx := context.Background()
	app, workloads, svc, opts := fixture(t)
	deploy(t, app, opts, "backend", scoreFor("backend", "example.invalid/backend:v1", false))
	if _, err := workloads.Save(ctx, opts.ApplicationKey, opts.EnvironmentKey, "frontend", consumerOf("frontend", "backend"), 0); err != nil {
		t.Fatal(err)
	}
	first, err := svc.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}
	if report, err := svc.Deploy(ctx, opts.ApplicationKey, opts.EnvironmentKey, "test", first.Token); err != nil || report.Status != "SUCCEEDED" {
		t.Fatalf("deploy consumer: %+v %v", report, err)
	}

	view, _ := workloads.List(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if _, err := workloads.Delete(ctx, opts.ApplicationKey, opts.EnvironmentKey, "backend", view.DraftVersion); err != nil {
		t.Fatal(err)
	}
	applied := len(app.FakeDeploy.Applied)
	if _, err := svc.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey); !errors.Is(err, pending.ErrInvalid) || !strings.Contains(err.Error(), "references a Service") {
		t.Fatalf("dangling Service reference accepted: %v", err)
	}

	view, _ = workloads.List(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if _, err := workloads.Delete(ctx, opts.ApplicationKey, opts.EnvironmentKey, "frontend", view.DraftVersion); err != nil {
		t.Fatal(err)
	}
	both, err := svc.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil || len(both.Changes) != 2 {
		t.Fatalf("removing consumer and provider together: %+v %v", both.Changes, err)
	}
	report, err := svc.Deploy(ctx, opts.ApplicationKey, opts.EnvironmentKey, "test", both.Token)
	if err != nil || report.Status != "SUCCEEDED" || len(app.FakeDeploy.Applied) != applied {
		t.Fatalf("batch removal: %+v %v", report, err)
	}
}

func TestDirectRemovalOfReferencedServiceFailsBeforeSideEffects(t *testing.T) {
	ctx := context.Background()
	app, workloads, svc, opts := fixture(t)
	backend := scoreFor("backend", "example.invalid/backend:v1", false)
	deploy(t, app, opts, "backend", backend)
	if _, err := workloads.Save(ctx, opts.ApplicationKey, opts.EnvironmentKey, "frontend", consumerOf("frontend", "backend"), 0); err != nil {
		t.Fatal(err)
	}
	p, _ := svc.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if _, err := svc.Deploy(ctx, opts.ApplicationKey, opts.EnvironmentKey, "test", p.Token); err != nil {
		t.Fatal(err)
	}
	env, _ := app.Store.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	removed := len(app.FakeDeploy.Removed)
	_, err := app.Deployments.DeployWorkload(ctx, deployCommand(opts, "backend", backend, nil))
	var stage *planning.StageError
	if !errors.As(err, &stage) || stage.Stage != planning.StageServiceRefs {
		t.Fatalf("direct removal of a referenced Service: %v", err)
	}
	after, _ := app.Store.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if after.Version != env.Version || len(app.FakeDeploy.Removed) != removed {
		t.Fatal("rejected removal changed runtime or current set")
	}
}

func toJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func deployCommand(opts seed.Options, id string, before, after map[string]any) appsvc.DeployCommand {
	return appsvc.DeployCommand{OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey, WorkloadID: id, ScoreBefore: before, ScoreAfter: after, Actor: "test"}
}

type sentinelDeployer struct{}

const runtimeSentinel = "sentinel-kubectl-token-abc123"

func (sentinelDeployer) Apply(context.Context, execution.Target, []execution.Manifest) error {
	return errors.New("kubectl apply failed: bearer " + runtimeSentinel)
}
func (sentinelDeployer) WaitReady(context.Context, execution.Target, []execution.WorkloadRef) error {
	return nil
}
func (sentinelDeployer) Remove(context.Context, execution.Target, string) error { return nil }

func TestDeployReportDoesNotEchoRuntimeErrors(t *testing.T) {
	ctx := context.Background()
	opts := seed.Defaults()
	app, err := bootstrap.Build(ctx, bootstrap.Options{Seed: opts, Adapters: bootstrap.AdapterFake, DeployerOverride: sentinelDeployer{}})
	if err != nil {
		t.Fatal(err)
	}
	workloads := workloadconfig.NewService(app.Store)
	svc := pending.NewService(app.Store, planning.NewService(), workloads, tf.NewInspector())
	svc.SetDeployer(app.Deployments)
	if _, err := workloads.Save(ctx, opts.ApplicationKey, opts.EnvironmentKey, "backend", scoreFor("backend", "example.invalid/backend:v1", false), 0); err != nil {
		t.Fatal(err)
	}
	p, err := svc.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}
	report, err := svc.Deploy(ctx, opts.ApplicationKey, opts.EnvironmentKey, "test", p.Token)
	if err != nil || report.Status != "FAILED" || len(report.Results) != 1 {
		t.Fatalf("report: %+v %v", report, err)
	}
	if msg := report.Results[0].Error; msg != appsvc.FailureRuntime {
		t.Fatalf("runtime error echoed or not actionable: %q", msg)
	}
	// UC-09 scoped history and detail show the same safe summary.
	list, err := app.Queries.ListDeployments(ctx, appsvc.ListDeploymentsQuery{OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey})
	if err != nil || len(list) == 0 {
		t.Fatalf("history: %v", err)
	}
	detail, err := app.Queries.GetDeployment(ctx, appsvc.GetDeploymentQuery{OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey, DeploymentID: list[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{toJSON(t, list), toJSON(t, detail)} {
		if strings.Contains(text, runtimeSentinel) || !strings.Contains(text, appsvc.FailureRuntime) {
			t.Fatalf("history exposes the runtime cause or lacks the summary: %s", text)
		}
	}
}
