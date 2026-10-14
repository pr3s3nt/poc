package catalog_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"orchestrator/internal/adapters/terraform/registry"
	"orchestrator/internal/application/catalog"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/ports/moduleregistry"
)

func classesOf(t *testing.T, svc *catalog.Service, org, typeKey string) []string {
	t.Helper()
	views, err := svc.ListResourceTypeViews(context.Background(), org)
	if err != nil {
		t.Fatal(err)
	}
	for _, view := range views {
		if view.Key == typeKey {
			if view.Classes == nil {
				t.Fatalf("%s classes must be [] not null", typeKey)
			}
			return view.Classes
		}
	}
	t.Fatalf("type %s missing", typeKey)
	return nil
}

// UC-16 API mapping (I04-07): classes are computed from the scoped catalog
// Definitions and verified registry metadata only; never persisted, guessed
// from module names or defaulted.
func TestResourceTypeClassesComeFromVerifiedCatalogMetadata(t *testing.T) {
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
	if got := classesOf(t, svc, opts.OrganizationKey, "redis"); len(got) != 0 {
		t.Fatalf("a Type no Definition serves has no classes: %v", got)
	}
	if got := classesOf(t, svc, opts.OrganizationKey, "postgres"); len(got) != 0 {
		t.Fatalf("classless seed Definitions offer no class (default is not mapped): %v", got)
	}
	ns := "${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}"
	out := []string{"host", "port", "username", "password", "tls"}
	for key, class := range map[string]string{"redis-in-cluster": "in-cluster", "redis-custom": "team-a"} {
		if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, k8sDefinition(key, "redis", refs["redis-k8s"], map[string]any{"namespace": ns}, nil, out, resource.Criterion{Class: class})); err != nil {
			t.Fatal(err)
		}
	}
	// A duplicate class and a classless wildcard add nothing.
	if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, k8sDefinition("redis-in-cluster-2", "redis", refs["redis-k8s"], map[string]any{"namespace": ns}, nil, out, resource.Criterion{Class: "in-cluster", ApplicationID: "other"})); err != nil {
		t.Fatal(err)
	}
	if got := classesOf(t, svc, opts.OrganizationKey, "redis"); !reflect.DeepEqual(got, []string{"in-cluster", "team-a"}) {
		t.Fatalf("classes %v", got)
	}
}

// metadataFailure makes registry metadata unavailable.
type metadataFailure struct {
	moduleregistry.Registry
	err error
}

func (f metadataFailure) Bundle(context.Context, tfmodule.SourceRef) (tfmodule.Bundle, error) {
	return tfmodule.Bundle{}, f.err
}

// A pin that is not a verified bundle offers no class; an unavailable
// registry is an error, never an empty or guessed class list.
func TestResourceTypeClassesNeverGuessOnRegistryFailure(t *testing.T) {
	ctx := context.Background()
	st, svc, opts := seededCatalog(t, withRegistry)
	refs, err := registry.PackagedRefs()
	if err != nil {
		t.Fatal(err)
	}
	ns := "${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}"
	if _, err := svc.RegisterResourceType(ctx, opts.OrganizationKey, resource.Type{Key: "redis", Outputs: []resource.OutputField{
		{Name: "host", Type: "string", Required: true}, {Name: "port", Type: "number", Required: true}, {Name: "username", Type: "string", Required: true},
		{Name: "password", Type: "string", Required: true, Secret: true}, {Name: "tls", Type: "bool", Required: true}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, k8sDefinition("redis-in-cluster", "redis", refs["redis-k8s"], map[string]any{"namespace": ns}, nil, []string{"host", "port", "username", "password", "tls"}, resource.Criterion{Class: "in-cluster"})); err != nil {
		t.Fatal(err)
	}
	notVerified := catalog.NewService(st, metadataFailure{err: moduleregistry.ErrNotVerified})
	if got := classesOf(t, notVerified, opts.OrganizationKey, "redis"); len(got) != 0 {
		t.Fatalf("unverified pin must offer no class: %v", got)
	}
	unavailable := catalog.NewService(st, metadataFailure{err: errors.New("registry exploded: /private/cache/path")})
	if _, err := unavailable.ListResourceTypeViews(ctx, opts.OrganizationKey); err == nil {
		t.Fatal("registry failure must be an error, not an empty class list")
	}
}
