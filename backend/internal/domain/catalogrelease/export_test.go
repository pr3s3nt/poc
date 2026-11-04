package catalogrelease

import (
	"orchestrator/internal/domain/resource"
	"os"
	"path/filepath"
	"testing"
)

func TestExportedCatalogDocumentsUseReleaseLocalComponents(t *testing.T) {
	root := "../../../../platform-catalog"
	raw, err := os.ReadFile(filepath.Join(root, "catalog.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := ParseManifest(raw)
	if err != nil {
		t.Fatal(err)
	}
	components := map[string]Component{}
	for _, c := range manifest.Components {
		components[c.ID] = c
		if _, err := os.Stat(filepath.Join(root, c.Path)); err != nil {
			t.Fatal(c.ID, err)
		}
	}
	for _, c := range manifest.Components {
		if c.Kind != Definition {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(root, c.Path))
		if err != nil {
			t.Fatal(err)
		}
		doc, err := ParseDefinition(raw)
		if err != nil {
			t.Fatal(c.ID, err)
		}
		switch doc.Definition.DriverType {
		case resource.DriverTerraform:
			target, ok := components[doc.Component]
			if !ok || target.Kind != TerraformModule || target.ResourceType != doc.Definition.ResourceTypeKey {
				t.Fatal("missing module", c.ID)
			}
		case resource.DriverTemplate:
			target, ok := components[doc.Component]
			if !ok || target.Kind != TemplateSource {
				t.Fatal("missing renderer", c.ID)
			}
		}
	}
}
