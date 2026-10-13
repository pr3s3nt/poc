package provisioning_test

import (
	"context"
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
	"orchestrator/internal/ports/moduleregistry"
)

// moduleExecutor stands in for an executor that runs module code: it
// requires a verified artifact and returns module-named outputs.
type moduleExecutor struct{ requests []execution.ProvisionRequest }

func (m *moduleExecutor) Resolve(resource.DriverType) (execution.ResourceExecutor, error) {
	return m, nil
}
func (m *moduleExecutor) RequiresVerifiedModule() bool { return true }
func (m *moduleExecutor) Provision(_ context.Context, req execution.ProvisionRequest) (execution.ProvisionResult, error) {
	m.requests = append(m.requests, req)
	return execution.ProvisionResult{Outputs: map[string]any{"endpoint": "cache.internal", "auth": "s3cr3t-value", "internal_arn": "arn:aws:x"}, State: map[string]any{}}, nil
}

type stubVerifier struct {
	err   error
	calls int
}

func (v *stubVerifier) VerifyHistoricalForExecution(context.Context, tfmodule.Bundle) (tfmodule.VerifiedArtifact, error) {
	return tfmodule.VerifiedArtifact{}, errors.New("not used")
}

func (v *stubVerifier) VerifyForExecution(_ context.Context, ref tfmodule.SourceRef) (tfmodule.VerifiedArtifact, error) {
	v.calls++
	if v.err != nil {
		return tfmodule.VerifiedArtifact{}, v.err
	}
	bundle := tfmodule.Bundle{Ref: ref, Contract: tfmodule.Contract{Outputs: map[string]tfmodule.Output{"endpoint": {Type: "string"}, "auth": {Type: "string", Sensitive: true}, "internal_arn": {}}}}
	return tfmodule.VerifiedArtifact{Bundle: bundle, ModuleDir: "/private/cache/module", TerraformBinary: "/usr/local/bin/terraform"}, nil
}

func runVerified(t *testing.T, app *bootstrap.App, org string, verifier moduleregistry.ExecutionVerifier, exec *moduleExecutor) error {
	t.Helper()
	descriptor := "cache.default#applications.a"
	refs, _ := registry.PackagedRefs()
	def := resource.Definition{Key: "cache-def", ResourceTypeKey: "cache", DriverType: resource.DriverTerraform, ExecutionProfile: "aws-eks", Criteria: []resource.Criterion{{}},
		DriverInputs: map[string]any{"values": map[string]any{
			"source":            refs["vpc"].Map(),
			"variables":         map[string]any{"memory_mb": 512},
			"input_mapping":     map[string]any{"memory_mb": map[string]any{"param": "size"}},
			"output_mapping":    map[string]any{"endpoint": "host", "auth": "password"},
			"provider_bindings": map[string]any{"aws": map[string]any{"target": "environment"}},
		}}}
	typ := resource.Type{Key: "cache", Inputs: []resource.InputField{{Name: "size", Type: "number"}}, Outputs: []resource.OutputField{{Name: "host", Type: "string", Required: true}, {Name: "password", Type: "string", Required: true}}}
	if err := app.Store.SaveResourceType(context.Background(), org, typ); err != nil {
		t.Fatal(err)
	}
	if err := app.Store.SaveResourceDefinition(context.Background(), org, def); err != nil {
		t.Fatal(err)
	}
	service := provisioning.NewService(app.Store, exec, secrets.NewMemory(), clock.System{})
	if verifier != nil {
		service.SetModuleVerifier(verifier)
	}
	_, err := service.Provision(context.Background(), provisioning.Request{
		DeploymentID: "dep-verify", RunID: "run-1",
		Context: planning.Context{OrganizationKey: org, App: application.Application{Key: "a", OrganizationKey: org}, Env: environment.Environment{Key: "staging", Profile: application.ProfileAWSEKS, ConnectionKey: "aws-a", Region: "us-east-1"}},
		Plan: &planning.Plan{
			Graph:   planning.Graph{Nodes: []planning.Node{{Descriptor: descriptor, Kind: planning.NodeResource, ResourceType: "cache", Class: "default", Params: map[string]any{"size": 2048}, Scope: resource.Scope{Type: resource.ScopeApplication, ID: "a"}}}},
			Matches: map[string]planning.Match{descriptor: {Descriptor: descriptor, DefinitionKey: def.Key, DriverType: def.DriverType}},
			Batches: [][]string{{descriptor}},
		},
		Types:       map[string]resource.Type{"cache": typ},
		Definitions: map[string]resource.Definition{def.Key: def},
	})
	return err
}

