package planning_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/adapters/terraform/registry"
	"orchestrator/internal/adapters/terraform/registry/registrytest"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/planning"
	"orchestrator/internal/planning/score"
	"orchestrator/internal/seed"
	"orchestrator/internal/seed/seedtest"
)

// ADR-017 / V05-V06: the Console-authored Definition catalog (ONE JSON file shared with the headed scenarios) plans
// over the REAL verified registry bundles with no Environment profile, Connection or account: every AWS Definition
// brings its own Driver Account and literal region, every Kubernetes module binds the cluster RESOURCE, and nothing
// is inserted because an Environment exists. No cloud is touched.

type definitionCatalog struct {
	Types       []catalogType                  `json:"types"`
	Definitions map[string][]catalogDefinition `json:"definitions"`
	Classes     map[string]map[string]string   `json:"classes"`
}

type catalogType struct {
	Key     string  `json:"key"`
	Inputs  [][]any `json:"inputs"`
	Outputs [][]any `json:"outputs"`
}

type catalogDefinition struct {
	ID              string      `json:"id"`
	Type            string      `json:"type"`
	Module          string      `json:"module"`
	Provider        string      `json:"provider"`
	ClusterClass    string      `json:"clusterClass"`
	Klass           string      `json:"klass"`
	Input           bool        `json:"input"`
	PresetName      string      `json:"presetName"`
	PresetNamespace bool        `json:"presetNamespace"`
	Overrides       [][2]string `json:"overrides"`
}

