package catalog

import (
	"context"
	"orchestrator/internal/adapters/store"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
	"testing"
)

func TestImportTypeDoesNotOverwrite(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	_ = st.SaveOrganization(ctx, application.Organization{Key: "org", Name: "Org"})
	s := NewService(st)
	typ := resource.Type{Key: "queue", Outputs: []resource.OutputField{{Name: "host", Type: "string"}}}
	for _, want := range []string{"CREATED", "REUSED"} {
		r, err := s.ImportResourceType(ctx, "org", typ)
		if err != nil || r.Status != want {
			t.Fatalf("%+v %v", r, err)
		}
	}
	typ.Outputs[0].Secret = true
	r, err := s.ImportResourceType(ctx, "org", typ)
	if err != nil || r.Status != "CONFLICT" || len(r.Fields) == 0 {
		t.Fatalf("%+v %v", r, err)
	}
	saved, _ := st.ListResourceTypes(ctx, "org")
	if len(saved) != 1 || saved[0].Outputs[0].Secret {
		t.Fatal("conflict wrote contract")
	}
}
func TestDocumentComparisonDefaultsAndMetadata(t *testing.T) {
	a := resource.Type{Key: "queue"}
	b := resource.Type{Key: "queue", Inputs: []resource.InputField{}, Outputs: []resource.OutputField{}}
	if r := importComparison(a, b); r.Status != "REUSED" {
		t.Fatal(r)
	}
	d := resource.Definition{Key: "endpoint", Version: 4, RevisionID: "r4", SourceFingerpr: "hash", Criteria: []resource.Criterion{{Class: "a"}, {Class: "b"}}}
	e := d
	e.Version = 0
	e.RevisionID = ""
	e.SourceFingerpr = ""
	e.Criteria = []resource.Criterion{{Class: "b"}, {Class: "a"}}
	if r := importComparison(d, e); r.Status != "REUSED" {
		t.Fatal(r)
	}
}

func TestImportDefinitionDoesNotCreateRevisionOnReuseOrConflict(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	_ = st.SaveOrganization(ctx, application.Organization{Key: "org", Name: "Org"})
	s := NewService(st)
	_, err := s.RegisterResourceType(ctx, "org", resource.Type{Key: "endpoint", Outputs: []resource.OutputField{{Name: "hostname", Type: "string"}, {Name: "url", Type: "string"}}})
	if err != nil {
		t.Fatal(err)
	}
	d := resource.Definition{Key: "edge", Name: "Edge", ResourceTypeKey: "endpoint", DriverType: resource.DriverEndpoint, Criteria: []resource.Criterion{{}}, DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{"rootDomain": "apps.example.com", "scheme": "https", "gatewayName": "traefik", "gatewayNamespace": "traefik"}}}}
	for _, want := range []string{"CREATED", "REUSED"} {
		r, err := s.ImportResourceDefinition(ctx, "org", d)
		if err != nil || r.Status != want {
			t.Fatalf("%+v %v", r, err)
		}
	}
	before, err := s.loadDefinition(ctx, "org", "edge")
	if err != nil {
		t.Fatal(err)
	}
	d.Name = "Changed"
	r, err := s.ImportResourceDefinition(ctx, "org", d)
	if err != nil || r.Status != "CONFLICT" {
		t.Fatalf("%+v %v", r, err)
	}
	after, _ := s.loadDefinition(ctx, "org", "edge")
	if before.RevisionID != after.RevisionID || before.Version != after.Version || after.Name != "Edge" {
		t.Fatal("import changed stored revision")
	}
}

func TestImportComparisonKeepsArbitraryValuesButAppliesModuleDefaults(t *testing.T) {
	s := NewService(store.New())
	ctx := context.Background()
	a := resource.Definition{DriverType: resource.DriverTerraform, DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{}}}}
	b := resource.Definition{DriverType: resource.DriverTerraform, DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{"count": float64(2)}}}}
	bundle := &tfmodule.Bundle{Contract: tfmodule.Contract{Inputs: map[string]tfmodule.Input{"count": {HasDefault: true, Default: 2}}}}
	left, err := s.normalizedDefinitionDocument(ctx, "org", a, bundle)
	if err != nil {
		t.Fatal(err)
	}
	right, err := s.normalizedDefinitionDocument(ctx, "org", b, bundle)
	if err != nil {
		t.Fatal(err)
	}
	if r := importComparison(left, right); r.Status != "REUSED" {
		t.Fatal(r)
	}
	b.DriverInputs["values"].(map[string]any)["variables"].(map[string]any)["count"] = nil
	right, _ = s.normalizedDefinitionDocument(ctx, "org", b, bundle)
	if r := importComparison(left, right); r.Status != "CONFLICT" {
		t.Fatal("null must preserve fallback semantics", r)
	}
}
