package seed_test

import (
	"context"
	"testing"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/seed"
)

func TestStagingSeedsOnlyMissingContractsAndPreservesOperatorChanges(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	opts := seed.Defaults()
	opts.Catalog = seed.CatalogStaging
	if err := seed.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	types, err := st.ListResourceTypes(ctx, opts.OrganizationKey)
	if err != nil || len(types) != 5 {
		t.Fatalf("types = %v, %v", types, err)
	}
	expected := map[string]bool{"workload": true, "k8s-cluster": true, "k8s-namespace": true, "postgres": true, "redis": true}
	for _, typ := range types {
		if !expected[typ.Key] {
			t.Fatalf("unexpected type %s", typ.Key)
		}
		delete(expected, typ.Key)
	}
	if len(expected) != 0 {
		t.Fatalf("missing types %v", expected)
	}
	defs, err := st.ListResourceDefinitions(ctx, opts.OrganizationKey)
	if err != nil || len(defs) != 0 {
		t.Fatalf("definitions = %v, %v", defs, err)
	}
	accounts, err := st.ListConnections(ctx, opts.OrganizationKey)
	if err != nil || len(accounts) != 0 {
		t.Fatalf("accounts = %v, %v", accounts, err)
	}
	apps, err := st.ListApplications(ctx)
	if err != nil || len(apps) != 0 {
		t.Fatalf("apps = %v, %v", apps, err)
	}
	custom := resource.Type{Key: "postgres", Outputs: []resource.OutputField{{Name: "operator_output", Type: "string"}}}
	if err := st.SaveResourceType(ctx, opts.OrganizationKey, custom); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := seed.Apply(ctx, st, opts); err != nil {
			t.Fatal(err)
		}
	}
	types, err = st.ListResourceTypes(ctx, opts.OrganizationKey)
	if err != nil || len(types) != 5 {
		t.Fatalf("rerun types = %v, %v", types, err)
	}
	for _, typ := range types {
		if typ.Key == "postgres" && (len(typ.Outputs) != 1 || typ.Outputs[0].Name != "operator_output") {
			t.Fatalf("operator schema overwritten: %v", typ)
		}
	}
}
