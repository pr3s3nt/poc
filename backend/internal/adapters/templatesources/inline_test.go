package templatesources_test

import (
	"context"
	"orchestrator/internal/adapters/store"
	"orchestrator/internal/adapters/templatedriver"
	"orchestrator/internal/adapters/templatesources"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/ports/renderregistry"
	standardtemplates "orchestrator/template-sources"
	"os"
	"strings"
	"testing"
)

func TestInlinePreparationPinsContentAndRepairsCache(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	if err := st.SaveOrganization(ctx, application.Organization{ID: "org-id", Key: "org", Name: "Org"}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	cache, err := templatesources.OpenCache(dir)
	if err != nil {
		t.Fatal(err)
	}
	r := &templatesources.Registry{Cache: cache, Store: st, Runtime: templatedriver.New()}
	req := renderregistry.InlineInspection{Files: standardtemplates.Files()}
	first, err := r.InspectInline(ctx, "org", req)
	if err != nil {
		t.Fatal(err)
	}
	again, err := r.InspectInline(ctx, "org", req)
	if err != nil || first.ID != again.ID {
		t.Fatal("repeated inline preparation changed pin", err)
	}
	req.Files["defaults.yaml"] = strings.Replace(req.Files["defaults.yaml"], "cpu: 10m", "cpu: 20m", 1)
	next, err := r.InspectInline(ctx, "org", req)
	if err != nil {
		t.Fatal(err)
	}
	if next.ID == first.ID || next.Source.ContentSHA256 == first.Source.ContentSHA256 {
		t.Fatal("changed content reused old pin")
	}
	original, err := st.GetTemplateSource(ctx, "org", first.ID)
	if err != nil || original.Source != first.Source {
		t.Fatal("old source changed", err)
	}
	if err := r.Available(ctx, original); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetTemplateSource(ctx, "other", first.ID); err == nil {
		t.Fatal("foreign source accessible")
	}
}
