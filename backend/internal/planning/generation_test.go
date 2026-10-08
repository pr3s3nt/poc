package planning

import (
	"strings"
	"testing"

	"orchestrator/internal/domain/resource"
)

// A later target generation of an aws-eks Environment never reuses the VPC/EKS
// identity, scope or provider-visible name of the previous generation (local
// planning only; no cloud account is contacted).
func TestPlan_AWSTargetGenerationsAreIsolated(t *testing.T) {
	plan := func(generation int64) *Plan {
		req := scopedAWSRequest(t, "pay", "staging")
		req.Env.TargetGeneration = generation
		p, err := NewService().Plan(req)
		if err != nil {
			t.Fatalf("generation %d: %v", generation, err)
		}
		return p
	}
	g0, g1, g2 := plan(0), plan(1), plan(2)
	for _, kind := range [][2]string{{"vpc", ""}, {"k8s-cluster", "eks"}, {"k8s-namespace", ""}} {
		n0, n1, n2 := nodeByType(t, g0, kind[0], kind[1]), nodeByType(t, g1, kind[0], kind[1]), nodeByType(t, g2, kind[0], kind[1])
		if n0.Descriptor == n1.Descriptor || n1.Descriptor == n2.Descriptor {
			t.Fatalf("%s descriptors must differ per generation: %s %s %s", kind[0], n0.Descriptor, n1.Descriptor, n2.Descriptor)
		}
		if n0.Scope.ID != "pay.staging" || n1.Scope.ID != "pay.staging~g1" || n2.Scope.ID != "pay.staging~g2" || n1.Scope.Type != resource.ScopeEnvironment {
			t.Fatalf("%s scopes: %+v %+v %+v", kind[0], n0.Scope, n1.Scope, n2.Scope)
		}
	}
	// Generation 0 keeps the exact ADR-011 descriptor.
	if got := nodeByType(t, g0, "vpc", "").Descriptor; got != "vpc.default#environments.pay.staging" {
		t.Fatalf("generation 0 changed: %s", got)
	}
	name := func(p *Plan) string { return contractValue(t, p, "vpc-aws", "name").(string) }
	if name(g0) == name(g1) || name(g1) == name(g2) {
		t.Fatalf("provider names collide: %s %s %s", name(g0), name(g1), name(g2))
	}
	for _, p := range []*Plan{g0, g1, g2} {
		if len(name(p)) > 49 {
			t.Fatalf("name exceeds the provider bound: %s", name(p))
		}
	}
	if !strings.HasPrefix(nodeByType(t, g1, "k8s-namespace", "").Descriptor, "k8s-namespace.default#environments.pay.staging.g1") {
		t.Fatal("namespace descriptor must carry the generation")
	}
}

func TestPlan_GenerationZeroResourcesAreNotOwnedByLaterGenerations(t *testing.T) {
	ctx := Context{OrganizationKey: "acme", App: scopedAWSRequest(t, "pay", "staging").App, Env: scopedAWSRequest(t, "pay", "staging").Env}
	ctx.Env.TargetGeneration = 1
	for _, scope := range []resource.Scope{{Type: resource.ScopeEnvironment, ID: "pay.staging"}, {Type: resource.ScopeWorkload, ID: "pay.staging.web"}, {Type: resource.ScopeShared, ID: "pay.staging"}} {
		if ownedByEnvironment(ctx, scope) {
			t.Fatalf("generation 1 must not own generation 0 scope %+v", scope)
		}
	}
	ctx.Env.TargetGeneration = 0
	for _, scope := range []resource.Scope{{Type: resource.ScopeEnvironment, ID: "pay.staging~g1"}, {Type: resource.ScopeWorkload, ID: "pay.staging~g1.web"}, {Type: resource.ScopeShared, ID: "pay.staging~g2"}} {
		if ownedByEnvironment(ctx, scope) {
			t.Fatalf("generation 0 must not own later scope %+v", scope)
		}
	}
	if !ownedByEnvironment(ctx, resource.Scope{Type: resource.ScopeWorkload, ID: "pay.staging.web"}) {
		t.Fatal("generation 0 keeps its own workload scopes")
	}
}

func TestParseDescriptorText_AtEnvResolvesTheGenerationNamespace(t *testing.T) {
	req := scopedAWSRequest(t, "pay", "staging")
	for generation, want := range map[int64]string{0: "k8s-namespace.default#environments.pay.staging", 3: "k8s-namespace.default#environments.pay.staging.g3"} {
		ctx := Context{OrganizationKey: "acme", App: req.App, Env: req.Env}
		ctx.Env.TargetGeneration = generation
		d, err := ParseDescriptorText("k8s-namespace.default#environments.@app.@env", nil, ctx)
		if err != nil || d.String() != want {
			t.Fatalf("generation %d: %s %v", generation, d, err)
		}
	}
}
