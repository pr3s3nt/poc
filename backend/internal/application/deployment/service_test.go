package deployment_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	appsvc "orchestrator/internal/application/deployment"
	"orchestrator/internal/bootstrap"
	domain "orchestrator/internal/domain/deployment"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/seed"
)

func newApp(t *testing.T, opts ...func(*bootstrap.Options)) (*bootstrap.App, seed.Options) {
	t.Helper()
	seedOptions := seed.Defaults()
	buildOptions := bootstrap.Options{Seed: seedOptions, Adapters: bootstrap.AdapterFake}
	for _, apply := range opts {
		apply(&buildOptions)
	}
	app, err := bootstrap.Build(context.Background(), buildOptions)
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	return app, buildOptions.Seed
}

func deployAll(t *testing.T, app *bootstrap.App, seedOptions seed.Options) []*appsvc.DeployResult {
	t.Helper()
	scores := seed.AcceptanceScores(seedOptions)
	var results []*appsvc.DeployResult
	for _, workload := range seed.AcceptanceOrder() {
		result, err := app.Deployments.DeployWorkload(context.Background(), appsvc.DeployCommand{
			OrganizationKey: seedOptions.OrganizationKey,
			ApplicationKey:  seedOptions.ApplicationKey,
			EnvironmentKey:  seedOptions.EnvironmentKey,
			WorkloadID:      workload,
			ScoreAfter:      scores[workload],
			Actor:           "test",
		})
		if err != nil {
			t.Fatalf("deploy %s: %v", workload, err)
		}
		results = append(results, result)
	}
	return results
}

func TestDeployAcceptance_FrontendBackendWorkerSharedDatabase(t *testing.T) {
	ctx := context.Background()
	app, seedOptions := newApp(t)
	results := deployAll(t, app, seedOptions)

	for _, r := range results {
		if r.Status != domain.StatusSucceeded {
			t.Fatalf("workload %s ended in %s", r.WorkloadID, r.Status)
		}
	}

	env, err := app.Store.GetEnvironment(ctx, seedOptions.ApplicationKey, seedOptions.EnvironmentKey)
	if err != nil {
		t.Fatalf("get environment: %v", err)
	}
	set, err := app.Store.GetDeploymentSet(ctx, env.CurrentDeploymentSetID)
	if err != nil {
		t.Fatalf("get current set: %v", err)
	}
	if got := set.Document.ModuleIDs(); len(got) != 3 {
		t.Fatalf("expected three modules in the current set, got %v", got)
	}
	entry, ok := set.Document.Shared[seedOptions.SharedDatabaseID]
	if !ok {
		t.Fatalf("shared database entry is missing: %v", set.Document.Shared)
	}
	if entry.Type != "postgres" || entry.Params["database"] != "acceptance" {
		t.Fatalf("unexpected shared entry: %#v", entry)
	}

	instances, err := app.Store.ListWorkloadInstances(ctx, seedOptions.ApplicationKey+"/"+seedOptions.EnvironmentKey)
	if err != nil {
		t.Fatalf("list instances: %v", err)
	}
	if len(instances) != 3 {
		t.Fatalf("expected three workload instances, got %d", len(instances))
	}
	for _, instance := range instances {
		if instance.Status != domain.InstanceReady {
			t.Fatalf("workload %s is %s", instance.WorkloadID, instance.Status)
		}
	}

	active, err := app.Store.ListActiveResources(ctx, seedOptions.OrganizationKey)
	if err != nil {
		t.Fatalf("list active resources: %v", err)
	}
	postgres := 0
	for _, a := range active {
		if a.Descriptor.Type == "postgres" {
			postgres++
		}
	}
	if postgres != 1 {
		t.Fatalf("expected exactly one shared postgres Active Resource, got %d", postgres)
	}
}

func TestDeployWorkload_ResolvesResourceOutputsBeforeRender(t *testing.T) {
	app, seedOptions := newApp(t)
	deployAll(t, app, seedOptions)

	var host, secretValue string
	for _, applied := range app.FakeDeploy.Applied {
		for _, m := range applied.Manifests {
			if m.Kind == "Deployment" && m.Name == "backend" {
				containers := m.Object["spec"].(map[string]any)["template"].(map[string]any)["spec"].(map[string]any)["containers"].([]any)
				for _, item := range containers[0].(map[string]any)["env"].([]any) {
					entry := item.(map[string]any)
					if entry["name"] == "PGHOST" {
						host, _ = entry["value"].(string)
					}
					if entry["name"] == "PGPASSWORD" && entry["value"] != nil {
						t.Fatal("the database password must not be inlined in the Deployment")
					}
				}
			}
			if m.Kind == "Secret" && m.Name == "backend-env" {
				data := m.Object["stringData"].(map[string]any)
				secretValue, _ = data["main_PGPASSWORD"].(string)
			}
		}
	}
	if host == "" || strings.Contains(host, "${") {
		t.Fatalf("PGHOST was not resolved from the provider outputs: %q", host)
	}
	if secretValue == "" {
		t.Fatal("the database password was not delivered through a Secret")
	}
}

