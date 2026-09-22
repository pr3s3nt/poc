package planning

import (
	"strings"
	"testing"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning/score"
	"orchestrator/internal/seed"
)

func seedOptions() seed.Options {
	o := seed.Defaults()
	o.Region = "us-east-1"
	o.AccountID = "000000000000"
	o.RunID = "run-test"
	return o
}

func catalogOf(o seed.Options) Catalog {
	catalog := Catalog{Types: map[string]resource.Type{}, Definitions: seed.ResourceDefinitions(o)}
	for _, rt := range seed.ResourceTypes() {
		catalog.Types[rt.Key] = rt
	}
	return catalog
}

func testRequest(t *testing.T, profile application.ExecutionProfile, workload string) Request {
	t.Helper()
	o := seedOptions()
	appKey, namespace, connectionKey := o.ApplicationKey, o.NamespaceIdentity, o.ConnectionKey
	if profile == application.ProfileAWSEKS {
		appKey, namespace, connectionKey = o.CloudApplicationKey, o.CloudNamespaceIdentity, o.CloudConnectionKey
	}

	doc, err := score.FromMap(seed.AcceptanceScores(o)[workload])
	if err != nil {
		t.Fatalf("score: %v", err)
	}

	app := application.Application{
		Key: appKey, OrganizationKey: o.OrganizationKey, Name: appKey,
		Profile: profile, ConnectionKey: connectionKey, Version: 1,
	}
	if profile == application.ProfileAWSEKS {
		app.Region = o.Region
	}
	env := environment.Environment{
		Key: o.EnvironmentKey, ApplicationKey: appKey, Name: o.EnvironmentName,
		Type: o.EnvironmentType, NamespaceIdentity: namespace, Version: 1,
	}
	conn := application.Connection{Key: connectionKey, Status: application.ConnectionReady,
		Config: map[string]any{"cluster": o.ClusterName, "kubeContext": o.KubeContext}}

	return Request{
		OrganizationKey: o.OrganizationKey,
		App:             app,
		Env:             env,
		Connection:      conn,
		BaseSet:         environment.NewDocument(),
		After:           doc,
		WorkloadID:      workload,
		RunID:           o.RunID,
		Catalog:         catalogOf(o),
		Terraform:       stubInspector{},
	}
}

// stubInspector mirrors the embedded Terraform modules without reading them, so
// planning tests stay independent of the adapter.
type stubInspector struct{}

func (stubInspector) ExecutorVariables() []string {
	return []string{"region", "tags", "master_password"}
}

func (stubInspector) ExecutorOutputs(module string) []string {
	if module == "eks" {
		return []string{"kubeContext", "kubeconfig"}
	}
	return nil
}

func (stubInspector) Inspect(module string) (ModuleContract, error) {
	contracts := map[string]ModuleContract{
		"vpc": {
			Module: "vpc", Fingerprint: "sha256:vpc",
			Outputs: []string{"cidr", "id", "subnetIds"},
			Variables: map[string]ModuleVariable{
				"region": {Name: "region"}, "name": {Name: "name"}, "tags": {Name: "tags"},
				"cidr": {Name: "cidr"}, "az_count": {Name: "az_count", HasDefault: true},
			},
		},
		"eks": {
			Module: "eks", Fingerprint: "sha256:eks",
			Outputs: []string{"cluster_security_group_id", "endpoint", "name", "node_group_name", "node_role_arn"},
			Variables: map[string]ModuleVariable{
				"region": {Name: "region"}, "name": {Name: "name"}, "tags": {Name: "tags"},
				"subnet_ids": {Name: "subnet_ids"}, "kubernetes_version": {Name: "kubernetes_version"},
				"node_instance_type": {Name: "node_instance_type"}, "node_capacity_type": {Name: "node_capacity_type"},
				"node_disk_size": {Name: "node_disk_size"}, "node_ami_type": {Name: "node_ami_type"},
			},
		},
		"aurora": {
			Module: "aurora", Fingerprint: "sha256:aurora",
			Outputs: []string{"cluster_identifier", "database", "host", "password", "port", "security_group_id", "username", "writer_identifier"},
			Variables: map[string]ModuleVariable{
				"region": {Name: "region"}, "name": {Name: "name"}, "tags": {Name: "tags"},
				"vpc_id": {Name: "vpc_id"}, "vpc_cidr": {Name: "vpc_cidr"}, "subnet_ids": {Name: "subnet_ids"},
				"database": {Name: "database"}, "username": {Name: "username"},
				"master_password": {Name: "master_password"}, "engine_version": {Name: "engine_version"},
				"min_capacity": {Name: "min_capacity"}, "max_capacity": {Name: "max_capacity"},
				"seconds_until_auto_pause": {Name: "seconds_until_auto_pause", HasDefault: true},
			},
		},
	}
	contract, ok := contracts[module]
	if !ok {
		return ModuleContract{}, errUnknownModule(module)
	}
	return contract, nil
}

