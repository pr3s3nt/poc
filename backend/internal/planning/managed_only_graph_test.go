package planning

import (
	"strings"
	"testing"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
)

func TestManagedOnlyGraphContainsRequestedResourcesAndNetwork(t *testing.T) {
	ctx := Context{
		OrganizationKey: "acme",
		App:             application.Application{Key: "shop"},
		Env:             environment.Environment{Key: "dev", Type: "development"},
		RunID:           "managed-only-test",
	}
	const (
		vpc    = "vpc.default#environments.shop.dev"
		aurora = "postgres.cloud-managed#shared.db"
		cache  = "redis.cloud-managed#shared.cache"
		vpcRef = "${resources['vpc.default#environments.@app.@env'].outputs.id}"
	)
	types := map[string]resource.Type{
		"vpc":      {Key: "vpc"},
		"postgres": {Key: "postgres"},
		"redis":    {Key: "redis"},
	}
	definition := func(key, typ, class string, inputs map[string]any) resource.Definition {
		return resource.Definition{
			Key: key, Name: key, ResourceTypeKey: typ, DriverType: resource.DriverEcho, Version: 1,
			Criteria:     []resource.Criterion{{Class: class}},
			DriverInputs: inputs,
		}
	}
	catalog := Catalog{
		Types: types,
		Definitions: []resource.Definition{
			definition("network", "vpc", "default", nil),
			definition("managed-aurora", "postgres", "cloud-managed", map[string]any{"network": vpcRef}),
			definition("managed-cache", "redis", "cloud-managed", map[string]any{"network": vpcRef}),
		},
	}
	builder := newGraphBuilder(ctx, catalog)
	for _, descriptor := range []string{vpc, aurora, cache} {
		d, err := resource.ParseDescriptor(descriptor)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := builder.ensure(d, NodeResource, OriginShared, nil); err != nil {
			t.Fatal(err)
		}
	}
	matcher, err := newDefinitionMatcher(ctx, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := builder.expand(matcher, map[string]Match{}, nil); err != nil {
		t.Fatal(err)
	}
	graph := builder.graph()
	wantNodes := map[string]bool{vpc: false, aurora: false, cache: false}
	for _, node := range graph.Nodes {
		if _, wanted := wantNodes[node.Descriptor]; !wanted {
			t.Fatalf("managed-only graph inserted an unrequested node: %+v", node)
		}
		wantNodes[node.Descriptor] = true
		if strings.HasPrefix(node.Descriptor, "workload.") || strings.HasPrefix(node.Descriptor, "k8s-namespace.") || strings.HasPrefix(node.Descriptor, "k8s-cluster.eks") {
			t.Fatalf("managed-only graph inserted workload, namespace or EKS: %+v", node)
		}
	}
	for descriptor, found := range wantNodes {
		if !found {
			t.Fatalf("managed-only graph omitted requested node %s: %+v", descriptor, graph.Nodes)
		}
	}
	for _, consumer := range []string{aurora, cache} {
		found := false
		for _, edge := range graph.Edges {
			if edge.Consumer == consumer && edge.Provider == vpc && edge.Reason == ReasonReference {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s must reference the requested network: %+v", consumer, graph.Edges)
		}
	}
}
