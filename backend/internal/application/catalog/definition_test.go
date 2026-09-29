package catalog_test

import (
	"context"
	"errors"
	"testing"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/adapters/terraform"
	"orchestrator/internal/application/catalog"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/seed"
)

func TestRegisterResourceDefinition_ContractsAndOrganization(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	opts := seed.Defaults()
	if err := seed.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	svc := catalog.NewService(st, terraform.NewInspector())
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

func TestRegisterResourceDefinition_KubernetesAndOutputMismatch(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	opts := seed.Defaults()
	if err := seed.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	svc := catalog.NewService(st, terraform.NewInspector())
	var postgres resource.Definition
	for _, candidate := range seed.ResourceDefinitions(opts) {
		if candidate.Key == "postgres-internal-statefulset" {
			postgres = candidate
			break
		}
	}
	postgres.Key = "postgres-internal-custom"
	if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, postgres); err != nil {
		t.Fatalf("register Kubernetes: %v", err)
	}
	bad := postgres
	bad.Key = "bad-reference"
	bad.DriverInputs = map[string]any{"values": map[string]any{"variables": map[string]any{"namespace": "${resources['k8s-namespace.default#environments.@app.@env'].outputs.missing}"}}}
	if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, bad); !errors.Is(err, catalog.ErrInvalid) {
		t.Fatalf("reference: %v", err)
	}
	if err := st.SaveResourceType(ctx, opts.OrganizationKey, resource.Type{Key: "unsupported", Outputs: []resource.OutputField{{Name: "host", Type: "string"}}}); err != nil {
		t.Fatal(err)
	}
	bad = postgres
	bad.Key = "wrong-type"
	bad.ResourceTypeKey = "unsupported"
	if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, bad); !errors.Is(err, catalog.ErrInvalid) {
		t.Fatalf("driver/type: %v", err)
	}
	if err := st.SaveResourceType(ctx, opts.OrganizationKey, resource.Type{Key: "k8s-namespace", Outputs: []resource.OutputField{{Name: "unknown", Type: "string"}}}); err != nil {
		t.Fatal(err)
	}
	bad = resource.Definition{Key: "namespace-output-mismatch", ResourceTypeKey: "k8s-namespace", DriverType: resource.DriverKubernetes, DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{"name": "sample"}}}, Criteria: []resource.Criterion{{}}}
	if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, bad); !errors.Is(err, catalog.ErrInvalid) {
		t.Fatalf("output mismatch: %v", err)
	}
}