type errUnknownModule string

func (e errUnknownModule) Error() string { return "unknown module " + string(e) }

func TestPlan_UsesDeploymentSetPathsAsResourceIdentity(t *testing.T) {
	plan, err := NewService().Plan(testRequest(t, application.ProfileInternalK8s, "backend"))
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	want := map[string]bool{
		"workload.default#modules.backend":                  true,
		"postgres.default#shared.acceptance-db":             true,
		"k8s-namespace.default#environments.acceptance.dev": true,
		"k8s-cluster.internal#connections.internal-cluster": true,
	}
	got := map[string]bool{}
	for _, n := range plan.Graph.Nodes {
		got[n.Descriptor] = true
	}
	for descriptor := range want {
		if !got[descriptor] {
			t.Fatalf("missing node %s in %v", descriptor, got)
		}
	}
}

func TestPlan_InternalGraphIsProviderFirst(t *testing.T) {
	plan, err := NewService().Plan(testRequest(t, application.ProfileInternalK8s, "backend"))
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if len(plan.Batches) != 3 {
		t.Fatalf("expected three batches, got %v", plan.Batches)
	}
	if !strings.HasPrefix(plan.Batches[0][0], "k8s-cluster.internal#") {
		t.Fatalf("expected the registered cluster first, got %v", plan.Batches)
	}
	if !strings.HasPrefix(plan.Batches[1][0], "k8s-namespace.default#") {
		t.Fatalf("expected the namespace in batch 1, got %v", plan.Batches)
	}
	if !strings.HasPrefix(plan.Batches[2][0], "postgres.default#") {
		t.Fatalf("expected postgres last, got %v", plan.Batches)
	}
	for _, n := range plan.Graph.Nodes {
		if n.ResourceType == TypeVPC {
			t.Fatalf("internal-k8s must not add a VPC node: %v", n)
		}
	}
}

func TestPlan_AWSAddsImplicitVPCAndEKS(t *testing.T) {
	plan, err := NewService().Plan(testRequest(t, application.ProfileAWSEKS, "backend"))
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	var hasVPC, hasEKS bool
	for _, n := range plan.Graph.Nodes {
		if n.ResourceType == TypeVPC {
			hasVPC = true
		}
		if n.ResourceType == TypeCluster && n.Class == ClassEKS {
			hasEKS = true
		}
	}
	if !hasVPC || !hasEKS {
		t.Fatalf("aws-eks must add implicit VPC and EKS nodes: %v", plan.Graph.Nodes)
	}
	if !strings.HasPrefix(plan.Batches[0][0], "vpc.default#") {
		t.Fatalf("VPC must be provisioned first, got %v", plan.Batches)
	}
	last := plan.Batches[len(plan.Batches)-1]
	if !strings.HasPrefix(last[0], "k8s-namespace.default#") {
		t.Fatalf("namespace must come after the cluster, got %v", plan.Batches)
	}
}