func loadDefinitionCatalog(t *testing.T) definitionCatalog {
	t.Helper()
	raw, err := os.ReadFile("../../test/integration/v2run/testdata/definition-catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	var file definitionCatalog
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	return file
}

func (c definitionCatalog) types() map[string]resource.Type {
	out := map[string]resource.Type{}
	for _, typ := range c.Types {
		rt := resource.Type{Key: typ.Key}
		for _, in := range typ.Inputs {
			rt.Inputs = append(rt.Inputs, resource.InputField{Name: in[0].(string), Type: in[1].(string), Required: len(in) > 2 && in[2].(bool)})
		}
		for _, o := range typ.Outputs {
			secret := len(o) > 3 && o[3].(bool)
			rt.Outputs = append(rt.Outputs, resource.OutputField{Name: o[0].(string), Type: o[1].(string), Required: o[2].(bool), Secret: secret})
		}
		out[typ.Key] = rt
	}
	return out
}

// moduleOutputs are the outputs of the REAL verified bundles (executor-added ones included).
var catalogModuleOutputs = map[string][]string{
	"vpc": {"id", "cidr", "subnetIds", "privateSubnetIds"}, "eks": {"name", "endpoint", "kubeContext", "kubeconfig"}, "k8s-namespace": {"name"},
	"postgres-k8s": {"host", "port", "database", "username", "password"}, "redis-k8s": {"host", "port", "username", "password", "tls"},
	"aurora": {"host", "port", "database", "username", "password"}, "elasticache": {"host", "port", "username", "password", "tls"},
}

// authoredDefinition converts one form entry into the Definition the Console stores.
func authoredDefinition(t *testing.T, e catalogDefinition, refs map[string]tfmodule.SourceRef, types map[string]resource.Type, account, region string) resource.Definition {
	t.Helper()
	nsOut := "${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}"
	variables := map[string]any{}
	for _, o := range e.Overrides {
		text := strings.NewReplacer("@EGRESS@", "203.0.113.7/32", "@NODE_TYPE@", "t4g.medium", "@REGION@", region).Replace(o[1])
		var v any
		if err := json.Unmarshal([]byte(text), &v); err != nil {
			t.Fatalf("override %s of %s is not JSON: %v", o[0], e.ID, err)
		}
		variables[o[0]] = v
	}
	if e.PresetName != "" {
		variables[e.PresetName] = "${context.env.namespace}"
	}
	if e.PresetNamespace {
		variables["namespace"] = nsOut
	}
	inputs := map[string]any{}
	if e.Input {
		for _, in := range types[e.Type].Inputs {
			inputs[in.Name] = map[string]any{"param": in.Name}
		}
	}
	have := map[string]bool{}
	for _, name := range catalogModuleOutputs[e.Module] {
		have[name] = true
	}
	outputs := map[string]any{}
	for _, o := range types[e.Type].Outputs {
		if have[o.Name] && !(e.Type == "k8s-cluster" && o.Name == "cluster") {
			outputs[o.Name] = o.Name
		}
	}
	values := map[string]any{"source": refs[e.Module].Map(), "variables": variables, "input_mapping": inputs, "output_mapping": outputs}
	def := resource.Definition{Key: e.ID, Name: e.ID, Version: 1, ResourceTypeKey: e.Type, DriverType: resource.DriverTerraform, Criteria: []resource.Criterion{{Class: e.Klass}}}
	if e.Provider == "kubernetes" {
		values["provider_bindings"] = map[string]any{"kubernetes": map[string]any{"target": tfmodule.TargetResource, "resource_ref": "k8s-cluster." + e.ClusterClass + "#environments.@app.@env"}}
	} else {
		variables["region"] = region
		values["provider_bindings"] = map[string]any{"aws": map[string]any{"target": tfmodule.TargetAccount}}
		def.DriverAccount = account
	}
	def.DriverInputs = map[string]any{"values": values}
	return def
}

const (
	eksClusterRef = "${resources['k8s-cluster.eks#environments.@app.@env'].outputs.cluster}"
	eksNSRef      = "${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}"
)

type catalogPlan struct {
	catalog definitionCatalog
	req     planning.Request
}

// catalogRequest plans the backend workload of the acceptance Scores for one scenario with the authored catalog.
func catalogRequest(t *testing.T, scenario string, mutate func(defs []resource.Definition, accounts map[string]application.Connection) []resource.Definition) catalogPlan {
	t.Helper()
	return catalogRequestWithScore(t, scenario, mutate, nil)
}

// catalogRequestWithScore is catalogRequest with a last chance to reshape the backend Score document (extra resources).
func catalogRequestWithScore(t *testing.T, scenario string, mutate func(defs []resource.Definition, accounts map[string]application.Connection) []resource.Definition, mutateScore func(doc map[string]any)) catalogPlan {
	t.Helper()
	cat := loadDefinitionCatalog(t)
	refs, err := registry.PackagedRefs()
	if err != nil {
		t.Fatal(err)
	}
	types := cat.types()
	var defs []resource.Definition
	for _, entry := range append(append([]catalogDefinition{}, cat.Definitions["common"]...), cat.Definitions[scenario]...) {
		defs = append(defs, authoredDefinition(t, entry, refs, types, "aws-a", "us-east-1"))
	}
	defs = append(defs, workloadDefinition("workload-render", map[string]any{"cluster": eksClusterRef, "namespace": eksNSRef}, resource.Criterion{}))
	accounts := map[string]application.Connection{
		"aws-a": {Key: "aws-a", Kind: application.ConnectionAWS, OrganizationKey: "acme", Status: application.ConnectionReady, Version: 5},
		"aws-b": {Key: "aws-b", Kind: application.ConnectionAWS, OrganizationKey: "acme", Status: application.ConnectionReady, Version: 9},
		"k8s-a": {Key: "k8s-a", Kind: application.ConnectionKubernetes, OrganizationKey: "acme", Status: application.ConnectionReady, Version: 3},
	}
	if mutate != nil {
		defs = mutate(defs, accounts)
	}
	o := seedtest.Defaults()
	doc := seed.AcceptanceScoresRedis(o, "acceptance-cache", cat.Classes[scenario]["redis"], "shop", "dev", "run-1")["backend"]
	doc["resources"].(map[string]any)["db"].(map[string]any)["class"] = cat.Classes[scenario]["postgres"]
	if mutateScore != nil {
		mutateScore(doc)
	}
	parsed, err := score.FromMap(doc)
	if err != nil {
		t.Fatal(err)
	}
	req := placedRequest(t, doc, defs...)
	req.After = parsed
	req.WorkloadID = "backend"
	req.Catalog.Types = types
	req.Catalog.Accounts = accounts
	req.Terraform = registrytest.Inspector(t, store.New())
	return catalogPlan{catalog: cat, req: req}
}

const (
	eksNode      = "k8s-cluster.eks#environments.shop.dev"
	vpcNode      = "vpc.default#environments.shop.dev"
	nsNode       = "k8s-namespace.default#environments.shop.dev"
	pgEKSNode    = "postgres.in-cluster#shared.acceptance-db"
	redisEKSNode = "redis.in-cluster#shared.acceptance-cache"
	auroraNode   = "postgres.cloud-managed#shared.acceptance-db"
	cacheNode    = "redis.cloud-managed#shared.acceptance-cache"
	backendNode  = "workload.default#modules.backend"
)

func pinOf(t *testing.T, plan *planning.Plan, node, kind string) tfmodule.ProviderPin {
	t.Helper()
	for _, contract := range plan.Terraform {
		if contract.Descriptor != node {
			continue
		}
		for _, pin := range contract.ProviderBindings {
			if pin.Kind == kind {
				return pin
			}
		}
	}
	t.Fatalf("no %s provider pin for %s in %+v", kind, node, plan.Terraform)
	return tfmodule.ProviderPin{}
}

func hasNode(plan *planning.Plan, descriptor string) bool {
	_, ok := plan.Graph.Node(descriptor)
	return ok
}

func TestDefinitionCatalog_V05PlansTheEKSChainFromDefinitionsAlone(t *testing.T) {
	cp := catalogRequest(t, "V05", nil)
	plan, err := placedPlan(t, cp.req)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Target != cp.req.Env.Binding() {
		t.Fatalf("an Environment target leaked into the plan: %+v", plan.Target)
	}
	for _, node := range []string{vpcNode, eksNode, nsNode, pgEKSNode, redisEKSNode, backendNode} {
		if !hasNode(plan, node) {
			t.Fatalf("the plan lacks %s: %+v", node, plan.Graph.Nodes)
		}
	}
	// Ordering: VPC -> EKS (reference) -> namespace/PostgreSQL/Redis (provider binding) -> workload.
	for _, want := range [][3]string{
		{eksNode, vpcNode, planning.ReasonReference}, {nsNode, eksNode, planning.ReasonProviderBinding}, {pgEKSNode, eksNode, planning.ReasonProviderBinding},
		{redisEKSNode, eksNode, planning.ReasonProviderBinding}, {pgEKSNode, nsNode, planning.ReasonReference}, {backendNode, eksNode, planning.ReasonReference}, {backendNode, nsNode, planning.ReasonReference},
	} {
		if !hasEdge(plan, want[0], want[1], want[2]) {
			t.Errorf("missing %s edge %s -> %s", want[2], want[0], want[1])
		}
	}
	if !(batchOf(plan, vpcNode) < batchOf(plan, eksNode) && batchOf(plan, eksNode) < batchOf(plan, nsNode) && batchOf(plan, nsNode) < batchOf(plan, pgEKSNode)) {
		t.Fatalf("provider-first batches = %v", plan.Batches)
	}
	// Each AWS node pins the Definition's own account, version and literal region.
	for _, node := range []string{vpcNode, eksNode} {
		pin := pinOf(t, plan, node, tfmodule.BindingAWS)
		if pin.ConnectionKey != "aws-a" || pin.AccountVersion != 5 || pin.Region != "us-east-1" || pin.ConnectionKind != "AWS" {
			t.Fatalf("%s AWS pin = %+v", node, pin)
		}
	}
	// The Kubernetes consumers carry NO Kubernetes Connection: they pin the producing EKS identity (original account, version, region).
	for _, node := range []string{nsNode, pgEKSNode, redisEKSNode} {
		pin := pinOf(t, plan, node, tfmodule.BindingKubernetes)
		if pin.ConnectionKey != "" || pin.ClusterProvider != planning.ClusterEKS || pin.ClusterAccount != "aws-a" || pin.AccountVersion != 5 || pin.Region != "us-east-1" || pin.Provider != eksNode {
			t.Fatalf("%s Kubernetes pin = %+v", node, pin)
		}
	}
	// Every consumer is placed on the EKS cluster the plan itself creates; endpoint and Ready state are unknown here.
	for _, node := range []string{nsNode, pgEKSNode, redisEKSNode, backendNode} {
		placement, ok := plan.Placements[node]
		if !ok || placement.Cluster != eksNode || placement.Provider != planning.ClusterEKS || placement.Region != "us-east-1" || placement.Account != "aws-a" || placement.AccountVersion != 5 || placement.ClusterDefinition != "cluster-eks" {
			t.Fatalf("%s placement = %+v", node, placement)
		}
	}
	if got := plan.Placements[backendNode]; got.Namespace != nsNode || got.Source != planning.PlacementWorkloadReference {
		t.Fatalf("workload placement = %+v", got)
	}
	// Preview is pure and deterministic: no provider call, same hash.
	again, err := placedPlan(t, cp.req)
	if err != nil || again.PlanHash != plan.PlanHash {
		t.Fatalf("plan must be deterministic: %v", err)
	}
}

func TestDefinitionCatalog_ClusterTargetOutputIsPlatformOwnedNotModuleMapped(t *testing.T) {
	// A Definition cannot map a module output onto the typed target.
	cp := catalogRequest(t, "V05", func(defs []resource.Definition, _ map[string]application.Connection) []resource.Definition {
		for i := range defs {
			if defs[i].Key == "cluster-eks" {
				defs[i].DriverInputs["values"].(map[string]any)["output_mapping"].(map[string]any)["endpoint"] = "cluster"
			}
		}
		return defs
	})
	if _, err := placedPlan(t, cp.req); err == nil || !strings.Contains(err.Error(), "platform-owned") {
		t.Fatalf("a module cannot author its own target: %v", err)
	}
}

func contractInput(t *testing.T, plan *planning.Plan, definition, input string) any {
	t.Helper()
	for _, c := range plan.Terraform {
		if c.DefinitionKey != definition {
			continue
		}
		for _, in := range c.Inputs {
			if in.Name == input {
				return in.Value
			}
		}
	}
	t.Fatalf("contract %s has no input %s", definition, input)
	return nil
}

func TestDefinitionCatalog_EveryTerraformNodeIsInspectedAgainstItsVerifiedBundle(t *testing.T) {
	plan, err := placedPlan(t, catalogRequest(t, "V05", nil).req)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Terraform) != 5 {
		t.Fatalf("a contract per Terraform node (vpc, eks, namespace, postgres, redis), got %d", len(plan.Terraform))
	}
	for _, contract := range plan.Terraform {
		if !strings.HasPrefix(contract.Fingerprint, "sha256:") || contract.BundleID == "" || contract.Source == nil {
			t.Fatalf("contract %s is not pinned to a verified bundle: %+v", contract.Descriptor, contract)
		}
		if len(contract.Inputs) == 0 {
			t.Fatalf("contract %s has no inputs", contract.Descriptor)
		}
	}
}

