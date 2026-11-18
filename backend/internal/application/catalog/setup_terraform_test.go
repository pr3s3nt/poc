package catalog_test

import (
	"context"
	"orchestrator/internal/application/catalog"
	"testing"
)

func TestSetupPreviewReusesTerraformDefinitionWithoutAuthoredVersion(t *testing.T) {
	ctx := context.Background()
	c := newTerraformCatalog(t, cacheType())
	svc := catalog.NewService(c.st, c.reg)
	def := setValue(c.definition("setup-cache"), "source", map[string]any{"url": cacheURL, "ref": cacheCommit, "path": "modules/cache"})
	draft := documentDraft(t, def)
	docs := []catalog.SetupDocument{{File: "cache.yaml", Definition: &draft}}
	first, err := svc.PreviewSetup(ctx, c.opts.OrganizationKey, docs)
	if err != nil || first[0].Status != "NEW" {
		t.Fatal(first, err)
	}
	if _, err := svc.ImportResourceDefinition(ctx, c.opts.OrganizationKey, first[0].Definition.Definition()); err != nil {
		t.Fatal(err)
	}
	again, err := svc.PreviewSetup(ctx, c.opts.OrganizationKey, docs)
	if err != nil || again[0].Status != "REUSED" {
		t.Fatal(again, err)
	}
	if docs[0].Definition.Definition().Version != 0 {
		t.Fatal("preview mutated authored version")
	}
}
