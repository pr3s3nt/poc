package provisioning_test

import (
	"context"
	"orchestrator/internal/seed/seedtest"
	"strings"
	"sync"
	"testing"

	"errors"
	"orchestrator/internal/adapters/fake"
	appsvc "orchestrator/internal/application/deployment"
	"orchestrator/internal/bootstrap"
	"orchestrator/internal/bootstrap/bootstraptest"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/seed"
)

// v2Executor is a FAKE Terraform v2 executor: it records what provisioning
// hands a self-verifying executor and delegates outputs to the fake executor.
type v2Executor struct {
	mu    sync.Mutex
	calls []execution.ProvisionRequest
	fake  *fake.ResourceExecutor
}

func (e *v2Executor) VerifiesModules() bool { return true }

func (e *v2Executor) Provision(ctx context.Context, req execution.ProvisionRequest) (execution.ProvisionResult, error) {
	e.mu.Lock()
	e.calls = append(e.calls, req)
	e.mu.Unlock()
	out, err := e.fake.Provision(ctx, req)
	if err != nil {
		return out, err
	}
	// Return module-named outputs, as the real executor does.
	mapped := map[string]any{}
	for module, target := range req.OutputMapping {
		if v, ok := out.Outputs[target]; ok {
			mapped[module] = v
		}
	}
	out.Outputs = mapped
	out.InstanceID = "instance-of-" + req.Descriptor
	if req.ResourceType == "postgres" {
		out.Sensitive = map[string]bool{"password": true}
	}
	if req.ResourceType == "k8s-cluster" {
		out.Target = &execution.Target{Kind: "kubernetes", Provider: "eks", ClusterName: "orc-cluster", Organization: req.OrganizationKey, Connection: req.Connection.Key, Region: "us-east-1"}
	}
	return out, nil
}

type v2Registry struct {
	v2   *v2Executor
	fake *fake.ResourceExecutor
	// override, when set, replaces the Terraform executor.
	override execution.ResourceExecutor
}

func (r v2Registry) Resolve(driver resource.DriverType) (execution.ResourceExecutor, error) {
	if driver == resource.DriverTerraform {
		if r.override != nil {
			return r.override, nil
		}
		return r.v2, nil
	}
	return r.fake, nil
}

func (r v2Registry) VerifiesModules(driver resource.DriverType) bool {
	return driver == resource.DriverTerraform
}

func deployV2(t *testing.T) (*v2Executor, seed.Options) {
	t.Helper()
	opts := seedtest.Defaults()
	opts.Region, opts.AccountID = "us-east-1", "000000000000"
	reg := v2Registry{v2: &v2Executor{fake: fake.NewResourceExecutor()}, fake: fake.NewResourceExecutor()}
	app, err := bootstraptest.Build(context.Background(), bootstrap.Options{Seed: opts, Adapters: bootstrap.AdapterFake, RegistryOverride: reg})
	if err != nil {
		t.Fatal(err)
	}
	scores := seed.AcceptanceScores(opts)
	for i, run := range []string{"run-a", "run-b"} {
		workload := []string{"backend", "worker"}[i]
		if _, err := app.Deployments.DeployWorkload(context.Background(), appsvc.DeployCommand{
			OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.CloudApplicationKey, EnvironmentKey: opts.EnvironmentKey,
			WorkloadID: workload, ScoreAfter: scores[workload], Actor: "test", RunID: run,
		}); err != nil {
			t.Fatalf("deploy %s: %v", workload, err)
		}
	}
	return reg.v2, opts
}