func TestProvision_ProviderOutputsFeedConsumerInputs(t *testing.T) {
	app, seedOptions := newApp(t)
	deployAll(t, app, seedOptions)

	var namespaceIndex, postgresIndex int = -1, -1
	for i, call := range app.FakeExec.Calls {
		switch call.ResourceType {
		case "k8s-namespace":
			if namespaceIndex < 0 {
				namespaceIndex = i
			}
		case "postgres":
			if postgresIndex < 0 {
				postgresIndex = i
			}
			if call.Target.Namespace != seedOptions.NamespaceIdentity {
				t.Fatalf("postgres target namespace is %q, want %q", call.Target.Namespace, seedOptions.NamespaceIdentity)
			}
		}
	}
	if namespaceIndex < 0 || postgresIndex < 0 {
		t.Fatalf("expected namespace and postgres executions, got %v", app.FakeExec.Descriptors())
	}
	if namespaceIndex > postgresIndex {
		t.Fatalf("namespace must be provisioned before postgres: %v", app.FakeExec.Descriptors())
	}
}

type failingDeployer struct{ err error }

type recordingRoutes struct{ calls []execution.PublicRoute }

func (r *recordingRoutes) Reconcile(_ context.Context, _ execution.Target, route execution.PublicRoute) error {
	r.calls = append(r.calls, route)
	return nil
}

func TestPublicRouteFollowsReadyWorkloadAndCanBeRemoved(t *testing.T) {
	app, opts := newApp(t)
	deployAll(t, app, opts)
	routes := &recordingRoutes{}
	app.Deployments.SetPublicRouteManager(routes, "example.com")
	before := seed.AcceptanceScores(opts)["frontend"]
	after := seed.AcceptanceScores(opts)["frontend"]
	after["service"].(map[string]any)["publicPort"] = "http"
	cmd := appsvc.DeployCommand{OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey, WorkloadID: "frontend", ScoreBefore: before, ScoreAfter: after, Actor: "test"}
	if _, err := app.Deployments.DeployWorkload(context.Background(), cmd); err != nil {
		t.Fatal(err)
	}
	if len(routes.calls) != 1 || routes.calls[0].Host != "acceptance.example.com" || len(routes.calls[0].Paths) != 1 || routes.calls[0].Paths[0].PortName != "http" {
		t.Fatalf("wrong public route: %+v", routes.calls)
	}
	cmd.ScoreBefore, cmd.ScoreAfter = after, before
	if _, err := app.Deployments.DeployWorkload(context.Background(), cmd); err != nil {
		t.Fatal(err)
	}
	if len(routes.calls) != 2 || len(routes.calls[1].Paths) != 0 {
		t.Fatalf("public route was not removed: %+v", routes.calls)
	}
}

func (f failingDeployer) Apply(context.Context, execution.Target, []execution.Manifest) error {
	return f.err
}

func (f failingDeployer) WaitReady(context.Context, execution.Target, []execution.WorkloadRef) error {
	return f.err
}
func (f failingDeployer) Remove(context.Context, execution.Target, string) error { return f.err }