// An executor that runs module code gets only a just-verified artifact;
// verification failure or a missing verifier stops the node before the call.
func TestProvision_VerifiesModuleImmediatelyBeforeExecution(t *testing.T) {
	ctx := context.Background()
	opts := seedtest.Defaults()
	for name, verifier := range map[string]*stubVerifier{"drift": {err: moduleregistry.ErrDrift}, "binary": {err: moduleregistry.ErrBinaryUnverified}, "missing verifier": nil} {
		app, err := bootstraptest.Build(ctx, bootstrap.Options{Seed: opts, Adapters: bootstrap.AdapterFake})
		if err != nil {
			t.Fatal(err)
		}
		saveAWSConnections(t, app, opts.OrganizationKey, "aws-a")
		exec := &moduleExecutor{}
		var port moduleregistry.ExecutionVerifier
		if verifier != nil {
			port = verifier
		}
		err = runVerified(t, app, opts.OrganizationKey, port, exec)
		if err == nil || len(exec.requests) != 0 {
			t.Fatalf("%s: err=%v executor calls=%d", name, err, len(exec.requests))
		}
		if verifier != nil && (!errors.Is(err, verifier.err) || verifier.calls != 1) {
			t.Fatalf("%s: err=%v calls=%d", name, err, verifier.calls)
		}
	}

	app, err := bootstraptest.Build(ctx, bootstrap.Options{Seed: opts, Adapters: bootstrap.AdapterFake})
	if err != nil {
		t.Fatal(err)
	}
	saveAWSConnections(t, app, opts.OrganizationKey, "aws-a")
	exec, verifier := &moduleExecutor{}, &stubVerifier{}
	if err := runVerified(t, app, opts.OrganizationKey, verifier, exec); err != nil {
		t.Fatal(err)
	}
	if len(exec.requests) != 1 || exec.requests[0].Artifact == nil || exec.requests[0].Artifact.TerraformBinary == "" {
		t.Fatalf("executor did not receive the verified artifact: %+v", exec.requests)
	}
	inputs := exec.requests[0].Inputs
	if inputs["memory_mb"] != 2048 || inputs["size"] != nil {
		t.Fatalf("params must reach module variables only through input_mapping: %v", inputs)
	}
	active, _ := app.Store.ListActiveResources(ctx, opts.OrganizationKey)
	for _, a := range active {
		if a.Descriptor.Type != "cache" {
			continue
		}
		if a.Outputs["host"] != "cache.internal" || a.Outputs["internal_arn"] != nil {
			t.Fatalf("outputs not mapped: %v", a.Outputs)
		}
		ref, ok := a.Outputs["password"].(map[string]any)
		if !ok || ref["secretRef"] == nil || strings.Contains(stringify(a.Outputs), "s3cr3t-value") {
			t.Fatalf("a module-sensitive output was stored in clear: %v", a.Outputs)
		}
		return
	}
	t.Fatal("no cache ActiveResource")
}

func stringify(m map[string]any) string {
	var b strings.Builder
	for k, v := range m {
		b.WriteString(k)
		if s, ok := v.(string); ok {
			b.WriteString(s)
		}
	}
	return b.String()
}
