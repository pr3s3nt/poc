package provisioning

import (
	"context"
	"encoding/json"
	"orchestrator/internal/adapters/secrets"
	"orchestrator/internal/adapters/store"
	"orchestrator/internal/application/tfexec"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/planning"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence/persistencetest"
	"strings"
	"testing"
)

func TestRepairInputBindingsUseFrozenDefinitionAndFreshProviderOutputs(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	binding := persistencetest.DeliveryBindings(t, st)
	persistencetest.DeliverySelections(t, st, binding)
	app, _ := st.GetApplication(ctx, "delivery-app")
	env, _ := st.GetEnvironment(ctx, app.Key, "staging")
	scope := resource.Scope{Type: resource.ScopeEnvironment, ID: app.Key + ".staging"}
	source := tfmodule.SourceRef{SourceID: "source", ModuleID: "module", Revision: "v1", Subdirectory: ".", ContentSHA256: strings.Repeat("a", 64)}
	provider := "redis.default#shared.cache"
	consumer := "postgres.default#shared.db"
	definition := resource.Definition{Key: "original-definition", DriverType: resource.DriverTerraform, DriverInputs: map[string]any{"values": map[string]any{"source": source.Map(), "variables": map[string]any{"host": "${resources['redis.default#shared.cache'].outputs.host}", "note": "frozen"}, "input_mapping": map[string]any{}, "output_mapping": map[string]any{}, "provider_bindings": map[string]any{}}}}
	plan := planning.Plan{Graph: planning.Graph{Nodes: []planning.Node{{Descriptor: provider, Kind: planning.NodeResource, ResourceType: "redis", Scope: scope}, {Descriptor: consumer, Kind: planning.NodeResource, ResourceType: "postgres", Scope: scope, Bindings: map[string]string{provider: provider}}}}, Matches: map[string]planning.Match{consumer: {DefinitionKey: definition.Key, DriverType: resource.DriverTerraform}}, DefinitionSnapshots: []resource.Definition{definition}}
	body, _ := json.Marshal(plan)
	var snapshot map[string]any
	_ = json.Unmarshal(body, &snapshot)
	deployment := ids.New()
	if err := st.SavePlan(ctx, deployment, snapshot); err != nil {
		t.Fatal(err)
	}
	desc, _ := resource.ParseDescriptor(provider)
	active, err := st.UpsertActiveResource(ctx, resource.ActiveResource{OrganizationKey: app.OrganizationKey, Descriptor: desc, Scope: scope, Status: resource.StatusReady, Outputs: map[string]any{"host": "new-provider.internal"}})
	if err != nil {
		t.Fatal(err)
	}
	resolver := &RepairBindings{Provisioning: &Service{store: st, secrets: secrets.NewMemory()}}
	original := tfexec.ExecuteRequest{DeploymentID: deployment, OrganizationKey: app.OrganizationKey, Descriptor: consumer, Scope: scope, Identity: tfresource.Identity{OrganizationID: binding.OrganizationID, ApplicationID: app.ID, EnvironmentID: env.ID, TargetGeneration: env.TargetGeneration, Descriptor: consumer, Scope: scope}, Source: source, OutputMapping: map[string]string{}, Inputs: map[string]any{"host": "old-provider.internal", "note": "frozen"}}
	resolved, err := resolver.ResolveRepairInputs(ctx, original)
	if err != nil || resolved.Inputs["host"] != "new-provider.internal" || resolved.Inputs["note"] != "frozen" {
		t.Fatal("pinned input binding not resolved", err)
	}
	// A runtime secret classification cannot be downgraded by a public variable.
	secretStore := resolver.Provisioning.secrets
	ref, err := secretStore.Put(ctx, "private-host", "must-not-leak")
	if err != nil {
		t.Fatal(err)
	}
	active.Outputs["host"] = map[string]any{"secretRef": ref}
	if _, err := st.UpsertActiveResource(ctx, active); err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.ResolveRepairInputs(ctx, original); err == nil {
		t.Fatal("secret output entered a public module input")
	}
	// A missing original plan is never reconstructed from today's catalog.
	original.DeploymentID = ids.New()
	if _, err := resolver.ResolveRepairInputs(ctx, original); err == nil {
		t.Fatal("missing applied plan was accepted")
	}
}