func TestDeployWorkload_CommitsCurrentSetOnlyAfterReadiness(t *testing.T) {
	ctx := context.Background()
	wantErr := errors.New("readiness timed out")
	app, seedOptions := newApp(t, func(o *bootstrap.Options) {
		o.DeployerOverride = failingDeployer{err: wantErr}
	})

	before, err := app.Store.GetEnvironment(ctx, seedOptions.ApplicationKey, seedOptions.EnvironmentKey)
	if err != nil {
		t.Fatalf("get environment: %v", err)
	}

	scores := seed.AcceptanceScores(seedOptions)
	_, err = app.Deployments.DeployWorkload(ctx, appsvc.DeployCommand{
		OrganizationKey: seedOptions.OrganizationKey,
		ApplicationKey:  seedOptions.ApplicationKey,
		EnvironmentKey:  seedOptions.EnvironmentKey,
		WorkloadID:      "backend",
		ScoreAfter:      scores["backend"],
		Actor:           "test",
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected the readiness failure to surface, got %v", err)
	}

	after, err := app.Store.GetEnvironment(ctx, seedOptions.ApplicationKey, seedOptions.EnvironmentKey)
	if err != nil {
		t.Fatalf("get environment: %v", err)
	}
	if after.CurrentDeploymentSetID != before.CurrentDeploymentSetID || after.Version != before.Version {
		t.Fatalf("the current Deployment Set must not move on failure: %#v -> %#v", before, after)
	}

	deployments, err := app.Queries.ListDeployments(ctx, appsvc.ListDeploymentsQuery{OrganizationKey: seedOptions.OrganizationKey, ApplicationKey: seedOptions.ApplicationKey, EnvironmentKey: seedOptions.EnvironmentKey})
	if err != nil {
		t.Fatalf("list deployments: %v", err)
	}
	if len(deployments) != 1 || deployments[0].Status != domain.StatusFailed {
		t.Fatalf("expected one FAILED deployment, got %#v", deployments)
	}
	if deployments[0].DeltaSnapshotID == "" {
		t.Fatal("execution failure must retain the already-created Delta Snapshot")
	}
}

func TestGetDeployment_ReturnsPersistedPlanAndStatuses(t *testing.T) {
	ctx := context.Background()
	app, seedOptions := newApp(t)
	results := deployAll(t, app, seedOptions)
	last := results[len(results)-1]

	view, err := app.Queries.GetDeployment(ctx, appsvc.GetDeploymentQuery{OrganizationKey: seedOptions.OrganizationKey, ApplicationKey: seedOptions.ApplicationKey, EnvironmentKey: seedOptions.EnvironmentKey, DeploymentID: last.DeploymentID})
	if err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	if view.Deployment.Status != domain.StatusSucceeded {
		t.Fatalf("unexpected status: %s", view.Deployment.Status)
	}
	if view.Graph == nil || view.Batches == nil || view.Matches == nil {
		t.Fatal("the view must carry the persisted graph, batches and matches")
	}
	if view.PlanHash == "" {
		t.Fatal("the view must carry the plan hash")
	}
	if len(view.Resources) == 0 {
		t.Fatal("the view must carry resource progress")
	}
	if len(view.Workloads) != 1 || view.Workloads[0].WorkloadID != last.WorkloadID {
		t.Fatalf("the view must list only this Deployment's workload snapshot, got %#v", view.Workloads)
	}
	if len(view.DeploymentSet.ModuleIDs()) != 3 {
		t.Fatalf("the view must carry the committed Deployment Set, got %v", view.DeploymentSet.ModuleIDs())
	}
}

func TestGetDeployment_PlanningFailureHasNoInventedSnapshot(t *testing.T) {
	ctx := context.Background()
	app, opts := newApp(t)
	score := seed.AcceptanceScores(opts)["backend"]
	score["resources"].(map[string]any)["db"].(map[string]any)["type"] = "unsupported-resource"
	_, err := app.Deployments.DeployWorkload(ctx, appsvc.DeployCommand{
		OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey,
		EnvironmentKey: opts.EnvironmentKey, WorkloadID: "backend", ScoreAfter: score, Actor: "test",
	})
	if err == nil {
		t.Fatal("expected planning failure")
	}
	records, err := app.Queries.ListDeployments(ctx, appsvc.ListDeploymentsQuery{OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey})
	if err != nil || len(records) != 1 {
		t.Fatalf("deployment records: %#v, %v", records, err)
	}
	view, err := app.Queries.GetDeployment(ctx, appsvc.GetDeploymentQuery{OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey, DeploymentID: records[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if view.Deployment.Status != domain.StatusFailed || view.Deployment.FailureReason == "" || view.Deployment.DeltaSnapshotID != "" || view.Delta != nil || view.Graph != nil || view.PlanHash != "" {
		t.Fatalf("planning failure must have status but no invented plan: %#v", view)
	}
}

func TestGetDeployment_RedactsSecretOutputs(t *testing.T) {
	ctx := context.Background()
	app, seedOptions := newApp(t)
	results := deployAll(t, app, seedOptions)

	for _, r := range results {
		view, err := app.Queries.GetDeployment(ctx, appsvc.GetDeploymentQuery{OrganizationKey: seedOptions.OrganizationKey, ApplicationKey: seedOptions.ApplicationKey, EnvironmentKey: seedOptions.EnvironmentKey, DeploymentID: r.DeploymentID})
		if err != nil {
			t.Fatalf("get deployment: %v", err)
		}
		for _, res := range view.Resources {
			if res.ResourceType != "postgres" {
				continue
			}
			password, ok := res.Outputs["password"]
			if !ok {
				t.Fatalf("postgres outputs must list password: %v", res.Outputs)
			}
			if password != appsvc.RedactedValue {
				t.Fatalf("password was not redacted: %v", password)
			}
			if host, ok := res.Outputs["host"].(string); !ok || host == "" {
				t.Fatalf("non-secret outputs must stay visible: %v", res.Outputs)
			}
		}
	}
}
