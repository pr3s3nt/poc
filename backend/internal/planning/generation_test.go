package planning

import (
	"testing"

	"orchestrator/internal/domain/resource"
)

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
