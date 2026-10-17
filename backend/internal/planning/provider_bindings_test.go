package planning

import (
	"strings"
	"testing"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
)

// kubernetesModuleDefinition is a Terraform postgres Definition whose module
// takes a Kubernetes provider binding and whose variables never mention the
// cluster, so only the binding can create the dependency.
func kubernetesModuleDefinition(binding map[string]any) resource.Definition {
	ref := tfmodule.SourceRef{SourceID: "platform", ModuleID: "pgk8s", Revision: "v1", Subdirectory: "pgk8s", ContentSHA256: strings.Repeat("a", 64)}
	return resource.Definition{
		Key: "postgres-k8s-module", Name: "postgres-k8s-module", Version: 1, ResourceTypeKey: "postgres", DriverType: resource.DriverTerraform,
		DriverInputs: map[string]any{"values": map[string]any{
			"source": ref.Map(), "variables": map[string]any{},
			"input_mapping":     map[string]any{"database": map[string]any{"param": "database"}, "username": map[string]any{"param": "username"}},
			"output_mapping":    map[string]any{"host": "host", "port": "port", "database": "database", "username": "username", "password": "password"},
			"provider_bindings": map[string]any{"kubernetes": binding},
		}},
		Criteria: []resource.Criterion{{Class: "default"}},
	}
}

func edgeReason(plan *Plan, consumerPrefix, provider string) (Edge, bool) {
	for _, e := range plan.Graph.Edges {
		if strings.HasPrefix(e.Consumer, consumerPrefix) && e.Provider == provider && e.Reason == ReasonProviderBinding {
			return e, true
		}
	}
	return Edge{}, false
}

func contractFor(t *testing.T, plan *Plan, definition string) TerraformContract {
	t.Helper()
	for _, c := range plan.Terraform {
		if c.DefinitionKey == definition {
			return c
		}
	}
	t.Fatalf("no Terraform contract for %s", definition)
	return TerraformContract{}
}

// AWS bindings pin the Definition's Driver Account and literal region.
func TestPlan_AWSBindingsPinDriverAccountAndRegion(t *testing.T) {
	req := testRequest(t, application.ProfileAWSEKS, "backend")
	plan, err := NewService().Plan(req)
	if err != nil {
		t.Fatal(err)
	}
	for _, definition := range []string{"vpc-aws", "cluster-aws-eks", "postgres-aws-aurora"} {
		pins := contractFor(t, plan, definition).ProviderBindings
		if len(pins) != 1 || pins[0] != (tfmodule.ProviderPin{Name: "aws", Kind: tfmodule.BindingAWS, Target: tfmodule.TargetAccount, ConnectionKey: "aws-account", ConnectionKind: "AWS", AccountVersion: 3, Region: "us-east-1", Profile: "aws-eks"}) {
			t.Fatalf("%s pins %+v", definition, pins)
		}
	}
	// A Definition region change changes the pinned plan.
	other := req
	other.Catalog.Definitions = append([]resource.Definition(nil), req.Catalog.Definitions...)
	mutateDefinition(&other, "vpc-aws", func(d *resource.Definition) {
		vars := map[string]any{}
		for k, v := range d.DriverInputs["values"].(map[string]any)["variables"].(map[string]any) {
			vars[k] = v
		}
		vars["region"] = "eu-west-1"
		values := map[string]any{}
		for k, v := range d.DriverInputs["values"].(map[string]any) {
			values[k] = v
		}
		values["variables"] = vars
		d.DriverInputs = map[string]any{"values": values}
	})
	again, err := NewService().Plan(other)
	if err != nil || again.PlanHash == plan.PlanHash {
		t.Fatalf("region change did not change the plan hash: %v", err)
	}
	// An AWS binding still needs the account, not an Environment: the
	// Definition-owned binding plans in an internal-k8s Environment too, when
	// the Definition matches (no profile guard).
	internal := testRequest(t, application.ProfileInternalK8s, "backend")
	internal.Catalog = req.Catalog
	aws := seedAWSDefinition()
	aws.Key, aws.Name = "postgres-aws-forced", "forced"
	aws.Criteria = []resource.Criterion{{ApplicationID: internal.App.Key, Class: "default"}}
	internal.Catalog.Definitions = append(append([]resource.Definition(nil), internal.Catalog.Definitions...), aws)
	if _, err := NewService().Plan(internal); err != nil {
		t.Fatalf("AWS node from a Definition account: %v", err)
	}
}

func seedAWSDefinition() resource.Definition {
	for _, d := range catalogOf(seedOptions()).Definitions {
		if d.Key == "postgres-aws-aurora" {
			return d
		}
	}
	panic("missing seed definition")
}