func TestPlan_PostgresResolvesPerApplication(t *testing.T) {
	internal, err := NewService().Plan(testRequest(t, application.ProfileInternalK8s, "backend"))
	if err != nil {
		t.Fatalf("plan internal: %v", err)
	}
	cloud, err := NewService().Plan(testRequest(t, application.ProfileAWSEKS, "backend"))
	if err != nil {
		t.Fatalf("plan cloud: %v", err)
	}
	if got := matchOf(t, internal, "postgres"); got != "postgres-internal-statefulset" {
		t.Fatalf("internal postgres matched %q", got)
	}
	if got := matchOf(t, cloud, "postgres"); got != "postgres-aws-aurora" {
		t.Fatalf("cloud postgres matched %q", got)
	}
}

func matchOf(t *testing.T, plan *Plan, resourceType string) string {
	t.Helper()
	for _, n := range plan.Graph.Nodes {
		if n.ResourceType == resourceType {
			return plan.Matches[n.Descriptor].DefinitionKey
		}
	}
	t.Fatalf("no node of type %q", resourceType)
	return ""
}

func TestPlan_ScoreParamsBecomeResourceInputs(t *testing.T) {
	plan, err := NewService().Plan(testRequest(t, application.ProfileInternalK8s, "backend"))
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	entry, ok := plan.CandidateSet.Shared["acceptance-db"]
	if !ok {
		t.Fatalf("shared entry missing: %v", plan.CandidateSet.Shared)
	}
	if entry.Params["database"] != "acceptance" || entry.Params["username"] != "app" {
		t.Fatalf("params did not reach the Deployment Set: %v", entry.Params)
	}
	node, ok := plan.Graph.Node("postgres.default#shared.acceptance-db")
	if !ok {
		t.Fatal("postgres node missing")
	}
	if node.Params["database"] != "acceptance" {
		t.Fatalf("params did not reach the node: %v", node.Params)
	}
}

func TestPlan_TerraformContractIsInspected(t *testing.T) {
	plan, err := NewService().Plan(testRequest(t, application.ProfileAWSEKS, "backend"))
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if len(plan.Terraform) != 3 {
		t.Fatalf("expected a contract per Terraform node, got %d", len(plan.Terraform))
	}
	for _, contract := range plan.Terraform {
		if !strings.HasPrefix(contract.Fingerprint, "sha256:") {
			t.Fatalf("contract %s has no fingerprint", contract.Descriptor)
		}
		if len(contract.Inputs) == 0 {
			t.Fatalf("contract %s has no inputs", contract.Descriptor)
		}
	}
}

func TestPlan_RejectsDriverVariableTheModuleDoesNotDeclare(t *testing.T) {
	req := testRequest(t, application.ProfileAWSEKS, "backend")
	defs := append([]resource.Definition(nil), req.Catalog.Definitions...)
	for i, def := range defs {
		if def.Key != "vpc-aws" {
			continue
		}
		values := def.DriverValues()
		variables := map[string]any{}
		for k, v := range def.Variables() {
			variables[k] = v
		}
		variables["not_a_module_variable"] = "x"
		defs[i].DriverInputs = map[string]any{"values": map[string]any{
			"source": values["source"], "variables": variables,
		}}
	}
	req.Catalog.Definitions = defs
	if _, err := NewService().Plan(req); err == nil {
		t.Fatal("expected an unknown Terraform input error")
	}
}

func TestPlan_DeltaKeepsInvariant(t *testing.T) {
	plan, err := NewService().Plan(testRequest(t, application.ProfileInternalK8s, "backend"))
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if err := VerifyDelta(plan.BaseSet, plan.Delta, plan.CandidateSet); err != nil {
		t.Fatal(err)
	}
}

