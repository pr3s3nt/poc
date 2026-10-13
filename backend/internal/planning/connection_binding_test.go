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
	// AWS modules take no Connection/account override (ADR-014): an external
	// database account is a mismatch, never a silent retarget.
	external := testRequest(t, application.ProfileAWSEKS, "backend")
	withConnection(&external, "postgres-aws-aurora", "database-account")
	if _, err := NewService().Plan(external); !errors.Is(err, ErrConnectionMismatch) {
		t.Fatalf("database for another account: %v", err)
	}
}

// T16: seeded VPC/EKS Definitions have no ConnectionKey; the effective AWS
// target is the Environment connection, recorded in the plan and its hash.
func TestPlan_AWSVPCAndEKSInheritIndependentEnvironmentTargets(t *testing.T) {
	hashes := map[string]string{}
	for _, target := range []string{"aws-a", "aws-b"} {
		req := testRequest(t, application.ProfileAWSEKS, "backend")
		req.Env.ConnectionKey, req.Connection.Key = target, target
		for _, def := range req.Catalog.Definitions {
			if (def.Key == "vpc-aws" || def.Key == "cluster-aws-eks") && def.ConnectionKey != "" {
				t.Fatalf("seeded %s keeps connection %q", def.Key, def.ConnectionKey)
			}
		}
		plan, err := NewService().Plan(req)
		if err != nil {
			t.Fatalf("%s: %v", target, err)
		}
		if plan.Target.ConnectionKey != target {
			t.Fatalf("%s: plan target = %+v", target, plan.Target)
		}
		for _, match := range plan.Matches {
			if (match.DefinitionKey == "vpc-aws" || match.DefinitionKey == "cluster-aws-eks") && match.ConnectionKey != "" {
				t.Fatalf("%s: inherited match carries a Definition key: %+v", target, match)
			}
		}
		hashes[target] = plan.PlanHash
	}
	if hashes["aws-a"] == hashes["aws-b"] {
		t.Fatal("plan hash must change with the effective Environment connection")
	}
}

// A malformed winning VPC/EKS Definition is rejected; no other candidate is
// tried. Empty keys are never a mismatch.
func TestPlan_MalformedWinningVPCOrEKSKeyIsRejectedWithoutFallback(t *testing.T) {
	for _, base := range []string{"vpc-aws", "cluster-aws-eks"} {
		req := testRequest(t, application.ProfileAWSEKS, "backend")
		var malformed resource.Definition
		for _, def := range req.Catalog.Definitions {
			if def.Key == base {
				malformed = def
			}
		}
		// More specific than the seeded fallback so it wins matching.
		malformed.Key = base + "-malformed"
		malformed.ConnectionKey = "other-account"
		malformed.Criteria = []resource.Criterion{{ApplicationID: req.App.Key, Class: malformed.Criteria[0].Class}}
		req.Catalog.Definitions = append(req.Catalog.Definitions, malformed)
		if _, err := NewService().Plan(req); !errors.Is(err, ErrConnectionMismatch) {
			t.Fatalf("%s: malformed winner must fail without falling back to %s: %v", malformed.Key, base, err)
		}
	}
	env := testRequest(t, application.ProfileAWSEKS, "backend").Env
	vpc := resource.Definition{Key: "v", ResourceTypeKey: TypeVPC, DriverType: resource.DriverTerraform}
	if ConnectionMismatch(env, vpc) {
		t.Fatal("an empty key is not a mismatch")
	}
	vpc.ConnectionKey = env.ConnectionKey
	if ConnectionMismatch(env, vpc) {
		t.Fatal("runtime guard is unchanged for a key equal to the target")
	}
	vpc.ConnectionKey = "other-account"
	if !ConnectionMismatch(env, vpc) {
		t.Fatal("a different key must remain a mismatch")
	}
}
