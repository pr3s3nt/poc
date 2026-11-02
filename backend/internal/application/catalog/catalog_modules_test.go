package catalog_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"orchestrator/internal/application/catalog"
	domain "orchestrator/internal/domain/catalogrelease"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/seed"
)

func TestCatalogModuleSelectionPinsAndValidatesProvenance(t *testing.T) {
	ctx := context.Background()
	st, svc, opts := seededCatalog(t, withRegistry)
	var def resource.Definition
	for _, d := range seed.ResourceDefinitions(opts) {
		if d.Key == "postgres-aws-aurora" {
			def = d
			break
		}
	}
	pin := def.Source()
	bundles, err := st.ListModuleBundles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	bundleID := ""
	for _, b := range bundles {
		if b.Ref.ModuleID == pin["module_id"] && b.Ref.Revision == pin["revision"] {
			bundleID = b.ID
			break
		}
	}
	if bundleID == "" {
		t.Fatal("missing fixture module")
	}
	b, err := st.GetModuleBundleByID(ctx, bundleID)
	if err != nil {
		t.Fatal(err)
	}
	component := domain.Component{ID: "database", Kind: domain.TerraformModule, Path: "modules/database", ResourceType: "postgres"}
	manifest, _ := json.Marshal(domain.Manifest{APIVersion: domain.APIVersion, Components: []domain.Component{component}})
	release := domain.Release{Organization: opts.OrganizationKey, SourceID: "modules", Tag: "v1.0.0", Commit: strings.Repeat("b", 40), BranchTip: strings.Repeat("b", 40), Manifest: manifest, ManifestSHA256: domain.Digest(manifest), ObservedAt: time.Now().UTC(), Components: []domain.VerifiedComponent{{Component: component, BundleID: b.ID, SHA256: b.Ref.ContentSHA256}}}
	err = st.Transact(ctx, func(ctx context.Context) error {
		if err := st.SaveCatalogSource(ctx, 0, persistence.CatalogSource{Source: domain.Source{Organization: opts.OrganizationKey, ID: "modules", Repository: "https://example.com/modules.git", Branch: "main", TagPattern: "v*", ManifestPath: "catalog.yaml", Version: 1}}); err != nil {
			return err
		}
		return st.InsertCatalogRelease(ctx, release)
	})
	if err != nil {
		t.Fatal(err)
	}
	choices, err := svc.CatalogModules(ctx, opts.OrganizationKey, "modules", "v1.0.0", "postgres")
	if err != nil || len(choices) != 1 || choices[0].Capability.Source.ContentSHA256 != b.Ref.ContentSHA256 {
		t.Fatal(choices, err)
	}
	empty, err := svc.CatalogModules(ctx, opts.OrganizationKey, "modules", "v1.0.0", "vpc")
	if err != nil || len(empty) != 0 {
		t.Fatal(empty, err)
	}
	if _, err := svc.CatalogModules(ctx, "other", "modules", "v1.0.0", "postgres"); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatal("cross-org leak", err)
	}
	def.Key = "catalog-postgres"
	def.DriverInputs["catalog"] = map[string]any{"sourceId": "modules", "tag": "v1.0.0", "componentId": "database"}
	saved, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, def)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Source()["content_sha256"] != b.Ref.ContentSHA256 || saved.DriverInputs["catalog"] == nil {
		t.Fatal("pin/provenance lost")
	}
	// New versions never rewrite the saved module pin.
	next := release
	next.Tag = "v2.0.0"
	next.Commit = strings.Repeat("c", 40)
	if err := st.InsertCatalogRelease(ctx, next); err != nil {
		t.Fatal(err)
	}
	defs, err := st.ListResourceDefinitions(ctx, opts.OrganizationKey)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range defs {
		if d.Key == saved.Key && d.DriverInputs["catalog"].(map[string]any)["tag"] != "v1.0.0" {
			t.Fatal("implicit upgrade")
		}
	}
	for _, test := range []struct {
		name   string
		mutate func(resource.Definition)
	}{
		{"wrong component", func(d resource.Definition) {
			d.DriverInputs["catalog"] = map[string]any{"sourceId": "modules", "tag": "v1.0.0", "componentId": "missing"}
		}},
		{"wrong pin", func(d resource.Definition) { d.Source()["content_sha256"] = strings.Repeat("0", 64) }},
		{"unknown provenance field", func(d resource.Definition) { d.DriverInputs["catalog"].(map[string]any)["extra"] = "invalid" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			raw, _ := json.Marshal(saved)
			var bad resource.Definition
			json.Unmarshal(raw, &bad)
			bad.Key = "invalid-catalog"
			test.mutate(bad)
			if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, bad); !errors.Is(err, catalog.ErrInvalid) {
				t.Fatal("invalid provenance accepted", err)
			}
		})
	}
	// Hash drift in retained metadata must not produce a usable module choice.
	broken := release
	broken.Tag = "v3.0.0"
	broken.Components = append([]domain.VerifiedComponent(nil), release.Components...)
	broken.Components[0].SHA256 = strings.Repeat("0", 64)
	if err := st.InsertCatalogRelease(ctx, broken); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CatalogModules(ctx, opts.OrganizationKey, "modules", "v3.0.0", "postgres"); err == nil {
		t.Fatal("drifted component accepted")
	}
}
