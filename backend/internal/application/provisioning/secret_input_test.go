package provisioning_test

import (
	"context"
	"encoding/json"
	"errors"
	"orchestrator/internal/seed/seedtest"
	"strings"
	"testing"

	"orchestrator/internal/adapters/secrets"
	"orchestrator/internal/adapters/terraform/registry"
	"orchestrator/internal/application/provisioning"
	"orchestrator/internal/bootstrap"
	"orchestrator/internal/bootstrap/bootstraptest"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/planning"
	"orchestrator/internal/platform/clock"
	"orchestrator/internal/ports/execution"
)

const leakedPassword = "s3cr3t-pw-value-0123456789"

// classExecutor is a FAKE module executor: it returns scripted outputs per
// resource type and records every call, so a rejected consumer is proven never
// to have reached the runner.
type classExecutor struct {
	calls     map[string]int
	sensitive map[string]bool // runtime-sensitive flag reported for the provider
	inputs    map[string]map[string]any
}

func (c *classExecutor) Resolve(resource.DriverType) (execution.ResourceExecutor, error) {
	return c, nil
}
func (c *classExecutor) RequiresVerifiedModule() bool { return true }
func (c *classExecutor) Provision(_ context.Context, req execution.ProvisionRequest) (execution.ProvisionResult, error) {
	c.calls[req.ResourceType]++
	c.inputs[req.ResourceType] = req.Inputs
	if req.ResourceType == "annotated" {
		return execution.ProvisionResult{Outputs: map[string]any{"ok": "yes"}, State: map[string]any{}}, nil
	}
	return execution.ProvisionResult{Outputs: map[string]any{"h": "db.internal", "pw": leakedPassword}, Sensitive: c.sensitive, State: map[string]any{}}, nil
}

// classVerifier verifies any pin; the module-only secret case marks the
// provider module output `pw` sensitive.
type classVerifier struct{ moduleSensitive bool }

func (v classVerifier) VerifyHistoricalForExecution(context.Context, tfmodule.Bundle) (tfmodule.VerifiedArtifact, error) {
	return tfmodule.VerifiedArtifact{}, errors.New("not used")
}
func (v classVerifier) VerifyForExecution(_ context.Context, ref tfmodule.SourceRef) (tfmodule.VerifiedArtifact, error) {
	bundle := tfmodule.Bundle{Ref: ref, Contract: tfmodule.Contract{Outputs: map[string]tfmodule.Output{
		"h": {Type: "string"}, "pw": {Type: "string", Sensitive: v.moduleSensitive}, "ok": {Type: "string"}}}}
	return tfmodule.VerifiedArtifact{Bundle: bundle, ModuleDir: "/private/cache/module", TerraformBinary: "/usr/local/bin/terraform"}, nil
}