// A Terraform v2 executor receives the pinned module, the plan-pinned
// provider bindings, a canonical identity without any run ID and the instance
// IDs of its Terraform providers; the node's provider runs first.
func TestProvision_V2ExecutorGetsPinsIdentityAndProviderInstances(t *testing.T) {
	v2, opts := deployV2(t)
	byDescriptor := map[string][]execution.ProvisionRequest{}
	order := []string{}
	for _, call := range v2.calls {
		if _, seen := byDescriptor[call.Descriptor]; !seen {
			order = append(order, call.Descriptor)
		}
		byDescriptor[call.Descriptor] = append(byDescriptor[call.Descriptor], call)
	}
	position := func(prefix string) int {
		for i, d := range order {
			if len(d) >= len(prefix) && d[:len(prefix)] == prefix {
				return i
			}
		}
		t.Fatalf("no call for %s in %v", prefix, order)
		return -1
	}
	if !(position("vpc.") < position("k8s-cluster.") && position("vpc.") < position("postgres.")) {
		t.Fatalf("a provider did not run before its consumer: %v", order)
	}
	for descriptor, calls := range byDescriptor {
		first, second := calls[0], calls[len(calls)-1]
		if first.Source == nil || first.Artifact != nil || first.Source.ModuleID == "" {
			t.Fatalf("%s: a self-verifying executor must get the pin, not an artifact: %+v", descriptor, first.Source)
		}
		if len(first.ProviderPins) != 1 || first.ProviderPins[0].Kind != tfmodule.BindingAWS || first.ProviderPins[0].ConnectionKey != opts.CloudConnectionKey || first.ProviderPins[0].Region != "us-east-1" {
			t.Fatalf("%s: pins %+v", descriptor, first.ProviderPins)
		}
		if first.Identity.OrganizationID == "" || first.Identity.EnvironmentID == "" || first.Identity.Descriptor != descriptor {
			t.Fatalf("%s: identity %+v", descriptor, first.Identity)
		}
		// The same node in another run has the identical identity and state key.
		if first.RunID == second.RunID {
			continue
		}
		if first.Identity.StateKey() != second.Identity.StateKey() {
			t.Fatalf("%s: the instance identity changed with the run", descriptor)
		}
	}
	eks := byDescriptor[order[position("k8s-cluster.")]][0]
	if len(eks.ProviderInstanceIDs) != 1 || eks.ProviderInstanceIDs[0] != "instance-of-"+order[position("vpc.")] {
		t.Fatalf("eks provider instances %v", eks.ProviderInstanceIDs)
	}
	if got := byDescriptor[order[position("postgres.")]][0].Type.Key; got != "postgres" {
		t.Fatalf("type contract not passed: %q", got)
	}
}

// Runtime sensitivity reported by the executor makes the output secret even
// though the Resource Type is the only other source: stored values are refs.
func TestProvision_V2RuntimeSensitiveOutputsAreStoredAsReferences(t *testing.T) {
	opts := seedtest.Defaults()
	opts.Region, opts.AccountID = "us-east-1", "000000000000"
	reg := v2Registry{v2: &v2Executor{fake: fake.NewResourceExecutor()}, fake: fake.NewResourceExecutor()}
	app, err := bootstraptest.Build(context.Background(), bootstrap.Options{Seed: opts, Adapters: bootstrap.AdapterFake, RegistryOverride: reg})
	if err != nil {
		t.Fatal(err)
	}
	scores := seed.AcceptanceScores(opts)
	if _, err := app.Deployments.DeployWorkload(context.Background(), appsvc.DeployCommand{
		OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.CloudApplicationKey, EnvironmentKey: opts.EnvironmentKey,
		WorkloadID: "backend", ScoreAfter: scores["backend"], Actor: "test", RunID: "run-a",
	}); err != nil {
		t.Fatal(err)
	}
	active, err := app.Store.ListActiveResources(context.Background(), opts.OrganizationKey)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range active {
		if a.Descriptor.Type != "postgres" {
			continue
		}
		ref, ok := a.Outputs["password"].(map[string]any)
		if !ok || ref["secretRef"] == "" || a.Outputs["host"] == "" {
			t.Fatalf("postgres outputs %+v", a.Outputs)
		}
		return
	}
	t.Fatal("no postgres Active Resource")
}

// holdingExecutor is a FAKE v2 executor that reaches an unknown outcome.
type holdingExecutor struct{ v2Executor }

func (e *holdingExecutor) Provision(ctx context.Context, req execution.ProvisionRequest) (execution.ProvisionResult, error) {
	persistence.MarkHold(ctx)
	return execution.ProvisionResult{}, errors.New("unknown outcome of a Terraform step")
}

