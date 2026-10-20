package catalog_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"orchestrator/internal/adapters/terraform/registry"
	"orchestrator/internal/application/catalog"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
	"reflect"
)

func k8sDefinition(key, typ string, ref tfmodule.SourceRef, variables map[string]any, inputs, outputs []string, criteria resource.Criterion) resource.Definition {
	inputMapping := map[string]any{}
	for _, name := range inputs {
		inputMapping[name] = map[string]any{"param": name}
	}
	outputMapping := map[string]any{}
	for _, name := range outputs {
		outputMapping[name] = name
	}
	return resource.Definition{
		Key: key, Name: key, Version: 1, ResourceTypeKey: typ, DriverType: resource.DriverTerraform,
		DriverInputs: map[string]any{"values": map[string]any{
			"source": ref.Map(), "variables": variables, "input_mapping": inputMapping, "output_mapping": outputMapping,
			"provider_bindings": map[string]any{"kubernetes": map[string]any{"target": tfmodule.TargetResource, "resource_ref": "k8s-cluster.eks#environments.@app.@env"}},
		}},
		Criteria: []resource.Criterion{criteria},
	}
}

// The packaged Kubernetes modules register through the public Definition path
// over their real verified bundles; Redis keeps its Type contract, and
// credentials, unknown variables and a Workload/Developer selector are refused.
func TestRegisterKubernetesModuleDefinitions(t *testing.T) {
	ctx := context.Background()
	_, svc, opts := seededCatalog(t, withRegistry)
	refs, err := registry.PackagedRefs()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RegisterResourceType(ctx, opts.OrganizationKey, resource.Type{Key: "redis", Outputs: []resource.OutputField{
		{Name: "host", Type: "string", Required: true}, {Name: "port", Type: "number", Required: true}, {Name: "username", Type: "string", Required: true},
		{Name: "password", Type: "string", Required: true, Secret: true}, {Name: "tls", Type: "bool", Required: true}}}); err != nil {
		t.Fatal(err)
	}
	ns := "${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}"
	redisOut := []string{"host", "port", "username", "password", "tls"}
	pgOut := []string{"host", "port", "database", "username", "password"}
	ok := map[string]resource.Definition{
		"namespace": k8sDefinition("ns-module", "k8s-namespace", refs["k8s-namespace"], map[string]any{"name": "${context.env.namespace}"}, nil, []string{"name"}, resource.Criterion{ApplicationID: "t1"}),
		"postgres":  k8sDefinition("pg-module", "postgres", refs["postgres-k8s"], map[string]any{"namespace": ns}, []string{"database", "username"}, pgOut, resource.Criterion{ApplicationID: "t1"}),
		"redis":     k8sDefinition("redis-module", "redis", refs["redis-k8s"], map[string]any{"namespace": ns}, nil, redisOut, resource.Criterion{ApplicationID: "t1"}),
		// Test graph fixture: a nonsecret annotation bound to the shared PostgreSQL output.
		"redis fixture": k8sDefinition("redis-fixture", "redis", refs["redis-k8s"], map[string]any{"namespace": ns,
			"annotations": map[string]any{"example.test/postgres-host": "${resources['postgres.default#shared.acceptance-db'].outputs.host}"}}, nil, redisOut, resource.Criterion{ApplicationID: "t2"}),
		// E02 fault setting: a well-formed digest that does not exist.
		"redis fault": k8sDefinition("redis-fault", "redis", refs["redis-k8s"], map[string]any{"namespace": ns,
			"image": "redis:7.4.7-alpine@sha256:095bac20aafb160ab173e6fed571218d4feba5644f7189f3480fd1b07b52ca03"}, nil, redisOut, resource.Criterion{ApplicationID: "t3"}),
	}
	for name, def := range ok {
		if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, def); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	bad := map[string]resource.Definition{
		"password literal":      k8sDefinition("bad-1", "redis", refs["redis-k8s"], map[string]any{"namespace": ns, "password": "chosen"}, nil, redisOut, resource.Criterion{ApplicationID: "b"}),
		"tags override":         k8sDefinition("bad-2", "redis", refs["redis-k8s"], map[string]any{"namespace": ns, "tags": map[string]any{"a": "b"}}, nil, redisOut, resource.Criterion{ApplicationID: "b"}),
		"unknown variable":      k8sDefinition("bad-3", "redis", refs["redis-k8s"], map[string]any{"namespace": ns, "nodeSelector": "x"}, nil, redisOut, resource.Criterion{ApplicationID: "b"}),
		"password param":        k8sDefinition("bad-4", "redis", refs["redis-k8s"], map[string]any{"namespace": ns}, []string{"password"}, redisOut, resource.Criterion{ApplicationID: "b"}),
		"image param selector":  k8sDefinition("bad-5", "redis", refs["redis-k8s"], map[string]any{"namespace": ns}, []string{"image"}, redisOut, resource.Criterion{ApplicationID: "b"}),
		"missing namespace":     k8sDefinition("bad-6", "redis", refs["redis-k8s"], map[string]any{}, nil, redisOut, resource.Criterion{ApplicationID: "b"}),
		"missing output":        k8sDefinition("bad-7", "redis", refs["redis-k8s"], map[string]any{"namespace": ns}, nil, []string{"host", "port", "username", "password"}, resource.Criterion{ApplicationID: "b"}),
		"tls from wrong output": k8sDefinition("bad-8", "redis", refs["redis-k8s"], map[string]any{"namespace": ns}, nil, []string{"host", "port", "username", "password", "port"}, resource.Criterion{ApplicationID: "b"}),
		"postgres without user": k8sDefinition("bad-9", "postgres", refs["postgres-k8s"], map[string]any{"namespace": ns}, []string{"database"}, pgOut, resource.Criterion{ApplicationID: "b"}),
		"wrong type for module": k8sDefinition("bad-10", "redis", refs["postgres-k8s"], map[string]any{"namespace": ns}, nil, redisOut, resource.Criterion{ApplicationID: "b"}),
	}
	// "tls from wrong output" maps two outputs to one name; build it explicitly.
	wrong := bad["tls from wrong output"]
	values := wrong.DriverInputs["values"].(map[string]any)
	values["output_mapping"] = map[string]any{"host": "host", "port": "port", "username": "username", "password": "password", "username2": "tls"}
	for name, def := range bad {
		_, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, def)
		if !errors.Is(err, catalog.ErrInvalid) {
			t.Errorf("%s was not rejected: %v", name, err)
			continue
		}
		if strings.Contains(err.Error(), "chosen") {
			t.Errorf("%s echoes a value: %v", name, err)
		}
	}
}

