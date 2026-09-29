package catalog_test

import (
	"context"
	"errors"
	"testing"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/application/catalog"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/resource"
)

func TestRegisterResourceType(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	if err := st.SaveOrganization(ctx, application.Organization{Key: "acme", Name: "Acme"}); err != nil {
		t.Fatal(err)
	}
	svc := catalog.NewService(st)
	typ := resource.Type{Key: "redis", Inputs: []resource.InputField{{Name: "size", Type: "number"}}, Outputs: []resource.OutputField{{Name: "host", Type: "string", Required: true}}}
	if _, err := svc.RegisterResourceType(ctx, "acme", typ); err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, err := svc.RegisterResourceType(ctx, "acme", typ); !errors.Is(err, catalog.ErrDuplicate) {
		t.Fatalf("expected duplicate rejection, got %v", err)
	}
	if _, err := svc.RegisterResourceType(ctx, "acme", resource.Type{Key: "broken", Outputs: []resource.OutputField{{Name: "host", Type: "object"}}}); !errors.Is(err, catalog.ErrInvalid) {
		t.Fatalf("expected invalid schema rejection, got %v", err)
	}
	if _, err := svc.RegisterResourceType(ctx, "missing", typ); err == nil {
		t.Fatal("expected missing Organization rejection")
	}
	types, err := st.ListResourceTypes(ctx, "acme")
	if err != nil || len(types) != 1 || types[0].Key != "redis" {
		t.Fatalf("persisted types: %#v, %v", types, err)
	}
}
