package catalog_test

import (
	"context"
	"encoding/json"
	"errors"
	"orchestrator/internal/adapters/store"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/ports/moduleregistry"
	"reflect"
	"strings"
	"testing"

	"orchestrator/internal/adapters/terraform/registry/registrytest"
	"orchestrator/internal/application/catalog"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/resource"
)

func capabilityIndex(caps []catalog.Capability) map[string]catalog.Capability {
	out := map[string]catalog.Capability{}
	for _, c := range caps {
		out[c.ResourceType+"/"+c.DriverType+"/"+c.Module] = c
	}
	return out
}

func variableIndex(c catalog.Capability) map[string]catalog.CapabilityVariable {
	out := map[string]catalog.CapabilityVariable{}
	for _, v := range c.Variables {
		out[v.Name] = v
	}
	return out
}

// TestListDefinitionCapabilities_PairsAndVariables covers UC-03 BR-23..25.
func TestListDefinitionCapabilities_PairsAndVariables(t *testing.T) {
	ctx := context.Background()
	_, svc, opts := seededCatalog(t, withRegistry)
	caps, err := svc.ListDefinitionCapabilities(ctx, opts.OrganizationKey)
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, c := range caps {
		keys = append(keys, c.ResourceType+"/"+c.DriverType+"/"+c.Module)
	}
	want := []string{"k8s-cluster/terraform/eks", "k8s-namespace/kubernetes/", "k8s-namespace/terraform/k8s-namespace", "postgres/kubernetes/", "postgres/terraform/aurora", "postgres/terraform/postgres-k8s", "redis/terraform/elasticache", "redis/terraform/redis-k8s", "vpc/terraform/vpc"}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("pairs %v, want %v", keys, want)
	}
	idx := capabilityIndex(caps)

	ns := variableIndex(idx["k8s-namespace/kubernetes/"])
	if v := ns["name"]; !v.Required || v.Preset != "${context.env.namespace}" || v.HasDefault || v.ResourceParam {
		t.Fatalf("namespace name %+v", v)
	}
	pg := variableIndex(idx["postgres/kubernetes/"])
	if v := pg["storage"]; v.Required || !v.HasDefault || v.Default != "1Gi" {
		t.Fatalf("storage %+v", v)
	}
	if v := pg["database"]; v.Default != "app" || !v.ResourceParam {
		t.Fatalf("database %+v (Resource Type declares database)", v)
	}
	if v := pg["image"]; v.Default != "postgres:16-alpine" || v.ResourceParam {
		t.Fatalf("image %+v", v)
	}

	vpc := idx["vpc/terraform/vpc"]
	vv := variableIndex(vpc)
	if v := vv["cidr"]; v.Required || !v.HasDefault || v.Default != "10.42.0.0/16" || v.Type != "string" {
		t.Fatalf("vpc cidr %+v", v)
	}
	if v := vv["az_count"]; v.Type != "number" || v.Default != float64(2) {
		t.Fatalf("vpc az_count %+v", v)
	}
	if v := vv["name"]; !v.Required || v.Preset != "${context.infra.resourceName}" {
		t.Fatalf("vpc name %+v", v)
	}
	eks := variableIndex(idx["k8s-cluster/terraform/eks"])
	if v := eks["subnet_ids"]; v.Type != "list(string)" || v.Preset != "${resources['vpc.default#@infra'].outputs.subnetIds}" {
		t.Fatalf("eks subnet_ids %+v", v)
	}
	// The EKS public API allowlist is a PE-owned required list: no default, no
	// preset (nothing infers an address) and not a Developer Workload param.
	if v := eks["public_access_cidrs"]; v.Type != "list(string)" || !v.Required || v.HasDefault || v.Default != nil || v.Preset != "" || v.ResourceParam {
		t.Fatalf("eks public_access_cidrs %+v", v)
	}
	aurora := variableIndex(idx["postgres/terraform/aurora"])
	for name, output := range map[string]string{"vpc_id": "id", "private_subnet_ids": "privateSubnetIds", "subnet_ids": "subnetIds"} {
		if aurora[name].Preset != "${resources['vpc.default#@infra'].outputs."+output+"}" {
			t.Fatalf("aurora %s preset %q", name, aurora[name].Preset)
		}
	}
	if v := aurora["min_capacity"]; v.Default != nil || v.HasDefault || !v.Required {
		t.Fatalf("no invented capacity default: %+v", v)
	}
	if v := aurora["engine_version"]; v.HasDefault {
		t.Fatalf("no invented engine default: %+v", v)
	}
	if v := aurora["database"]; !v.ResourceParam || !v.Required {
		t.Fatalf("aurora database %+v", v)
	}

	// Executor-owned variables never appear as editable variables.
	for key, c := range idx {
		for _, name := range []string{"region", "tags", "master_password"} {
			if _, ok := variableIndex(c)[name]; ok {
				t.Errorf("%s exposes executor-owned %s", key, name)
			}
		}
		if c.ExecutorOwned == nil || c.Variables == nil {
			t.Errorf("%s has nil slices", key)
		}
	}
	if got := idx["postgres/terraform/aurora"].ExecutorOwned; !reflect.DeepEqual(got, []string{"master_password", "region", "tags"}) {
		t.Fatalf("aurora executor-owned %v", got)
	}
	if got := idx["vpc/terraform/vpc"].ExecutorOwned; !reflect.DeepEqual(got, []string{"region", "tags"}) {
		t.Fatalf("vpc executor-owned %v", got)
	}

	// Public metadata carries the controlled pin (IDs, revision, digest) but
	// no URL, cache/file path, credential or internal fingerprint content.
	raw, _ := json.Marshal(caps)
	for _, leak := range []string{"fingerprint", "main.tf", "modules/", "secret", "credential", `"path"`, "https://", "bundles/", "sources/", "/tmp", "lock.hcl", "h1:", "zh:"} {
		if strings.Contains(strings.ToLower(string(raw)), leak) {
			t.Errorf("metadata leaks %q: %s", leak, raw)
		}
	}
}

