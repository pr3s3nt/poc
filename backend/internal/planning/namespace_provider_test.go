package planning_test

import (
	"strings"
	"testing"

	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/planning"
)

func namespaceProviderRequest(t *testing.T, mutate func(map[string]any)) planning.Request {
	t.Helper()
	req := k8sRequest(t, false)
	for i := range req.Catalog.Definitions {
		def := &req.Catalog.Definitions[i]
		if def.Key != "postgres-k8s-module" {
			continue
		}
		values := def.DriverValues()
		values["provider_bindings"] = map[string]any{"kubernetes": map[string]any{
			"target": tfmodule.TargetResource, "resource_ref": "k8s-namespace.default#environments.@app.@env",
		}}
		if mutate != nil {
			mutate(values)
		}
	}
	return req
}

func TestNamespaceProviderInheritsPinnedClusterAndDependsOnNamespace(t *testing.T) {
	req := namespaceProviderRequest(t, nil)
	plan, err := planning.NewService(nil).Plan(req)
	if err != nil {
		t.Fatal(err)
	}
	var pg, ns string
	for _, node := range plan.Graph.Nodes {
		if node.ResourceType == "postgres" {
			pg = node.Descriptor
		}
		if node.ResourceType == planning.TypeNamespace {
			ns = node.Descriptor
		}
	}
	if pg == "" || ns == "" {
		t.Fatal("database or namespace is absent")
	}
	for _, contract := range plan.Terraform {
		if contract.Descriptor != pg {
			continue
		}
		pin := contract.ProviderBindings[0]
		if pin.Namespace != ns || pin.Provider != plan.Placements[ns].Cluster || pin.AccountVersion != plan.Placements[ns].AccountVersion {
			t.Fatalf("namespace provenance was not preserved: %+v", pin)
		}
	}
	if plan.Placements[pg].Namespace != ns || plan.Placements[pg].Account != plan.Placements[ns].Account {
		t.Fatalf("database placement does not inherit namespace: %+v", plan.Placements[pg])
	}
	for _, edge := range plan.Graph.Edges {
		if edge.Consumer == pg && edge.Provider == ns && edge.Reason == planning.ReasonProviderBinding {
			return
		}
	}
	t.Fatal("namespace provider dependency is absent")
}

func TestNamespaceProviderRejectsConflictingOrOverridableNamespace(t *testing.T) {
	for name, mutate := range map[string]func(map[string]any){
		"literal": func(v map[string]any) { v["variables"].(map[string]any)["namespace"] = "other" },
		"wrong output": func(v map[string]any) {
			v["variables"].(map[string]any)["namespace"] = "${resources['k8s-namespace.default#environments.@app.@env'].outputs.other}"
		},
		"different namespace": func(v map[string]any) {
			v["variables"].(map[string]any)["namespace"] = "${resources['k8s-namespace.default#environments.@app.@env-other'].outputs.name}"
		},
		"param override": func(v map[string]any) {
			v["input_mapping"].(map[string]any)["namespace"] = map[string]any{"param": "namespace"}
		},
	} {
		t.Run(name, func(t *testing.T) {
			req := namespaceProviderRequest(t, mutate)
			typ := req.Catalog.Types["postgres"]
			typ.Inputs = append(typ.Inputs, resource.InputField{Name: "namespace", Type: "string"})
			req.Catalog.Types["postgres"] = typ
			_, err := planning.NewService(nil).Plan(req)
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), "namespace") {
				t.Fatalf("expected namespace rejection, got %v", err)
			}
		})
	}
}
