package planning

import (
	"errors"
	"testing"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/resource"
)

func labCluster(connection string) resource.Definition {
	return resource.Definition{
		Key: "cluster-lab", ResourceTypeKey: "k8s-cluster", DriverType: resource.DriverExistingCluster,
		ExecutionProfile: "internal-k8s", ConnectionKey: connection,
		DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{"name": "${context.connection.cluster}", "kubeContext": "${context.connection.context}"}}},
		Criteria:     []resource.Criterion{{Class: "internal", ResourceID: "connections.lab"}},
	}
}

// ADR-013: the Environment connection selects the cluster node directly. No
// cluster Definition is needed and authored Definitions cannot redirect it.
func TestPlan_InternalClusterFollowsEnvironmentConnectionWithoutDefinition(t *testing.T) {
	req := testRequest(t, application.ProfileInternalK8s, "backend")
	req.Env.ConnectionKey = "lab"
	req.Connection.Key = "lab"

	plan, err := NewService().Plan(req)
	if err != nil {
		t.Fatalf("no cluster definition must be needed: %v", err)
	}
	match := plan.Matches["k8s-cluster.internal#connections.lab"]
	if match.DefinitionKey != BuiltinClusterKey || match.ConnectionKey != "lab" || match.Binding != BindingEnvironmentConnection {
		t.Fatalf("cluster match = %+v", match)
	}
	again, err := NewService().Plan(req)
	if err != nil || again.PlanHash != plan.PlanHash {
		t.Fatalf("plan must be deterministic: %v", err)
	}

	// Authored cluster Definitions, specific, tied or for another connection,
	// never retarget the node nor create ambiguity.
	req.Catalog.Definitions = append(req.Catalog.Definitions, labCluster("lab"), labCluster("internal-cluster"))
	req.Catalog.Definitions[len(req.Catalog.Definitions)-1].Key = "cluster-tie"
	authored, err := NewService().Plan(req)
	if err != nil {
		t.Fatalf("authored definitions: %v", err)
	}
	if got := authored.Matches["k8s-cluster.internal#connections.lab"]; got != match || authored.PlanHash != plan.PlanHash {
		t.Fatalf("authored definition changed the cluster binding: %+v", got)
	}

	// The reference opt-out keeps Definition matching and ConnectionMismatch.
	req.ReferenceCluster = true
	req.Catalog.Definitions = req.Catalog.Definitions[:len(req.Catalog.Definitions)-2]
	if _, err := NewService().Plan(req); err == nil {
		t.Fatal("reference matching needs a cluster definition")
	}
}

func TestPlan_InternalClusterRejectsUntrustedConnectionAndReservedKey(t *testing.T) {
	cases := map[string]func(*Request){
		"other identity": func(r *Request) { r.Connection.Key = "someone-else" },
		"not ready":      func(r *Request) { r.Connection.Status = application.ConnectionVerifying },
		"wrong kind":     func(r *Request) { r.Connection.Kind = application.ConnectionAWS },
		"other org":      func(r *Request) { r.Connection.OrganizationKey = "globex" },
		"forged reserved definition": func(r *Request) {
			forged := BuiltinClusterDefinition()
			forged.Criteria = []resource.Criterion{{}}
			r.Catalog.Definitions = append(r.Catalog.Definitions, forged)
		},
	}
	for name, mutate := range cases {
		req := testRequest(t, application.ProfileInternalK8s, "backend")
		mutate(&req)
		if _, err := NewService().Plan(req); err == nil {
			t.Fatalf("%s: plan must fail closed", name)
		}
	}
	// The exact stored system definition is accepted and never matched.
	req := testRequest(t, application.ProfileInternalK8s, "backend")
	req.Catalog.Definitions = append(req.Catalog.Definitions, BuiltinClusterDefinition())
	if _, err := NewService().Plan(req); err != nil {
		t.Fatalf("stored system definition: %v", err)
	}
}