// An unknown outcome marked by the executor leaves the Environment held
// (INTERRUPTED) even though the deploy path records an ordinary failure: no
// second deploy can start until an operator confirms the process stopped.
func TestDeployWithUnknownTerraformOutcomeHoldsTheEnvironment(t *testing.T) {
	opts := seedtest.Defaults()
	opts.Region, opts.AccountID = "us-east-1", "000000000000"
	held := &holdingExecutor{v2Executor{fake: fake.NewResourceExecutor()}}
	reg := v2Registry{v2: &held.v2Executor, fake: fake.NewResourceExecutor()}
	reg.override = held
	app, err := bootstraptest.Build(context.Background(), bootstrap.Options{Seed: opts, Adapters: bootstrap.AdapterFake, RegistryOverride: reg})
	if err != nil {
		t.Fatal(err)
	}
	scores := seed.AcceptanceScores(opts)
	cmd := appsvc.DeployCommand{OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.CloudApplicationKey, EnvironmentKey: opts.EnvironmentKey, WorkloadID: "backend", ScoreAfter: scores["backend"], Actor: "test", RunID: "run-a"}
	if _, err := app.Deployments.DeployWorkload(context.Background(), cmd); err == nil {
		t.Fatal("the deploy succeeded after an unknown outcome")
	}
	op, ok, err := app.Store.ActiveOperation(context.Background(), opts.CloudApplicationKey, opts.EnvironmentKey)
	if err != nil || !ok || op.Status != environment.OpInterrupted {
		t.Fatalf("operation after an unknown outcome: %+v ok=%v err=%v", op, ok, err)
	}
	if _, err := app.Deployments.DeployWorkload(context.Background(), cmd); !errors.Is(err, persistence.ErrEnvironmentBusy) {
		t.Fatalf("a held Environment accepted another deploy: %v", err)
	}
}

// shortOutputExecutor is a FAKE v2 executor that omits a required output of the PostgreSQL Type.
type shortOutputExecutor struct{ v2Executor }

func (e *shortOutputExecutor) Provision(ctx context.Context, req execution.ProvisionRequest) (execution.ProvisionResult, error) {
	out, err := e.v2Executor.Provision(ctx, req)
	if err == nil && req.ResourceType == "postgres" {
		delete(out.Outputs, "host")
	}
	return out, err
}

// ADR-017: required outputs are checked before any consumer. A provider whose output misses a required field
// fails, no workload is rendered or applied, the failed node is recorded and the consumer is reported blocked
// with a safe code.
func TestProvision_MissingRequiredOutputBlocksTheConsumerAndIsRecorded(t *testing.T) {
	opts := seedtest.Defaults()
	opts.Region, opts.AccountID = "us-east-1", "000000000000"
	short := &shortOutputExecutor{v2Executor{fake: fake.NewResourceExecutor()}}
	reg := v2Registry{v2: &short.v2Executor, fake: fake.NewResourceExecutor(), override: short}
	app, err := bootstraptest.Build(context.Background(), bootstrap.Options{Seed: opts, Adapters: bootstrap.AdapterFake, RegistryOverride: reg})
	if err != nil {
		t.Fatal(err)
	}
	scores := seed.AcceptanceScores(opts)
	_, err = app.Deployments.DeployWorkload(context.Background(), appsvc.DeployCommand{
		OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.CloudApplicationKey, EnvironmentKey: opts.EnvironmentKey,
		WorkloadID: "backend", ScoreAfter: scores["backend"], Actor: "test", RunID: "run-a",
	})
	if err == nil || !strings.Contains(err.Error(), `requires output "host"`) {
		t.Fatalf("a missing required output must fail the deployment: %v", err)
	}
	history, err := app.Queries.ListDeployments(context.Background(), appsvc.ListDeploymentsQuery{OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.CloudApplicationKey, EnvironmentKey: opts.EnvironmentKey})
	if err != nil || len(history) != 1 || history[0].Status != "FAILED" {
		t.Fatalf("history = %+v %v", history, err)
	}
	if strings.Contains(history[0].FailureReason, "host") {
		t.Fatalf("the public failure reason must be a fixed sentence: %q", history[0].FailureReason)
	}
	view, err := app.Queries.GetDeployment(context.Background(), appsvc.GetDeploymentQuery{OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.CloudApplicationKey, EnvironmentKey: opts.EnvironmentKey, DeploymentID: history[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	failedNode := ""
	for _, r := range view.Resources {
		if r.Status == "FAILED" {
			failedNode = r.Descriptor
		}
	}
	if !strings.HasPrefix(failedNode, "postgres.") {
		t.Fatalf("the provider that broke its contract must be the recorded failed node, got %q in %+v", failedNode, view.Resources)
	}
	blocked := false
	for _, b := range view.Blocked {
		if b.Provider == failedNode && strings.HasPrefix(b.Consumer, "workload.") && b.Code == appsvc.RejectedBeforeRun {
			blocked = true
		}
	}
	if !blocked {
		t.Fatalf("the workload consumer must be reported blocked by the failed provider: %+v", view.Blocked)
	}
	if len(view.Workloads) != 0 {
		t.Fatalf("no workload may run before its dependency is ready: %+v", view.Workloads)
	}
}
