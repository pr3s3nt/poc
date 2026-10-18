package catalog_test

import (
	"context"
	"errors"
	"orchestrator/internal/adapters/store"
	"orchestrator/internal/application/catalog"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/seed/seedtest"
	"testing"
)

func TestRenderingRegistrationRejectsUnavailableOrInfrastructureInputs(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	opts := seedtest.Defaults()
	if err := seedtest.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	svc := catalog.NewService(st)
	withClusterOutput(t, st, opts)
	makeDef := func() resource.Definition {
		return resource.Definition{Key: "workload-render", Name: "workload-render", Version: 1, ResourceTypeKey: "workload", DriverType: resource.DriverScoreK8s, Criteria: []resource.Criterion{{}}, DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{"render_bundle": "installed", "namespace": "${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}", "cluster": "${resources['k8s-cluster.internal#environments.@app.@env'].outputs.cluster}"}}}}
	}
	if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, makeDef()); !errors.Is(err, catalog.ErrInvalid) {
		t.Fatalf("unavailable renderer: %v", err)
	}
	svc.SetRenderBundles(map[string]resource.RenderBundle{"installed": {ID: "installed", Version: "0.15.0", BinaryDigest: "binary", Digest: "bundle"}})
	for _, tc := range []struct {
		name   string
		mutate func(*resource.Definition)
	}{
		{"wrong-type", func(d *resource.Definition) { d.ResourceTypeKey = "postgres" }},
		{"driver account", func(d *resource.Definition) { d.DriverAccount = opts.ConnectionKey }},
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
	// A second renderer registers with its own explicit criteria; there is no profile (UC-03 V02-02).
	second := makeDef()
	second.Key, second.Criteria = "workload-render-cloud", []resource.Criterion{{ApplicationID: "cloud"}}
	if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, second); err != nil {
		t.Fatalf("second rendering Definition: %v", err)
	}
}