func TestValidateBuiltinMatchRejectsForgery(t *testing.T) {
	req := testRequest(t, application.ProfileInternalK8s, "backend")
	plan, err := NewService().Plan(req)
	if err != nil {
		t.Fatal(err)
	}
	ctx := Context{OrganizationKey: req.OrganizationKey, App: req.App, Env: req.Env, Connection: req.Connection}
	desc := "k8s-cluster.internal#connections." + req.Env.ConnectionKey
	node, _ := plan.Graph.Node(desc)
	if err := ValidateBuiltinMatch(ctx, node, plan.Matches[desc]); err != nil {
		t.Fatal(err)
	}
	forged := plan.Matches[desc]
	forged.ConnectionKey = "other"
	if ValidateBuiltinMatch(ctx, node, forged) == nil {
		t.Fatal("other connection accepted")
	}
	ns, _ := plan.Graph.Node("k8s-namespace.default#environments." + req.App.Key + "." + req.Env.Key)
	reserved := plan.Matches[desc]
	reserved.Descriptor = ns.Descriptor
	if ValidateBuiltinMatch(ctx, ns, reserved) == nil {
		t.Fatal("reserved key on an unrelated node accepted")
	}
	bound := plan.Matches[ns.Descriptor]
	bound.Binding = BindingEnvironmentConnection
	if ValidateBuiltinMatch(ctx, ns, bound) == nil {
		t.Fatal("forged binding accepted")
	}
}

func TestMatcher_InternalClusterRejectsParams(t *testing.T) {
	req := testRequest(t, application.ProfileInternalK8s, "backend")
	ctx := Context{OrganizationKey: req.OrganizationKey, App: req.App, Env: req.Env, Connection: req.Connection}
	matcher, err := newDefinitionMatcher(ctx, req.Catalog)
	if err != nil {
		t.Fatal(err)
	}
	d, _ := InternalClusterDescriptor(req.Env.ConnectionKey)
	node := &Node{Descriptor: d.String(), Kind: NodeResource, ResourceType: TypeCluster, Class: ClassInternal, Params: map[string]any{"kubeContext": "evil"}}
	if _, err := matcher.match(node); err == nil {
		t.Fatal("params on the implicit cluster must be rejected")
	}
	node.Params = nil
	if _, err := matcher.match(node); err != nil {
		t.Fatal(err)
	}
}

func TestPlan_KubernetesDefinitionConnectionMustMatchInternalEnvironment(t *testing.T) {
	req := testRequest(t, application.ProfileInternalK8s, "backend")
	for i, def := range req.Catalog.Definitions {
		if def.Key == "namespace-kubernetes" {
			req.Catalog.Definitions[i].ConnectionKey = "someone-else"
		}
	}
	if _, err := NewService().Plan(req); !errors.Is(err, ErrConnectionMismatch) {
		t.Fatalf("explicit kubernetes connection must match: %v", err)
	}
	for i, def := range req.Catalog.Definitions {
		if def.Key == "namespace-kubernetes" {
			req.Catalog.Definitions[i].ConnectionKey = req.Env.ConnectionKey
		}
	}
	if _, err := NewService().Plan(req); err != nil {
		t.Fatalf("equal connection: %v", err)
	}
}

func withConnection(req *Request, key, connection string) {
	for i, def := range req.Catalog.Definitions {
		if def.Key == key {
			req.Catalog.Definitions[i].ConnectionKey = connection
		}
	}
}

// UC-06 BR-20 for aws-eks: Terraform VPC and EKS must use the Environment
// account; external databases keep their own Driver Account.
func TestPlan_AWSTargetTerraformDefinitionsMustUseEnvironmentConnection(t *testing.T) {
	req := testRequest(t, application.ProfileAWSEKS, "backend")
	if _, err := NewService().Plan(req); err != nil {
		t.Fatalf("matching account: %v", err)
	}
	for _, key := range []string{"vpc-aws", "cluster-aws-eks"} {
		bad := testRequest(t, application.ProfileAWSEKS, "backend")
		withConnection(&bad, key, "other-account")
		if _, err := NewService().Plan(bad); !errors.Is(err, ErrConnectionMismatch) {
			t.Fatalf("%s with another account: %v", key, err)
		}
	}
	external := testRequest(t, application.ProfileAWSEKS, "backend")
	withConnection(&external, "postgres-aws-aurora", "database-account")
	plan, err := NewService().Plan(external)
	if err != nil {
		t.Fatalf("external database keeps its Driver Account: %v", err)
	}
	for _, match := range plan.Matches {
		if match.DefinitionKey == "postgres-aws-aurora" && match.ConnectionKey != "database-account" {
			t.Fatalf("database connection changed: %+v", match)
		}
	}
}
