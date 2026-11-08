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

// UC-02 EXT-01..04 (issue #52): Platform Engineers add optional outputs to an
// existing Type; nothing else of the contract can change.
func TestAddResourceTypeOutputs(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	if err := st.SaveOrganization(ctx, application.Organization{Key: "acme", Name: "Acme"}); err != nil {
		t.Fatal(err)
	}
	svc := catalog.NewService(st)
	pg := resource.Type{Key: "postgres", Inputs: []resource.InputField{{Name: "database", Type: "string", Required: true}}, Outputs: []resource.OutputField{{Name: "host", Type: "string", Required: true}, {Name: "password", Type: "string", Required: true, Secret: true}}}
	for _, typ := range []resource.Type{pg, {Key: "workload"}} {
		if err := st.SaveResourceType(ctx, "acme", typ); err != nil {
			t.Fatal(err)
		}
	}
	got, err := svc.AddResourceTypeOutputs(ctx, "acme", "postgres", []resource.OutputField{{Name: "uri", Type: "string", Secret: true}, {Name: "jdbcUrl", Type: "string"}})
	if err != nil {
		t.Fatalf("add outputs: %v", err)
	}
	if len(got.Outputs) != 4 || got.Outputs[2] != (resource.OutputField{Name: "uri", Type: "string", Secret: true}) || len(got.Inputs) != 1 {
		t.Fatalf("returned type = %+v", got)
	}
	cases := []struct {
		name    string
		org     string
		key     string
		outputs []resource.OutputField
		want    error
	}{
		{"required output", "acme", "postgres", []resource.OutputField{{Name: "dsn", Type: "string", Required: true}}, catalog.ErrInvalid},
		{"no output", "acme", "postgres", nil, catalog.ErrInvalid},
		{"unsupported type", "acme", "postgres", []resource.OutputField{{Name: "dsn", Type: "object"}}, catalog.ErrInvalid},
		{"workload type", "acme", "workload", []resource.OutputField{{Name: "dsn", Type: "string"}}, catalog.ErrInvalid},
		{"existing name", "acme", "postgres", []resource.OutputField{{Name: "host", Type: "string"}}, catalog.ErrDuplicate},
		{"added before", "acme", "postgres", []resource.OutputField{{Name: "uri", Type: "string", Secret: true}}, catalog.ErrDuplicate},
	}
	for _, tc := range cases {
		if _, err := svc.AddResourceTypeOutputs(ctx, tc.org, tc.key, tc.outputs); !errors.Is(err, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, err, tc.want)
		}
	}
	if _, err := svc.AddResourceTypeOutputs(ctx, "acme", "missing", []resource.OutputField{{Name: "dsn", Type: "string"}}); err == nil || errors.Is(err, catalog.ErrInvalid) {
		t.Errorf("missing Type must be not found, got %v", err)
	}
	if _, err := svc.AddResourceTypeOutputs(ctx, "other", "postgres", []resource.OutputField{{Name: "dsn", Type: "string"}}); err == nil {
		t.Error("another Organization must not reach the Type")
	}
	types, _ := st.ListResourceTypes(ctx, "acme")
	for _, typ := range types {
		if typ.Key == "postgres" && len(typ.Outputs) != 4 {
			t.Fatalf("rejected additions changed the stored contract: %+v", typ.Outputs)
		}
		if typ.Key == "workload" && len(typ.Outputs) != 0 {
			t.Fatalf("workload gained outputs: %+v", typ.Outputs)
		}
	}
}
