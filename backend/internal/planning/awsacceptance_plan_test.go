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
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/planning"
	"orchestrator/internal/planning/score"
	"orchestrator/internal/seed"
	"orchestrator/internal/seed/seedtest"
)

// The Console-authored AWS acceptance catalog lives in ONE JSON file shared with
// the headed scenario (backend/test/integration/v2run/testdata/aws-catalog.json).
// These tests plan exactly those Definitions over the REAL verified registry
// bundles (vpc, eks, k8s-namespace, aurora, elasticache, redis-k8s) so the
// scenario never types a Definition the planner would refuse. No cloud is touched.

type awsCatalogFile struct {
	Types []struct {
		Key     string  `json:"key"`
		Inputs  [][]any `json:"inputs"`
		Outputs [][]any `json:"outputs"`
	} `json:"types"`
	Definitions map[string][]awsDefinitionEntry `json:"definitions"`
}

type awsDefinitionEntry struct {
	ID              string      `json:"id"`
	Type            string      `json:"type"`
	Module          string      `json:"module"`
	Klass           string      `json:"klass"`
	Input           bool        `json:"input"`
	PresetName      string      `json:"presetName"`
	PresetNamespace bool        `json:"presetNamespace"`
	Overrides       [][2]string `json:"overrides"`
}

