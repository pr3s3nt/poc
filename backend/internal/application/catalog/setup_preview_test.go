package catalog

import (
	"context"
	"orchestrator/internal/adapters/store"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/resource"
	"testing"
)

func TestSetupPreviewUsesProspectiveTypesWithoutBusinessWrites(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	_ = st.SaveOrganization(ctx, application.Organization{Key: "org", Name: "Org"})
	svc := NewService(st)
	typ := resource.Type{Key: "endpoint", Outputs: []resource.OutputField{{Name: "hostname", Type: "string"}, {Name: "url", Type: "string"}}}
	def := DefinitionDraft{Key: "edge", Name: "Edge", ResourceTypeKey: "endpoint", DriverType: resource.DriverEndpoint, Criteria: []resource.Criterion{{}}, DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{"rootDomain": "apps.example.com", "scheme": "https", "gatewayName": "traefik", "gatewayNamespace": "traefik"}}}}
	docs := []SetupDocument{{File: "a", Type: &typ}, {File: "b", Definition: &def}}
	result, err := svc.PreviewSetup(ctx, "org", docs)
	if err != nil {
		t.Fatal(err)
	}
	if result[0].Status != "NEW" || result[1].Status != "NEW" {
		t.Fatalf("%+v", result)
	}
	types, _ := st.ListResourceTypes(ctx, "org")
	defs, _ := st.ListResourceDefinitions(ctx, "org")
	if len(types)+len(defs) != 0 {
		t.Fatal("preview wrote business data")
	}
	if _, err := svc.ImportResourceType(ctx, "org", typ); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ImportResourceDefinition(ctx, "org", result[1].Definition.Definition()); err != nil {
		t.Fatal(err)
	}
	result, err = svc.PreviewSetup(ctx, "org", docs)
	if err != nil || result[0].Status != "REUSED" || result[1].Status != "REUSED" {
		t.Fatalf("%+v %v", result, err)
	}
	def.Name = "Changed"
	result, err = svc.PreviewSetup(ctx, "org", docs)
	if err != nil || result[1].Status != "CONFLICT" {
		t.Fatal(result, err)
	}
}
