package provisioning

import (
	"context"
	"reflect"
	"testing"

	"orchestrator/internal/adapters/echo"
	"orchestrator/internal/adapters/secrets"
	"orchestrator/internal/adapters/store"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/endpoint"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning"
	"orchestrator/internal/platform/clock"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/seed/seedtest"
)

type echoGatewayRegistry struct{ calls []execution.ProvisionRequest }

func (r *echoGatewayRegistry) Resolve(driver resource.DriverType) (execution.ResourceExecutor, error) {
	return r, nil
}
func (r *echoGatewayRegistry) Provision(ctx context.Context, req execution.ProvisionRequest) (execution.ProvisionResult, error) {
	r.calls = append(r.calls, req)
	return (echo.Executor{}).Provision(ctx, req)
}

func TestEchoGatewayChecksPinsWithoutPassingConnectionOrTakingOwnership(t *testing.T) {
	for _, mode := range []string{"valid-upgrade", "valid-eks", "stale-pin", "foreign-target", "foreign-descriptor", "secret-metadata", "developer-override"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			st := store.New()
			opts := seedtest.Defaults()
			if err := seedtest.Apply(ctx, st, opts); err != nil {
				t.Fatal(err)
			}
			account := application.Connection{Key: "echo-lab", ID: "echo-lab", Name: "Echo lab", OrganizationKey: opts.OrganizationKey, Kind: application.ConnectionKubernetes, AuthenticationType: application.AuthKubeconfig, Status: application.ConnectionReady}
			if mode == "valid-eks" {
				account.Kind = application.ConnectionAWS
				account.AuthenticationType = application.AuthAWSAccessKey
			}
			if err := st.SaveConnection(ctx, account); err != nil {
				t.Fatal(err)
			}
			account, err := st.GetConnection(ctx, opts.OrganizationKey, account.Key)
			if err != nil {
				t.Fatal(err)
			}
			cluster := "k8s-cluster.internal#environments.shop.staging"
			descriptor := "k8s-gateway.default#environments.shop.staging"
			ref := "k8s-cluster.internal#environments.@app.@env"
			target := execution.Target{Kind: "kubernetes", Organization: opts.OrganizationKey, Connection: account.Key, ClusterName: "lab", Context: "lab-context"}
			if mode == "valid-eks" {
				target.Provider = "eks"
				target.Region = "us-east-1"
				target.Context = ""
			}
			output := target.ClusterOutput(cluster, account.Version)
			metadata := map[string]any{"cluster": "${resources['" + ref + "'].outputs.cluster}", "rootDomain": "apps.example.com", "scheme": "http", "gatewayName": "public", "gatewayNamespace": "gateway-system"}
			def := resource.Definition{Key: "echo-gateway", Name: "Echo Gateway", ResourceTypeKey: endpoint.GatewayType, DriverType: resource.DriverEcho, Version: 1, DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{"gateway": metadata}}}}
			typ := resource.Type{Key: endpoint.GatewayType, Outputs: []resource.OutputField{{Name: "gateway", Type: "any", Required: true}}}
			node := planning.Node{Descriptor: descriptor, Kind: planning.NodeResource, ResourceType: endpoint.GatewayType, Scope: resource.Scope{Type: resource.ScopeEnvironment, ID: "shop.staging"}, Bindings: map[string]string{ref: cluster}}
			plan := &planning.Plan{Graph: planning.Graph{Nodes: []planning.Node{node, {Descriptor: cluster, Kind: planning.NodeResource, ResourceType: planning.TypeCluster}}, Edges: []planning.Edge{{Consumer: descriptor, Provider: cluster}}}, Matches: map[string]planning.Match{descriptor: {DefinitionKey: def.Key, DriverType: resource.DriverEcho}}, Placements: map[string]planning.Placement{descriptor: {Cluster: cluster, Account: account.Key, AccountVersion: account.Version}}, Batches: [][]string{{descriptor}}}
			prior := &Result{Outputs: map[string]map[string]any{cluster: {"cluster": output, "name": "lab"}}, Targets: map[string]execution.Target{cluster: target}, Instances: map[string]string{}, Secrets: map[string]map[string]bool{}, NodeClusters: map[string]execution.ClusterRef{}}
			switch mode {
			case "stale-pin":
				p := plan.Placements[descriptor]
				p.AccountVersion++
				plan.Placements[descriptor] = p
			case "foreign-target":
				output["accountVersion"] = account.Version + 1
			case "foreign-descriptor":
				output["descriptor"] = "k8s-cluster.other#environments.shop.staging"
			case "secret-metadata":
				prior.Secrets[cluster] = map[string]bool{"cluster": true}
			case "developer-override":
				plan.Graph.Nodes[0].Params = map[string]any{"gateway": map[string]any{"gatewayName": "other"}}
			}
			parsed, _ := resource.ParseDescriptor(descriptor)
			old, err := st.UpsertActiveResource(ctx, resource.ActiveResource{OrganizationKey: opts.OrganizationKey, Descriptor: parsed, Scope: node.Scope, DefinitionKey: "legacy-gateway", ConnectionKey: account.Key, Status: resource.StatusReady, ExecutorState: map[string]any{"driver": "existing-gateway", "uid": "retained"}})
			if err != nil {
				t.Fatal(err)
			}
			registry := &echoGatewayRegistry{}
			svc := NewService(st, registry, secrets.NewMemory(), clock.System{})
			result, err := svc.Provision(ctx, Request{DeploymentID: "echo-test", Context: planning.Context{OrganizationKey: opts.OrganizationKey, App: application.Application{Key: "shop"}, Env: environment.Environment{Key: "staging"}}, Plan: plan, Types: map[string]resource.Type{typ.Key: typ}, Definitions: map[string]resource.Definition{def.Key: def}, Prior: prior})
			if mode != "valid-upgrade" && mode != "valid-eks" {
				if err == nil || len(registry.calls) != 0 {
					t.Fatalf("invalid metadata executed: calls=%d err=%v", len(registry.calls), err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(registry.calls) != 1 {
				t.Fatal("Echo not executed exactly once")
			}
			received := registry.calls[0]
			if received.Connection.Key != "" || !reflect.DeepEqual(received.Target, execution.Target{}) {
				t.Fatal("Echo received infrastructure access")
			}
			if result.Instances[descriptor] != "" || !reflect.DeepEqual(result.Targets[descriptor], execution.Target{}) {
				t.Fatal("Echo took physical ownership")
			}
			active, err := st.FindByLogicalIdentity(ctx, opts.OrganizationKey, parsed, node.Scope)
			if err != nil {
				t.Fatal(err)
			}
			if active.ID != old.ID || active.ConnectionKey != account.Key || active.DefinitionKey != def.Key {
				t.Fatal("explicit metadata upgrade lost logical identity/provenance")
			}
		})
	}
}
