package transition_test

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"orchestrator/internal/adapters/fake"
	appconfig "orchestrator/internal/application/configuration"
	appsvc "orchestrator/internal/application/deployment"
	"orchestrator/internal/application/transition"
	"orchestrator/internal/bootstrap"
	appdomain "orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/execution"
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
	opts := seed.Defaults()
	opts.RunID, opts.Region, opts.AccountID = "transition-test", "us-east-1", "000000000000"
	options := bootstrap.Options{Seed: opts, Adapters: bootstrap.AdapterFake, ConnectionCredentialStore: "memory"}
	for _, m := range mutate {
		m(&options)
	}
	app, err := bootstrap.Build(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	app.Transitions.SetAsync(false)
	h := &harness{t: t, app: app, opts: opts, cluster: app.FakeCluster, service: app.Transitions, scores: seed.AcceptanceScores(opts)}
	ctx := context.Background()
	conn := appdomain.Connection{ID: ids.New(), Key: destKey, Name: "Second cluster", OrganizationKey: opts.OrganizationKey, Kind: appdomain.ConnectionKubernetes,
		AuthenticationType: appdomain.AuthHostContext, Status: appdomain.ConnectionReady, Config: map[string]any{"cluster": "fake-second", "kubeContext": "fake-second-context", "endpoint": "https://second.invalid"},
		SecretRef: "host-kube-context://fake-second-context", Verification: map[string]any{}}
	if err := app.Store.SaveConnection(ctx, conn); err != nil {
		t.Fatal(err)
	}
	def := resource.Definition{Key: "cluster-internal-" + destKey, ResourceTypeKey: "k8s-cluster", DriverType: resource.DriverExistingCluster, ConnectionKey: destKey,
		DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{"name": "${context.connection.cluster}", "kubeContext": "${context.connection.context}"}}},
		Criteria:     []resource.Criterion{{Class: "internal", ResourceID: "connections." + destKey}}}
	if err := app.Store.SaveResourceDefinition(ctx, opts.OrganizationKey, def); err != nil {
		t.Fatal(err)
	}
	// The acceptance frontend gets a public route so cutover has an Ingress to move.
	service := h.scores["frontend"]["service"].(map[string]any)
	service["publicPort"] = "http"
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

// contextOf is the fake physical cluster a generation's Connection names.
func (h *harness) contextOf(env environment.Environment) string {
	conn, err := h.app.Store.GetConnection(context.Background(), h.opts.OrganizationKey, env.ConnectionKey)
	if err != nil {
		h.t.Fatal(err)
	}
	return conn.ConfigString("kubeContext")
}

func (h *harness) sourceTarget(env environment.Environment) execution.Target {
	return execution.Target{Context: h.contextOf(env), Namespace: env.Namespace()}
}

// databaseTarget is where the source database lives in the fake cluster.
func (h *harness) sourceDatabase() (execution.Target, string) {
	h.t.Helper()
	active, err := h.app.Store.ListActiveResources(context.Background(), h.opts.OrganizationKey)
	if err != nil {
		h.t.Fatal(err)
	}
	for _, a := range active {
		if a.Descriptor.Type == "postgres" && a.Scope.Type == resource.ScopeShared && a.Scope.ID == environment.ScopeID(h.opts.ApplicationKey, h.opts.EnvironmentKey, h.env().TargetGeneration) {
			name, _ := a.ExecutorState["name"].(string)
			namespace, _ := a.ExecutorState["namespace"].(string)
			return execution.Target{Context: h.contextOf(h.env()), Namespace: namespace}, name
		}
	}
	h.t.Fatal("no source database")
	return execution.Target{}, ""
}

func (h *harness) seedJobs(rows int64) {
	target, name := h.sourceDatabase()
	h.cluster.SeedDatabase(target, name, map[string]int64{"public.jobs": rows, "public.results": rows * 2})
}

func (h *harness) preview(mode environment.TransitionMode, mappings ...environment.ResourceMapping) transition.Preview {
	h.t.Helper()
	p, err := h.service.Preview(context.Background(), h.request(mode, mappings...))
	if err != nil {
		h.t.Fatalf("preview: %v", err)
	}
	return p
}

func (h *harness) execute(p transition.Preview, mode environment.TransitionMode, ack bool, mappings ...environment.ResourceMapping) (transition.Detail, error) {
	return h.service.Execute(context.Background(), transition.ExecuteRequest{Request: h.request(mode, mappings...), Token: p.Token, AcknowledgeDowntime: ack, Actor: "tester"})
}

func (h *harness) logIndex(prefix string) int {
	for i, line := range h.cluster.Calls() {
		if strings.HasPrefix(line, prefix) {
			return i
		}
	}
	return -1
}

func errorsIs(err error, target error) bool { return errors.Is(err, target) }

var _ = time.Second

func configSwitch(h *harness, store string, version, configVersion int64) appconfig.SwitchCommand {
	return appconfig.SwitchCommand{OrganizationKey: h.opts.OrganizationKey, ApplicationKey: h.opts.ApplicationKey, EnvironmentKey: h.opts.EnvironmentKey, StoreKey: store, ExpectedVersion: version, ExpectedConfigVersion: configVersion}
}

var generationSuffix = regexp.MustCompile(`-g1-[0-9a-f]{8}$`)

// destTarget is where destination generation 1 lives in the fake cluster.
func (h *harness) destTarget() execution.Target {
	return execution.Target{Context: h.contextOfGeneration(1), Namespace: h.destNamespace()}
}