func TestDefinitionCatalog_EKSWithoutPublicAccessAllowlistFailsBeforeEffects(t *testing.T) {
	cp := catalogRequest(t, "V05", func(defs []resource.Definition, _ map[string]application.Connection) []resource.Definition {
		for i := range defs {
			if defs[i].Key == "cluster-eks" {
				delete(defs[i].DriverInputs["values"].(map[string]any)["variables"].(map[string]any), "public_access_cidrs")
			}
		}
		return defs
	})
	if _, err := placedPlan(t, cp.req); err == nil || !strings.Contains(err.Error(), `required Terraform variable "public_access_cidrs"`) {
		t.Fatalf("plan without allowlist: %v", err)
	}
}

func TestDefinitionCatalog_AWSRegionMustBeALiteralAwsRegion(t *testing.T) {
	for name, region := range map[string]any{"missing": nil, "placeholder": "${context.env.region}", "bad shape": "mars-1", "number": 1} {
		cp := catalogRequest(t, "V05", func(defs []resource.Definition, _ map[string]application.Connection) []resource.Definition {
			for i := range defs {
				if defs[i].Key != "vpc-aws" {
					continue
				}
				variables := defs[i].DriverInputs["values"].(map[string]any)["variables"].(map[string]any)
				if region == nil {
					delete(variables, "region")
				} else {
					variables["region"] = region
				}
			}
			return defs
		})
		if _, err := placedPlan(t, cp.req); err == nil {
			t.Fatalf("%s region was planned", name)
		}
	}
}

