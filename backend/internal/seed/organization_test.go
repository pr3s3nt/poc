package seed_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/seed"
)

func TestOrganizationIdentityPreservesIDAndStopsConflictsBeforeSeedWrites(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	o := seed.Defaults()
	o.Catalog = seed.CatalogStaging
	o.OrganizationKey = "my-company"
	o.OrganizationName = "Công ty của tôi"
	if err := seed.Apply(ctx, st, o); err != nil {
		t.Fatal(err)
	}
	before, err := st.GetOrganization(ctx, o.OrganizationKey)
	if err != nil || before.Name != o.OrganizationName {
		t.Fatalf("organization: %+v %v", before, err)
	}
	if err := seed.Apply(ctx, st, o); err != nil {
		t.Fatal(err)
	}
	after, _ := st.GetOrganization(ctx, o.OrganizationKey)
	if before.ID != after.ID {
		t.Fatal("identity changed")
	}
	custom := resource.Type{Key: "postgres", Outputs: []resource.OutputField{{Name: "kept", Type: "string"}}}
	if err := st.SaveResourceType(ctx, o.OrganizationKey, custom); err != nil {
		t.Fatal(err)
	}
	o.OrganizationName = "Another name"
	if seed.Apply(ctx, st, o) == nil {
		t.Fatal("expected conflict")
	}
	after, _ = st.GetOrganization(ctx, o.OrganizationKey)
	if after != before {
		t.Fatal("organization mutated")
	}
	types, _ := st.ListResourceTypes(ctx, o.OrganizationKey)
	for _, typ := range types {
		if typ.Key == "postgres" && (len(typ.Outputs) != 1 || typ.Outputs[0].Name != "kept") {
			t.Fatal("operator contract changed")
		}
	}
	o.OrganizationName = ""
	if err := seed.Apply(ctx, st, o); err != nil {
		t.Fatal("legacy run should preserve name", err)
	}
}
func TestReadOrganizationFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "organization.json")
	for _, raw := range []string{`{"key":"my-company","name":"Công ty"}`, `{"key":"bad key","name":"Company"}`, `{"key":"company","name":"Company","token":"not-allowed"}`, `{"key":"company","name":"Company"} {}`, `{"key":"company","name":"Company\n"}`} {
		if err := os.WriteFile(p, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		_, err := seed.ReadOrganizationFile(p)
		if raw == `{"key":"my-company","name":"Công ty"}` {
			if err != nil {
				t.Fatal(err)
			}
		} else if err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	if err := os.WriteFile(p, []byte(`{"key":"company","name":"Company"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := seed.ReadOrganizationFile(p); err == nil {
		t.Fatal("accepted broad permissions")
	}
}
