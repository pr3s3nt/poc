package planning

import (
	"testing"

	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
)

func intentPlan(mutate func(*Plan)) *Plan {
	const descriptor = "redis.in-cluster#shared.cache"
	p := &Plan{
		Graph:   Graph{Nodes: []Node{{Descriptor: descriptor, Kind: NodeResource, ResourceType: "redis", Class: "in-cluster", Scope: resource.Scope{Type: resource.ScopeShared, ID: "cache"}, Params: map[string]any{"size": 1}}}},
		Matches: map[string]Match{descriptor: {Descriptor: descriptor, DefinitionKey: "redis-module", DriverType: resource.DriverTerraform}},
		Terraform: []TerraformContract{{
			Descriptor: descriptor, DefinitionKey: "redis-module", Module: "redis-k8s", Fingerprint: "fp-1", BundleID: "bundle-1", TerraformVersion: "1.9.8", ProviderLockDigest: "lock-1",
			Source:        &tfmodule.SourceRef{SourceID: "platform", ModuleID: "redis-k8s", Revision: "r1", Subdirectory: "redis-k8s", ContentSHA256: "aa"},
			Inputs:        []ContractInput{{Name: "namespace", Type: "string", Source: "definition", Value: "ns-a"}},
			OutputMapping: map[string]string{"host": "host"},
		}},
	}
	p.IntentEvidence = map[string]string{descriptor: "ev-1"}
	if mutate != nil {
		mutate(p)
	}
	return p
}

func intentOf(t *testing.T, now, before *Plan) string {
	t.Helper()
	got := now.ResourceIntents(func(Node) *Plan { return before })
	if len(got) != 1 {
		t.Fatalf("intents %+v", got)
	}
	return got[0].Intent
}

