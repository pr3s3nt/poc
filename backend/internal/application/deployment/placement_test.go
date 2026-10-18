package deployment_test

import (
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"orchestrator/internal/adapters/fake"
	"orchestrator/internal/adapters/postgres/postgrestest"
	"orchestrator/internal/adapters/scorek8s"
	"orchestrator/internal/adapters/store"
	"orchestrator/internal/adapters/terraform/registry"
	appsvc "orchestrator/internal/application/deployment"
	"orchestrator/internal/bootstrap"
	"orchestrator/internal/domain/application"
	domain "orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/planning"
	"orchestrator/internal/seed"
	"orchestrator/internal/seed/seedtest"
)

// ADR-016 execution: the workload deploys to the cluster its own Definition
// references name, using the cluster Definition's Internal account, and the
// applied target survives restarts and catalog edits.

type placedFixture struct {
	app      *bootstrap.App
	opts     seed.Options
	deployer *targetDeployer
	state    string
	appKey   string
}

func placedApp(t *testing.T) *placedFixture { return newPlacedApp(t, nil) }

// newPlacedApp builds the product with fake workload adapters and the real
// score-k8s renderer; mutate swaps the Console store (PostgreSQL variant).
func newPlacedApp(t *testing.T, mutate func(*bootstrap.Options)) *placedFixture {
	t.Helper()
	ctx := context.Background()
	path, err := exec.LookPath("score-k8s")
	if err != nil {
		t.Skip("local score-k8s unavailable")
	}
	opts := seedtest.Defaults()
	opts.Profile, opts.Catalog, opts.NamespaceIdentity = "test", seed.CatalogNone, "orch-v2-test-ns1"
	deployer := &targetDeployer{WorkloadDeployer: fake.NewWorkloadDeployer()}
	state := filepath.Join(t.TempDir(), "state.json")
	buildOptions := bootstrap.Options{Seed: opts, Adapters: bootstrap.AdapterFake, ScoreK8sPath: path, RequireScoreRenderer: true, StatePath: state, DeployerOverride: deployer}
	if mutate != nil {
		mutate(&buildOptions)
	}
	app, err := bootstrap.Build(ctx, buildOptions)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	f := &placedFixture{app: app, opts: opts, deployer: deployer, state: state, appKey: opts.ApplicationKey}
	for _, typ := range []resource.Type{
		{Key: "workload"},
		{Key: "k8s-namespace", Outputs: []resource.OutputField{{Name: "name", Type: "string", Required: true}}},
		{Key: "k8s-cluster", Outputs: []resource.OutputField{{Name: "name", Type: "string"}, {Name: "kubeContext", Type: "string"}, {Name: "cluster", Type: "any", Required: true}}},
	} {
		if err := app.Store.SaveResourceType(ctx, opts.OrganizationKey, typ); err != nil {
			t.Fatal(err)
		}
	}
	f.account(t, "lab", 4, application.ConnectionReady)
	f.account(t, "lab2", 9, application.ConnectionReady)
	refs, err := registry.PackagedRefs()
	if err != nil {
		t.Fatal(err)
	}
	defs := []resource.Definition{
		{Key: "cluster-lab", Name: "cluster-lab", Version: 1, ResourceTypeKey: "k8s-cluster", DriverType: resource.DriverExistingCluster, DriverAccount: "lab",
			DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{}}}, Criteria: []resource.Criterion{{Class: "internal"}}},
		{Key: "namespace-module", Name: "namespace", Version: 1, ResourceTypeKey: "k8s-namespace", DriverType: resource.DriverTerraform, Criteria: []resource.Criterion{{}},
			DriverInputs: map[string]any{"values": map[string]any{
				"source": refs["k8s-namespace"].Map(), "variables": map[string]any{"name": "${context.env.namespace}"},
				"input_mapping": map[string]any{}, "output_mapping": map[string]any{"name": "name"},
				"provider_bindings": map[string]any{"kubernetes": map[string]any{"target": tfmodule.TargetResource, "resource_ref": "k8s-cluster.internal#environments.@app.@env"}},
			}}},
		{Key: "workload-score", Name: "workload", Version: 1, ResourceTypeKey: "workload", DriverType: resource.DriverScoreK8s, Criteria: []resource.Criterion{{}},
			DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{
				"render_bundle": scorek8s.BundleID,
				"namespace":     "${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}",
				"cluster":       "${resources['k8s-cluster.internal#environments.@app.@env'].outputs.cluster}",
			}}}},
	}
	for _, def := range defs {
		if def.DriverType == resource.DriverScoreK8s {
			def.SourceFingerpr = ""
		}
		if err := app.Store.SaveResourceDefinition(ctx, opts.OrganizationKey, def); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func (f *placedFixture) account(t *testing.T, key string, version int64, status application.ConnectionStatus) {
	t.Helper()
	conn := application.Connection{ID: stableUUID(key), Key: key, Name: key, OrganizationKey: f.opts.OrganizationKey, Kind: application.ConnectionKubernetes,
		AuthenticationType: application.AuthKubeconfig, Status: status, Version: version, SecretRef: "fake-credential://" + key,
		Config: map[string]any{"cluster": key + "-cluster", "kubeContext": key + "-ctx"}}
	if err := f.app.Store.SaveConnection(context.Background(), conn); err != nil {
		t.Fatal(err)
	}
}

func (f *placedFixture) frontend(image string, extra ...func(map[string]any)) map[string]any {
	doc := map[string]any{
		"apiVersion": "score.dev/v1b1", "metadata": map[string]any{"name": "frontend"},
		"containers": map[string]any{"main": map[string]any{"image": image, "variables": map[string]any{"BACKEND_URL": "http://backend.invalid:8080"}}},
		"service":    map[string]any{"ports": map[string]any{"http": map[string]any{"port": 80, "targetPort": 8080}}},
	}
	for _, mutate := range extra {
		mutate(doc)
	}
	return doc
}

func (f *placedFixture) deploy(before, after map[string]any) error {
	_, err := f.app.Deployments.DeployWorkload(context.Background(), appsvc.DeployCommand{
		OrganizationKey: f.opts.OrganizationKey, ApplicationKey: f.appKey, EnvironmentKey: "staging", WorkloadID: "frontend",
		ScoreBefore: before, ScoreAfter: after, Actor: "test", RunID: "run-v03",
	})
	return err
}

func TestPlacedDeploy_WorkloadRunsOnTheClusterItsDefinitionsName(t *testing.T) {
	f := placedApp(t)
	ctx := context.Background()
	env, err := f.app.Store.GetEnvironment(ctx, f.appKey, "staging")
	if err != nil || env.Configured() || env.Profile != "" {
		t.Fatalf("the Environment must stay without a deployment Connection: %+v %v", env, err)
	}
	score := f.frontend("example.invalid/frontend:one")
	if err := f.deploy(nil, score); err != nil {
		t.Fatal(err)
	}
	if len(f.deployer.targets) != 1 {
		t.Fatalf("targets = %+v", f.deployer.targets)
	}
	target := f.deployer.targets[0]
	if target.Connection != "lab" || target.Organization != f.opts.OrganizationKey || target.ClusterName != "lab-cluster" || target.Context != "lab-ctx" || target.Namespace != "orch-v2-test-ns1" || target.Kubeconfig != "" {
		t.Fatalf("applied on %+v", target)
	}
	// The cluster and the namespace ran with the cluster Definition's account, in provider-first order.
	var order []string
	for _, call := range f.app.FakeExec.Calls {
		order = append(order, call.ResourceType+"@"+call.Connection.Key)
	}
	if strings.Join(order, ",") != "k8s-cluster@lab,k8s-namespace@lab" {
		t.Fatalf("executor order = %v", order)
	}
	instances, err := f.app.Store.ListWorkloadInstancesFor(ctx, f.appKey+"/staging", 0)
	if err != nil || len(instances) != 1 || instances[0].Status != domain.InstanceReady {
		t.Fatalf("instances = %+v %v", instances, err)
	}
	ref := instances[0].TargetRef
	placement, _ := ref["placement"].(map[string]any)
	if ref["connection"] != "lab" || ref["cluster"] != "lab-cluster" || ref["namespace"] != "orch-v2-test-ns1" ||
		placement["account"] != "lab" || placement["accountVersion"] != float64(4) || placement["clusterDefinition"] != "cluster-lab" ||
		placement["cluster"] != "k8s-cluster.internal#environments."+f.appKey+".staging" {
		t.Fatalf("persisted target = %+v", ref)
	}
	for _, secret := range []string{"kubeconfig", "token", "secret", "password"} {
		for key := range ref {
			if strings.Contains(strings.ToLower(key), secret) {
				t.Fatalf("persisted target holds %q", key)
			}
		}
	}
}

func TestPlacedDeploy_UpdateRemoveAndRestartUseTheOriginalTargetEvenIfTheCatalogChanges(t *testing.T) {
	f := placedApp(t)
	ctx := context.Background()
	one := f.frontend("example.invalid/frontend:one")
	if err := f.deploy(nil, one); err != nil {
		t.Fatal(err)
	}
	// A Platform Engineer repoints the cluster Definition at another account.
	def, err := loadDefinition(f, "cluster-lab")
	if err != nil {
		t.Fatal(err)
	}
	def.DriverAccount = "lab2"
	def.Version++
	if err := f.app.Store.SaveResourceDefinition(ctx, f.opts.OrganizationKey, def); err != nil {
		t.Fatal(err)
	}
	calls, applies := len(f.app.FakeExec.Calls), len(f.deployer.targets)
	two := f.frontend("example.invalid/frontend:two")
	err = f.deploy(one, two)
	if !errors.Is(err, appsvc.ErrTargetChanged) {
		t.Fatalf("an update must not silently move the workload: %v", err)
	}
	if len(f.app.FakeExec.Calls) != calls || len(f.deployer.targets) != applies {
		t.Fatalf("a refused move caused side effects: %d executor calls, %d applies", len(f.app.FakeExec.Calls)-calls, len(f.deployer.targets)-applies)
	}
	deployments, _ := f.app.Store.ListDeployments(ctx, f.appKey, "staging")
	last := deployments[0]
	for _, d := range deployments {
		if d.StartedAt.After(last.StartedAt) {
			last = d
		}
	}
	if last.Status != domain.StatusFailed || last.FailureReason != appsvc.FailureTargetChange {
		t.Fatalf("last deployment = %+v", last)
	}
	// Restart: reopen the persisted state; removal still reaches the ORIGINAL account.
	reopened, err := store.NewWithSnapshot(f.state)
	if err != nil {
		t.Fatal(err)
	}
	instances, err := reopened.ListWorkloadInstancesFor(ctx, f.appKey+"/staging", 0)
	if err != nil || len(instances) != 1 || instances[0].TargetRef["connection"] != "lab" {
		t.Fatalf("reopened instances = %+v %v", instances, err)
	}
	if err := f.deploy(one, nil); err != nil {
		t.Fatalf("remove: %v", err)
	}
	removed := f.deployer.targets[len(f.deployer.targets)-1]
	if removed.Connection != "lab" || removed.ClusterName != "lab-cluster" || removed.Namespace != "orch-v2-test-ns1" {
		t.Fatalf("removed on %+v, want the original lab target", removed)
	}
}

func TestPlacedDeploy_UnreadyOrWrongAccountFailsBeforeAnyConsumer(t *testing.T) {
	f := placedApp(t)
	f.account(t, "lab", 5, application.ConnectionVerifying) // cluster account is no longer READY
	err := f.deploy(nil, f.frontend("example.invalid/frontend:one"))
	var staged *planning.StageError
	if !errors.As(err, &staged) || staged.Reason != planning.ReasonPlacement {
		t.Fatalf("deploy = %v", err)
	}
	if len(f.app.FakeExec.Calls) != 0 || len(f.deployer.targets) != 0 {
		t.Fatalf("side effects: %d executor calls, %d applies", len(f.app.FakeExec.Calls), len(f.deployer.targets))
	}
	// A wrong-kind account never replaces the winner either, and READY lab2 is not a fallback.
	wrongKind := application.Connection{ID: stableUUID("lab"), Key: "lab", Name: "lab", OrganizationKey: f.opts.OrganizationKey, Kind: application.ConnectionAWS, AuthenticationType: application.AuthAWSAccessKey, Status: application.ConnectionReady, Version: 7}
	if err := f.app.Store.SaveConnection(context.Background(), wrongKind); err != nil {
		t.Fatal(err)
	}
	if err := f.deploy(nil, f.frontend("example.invalid/frontend:one")); err == nil {
		t.Fatal("a wrong-kind account must fail")
	}
	for _, call := range f.app.FakeExec.Calls {
		if call.Connection.Key == "lab2" {
			t.Fatal("another account became a fallback")
		}
	}
	if len(f.app.FakeDeploy.Applied) != 0 {
		t.Fatal("a workload was applied despite the failed placement")
	}
}

func TestPlacedDeploy_ClassSelectsTheDefinitionAndSurvivesTheSnapshot(t *testing.T) {
	f := placedApp(t)
	ctx := context.Background()
	gold, err := loadDefinition(f, "workload-score")
	if err != nil {
		t.Fatal(err)
	}
	gold.Key, gold.Criteria = "workload-gold", []resource.Criterion{{Class: "gold"}}
	if err := f.app.Store.SaveResourceDefinition(ctx, f.opts.OrganizationKey, gold); err != nil {
		t.Fatal(err)
	}
	plain, err := loadDefinition(f, "workload-score")
	if err != nil {
		t.Fatal(err)
	}
	plain.Criteria = []resource.Criterion{{Class: "default"}}
	plain.Version++
	if err := f.app.Store.SaveResourceDefinition(ctx, f.opts.OrganizationKey, plain); err != nil {
		t.Fatal(err)
	}
	withClass := f.frontend("example.invalid/frontend:one", func(d map[string]any) { d["metadata"] = map[string]any{"name": "frontend", "class": "gold"} })
	if err := f.deploy(nil, withClass); err != nil {
		t.Fatal(err)
	}
	reopened, err := store.NewWithSnapshot(f.state)
	if err != nil {
		t.Fatal(err)
	}
	if set, err := reopened.GetDeploymentSet(ctx, mustCurrentSet(t, reopened, f)); err != nil || set.Document.Modules["frontend"].Class != "gold" {
		t.Fatalf("class lost across reopen: %+v %v", set.Document.Modules["frontend"], err)
	}
	deployments, _ := reopened.ListDeployments(ctx, f.appKey, "staging")
	plan, err := reopened.GetPlan(ctx, deployments[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	rendering := plan["rendering"].(map[string]any)["frontend"].(map[string]any)
	if rendering["definitionKey"] != "workload-gold" || rendering["class"] != "gold" {
		t.Fatalf("rendering pin = %+v", rendering)
	}
}

func mustCurrentSet(t *testing.T, st *store.Store, f *placedFixture) string {
	t.Helper()
	env, err := st.GetEnvironment(context.Background(), f.appKey, "staging")
	if err != nil {
		t.Fatal(err)
	}
	return env.CurrentDeploymentSetID
}

func loadDefinition(f *placedFixture, key string) (resource.Definition, error) {
	defs, err := f.app.Store.ListResourceDefinitions(context.Background(), f.opts.OrganizationKey)
	if err != nil {
		return resource.Definition{}, err
	}
	for _, def := range defs {
		if def.Key == key {
			return def, nil
		}
	}
	return resource.Definition{}, errors.New("definition not found")
}

// SQL (ADR-016 fresh baseline): class, per-workload placement pins and the applied target of a
// workload survive a reopen of the PostgreSQL Console store, and a catalog edit after the reopen
// still cannot move it. Real PostgreSQL; skipped only without a configured server (never under REQUIRE_POSTGRES=1).
func TestPlacedDeploy_PostgresReopenKeepsClassTargetAndPlacements(t *testing.T) {
	db := postgrestest.Fresh(t)
	f := newPlacedApp(t, func(o *bootstrap.Options) { o.StatePath, o.DatabaseURL = "", db.Console })
	ctx := context.Background()
	withClass := f.frontend("example.invalid/frontend:one", func(d map[string]any) { d["metadata"] = map[string]any{"name": "frontend", "class": "default"} })
	if err := f.deploy(nil, withClass); err != nil {
		t.Fatal(err)
	}
	reopened := postgrestest.OpenConsole(t, db)
	instances, err := reopened.ListWorkloadInstancesFor(ctx, f.appKey+"/staging", 0)
	if err != nil || len(instances) != 1 {
		t.Fatalf("instances = %+v %v", instances, err)
	}
	ref := instances[0].TargetRef
	placement, _ := ref["placement"].(map[string]any)
	if ref["connection"] != "lab" || placement["account"] != "lab" || placement["accountVersion"] != float64(4) || placement["namespace"] != "k8s-namespace.default#environments."+f.appKey+".staging" {
		t.Fatalf("persisted target = %+v", ref)
	}
	env, err := reopened.GetEnvironment(ctx, f.appKey, "staging")
	if err != nil || env.Configured() {
		t.Fatalf("environment = %+v %v", env, err)
	}
	set, err := reopened.GetDeploymentSet(ctx, env.CurrentDeploymentSetID)
	if err != nil || set.Document.Modules["frontend"].Class != "default" {
		t.Fatalf("an explicit class must round-trip unchanged (never dropped or converted): %+v %v", set.Document.Modules["frontend"], err)
	}
	deployments, _ := reopened.ListDeployments(ctx, f.appKey, "staging")
	plan, err := reopened.GetPlan(ctx, deployments[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	placements, _ := plan["placements"].(map[string]any)
	if len(placements) != 2 {
		t.Fatalf("plan placements = %v", placements)
	}
	// Repointing the cluster Definition after the reopen is refused for the applied workload.
	def, err := loadDefinition(f, "cluster-lab")
	if err != nil {
		t.Fatal(err)
	}
	def.DriverAccount, def.Version = "lab2", def.Version+1
	if err := f.app.Store.SaveResourceDefinition(ctx, f.opts.OrganizationKey, def); err != nil {
		t.Fatal(err)
	}
	if err := f.deploy(withClass, f.frontend("example.invalid/frontend:two", func(d map[string]any) { d["metadata"] = map[string]any{"name": "frontend", "class": "default"} })); !errors.Is(err, appsvc.ErrTargetChanged) {
		t.Fatalf("a retarget must be refused after the reopen: %v", err)
	}
}

// stableUUID gives a connection one fixed identifier per key (PostgreSQL ids are UUIDs).
func stableUUID(key string) string {
	sum := md5.Sum([]byte("placement-" + key))
	return fmt.Sprintf("%x-%x-%x-%x-%x", sum[0:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16])
}

// UC-06 owns the lifecycle order: a resource that depends on the workload (is_dependent) is
// provisioned after the workload is applied; an empty rule is provisioned before it.
func TestPlacedDeploy_ProvisionFlagsOrderResourcesAroundTheWorkload(t *testing.T) {
	f := placedApp(t)
	ctx := context.Background()
	def, err := loadDefinition(f, "workload-score")
	if err != nil {
		t.Fatal(err)
	}
	def.Version++
	def.Provision = map[string]resource.ProvisionRule{
		"k8s-namespace.default#environments.@app.@env-pre":  {},
		"k8s-namespace.default#environments.@app.@env-post": {IsDependent: true},
	}
	if err := f.app.Store.SaveResourceDefinition(ctx, f.opts.OrganizationKey, def); err != nil {
		t.Fatal(err)
	}
	var atApply []string
	f.deployer.onApply = func() {
		for _, call := range f.app.FakeExec.Calls {
			atApply = append(atApply, call.Descriptor)
		}
	}
	if err := f.deploy(nil, f.frontend("example.invalid/frontend:one")); err != nil {
		t.Fatal(err)
	}
	has := func(list []string, suffix string) bool {
		for _, d := range list {
			if strings.HasSuffix(d, suffix) {
				return true
			}
		}
		return false
	}
	var all []string
	for _, call := range f.app.FakeExec.Calls {
		all = append(all, call.Descriptor)
	}
	if !has(atApply, "-pre") || has(atApply, "-post") {
		t.Fatalf("at apply time the empty rule must be provisioned and the dependent one not: %v", atApply)
	}
	if !has(all, "-post") || len(all) != len(atApply)+1 {
		t.Fatalf("the dependent resource must run after the workload: before=%v all=%v", atApply, all)
	}
	// An update runs the same order again: the post-workload resource follows its apply.
	before := len(f.app.FakeExec.Calls)
	var countAtApply int
	f.deployer.onApply = func() { countAtApply = len(f.app.FakeExec.Calls) }
	if err := f.deploy(f.frontend("example.invalid/frontend:one"), f.frontend("example.invalid/frontend:two")); err != nil {
		t.Fatal(err)
	}
	total := len(f.app.FakeExec.Calls)
	if countAtApply != before+3 || total != before+4 {
		t.Fatalf("update order: calls before=%d at apply=%d total=%d (cluster, namespace and the empty rule precede the apply; the dependent resource follows)", before, countAtApply, total)
	}
}