func TestPlan_SharedResourceKeepsOneDescriptorForBackendAndWorker(t *testing.T) {
	svc := NewService()
	first, err := svc.Plan(testRequest(t, application.ProfileInternalK8s, "backend"))
	if err != nil {
		t.Fatalf("plan backend: %v", err)
	}
	second := testRequest(t, application.ProfileInternalK8s, "worker")
	second.BaseSet = first.CandidateSet
	plan, err := svc.Plan(second)
	if err != nil {
		t.Fatalf("plan worker: %v", err)
	}
	count := 0
	for _, n := range plan.Graph.Nodes {
		if n.ResourceType == "postgres" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("backend and worker must share one postgres node, found %d", count)
	}
	if _, ok := plan.CandidateSet.Shared["acceptance-db"]; !ok {
		t.Fatalf("the shared entry was dropped: %v", plan.CandidateSet.Shared)
	}
	consumers := 0
	for _, e := range plan.Graph.Edges {
		if e.Provider == "postgres.default#shared.acceptance-db" && strings.HasPrefix(e.Consumer, "workload.") {
			consumers++
		}
	}
	if consumers < 2 {
		t.Fatalf("both workloads must consume the shared database, got %d edges", consumers)
	}
}

func TestPlan_RejectsPlanningFromStaleBeforeState(t *testing.T) {
	svc := NewService()
	first, err := svc.Plan(testRequest(t, application.ProfileInternalK8s, "backend"))
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	second := testRequest(t, application.ProfileInternalK8s, "backend")
	second.BaseSet = first.CandidateSet
	if _, err := svc.Plan(second); err == nil {
		t.Fatal("expected an error when redeploying without a before Score")
	}
}

// TestPlan_CoProvisionAddsDependentAndMatchesDependents exercises the
// provision rules of UC-06 BR-08 on a definition written for the test.
func TestPlan_CoProvisionAddsDependentAndMatchesDependents(t *testing.T) {
	req := testRequest(t, application.ProfileInternalK8s, "backend")
	catalog := req.Catalog
	catalog.Types["logging"] = resource.Type{
		Key:     "logging",
		Outputs: []resource.OutputField{{Name: "endpoint", Type: "string"}},
	}
	defs := append([]resource.Definition(nil), catalog.Definitions...)
	for i, def := range defs {
		if def.Key == "postgres-internal-statefulset" {
			defs[i].Provision = map[string]resource.ProvisionRule{
				"logging.default#shared.acceptance-logging": {IsDependent: true, MatchDependents: true},
			}
		}
	}
	defs = append(defs, resource.Definition{
		Key:             "logging-default",
		ResourceTypeKey: "logging",
		DriverType:      resource.DriverEcho,
		Criteria:        []resource.Criterion{{}},
	})
	catalog.Definitions = defs
	req.Catalog = catalog

	plan, err := NewService().Plan(req)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	child := "logging.default#shared.acceptance-logging"
	parent := "postgres.default#shared.acceptance-db"
	if _, ok := plan.Graph.Node(child); !ok {
		t.Fatalf("the co-provisioned resource is missing: %v", plan.Graph.Nodes)
	}

	var dependent, matched bool
	for _, e := range plan.Graph.Edges {
		if e.Consumer == child && e.Provider == parent && e.Reason == ReasonDependent {
			dependent = true
		}
		if e.Consumer == "workload.default#modules.backend" && e.Provider == child && e.Reason == ReasonMatchDependent {
			matched = true
		}
	}
	if !dependent {
		t.Fatal("is_dependent must add a child -> parent edge")
	}
	if !matched {
		t.Fatal("match_dependents must link every consumer of the parent to the child")
	}

	// The child must be scheduled after its parent.
	parentBatch, childBatch := -1, -1
	for i, batch := range plan.Batches {
		for _, descriptor := range batch {
			if descriptor == parent {
				parentBatch = i
			}
			if descriptor == child {
				childBatch = i
			}
		}
	}
	if parentBatch < 0 || childBatch <= parentBatch {
		t.Fatalf("expected the co-provisioned child after its parent, got batches %v", plan.Batches)
	}
}