// The authoring metadata of the Kubernetes modules lists only author-controlled
// variables (never the executor-owned credential or tags), marks the Type
// params, pins the image digest default and offers the namespace presets.
func TestListDefinitionCapabilities_KubernetesModules(t *testing.T) {
	ctx := context.Background()
	_, svc, opts := seededCatalog(t, withRegistry)
	caps, err := svc.ListDefinitionCapabilities(ctx, opts.OrganizationKey)
	if err != nil {
		t.Fatal(err)
	}
	idx := capabilityIndex(caps)
	nsPreset := "${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}"
	for key, want := range map[string]struct {
		owned []string
		vars  []string
	}{
		"k8s-namespace/terraform/k8s-namespace": {[]string{"tags"}, []string{"name"}},
		"redis/terraform/redis-k8s":             {[]string{"password", "tags"}, []string{"annotations", "image", "labels", "namespace", "storage", "storage_class", "username"}},
		"postgres/terraform/postgres-k8s":       {[]string{"password", "tags"}, []string{"annotations", "database", "image", "labels", "namespace", "storage", "storage_class", "username"}},
	} {
		c, ok := idx[key]
		if !ok {
			t.Fatalf("missing capability %s", key)
		}
		var names []string
		for _, v := range c.Variables {
			names = append(names, v.Name)
		}
		if !reflect.DeepEqual(c.ExecutorOwned, want.owned) || !reflect.DeepEqual(names, want.vars) {
			t.Errorf("%s owned %v variables %v", key, c.ExecutorOwned, names)
		}
		if len(c.Providers) != 1 || c.Providers[0].Binding != tfmodule.BindingKubernetes || c.Providers[0].Version != "2.38.0" {
			t.Errorf("%s providers %+v", key, c.Providers)
		}
	}
	if v := variableIndex(idx["k8s-namespace/terraform/k8s-namespace"])["name"]; !v.Required || v.Preset != "${context.env.namespace}" {
		t.Errorf("namespace name %+v", v)
	}
	redis := variableIndex(idx["redis/terraform/redis-k8s"])
	if v := redis["namespace"]; !v.Required || v.Preset != nsPreset {
		t.Errorf("redis namespace %+v", v)
	}
	if v := redis["image"]; v.Required || !strings.Contains(v.Default.(string), "@sha256:") {
		t.Errorf("redis image %+v", v)
	}
	pg := variableIndex(idx["postgres/terraform/postgres-k8s"])
	for _, name := range []string{"database", "username"} {
		if v := pg[name]; !v.Required || !v.ResourceParam {
			t.Errorf("postgres %s %+v", name, v)
		}
	}
	if v := pg["image"]; v.ResourceParam || !strings.Contains(v.Default.(string), "@sha256:") {
		t.Errorf("postgres image is a form param or unpinned: %+v", v)
	}
}