// A Definition edit or an account change makes an older plan stale: the plan hash covers the Definition version and
// content and the account version.
func TestDefinitionCatalog_HashFollowsDefinitionVersionAndAccountVersion(t *testing.T) {
	base, err := placedPlan(t, catalogRequest(t, "V05", nil).req)
	if err != nil {
		t.Fatal(err)
	}
	edited := catalogRequest(t, "V05", func(defs []resource.Definition, _ map[string]application.Connection) []resource.Definition {
		for i := range defs {
			if defs[i].Key == "vpc-aws" {
				defs[i].Name, defs[i].Version = "VPC mới", 2
			}
		}
		return defs
	})
	if p, err := placedPlan(t, edited.req); err != nil || p.PlanHash == base.PlanHash {
		t.Fatalf("definition version did not change the hash: %v", err)
	}
	bumped := catalogRequest(t, "V05", func(defs []resource.Definition, accounts map[string]application.Connection) []resource.Definition {
		a := accounts["aws-a"]
		a.Version = 6
		accounts["aws-a"] = a
		return defs
	})
	if p, err := placedPlan(t, bumped.req); err != nil || p.PlanHash == base.PlanHash {
		t.Fatalf("account version did not change the hash: %v", err)
	}
}

// Physical names the Definitions derive from the Environment identity stay inside the provider bounds (VPC/EKS 49,
// Aurora and IAM suffixes 63) whatever the run identifier is, and the run never changes them.
func TestDefinitionCatalog_ProviderVisibleNamesAreBoundedAndIndependentOfTheRun(t *testing.T) {
	names := map[string]string{}
	for _, run := range []string{"run-1", "envconn-kind-20261007121743-27878-with-a-very-long-suffix"} {
		cp := catalogRequest(t, "V05", nil)
		cp.req.RunID = run
		plan, err := placedPlan(t, cp.req)
		if err != nil {
			t.Fatal(err)
		}
		for _, definition := range []string{"vpc-aws", "cluster-eks"} {
			got, _ := contractInput(t, plan, definition, "name").(string)
			if len(got) == 0 || len(got) > 49 || len(got+"-cluster") > 63 || len(got+"-writer") > 63 || got[0] < 'a' || got[0] > 'z' {
				t.Fatalf("%s name = %q is outside the provider bounds", definition, got)
			}
			if prev, ok := names[definition]; ok && prev != got {
				t.Fatalf("the run changed the %s physical name: %q != %q", definition, prev, got)
			}
			names[definition] = got
		}
	}
}