// runClassified provisions a provider node then a consumer node whose
// Definition variable (and optionally a mapped param) holds the given value.
func runClassified(t *testing.T, typeSecret, moduleSensitive, runtimeSensitive bool, variable, param any) (*classExecutor, error, string) {
	t.Helper()
	ctx := context.Background()
	opts := seedtest.Defaults()
	app, err := bootstraptest.Build(ctx, bootstrap.Options{Seed: opts, Adapters: bootstrap.AdapterFake})
	if err != nil {
		t.Fatal(err)
	}
	org := opts.OrganizationKey
	saveAWSConnections(t, app, org, "aws-a")
	refs, _ := registry.PackagedRefs()
	pinned := func(key, typ string, variables map[string]any, mapping map[string]any, outputs map[string]any) resource.Definition {
		return resource.Definition{Key: key, Name: key, Version: 1, ResourceTypeKey: typ, DriverType: resource.DriverTerraform, DriverAccount: "aws-a", Criteria: []resource.Criterion{{}},
			DriverInputs: map[string]any{"values": map[string]any{"source": refs["vpc"].Map(), "variables": variables, "input_mapping": mapping, "output_mapping": outputs,
				"provider_bindings": map[string]any{"aws": map[string]any{"target": "account"}}}}}
	}
	provider := pinned("db-def", "db", map[string]any{}, map[string]any{}, map[string]any{"h": "host", "pw": "password"})
	consumerVars := map[string]any{}
	mapping := map[string]any{}
	node := planning.Node{Descriptor: "annotated.default#applications.a", Kind: planning.NodeResource, ResourceType: "annotated", Class: "default", Scope: resource.Scope{Type: resource.ScopeApplication, ID: "a"},
		Bindings: map[string]string{"db.default#applications.a": "db.default#applications.a"}}
	if variable != nil {
		consumerVars["annotations"] = variable
	}
	if param != nil {
		mapping["extra"] = map[string]any{"param": "extra"}
		node.Params = map[string]any{"extra": param}
	}
	consumer := pinned("annotated-def", "annotated", consumerVars, mapping, map[string]any{"ok": "ok"})
	dbType := resource.Type{Key: "db", Outputs: []resource.OutputField{{Name: "host", Type: "string", Required: true}, {Name: "password", Type: "string", Required: true, Secret: typeSecret}}}
	consumerType := resource.Type{Key: "annotated", Inputs: nil, Outputs: []resource.OutputField{{Name: "ok", Type: "string", Required: true}}}
	if param != nil {
		consumerType.Inputs = []resource.InputField{{Name: "extra", Type: "any"}}
	}
	exec := &classExecutor{calls: map[string]int{}, inputs: map[string]map[string]any{}}
	if runtimeSensitive {
		exec.sensitive = map[string]bool{"pw": true}
	}
	const dbDescriptor = "db.default#applications.a"
	service := provisioning.NewService(app.Store, exec, secrets.NewMemory(), clock.System{})
	service.SetModuleVerifier(classVerifier{moduleSensitive: moduleSensitive})
	req := provisioning.Request{
		DeploymentID: "dep-secret", RunID: "run-1",
		Context: planning.Context{OrganizationKey: org, App: application.Application{Key: "a", OrganizationKey: org}, Env: environment.Environment{Key: "staging", Profile: application.ProfileAWSEKS, ConnectionKey: "aws-a", Region: "us-east-1"}},
		Plan: &planning.Plan{
			Graph: planning.Graph{Nodes: []planning.Node{
				{Descriptor: dbDescriptor, Kind: planning.NodeResource, ResourceType: "db", Class: "default", Scope: resource.Scope{Type: resource.ScopeApplication, ID: "a"}},
				node,
			}},
			Matches: map[string]planning.Match{
				dbDescriptor:    {Descriptor: dbDescriptor, DefinitionKey: provider.Key, DriverType: provider.DriverType},
				node.Descriptor: {Descriptor: node.Descriptor, DefinitionKey: consumer.Key, DriverType: consumer.DriverType},
			},
			Batches: [][]string{{dbDescriptor}, {node.Descriptor}},
		},
		Types:       map[string]resource.Type{"db": dbType, "annotated": consumerType},
		Definitions: map[string]resource.Definition{provider.Key: provider, consumer.Key: consumer},
	}
	_, err = service.Provision(ctx, req)
	// Everything persisted for the deployment, as one string.
	stored, _ := app.Store.ListDeploymentResources(ctx, "dep-secret")
	raw, _ := json.Marshal(stored)
	active, _ := app.Store.ListActiveResources(ctx, org)
	rawActive, _ := json.Marshal(active)
	return exec, err, string(raw) + string(rawActive)
}

func TestProvision_SecretOutputNeverFeedsANonsecretTerraformInput(t *testing.T) {
	ref := "${resources['db.default#applications.a'].outputs.password}"
	placements := map[string]struct{ variable, param any }{
		"direct":       {ref, nil},
		"nested map":   {map[string]any{"example.test/k": ref}, nil},
		"list":         {[]any{"a", ref}, nil},
		"interpolated": {map[string]any{"example.test/k": "host-" + ref + "-end"}, nil},
		"deep":         {map[string]any{"a": []any{map[string]any{"b": "x:" + ref}}}, nil},
		"mapped param": {nil, map[string]any{"k": "pre-" + ref}},
	}
	classes := map[string]struct{ typeSecret, module, runtime bool }{
		"Resource Type secret":  {true, false, false},
		"module-only secret":    {false, true, false},
		"runtime-sensitive":     {false, false, true},
		"all three classifiers": {true, true, true},
	}
	for className, class := range classes {
		for placement, p := range placements {
			t.Run(className+"/"+placement, func(t *testing.T) {
				exec, err, stored := runClassified(t, class.typeSecret, class.module, class.runtime, p.variable, p.param)
				if !errors.Is(err, provisioning.ErrSecretInput) {
					t.Fatalf("not rejected: %v", err)
				}
				if exec.calls["annotated"] != 0 {
					t.Fatal("the consumer runner was opened")
				}
				if exec.calls["db"] != 1 {
					t.Fatalf("provider runs %d", exec.calls["db"])
				}
				if strings.Contains(err.Error(), leakedPassword) || strings.Contains(stored, leakedPassword) || strings.Contains(strings.ToLower(err.Error()), "secretref") {
					t.Fatalf("a secret or opaque ref leaked: %v / %s", err, stored)
				}
			})
		}
	}
}

// The nonsecret host of the same provider still feeds annotations, labels and
// params: the selected-set fixture keeps working.
func TestProvision_NonsecretOutputStillFeedsAnnotations(t *testing.T) {
	host := "${resources['db.default#applications.a'].outputs.host}"
	exec, err, stored := runClassified(t, true, true, false, map[string]any{"example.test/postgres-host": host, "list": []any{"x-" + host}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	annotations, _ := exec.inputs["annotated"]["annotations"].(map[string]any)
	if exec.calls["annotated"] != 1 || annotations["example.test/postgres-host"] != "db.internal" {
		t.Fatalf("calls %v inputs %v", exec.calls, exec.inputs["annotated"])
	}
	if strings.Contains(stored, leakedPassword) {
		t.Fatalf("the secret reached stored resources: %s", stored)
	}
}
