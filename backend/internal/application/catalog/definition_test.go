package catalog_test

import (
	"context"
	"errors"
	"testing"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/adapters/terraform/registry/registrytest"
	"orchestrator/internal/application/catalog"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/seed"
	"orchestrator/internal/seed/seedtest"
)

func TestRegisterResourceDefinition_ContractsAndOrganization(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	opts := seedtest.Defaults()
	opts.Region, opts.AccountID = "us-east-1", "000000000000"
	if err := seedtest.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	svc := catalog.NewService(st, registrytest.Open(t, st))
	definitions := seed.ResourceDefinitions(opts)
	var postgres resource.Definition
	for _, candidate := range definitions {
		if candidate.Key == "postgres-aws-aurora" {
			postgres = candidate
			break
		}
	}
	postgres.Key = "postgres-aws-aurora-custom"
	created, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, postgres)
	if err != nil {
		t.Fatalf("register Terraform: %v", err)
	}
	if created.SourceFingerpr == "" {
		t.Fatal("missing module fingerprint")
	}
	if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, postgres); !errors.Is(err, catalog.ErrDuplicate) {
		t.Fatalf("duplicate: %v", err)
	}
	bad := postgres
	bad.Key = "missing-criteria"
	bad.Criteria = nil
	if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, bad); !errors.Is(err, catalog.ErrInvalid) {
		t.Fatalf("criteria: %v", err)
	}
	bad = postgres
	bad.Key = "remote-source"
	bad.DriverInputs = map[string]any{"values": map[string]any{"source": map[string]any{"url": "https://example.invalid/module"}, "variables": map[string]any{}}}
	if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, bad); !errors.Is(err, catalog.ErrInvalid) {
		t.Fatalf("remote source: %v", err)
	}
	bad = postgres
	bad.Key = "wrong-org"
	if _, err := svc.RegisterResourceDefinition(ctx, "other", bad); !errors.Is(err, catalog.ErrInvalid) {
		t.Fatalf("org scope: %v", err)
	}
}

// UC-03 BR-08/BR-11/BR-15, ADR-014: a Kubernetes resource is registered only
// as a verified registry module. The static kubernetes/existing-cluster/echo
// drivers are not registrable, whatever their inputs.
func TestRegisterResourceDefinition_RejectsStaticResourceDrivers(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	opts := seedtest.Defaults()
	if err := seedtest.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	svc := catalog.NewService(st, registrytest.Open(t, st))
	var postgres resource.Definition
	for _, candidate := range seed.ResourceDefinitions(opts) {
		if candidate.Key == "postgres-internal-statefulset" {
			postgres = candidate
			break
		}
	}
	if postgres.DriverType != resource.DriverKubernetes {
		t.Fatalf("the reference seed keeps its read-only shape, got %q", postgres.DriverType)
	}
	for _, driver := range []resource.DriverType{resource.DriverKubernetes, resource.DriverExistingCluster, resource.DriverEcho} {
		for _, typeKey := range []string{"postgres", "k8s-namespace"} {
			def := postgres
			def.Key, def.DriverType, def.ResourceTypeKey = "static-"+typeKey, driver, typeKey
			_, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, def)
			var field *catalog.FieldError
			if !errors.Is(err, catalog.ErrInvalid) || !errors.As(err, &field) || field.Field != "definition.driverType" {
				t.Fatalf("%s/%s must be rejected at definition.driverType: %v", driver, typeKey, err)
			}
		}
	}
	if defs, _ := st.ListResourceDefinitions(ctx, opts.OrganizationKey); len(defs) != len(seed.ResourceDefinitions(opts)) {
		t.Fatalf("a rejected registration must not persist: %d definitions", len(defs))
	}
}