// An Environment Kubernetes binding makes the Environment's cluster a
// provider even when no variable mentions it, and pins the scoped target.
func TestPlan_KubernetesEnvironmentBindingCreatesClusterEdgeWithoutPlaceholder(t *testing.T) {
	for _, tc := range []struct {
		name    string
		profile application.ExecutionProfile
		cluster string
		pinConn string
	}{
		{"internal-k8s", application.ProfileInternalK8s, "k8s-cluster.internal#connections.internal-cluster", "internal-cluster"},
		{"aws-eks", application.ProfileAWSEKS, "k8s-cluster.eks#environments.acceptance-cloud.dev", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := testRequest(t, tc.profile, "backend")
			req.Catalog.Definitions = append(req.Catalog.Definitions, kubernetesModuleDefinition(map[string]any{"target": tfmodule.TargetEnvironment}))
			plan, err := NewService().Plan(req)
			if err != nil {
				t.Fatal(err)
			}
			edge, ok := edgeReason(plan, "postgres.", tc.cluster)
			if !ok || edge.Path != "/definitions/postgres-k8s-module/driver_inputs/values/provider_bindings/kubernetes" {
				t.Fatalf("no provider-binding edge to %s: %+v", tc.cluster, plan.Graph.Edges)
			}
			pins := contractFor(t, plan, "postgres-k8s-module").ProviderBindings
			if len(pins) != 1 || pins[0].Kind != tfmodule.BindingKubernetes || pins[0].Provider != tc.cluster || pins[0].ConnectionKey != tc.pinConn || pins[0].Target != tfmodule.TargetEnvironment {
				t.Fatalf("pins %+v", pins)
			}
			// Provider before consumer in the schedule.
			position := map[string]int{}
			for i, batch := range plan.Batches {
				for _, d := range batch {
					position[d] = i
				}
			}
			var consumer string
			for _, n := range plan.Graph.Nodes {
				if strings.HasPrefix(n.Descriptor, "postgres.") {
					consumer = n.Descriptor
				}
			}
			if position[tc.cluster] >= position[consumer] {
				t.Fatalf("cluster batch %d is not before consumer batch %d", position[tc.cluster], position[consumer])
			}
		})
	}
}

// A resource binding names its cluster explicitly; an unknown or non-cluster
// reference is refused and the binding is never treated as the legacy
// Connection key.
func TestPlan_KubernetesResourceBindingNamesClusterAndRejectsOthers(t *testing.T) {
	req := testRequest(t, application.ProfileAWSEKS, "backend")
	req.Catalog.Definitions = append(req.Catalog.Definitions, kubernetesModuleDefinition(map[string]any{"target": tfmodule.TargetResource, "resource_ref": "k8s-cluster.eks#environments.@app.@env"}))
	plan, err := NewService().Plan(req)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := edgeReason(plan, "postgres.", "k8s-cluster.eks#environments.acceptance-cloud.dev"); !ok {
		t.Fatal("resource binding did not create its edge")
	}
	if pin := contractFor(t, plan, "postgres-k8s-module").ProviderBindings[0]; pin.Target != tfmodule.TargetResource || pin.ConnectionKey != "" {
		t.Fatalf("resource pin %+v", pin)
	}

	notCluster := testRequest(t, application.ProfileAWSEKS, "backend")
	notCluster.Catalog.Definitions = append(notCluster.Catalog.Definitions, kubernetesModuleDefinition(map[string]any{"target": tfmodule.TargetResource, "resource_ref": "vpc.default#@infra"}))
	if _, err := NewService().Plan(notCluster); err == nil || !strings.Contains(err.Error(), "Kubernetes cluster resource") {
		t.Fatalf("non-cluster reference: %v", err)
	}
	foreign := testRequest(t, application.ProfileAWSEKS, "backend")
	foreign.Catalog.Definitions = append(foreign.Catalog.Definitions, kubernetesModuleDefinition(map[string]any{"target": tfmodule.TargetResource, "resource_ref": "k8s-cluster.nonexistent#environments.other.env"}))
	if _, err := NewService().Plan(foreign); err == nil {
		t.Fatal("a cluster with no matching Definition was planned")
	}
}

// Edges and pins are deterministic: the same inputs give the same plan hash.
func TestPlan_ProviderBindingPlanIsDeterministic(t *testing.T) {
	build := func() *Plan {
		req := testRequest(t, application.ProfileInternalK8s, "backend")
		req.Catalog.Definitions = append(req.Catalog.Definitions, kubernetesModuleDefinition(map[string]any{"target": tfmodule.TargetEnvironment}))
		plan, err := NewService().Plan(req)
		if err != nil {
			t.Fatal(err)
		}
		return plan
	}
	if a, b := build(), build(); a.PlanHash != b.PlanHash {
		t.Fatal("plan hash is not deterministic")
	}
}