// A Resource Type secret output is refused as a Definition value wherever the
// placeholder sits (labels/annotations map, list, concatenated text); the
// nonsecret host of the same provider registers (selected-set fixture).
func TestRegisterKubernetesModuleDefinitionRefusesSecretOutputReferences(t *testing.T) {
	ctx := context.Background()
	_, svc, opts := seededCatalog(t, withRegistry)
	refs, _ := registry.PackagedRefs()
	if _, err := svc.RegisterResourceType(ctx, opts.OrganizationKey, resource.Type{Key: "redis", Outputs: []resource.OutputField{{Name: "host", Type: "string", Required: true}}}); err != nil {
		t.Fatal(err)
	}
	ns := "${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}"
	password := "${resources['postgres.default#shared.acceptance-db'].outputs.password}"
	host := "${resources['postgres.default#shared.acceptance-db'].outputs.host}"
	secretValues := map[string]map[string]any{
		"annotations direct":       {"annotations": map[string]any{"example.test/k": password}},
		"annotations interpolated": {"annotations": map[string]any{"example.test/k": "pw=" + password}},
		"labels direct":            {"labels": map[string]any{"example.test/k": password}},
		"annotations after host":   {"annotations": map[string]any{"example.test/a": host, "example.test/b": "x" + host + password}},
	}
	i := 0
	for name, extra := range secretValues {
		i++
		variables := map[string]any{"namespace": ns}
		for k, v := range extra {
			variables[k] = v
		}
		def := k8sDefinition("redis-secret-"+string(rune('a'+i)), "redis", refs["redis-k8s"], variables, nil, []string{"host"}, resource.Criterion{ApplicationID: "s" + string(rune('a'+i))})
		_, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, def)
		if !errors.Is(err, catalog.ErrInvalid) || !strings.Contains(err.Error(), "secret output") || strings.Contains(err.Error(), "${") {
			t.Errorf("%s: %v", name, err)
		}
	}
	ok := k8sDefinition("redis-host-fixture", "redis", refs["redis-k8s"], map[string]any{"namespace": ns, "annotations": map[string]any{"example.test/postgres-host": "db=" + host}}, nil, []string{"host"}, resource.Criterion{ApplicationID: "fixture"})
	if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, ok); err != nil {
		t.Fatalf("nonsecret host fixture: %v", err)
	}
}
