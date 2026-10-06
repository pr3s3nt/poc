package catalog_test

import (
	"context"
	"errors"
	"orchestrator/internal/adapters/store"
	"orchestrator/internal/application/catalog"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/seed"
	"testing"
)

func TestRenderingRegistrationRejectsUnavailableOrInfrastructureInputs(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	opts := seed.Defaults()
	if err := seed.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	svc := catalog.NewService(st)
	makeDef := func() resource.Definition {
		return resource.Definition{Key: "workload-render", ResourceTypeKey: "workload", ExecutionProfile: "internal-k8s", DriverType: resource.DriverScoreK8s, Criteria: []resource.Criterion{{}}, DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{"render_bundle": "installed"}}}}
	}
	if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, makeDef()); !errors.Is(err, catalog.ErrInvalid) {
		t.Fatalf("unavailable renderer: %v", err)
	}
	svc.SetRenderBundles(map[string]resource.RenderBundle{"installed": {ID: "installed", Version: "0.15.0", BinaryDigest: "binary", Digest: "bundle"}})
	for _, tc := range []struct {
		name   string
		mutate func(*resource.Definition)
	}{
		{"cloud", func(d *resource.Definition) { d.ExecutionProfile = "aws-eks" }},
		{"wrong-type", func(d *resource.Definition) { d.ResourceTypeKey = "postgres" }},
		{"connection", func(d *resource.Definition) { d.ConnectionKey = opts.ConnectionKey }},
		{"provision", func(d *resource.Definition) {
			d.Provision = map[string]resource.ProvisionRule{"postgres.default#db": {}}
		}},
		{"unknown-bundle", func(d *resource.Definition) {
			d.DriverInputs["values"].(map[string]any)["variables"].(map[string]any)["render_bundle"] = "https://mutable.invalid/template"
		}},
		{"extra-variable", func(d *resource.Definition) {
			d.DriverInputs["values"].(map[string]any)["variables"].(map[string]any)["command"] = "do-something"
		}},
		{"template", func(d *resource.Definition) { d.DriverInputs["values"].(map[string]any)["template"] = "content" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			def := makeDef()
			tc.mutate(&def)
			if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, def); !errors.Is(err, catalog.ErrInvalid) {
				t.Fatalf("rejected registration: %v", err)
			}
		})
	}
	def := makeDef()
	created, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, def)
	if err != nil {
		t.Fatal(err)
	}
	if created.SourceFingerpr != "bundle" {
		t.Fatal("fingerprint not server-owned")
	}
	if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, def); !errors.Is(err, catalog.ErrDuplicate) {
		t.Fatalf("duplicate: %v", err)
	}
}