func TestPlan_RejectsBeforeScoreWithStaleSharedResource(t *testing.T) {
	svc := NewService()
	first, err := svc.Plan(testRequest(t, application.ProfileInternalK8s, "backend"))
	if err != nil {
		t.Fatalf("plan backend: %v", err)
	}

	// Somebody changed the shared database since the caller read it.
	base := first.CandidateSet
	entry := base.Shared["acceptance-db"]
	entry.Params = map[string]any{"database": "other", "username": "app"}
	base.Shared["acceptance-db"] = entry

	update := testRequest(t, application.ProfileInternalK8s, "backend")
	update.BaseSet = base
	update.Before = update.After
	if _, err := svc.Plan(update); err == nil {
		t.Fatal("expected a before-state mismatch on the shared resource")
	}
}

func TestPlan_RejectsSharedConflict(t *testing.T) {
	svc := NewService()
	first, err := svc.Plan(testRequest(t, application.ProfileInternalK8s, "backend"))
	if err != nil {
		t.Fatalf("plan backend: %v", err)
	}

	// A second workload claims the same shared id with different params.
	second := testRequest(t, application.ProfileInternalK8s, "worker")
	second.BaseSet = first.CandidateSet
	resources := second.After.Resources["db"]
	resources.Params = map[string]any{"database": "different", "username": "app"}
	second.After.Resources["db"] = resources

	_, err = svc.Plan(second)
	if err == nil {
		t.Fatal("expected a shared resource conflict")
	}
	if !strings.Contains(err.Error(), "different content") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPlan_DropsSharedResourceWhenTheLastWorkloadStopsDeclaringIt(t *testing.T) {
	svc := NewService()
	first, err := svc.Plan(testRequest(t, application.ProfileInternalK8s, "backend"))
	if err != nil {
		t.Fatalf("plan backend: %v", err)
	}

	// The same workload redeploys without the database.
	update := testRequest(t, application.ProfileInternalK8s, "backend")
	update.BaseSet = first.CandidateSet
	update.Before = update.After

	stripped := *update.After
	stripped.Resources = map[string]score.ResourceSpec{}
	containers := map[string]environment.Container{}
	for name, container := range stripped.Containers {
		container.Variables = map[string]string{"PORT": "8080"}
		containers[name] = container
	}
	stripped.Containers = containers
	update.After = &stripped

	plan, err := svc.Plan(update)
	if err != nil {
		t.Fatalf("plan update: %v", err)
	}
	if _, ok := plan.CandidateSet.Shared["acceptance-db"]; ok {
		t.Fatalf("the shared entry must go once nothing declares or references it: %v", plan.CandidateSet.Shared)
	}
	for _, n := range plan.Graph.Nodes {
		if n.ResourceType == "postgres" {
			t.Fatalf("the postgres node must leave the desired graph: %v", n)
		}
	}
}

func TestPlan_KeepsSharedResourceWhileAnotherWorkloadReferencesIt(t *testing.T) {
	svc := NewService()
	first, err := svc.Plan(testRequest(t, application.ProfileInternalK8s, "backend"))
	if err != nil {
		t.Fatalf("plan backend: %v", err)
	}
	second := testRequest(t, application.ProfileInternalK8s, "worker")
	second.BaseSet = first.CandidateSet
	withWorker, err := svc.Plan(second)
	if err != nil {
		t.Fatalf("plan worker: %v", err)
	}

	// The backend drops the database; the worker still uses it (UC-07 BR-03).
	update := testRequest(t, application.ProfileInternalK8s, "backend")
	update.BaseSet = withWorker.CandidateSet
	update.Before = update.After

	stripped := *update.After
	stripped.Resources = map[string]score.ResourceSpec{}
	containers := map[string]environment.Container{}
	for name, container := range stripped.Containers {
		container.Variables = map[string]string{"PORT": "8080"}
		containers[name] = container
	}
	stripped.Containers = containers
	update.After = &stripped

	plan, err := svc.Plan(update)
	if err != nil {
		t.Fatalf("plan update: %v", err)
	}
	if _, ok := plan.CandidateSet.Shared["acceptance-db"]; !ok {
		t.Fatalf("the shared entry must survive while the worker references it: %v", plan.CandidateSet.Shared)
	}
}