// UC-05 BR-10: RETAIN only when the complete authored and pinned execution
// intent is identical; any change of an input, mapping or pin is UPDATE; missing
// historical evidence is UNVERIFIED, never RETAIN.
func TestResourceIntentComparesTheCompleteExecutionIntent(t *testing.T) {
	cases := map[string]struct {
		change func(*Plan)
		want   string
	}{
		"identical":            {nil, IntentRetain},
		"fallback variable":    {func(p *Plan) { p.Terraform[0].Inputs[0].Value = "ns-b" }, IntentUpdate},
		"input mapping source": {func(p *Plan) { p.Terraform[0].Inputs[0].Source = "param" }, IntentUpdate},
		"extra input": {func(p *Plan) {
			p.Terraform[0].Inputs = append(p.Terraform[0].Inputs, ContractInput{Name: "image", Type: "string", Source: "definition", Value: "x"})
		}, IntentUpdate},
		"output mapping":       {func(p *Plan) { p.Terraform[0].OutputMapping = map[string]string{"host": "other"} }, IntentUpdate},
		"module revision":      {func(p *Plan) { p.Terraform[0].Source.Revision = "r2" }, IntentUpdate},
		"module digest":        {func(p *Plan) { p.Terraform[0].Source.ContentSHA256 = "bb" }, IntentUpdate},
		"bundle":               {func(p *Plan) { p.Terraform[0].BundleID = "bundle-2" }, IntentUpdate},
		"contract fingerprint": {func(p *Plan) { p.Terraform[0].Fingerprint = "fp-2" }, IntentUpdate},
		"terraform release":    {func(p *Plan) { p.Terraform[0].TerraformVersion = "1.9.9" }, IntentUpdate},
		"provider lock":        {func(p *Plan) { p.Terraform[0].ProviderLockDigest = "lock-2" }, IntentUpdate},
		"definition key": {func(p *Plan) {
			p.Matches["redis.in-cluster#shared.cache"] = Match{DefinitionKey: "redis-other", DriverType: resource.DriverTerraform}
		}, IntentUpdate},
		"connection": {func(p *Plan) {
			p.Matches["redis.in-cluster#shared.cache"] = Match{DefinitionKey: "redis-module", DriverType: resource.DriverTerraform, ConnectionKey: "lab"}
		}, IntentUpdate},
		"authored param": {func(p *Plan) { p.Graph.Nodes[0].Params = map[string]any{"size": 2} }, IntentUpdate},
		"provider target": {func(p *Plan) {
			p.Terraform[0].ProviderBindings = []tfmodule.ProviderPin{{Name: "kubernetes", Target: "environment", ConnectionKey: "lab"}}
		}, IntentUpdate},
		"secret output set":           {func(p *Plan) { p.Terraform[0].SecretOutputs = []string{"password"} }, IntentUpdate},
		"authored definition content": {func(p *Plan) { p.IntentEvidence["redis.in-cluster#shared.cache"] = "ev-2" }, IntentUpdate},
		"current lacks evidence":      {func(p *Plan) { p.IntentEvidence = nil }, IntentUnverified},
		"current lacks contract":      {func(p *Plan) { p.Terraform = nil }, IntentUnverified},
		"current lacks fingerprint":   {func(p *Plan) { p.Terraform[0].Fingerprint = "" }, IntentUnverified},
	}
	for name, tc := range cases {
		before := intentPlan(nil)
		now := intentPlan(tc.change)
		if got := intentOf(t, now, before); got != tc.want {
			t.Errorf("%s: %s, want %s", name, got, tc.want)
		}
	}
	// Historical evidence missing: no plan, or a plan without the contract.
	now := intentPlan(nil)
	if got := now.ResourceIntents(func(Node) *Plan { return nil })[0].Intent; got != IntentUnverified {
		t.Errorf("no baseline: %s", got)
	}
	if got := now.ResourceIntents(nil)[0].Intent; got != IntentUnverified {
		t.Errorf("no reader: %s", got)
	}
	if got := intentOf(t, now, intentPlan(func(p *Plan) { p.Terraform = nil })); got != IntentUnverified {
		t.Errorf("baseline without contract: %s", got)
	}
	if got := intentOf(t, now, intentPlan(func(p *Plan) { p.IntentEvidence = nil })); got != IntentUnverified {
		t.Errorf("baseline without authored-content evidence: %s", got)
	}
	// A node created by this plan is CREATE regardless of history.
	created := intentPlan(func(p *Plan) { p.Classification.New = []string{"redis.in-cluster#shared.cache"} })
	if got := intentOf(t, created, intentPlan(nil)); got != IntentCreate {
		t.Errorf("new node: %s", got)
	}
	// The comparison hash stays internal: the projection carries no hash field.
	if raw := now.ResourceIntents(func(Node) *Plan { return intentPlan(nil) }); raw[0].Module == nil {
		t.Fatal("module pin expected")
	}
}

// Non-Terraform, definitions-driven semantics: the driver, Connection and
// binding of the match are part of the comparison.
func TestResourceIntentComparesDriverAndBindingOfNonTerraformNodes(t *testing.T) {
	const descriptor = "k8s-cluster.internal#connections.lab"
	mk := func(match Match) *Plan {
		return &Plan{
			Graph:   Graph{Nodes: []Node{{Descriptor: descriptor, Kind: NodeResource, ResourceType: "k8s-cluster", Class: "internal", Scope: resource.Scope{Type: resource.ScopeApplication, ID: "a"}}}},
			Matches: map[string]Match{descriptor: match},
		}
	}
	base := Match{DefinitionKey: "builtin-existing-cluster", DriverType: resource.DriverExistingCluster, Binding: BindingEnvironmentConnection, ConnectionKey: "lab"}
	same := mk(base).ResourceIntents(func(Node) *Plan { return mk(base) })
	if same[0].Intent != IntentBind {
		t.Fatalf("binding node: %+v", same[0])
	}
	moved := base
	moved.ConnectionKey = "other"
	// The bound node is always BIND for display, but its stored fingerprint differs.
	if mk(base).intentFingerprints()[descriptor+"|"+mk(base).Graph.Nodes[0].Scope.Key()] == mk(moved).intentFingerprints()[descriptor+"|"+mk(moved).Graph.Nodes[0].Scope.Key()] {
		t.Fatal("a changed Connection must change the fingerprint")
	}
}