// TestListDefinitionCapabilities_ResourceParamIsScoped: a custom Resource Type
// never changes supported pairs and params come only from the scoped type.
func TestListDefinitionCapabilities_ResourceParamIsScoped(t *testing.T) {
	ctx := context.Background()
	st, svc, opts := seededCatalog(t, withRegistry)
	if _, err := svc.RegisterResourceType(ctx, opts.OrganizationKey, resource.Type{Key: "custom-store", Inputs: []resource.InputField{{Name: "name", Type: "string"}}, Outputs: []resource.OutputField{{Name: "x", Type: "string"}}}); err != nil {
		t.Fatal(err)
	}
	caps, err := svc.ListDefinitionCapabilities(ctx, opts.OrganizationKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(caps) != 9 {
		t.Fatalf("custom type changed the supported pairs: %d", len(caps))
	}
	// Another Organization without Resource Types still lists the pairs, no params.
	other, err := svc.ListDefinitionCapabilities(ctx, "other-org")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range other {
		for _, v := range c.Variables {
			if v.ResourceParam {
				t.Errorf("%s.%s marked resourceParam without a scoped type", c.ResourceType, v.Name)
			}
		}
	}
	_ = st
}

// TestListDefinitionCapabilities_ReadOnlyAndSafeErrors: no writes, safe errors.
func TestListDefinitionCapabilities_ReadOnlyAndSafeErrors(t *testing.T) {
	ctx := context.Background()
	st, svc, opts := seededCatalog(t, withRegistry)
	typesBefore, _ := st.ListResourceTypes(ctx, opts.OrganizationKey)
	defsBefore, _ := st.ListResourceDefinitions(ctx, opts.OrganizationKey)
	for i := 0; i < 2; i++ {
		if _, err := svc.ListDefinitionCapabilities(ctx, opts.OrganizationKey); err != nil {
			t.Fatal(err)
		}
	}
	typesAfter, _ := st.ListResourceTypes(ctx, opts.OrganizationKey)
	defsAfter, _ := st.ListResourceDefinitions(ctx, opts.OrganizationKey)
	if !reflect.DeepEqual(typesBefore, typesAfter) || !reflect.DeepEqual(defsBefore, defsAfter) {
		t.Fatal("metadata read changed the catalog")
	}

	_, noInspector, _ := seededCatalog(t, nil)
	if _, err := noInspector.ListDefinitionCapabilities(ctx, opts.OrganizationKey); err == nil || errors.Is(err, catalog.ErrInvalid) {
		t.Fatalf("missing inspector must be an internal error, got %v", err)
	}
}

type brokenRegistry struct{ moduleregistry.Registry }

func (brokenRegistry) List(context.Context) ([]tfmodule.Bundle, error) {
	return nil, errors.New("read /srv/secret-path/cache failed")
}

// An inspector failure is a platform error, never a validation result.
func TestListDefinitionCapabilities_InspectorFailureIsInternal(t *testing.T) {
	_, svc, opts := seededCatalog(t, func(t *testing.T, st *store.Store) moduleregistry.Registry {
		return brokenRegistry{registrytest.Open(t, st)}
	})
	_, err := svc.ListDefinitionCapabilities(context.Background(), opts.OrganizationKey)
	if err == nil || errors.Is(err, catalog.ErrInvalid) {
		t.Fatalf("want internal error, got %v", err)
	}
}

// TestListDefinitionCapabilities_NamespaceNameParamFollowsBR12: the namespace
// name only counts as a Resource Type param when its input is a required string.
func TestListDefinitionCapabilities_NamespaceNameParamFollowsBR12(t *testing.T) {
	ctx := context.Background()
	for name, tc := range map[string]struct {
		input resource.InputField
		want  bool
	}{
		"required string": {resource.InputField{Name: "name", Type: "string", Required: true}, true},
		"optional string": {resource.InputField{Name: "name", Type: "string"}, false},
		"required number": {resource.InputField{Name: "name", Type: "number", Required: true}, false},
	} {
		st, svc, _ := seededCatalog(t, withRegistry)
		if err := st.SaveOrganization(ctx, application.Organization{Key: "scratch-org"}); err != nil {
			t.Fatal(err)
		}
		typ := resource.Type{Key: "k8s-namespace", Inputs: []resource.InputField{tc.input}, Outputs: []resource.OutputField{{Name: "name", Type: "string"}}}
		if err := st.CreateResourceType(ctx, "scratch-org", typ); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		caps, err := svc.ListDefinitionCapabilities(ctx, "scratch-org")
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		got := variableIndex(capabilityIndex(caps)["k8s-namespace/kubernetes/"])["name"]
		if got.ResourceParam != tc.want || !got.Required {
			t.Errorf("%s: %+v want resourceParam=%v", name, got, tc.want)
		}
	}
}
