package catalog_test

import (
	"context"
	"errors"
	"testing"

	"orchestrator/internal/application/catalog"
	"orchestrator/internal/domain/resource"
)

// Module contract: EKS public_access_cidrs are explicit IPv4 /32 hosts chosen
// by the PE/Admin. Registration refuses wide prefixes, 0.0.0.0, empty lists,
// duplicates and non-literal garbage; it never adds an address by itself.
func TestRegisterEKSDefinitionRequiresExplicitIPv4HostAllowlist(t *testing.T) {
	ctx := context.Background()
	_, svc, opts := seededCatalog(t, withRegistry)
	register := func(key string, cidrs any) error {
		def := seededDefinition(t, opts, "cluster-aws-eks")
		def.Key = key
		def.Criteria = []resource.Criterion{{Class: key}}
		values := def.DriverInputs["values"].(map[string]any)
		variables := map[string]any{}
		for name, value := range values["variables"].(map[string]any) {
			variables[name] = value
		}
		if cidrs == nil {
			delete(variables, "public_access_cidrs")
		} else {
			variables["public_access_cidrs"] = cidrs
		}
		values["variables"] = variables
		_, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, def)
		return err
	}
	if err := register("eks-ok", []any{"203.0.113.10/32", "198.51.100.7/32"}); err != nil {
		t.Fatalf("explicit hosts: %v", err)
	}
	for name, value := range map[string]any{
		"missing":      nil,
		"empty":        []any{},
		"world":        []any{"0.0.0.0/0"},
		"wide":         []any{"203.0.113.0/24"},
		"zero-host":    []any{"0.0.0.0/32"},
		"bad-octet":    []any{"203.0.113.300/32"},
		"ipv6":         []any{"2001:db8::1/128"},
		"no-prefix":    []any{"203.0.113.10"},
		"duplicate":    []any{"203.0.113.10/32", "203.0.113.10/32"},
		"not-a-string": []any{float64(1)},
	} {
		if err := register("eks-"+name, value); !errors.Is(err, catalog.ErrInvalid) {
			t.Errorf("%s: want ErrInvalid, got %v", name, err)
		}
	}
	// A whole placeholder defers the check to resolution; it is not an executor-IP shortcut.
	if err := register("eks-placeholder", "${context.env.profile}"); err != nil {
		t.Errorf("whole placeholder: %v", err)
	}
}

// ADR-017: a cluster created by a verified EKS module registers with a Resource Type whose typed `cluster` output
// is required but produced by the platform. A module output can never be mapped onto it.
func TestRegisterEKSDefinitionLeavesTheTypedClusterOutputToThePlatform(t *testing.T) {
	ctx := context.Background()
	st, svc, opts := seededCatalog(t, withRegistry)
	if err := st.SaveResourceType(ctx, opts.OrganizationKey, resource.Type{Key: "k8s-cluster", Outputs: []resource.OutputField{{Name: "name", Type: "string"}, {Name: "cluster", Type: "any", Required: true}}}); err != nil {
		t.Fatal(err)
	}
	register := func(key string, mapping map[string]any) error {
		def := seededDefinition(t, opts, "cluster-aws-eks")
		def.Key = key
		def.Criteria = []resource.Criterion{{Class: key}}
		def.DriverInputs["values"].(map[string]any)["output_mapping"] = mapping
		_, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, def)
		return err
	}
	if err := register("eks-platform-target", map[string]any{"name": "name"}); err != nil {
		t.Fatalf("a cluster Definition that maps only the name must register: %v", err)
	}
	err := register("eks-module-target", map[string]any{"name": "name", "endpoint": "cluster"})
	if !errors.Is(err, catalog.ErrInvalid) {
		t.Fatalf("a module output mapped onto the platform-owned target must be refused: %v", err)
	}
}