func loadAWSCatalog(t *testing.T) awsCatalogFile {
	t.Helper()
	raw, err := os.ReadFile("../../test/integration/v2run/testdata/aws-catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	var file awsCatalogFile
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	return file
}

func awsTypes(t *testing.T, file awsCatalogFile) map[string]resource.Type {
	out := map[string]resource.Type{}
	for _, typ := range file.Types {
		rt := resource.Type{Key: typ.Key}
		for _, in := range typ.Inputs {
			rt.Inputs = append(rt.Inputs, resource.InputField{Name: in[0].(string), Type: in[1].(string), Required: in[2].(bool)})
		}
		for _, o := range typ.Outputs {
			rt.Outputs = append(rt.Outputs, resource.OutputField{Name: o[0].(string), Type: o[1].(string), Required: o[2].(bool), Secret: o[3].(bool)})
		}
		out[typ.Key] = rt
	}
	return out
}

// authored converts one form entry into the Definition the Console stores: only
// opted-in overrides become variables, matching outputs/inputs are mapped by the
// same name, AWS modules bind aws to the Environment, Kubernetes modules bind
// the Environment cluster (the EKS node on aws-eks).
func authored(t *testing.T, e awsDefinitionEntry, refs map[string]tfmodule.SourceRef, types map[string]resource.Type, moduleOutputs map[string][]string, egress, nodeType string) resource.Definition {
	t.Helper()
	nsOut := "${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}"
	variables := map[string]any{}
	for _, o := range e.Overrides {
		text := strings.NewReplacer("@EGRESS@", egress, "@NODE_TYPE@", nodeType).Replace(o[1])
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
	outputs := map[string]any{}
	have := map[string]bool{}
	for _, name := range moduleOutputs[e.Module] {
		have[name] = true
	}
	for _, o := range types[e.Type].Outputs {
		if have[o.Name] {
			outputs[o.Name] = o.Name
		}
	}
	providerKind := "aws"
	if e.Module == "k8s-namespace" || e.Module == "redis-k8s" || e.Module == "postgres-k8s" {
		providerKind = "kubernetes"
	}
	def := resource.Definition{
		Key: e.ID, ResourceTypeKey: e.Type, DriverType: resource.DriverTerraform, ExecutionProfile: "aws-eks",
		DriverInputs: map[string]any{"values": map[string]any{
			"source": refs[e.Module].Map(), "variables": variables, "input_mapping": inputs, "output_mapping": outputs,
			"provider_bindings": map[string]any{providerKind: map[string]any{"target": tfmodule.TargetEnvironment}},
		}},
		Criteria: []resource.Criterion{{Class: e.Klass}},
	}
	return def
}

func awsAcceptanceRequest(t *testing.T, scenario string) planning.Request {
	t.Helper()
	file := loadAWSCatalog(t)
	refs, err := registry.PackagedRefs()
	if err != nil {
		t.Fatal(err)
	}
	// The module output names come from the real verified bundles (executor outputs such as kubeContext included).
	moduleOutputs := map[string][]string{
		"vpc": {"id", "cidr", "subnetIds", "privateSubnetIds"}, "eks": {"name", "endpoint", "kubeContext", "kubeconfig"}, "k8s-namespace": {"name"},
		"aurora": {"host", "port", "database", "username", "password"}, "elasticache": {"host", "port", "username", "password", "tls"}, "redis-k8s": {"host", "port", "username", "password", "tls"},
	}
	o := seedtest.Defaults()
	o.RunID, o.Region = "run-test", "us-east-1"
	o.SharedDatabaseID = "acceptance-db"
	types := awsTypes(t, file)
	catalog := planning.Catalog{Types: types}
	for _, e := range append(append([]awsDefinitionEntry{}, file.Definitions["common"]...), file.Definitions[scenario]...) {
		catalog.Definitions = append(catalog.Definitions, authored(t, e, refs, types, moduleOutputs, "203.0.113.9/32", "t4g.medium"))
	}
	docs := seed.AcceptanceScoresRedis(o, "acceptance-cache", "default", o.CloudApplicationKey, "staging", "run-test")
	backend := docs["backend"]
	backend["resources"].(map[string]any)["db"].(map[string]any)["class"] = "cloud-managed"
	cacheClass := map[string]string{"A02": "cloud-managed", "A03": "in-cluster"}[scenario]
	backend["resources"].(map[string]any)["cache"].(map[string]any)["class"] = cacheClass
	doc, err := score.FromMap(backend)
	if err != nil {
		t.Fatal(err)
	}
	appKey, ns, conn := o.CloudApplicationKey, o.CloudNamespaceIdentity, o.CloudConnectionKey
	return planning.Request{
		OrganizationKey: o.OrganizationKey,
		App:             application.Application{Key: appKey, OrganizationKey: o.OrganizationKey, Name: appKey, Version: 1},
		Env: environment.Environment{Key: o.EnvironmentKey, ApplicationKey: appKey, Name: o.EnvironmentName, Type: o.EnvironmentType, NamespaceIdentity: ns, Version: 1,
			ConnectionKey: conn, Profile: application.ProfileAWSEKS, RuntimeStatus: application.RuntimeReady, InfrastructureScope: environment.ScopeEnvironment, Region: o.Region},
		Connection: application.Connection{Key: conn, Kind: application.ConnectionAWS, OrganizationKey: o.OrganizationKey, Status: application.ConnectionReady,
			Config: map[string]any{"cluster": o.ClusterName, "kubeContext": o.KubeContext}},
		BaseSet: environment.NewDocument(), After: doc, WorkloadID: "backend", RunID: o.RunID, Catalog: catalog,
		Terraform: registrytest.Inspector(t, store.New()),
	}
}

func TestConsoleAuthoredAWSCatalogPlansA02AndA03OverRealBundles(t *testing.T) {
	catalog := loadAWSCatalog(t)
	a03Redis := catalog.Definitions["A03"]
	var storageClass string
	for _, definition := range a03Redis {
		if definition.ID == "redis-eks-in-cluster" {
			for _, override := range definition.Overrides {
				if override[0] == "storage_class" && json.Unmarshal([]byte(override[1]), &storageClass) == nil {
					break
				}
			}
		}
	}
	if storageClass != "gp2" {
		t.Fatalf("A03 Redis must explicitly select the EKS-provided gp2 StorageClass; got %q", storageClass)
	}
	for scenario, want := range map[string]map[string]int{
		"A02": {"vpc": 1, "k8s-cluster": 1, "k8s-namespace": 1, "postgres": 1, "redis": 1},
		"A03": {"vpc": 1, "k8s-cluster": 1, "k8s-namespace": 1, "postgres": 1, "redis": 1},
	} {
		plan, err := planning.NewService().Plan(awsAcceptanceRequest(t, scenario))
		if err != nil {
			t.Fatalf("%s: %v", scenario, err)
		}
		counts := map[string]int{}
		modules := map[string]int{}
		for _, n := range plan.Graph.Nodes {
			counts[strings.SplitN(n.Descriptor, ".", 2)[0]]++
		}
		for _, c := range plan.Terraform {
			modules[c.Module]++
		}
		for typ, n := range want {
			if counts[typ] != n {
				t.Fatalf("%s: node counts %v", scenario, counts)
			}
		}
		wantModules := map[string]int{"vpc": 1, "eks": 1, "k8s-namespace": 1, "aurora": 1}
		if scenario == "A02" {
			wantModules["elasticache"] = 1
		} else {
			wantModules["redis-k8s"] = 1
		}
		for m, n := range wantModules {
			if modules[m] != n {
				t.Fatalf("%s: terraform instances %v", scenario, modules)
			}
		}
		for m := range modules {
			if _, ok := wantModules[m]; !ok {
				t.Fatalf("%s: unexpected module %s", scenario, m)
			}
		}
		// Provider-first order: the VPC before the EKS node before the namespace; Aurora and the cache after the VPC.
		var vpc, eks, ns string
		for _, n := range plan.Graph.Nodes {
			switch {
			case strings.HasPrefix(n.Descriptor, "vpc."):
				vpc = n.Descriptor
			case strings.HasPrefix(n.Descriptor, "k8s-cluster."):
				eks = n.Descriptor
			case strings.HasPrefix(n.Descriptor, "k8s-namespace."):
				ns = n.Descriptor
			}
		}
		if !(batchOf(plan, vpc) < batchOf(plan, eks) && batchOf(plan, eks) < batchOf(plan, ns)) {
			t.Fatalf("%s: VPC -> EKS -> namespace order is broken: %d %d %d", scenario, batchOf(plan, vpc), batchOf(plan, eks), batchOf(plan, ns))
		}
		for _, n := range plan.Graph.Nodes {
			if strings.HasPrefix(n.Descriptor, "postgres.") && batchOf(plan, n.Descriptor) <= batchOf(plan, vpc) {
				t.Fatalf("%s: Aurora is not planned after the VPC", scenario)
			}
		}
		for _, c := range plan.Terraform {
			if c.Source == nil || c.BundleID == "" || c.ProviderLockDigest == "" {
				t.Fatalf("%s: %s is not pinned to a verified bundle", scenario, c.Module)
			}
		}
	}
}

// The test is not vacuous: an Aurora Definition without its VPC wiring, and an
// EKS Definition without the explicit /32 allowlist, are refused by the planner.
func TestConsoleAuthoredAWSCatalogRefusesIncompleteDefinitions(t *testing.T) {
	for name, drop := range map[string]struct{ def, variable string }{
		"aurora without the VPC":    {"pg-aurora", "vpc_id"},
		"eks without the allowlist": {"cluster-aws-eks", "public_access_cidrs"},
	} {
		req := awsAcceptanceRequest(t, "A02")
		for i := range req.Catalog.Definitions {
			if req.Catalog.Definitions[i].Key == drop.def {
				delete(req.Catalog.Definitions[i].DriverInputs["values"].(map[string]any)["variables"].(map[string]any), drop.variable)
			}
		}
		if _, err := planning.NewService().Plan(req); err == nil {
			t.Fatalf("%s must be refused by the planner", name)
		}
	}
}
