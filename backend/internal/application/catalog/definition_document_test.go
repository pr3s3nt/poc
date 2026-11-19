package catalog_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"orchestrator/internal/application/catalog"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/ports/gitaccess"
	"orchestrator/internal/ports/moduleregistry"
)

type documentInspector struct {
	moduleregistry.Registry
	inspector   moduleregistry.GitInspector
	calls       int
	org         string
	branchMoved bool
}

func (r *documentInspector) InspectGit(ctx context.Context, request moduleregistry.GitInspection) (tfmodule.Bundle, error) {
	r.calls++
	r.org = gitaccess.Organization(ctx)
	if r.branchMoved {
		return tfmodule.Bundle{}, moduleregistry.ErrUnavailable
	}
	if request.Ref == "main" {
		request.Ref = cacheCommit
	}
	return r.inspector.InspectGit(ctx, request)
}

func documentDraft(t *testing.T, def resource.Definition) catalog.DefinitionDraft {
	t.Helper()
	data, err := json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	var draft catalog.DefinitionDraft
	if err := json.Unmarshal(data, &draft); err != nil {
		t.Fatal(err)
	}
	return draft
}

func TestGitDocumentPreparePinsBranchWithoutDefinitionWritesAndRoundTrips(t *testing.T) {
	ctx := context.Background()
	c := newTerraformCatalog(t, cacheType())
	org := c.opts.OrganizationKey
	spy := &documentInspector{Registry: c.reg, inspector: c.reg}
	s := catalog.NewService(c.st, spy)
	def := setValue(c.definition("document-cache"), "source", map[string]any{"url": cacheURL, "ref": "main", "path": "modules/cache"})
	before, _ := c.st.ListResourceDefinitions(ctx, org)
	input := documentDraft(t, def)
	prepared, err := s.PrepareDefinitionDocument(ctx, org, input)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := c.st.ListResourceDefinitions(ctx, org)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("prepare wrote Definitions")
	}
	if input.Definition().Source()["ref"] != "main" {
		t.Fatal("prepare mutated the authored source")
	}
	if spy.calls != 1 || spy.org != org || prepared.Source == nil || prepared.Source.Revision != cacheCommit || prepared.Source.URL != cacheURL {
		t.Fatalf("inspection/provenance: calls=%d org=%s source=%+v", spy.calls, spy.org, prepared.Source)
	}
	if prepared.Definition.Definition().Source()["revision"] != cacheCommit {
		t.Fatal("mutable ref escaped preparation")
	}
	// The branch moves/becomes unavailable after review. Registration must
	// consume the exact prepared bundle, never invoke branch inspection again.
	spy.branchMoved = true
	for _, want := range []string{"CREATED", "REUSED"} {
		result, err := s.ImportResourceDefinition(ctx, org, prepared.Definition.Definition())
		if err != nil || result.Status != want {
			t.Fatalf("import: %+v %v", result, err)
		}
	}
	history, err := c.st.GetDefinitionHistory(ctx, org, def.Key)
	if err != nil || len(history.Versions) != 1 {
		t.Fatalf("history: %+v %v", history, err)
	}
	for _, revision := range []string{"", history.Versions[0].ID} {
		doc, err := s.ExportDefinitionDocument(ctx, org, def.Key, revision, 0)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(doc.Definition().Source(), map[string]any{"url": cacheURL, "ref": cacheCommit, "path": "modules/cache"}) {
			t.Fatalf("export: %+v", doc)
		}
		if _, err := s.ExportDefinitionDocument(ctx, "another-org", def.Key, revision, 0); !errors.Is(err, catalog.ErrNotFound) {
			t.Fatalf("cross-org export: %v", err)
		}
	}
	if _, err := s.ExportDefinitionDocument(ctx, org, def.Key, "another-revision", 0); !errors.Is(err, catalog.ErrNotFound) {
		t.Fatalf("cross-revision export: %v", err)
	}
	if spy.calls != 1 {
		t.Fatal("registration/export resolved the branch again")
	}
	spy.branchMoved = false
	doc, err := s.ExportDefinitionDocument(ctx, org, def.Key, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	roundtrip, err := s.PrepareDefinitionDocument(ctx, org, doc)
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.ImportResourceDefinition(ctx, org, roundtrip.Definition.Definition())
	if err != nil || result.Status != "REUSED" {
		t.Fatalf("portable round-trip: %+v %v", result, err)
	}
	changed := roundtrip.Definition
	changed.Name = "Different name"
	result, err = s.ImportResourceDefinition(ctx, org, changed.Definition())
	if err != nil || result.Status != "CONFLICT" {
		t.Fatalf("conflict: %+v %v", result, err)
	}
	historyAfter, _ := c.st.GetDefinitionHistory(ctx, org, def.Key)
	if !reflect.DeepEqual(history, historyAfter) {
		t.Fatal("round-trip/conflict wrote revisions")
	}
	// A list row can become stale while another author saves a new version.
	// Export must preserve the version the user clicked, not silently use latest.
	updated := roundtrip.Definition.Definition()
	updated.Name = "Version two"
	if _, err := s.UpdateResourceDefinition(ctx, org, def.Key, 1, updated); err != nil {
		t.Fatal(err)
	}
	old, err := s.ExportDefinitionDocument(ctx, org, def.Key, "", 1)
	if err != nil || old.Name != def.Name {
		t.Fatalf("displayed version export: %+v %v", old, err)
	}
	latest, err := s.ExportDefinitionDocument(ctx, org, def.Key, "", 0)
	if err != nil || latest.Name != "Version two" {
		t.Fatalf("latest export: %+v %v", latest, err)
	}

}

func TestGitDocumentRejectsAmbiguousSourcesAndInvalidMappings(t *testing.T) {
	ctx := context.Background()
	c := newTerraformCatalog(t, cacheType())
	spy := &documentInspector{Registry: c.reg, inspector: c.reg}
	s := catalog.NewService(c.st, spy)
	for _, source := range []map[string]any{
		{"url": cacheURL, "ref": "main"},
		{"url": cacheURL, "ref": "main", "path": 3},
		{"url": cacheURL, "ref": "main", "path": "modules/cache", "content_sha256": "fake"},
		{"url": cacheURL, "ref": "main", "path": "modules/cache", "token": "not-allowed"},
	} {
		_, err := s.PrepareDefinitionDocument(ctx, c.opts.OrganizationKey, documentDraft(t, setValue(c.definition("invalid-doc"), "source", source)))
		if !errors.Is(err, catalog.ErrInvalid) {
			t.Fatalf("invalid source: %v", err)
		}
	}
	if spy.calls != 0 {
		t.Fatal("ambiguous source reached Git")
	}
	def := setValue(c.definition("invalid-doc"), "source", map[string]any{"url": cacheURL, "ref": "main", "path": "modules/cache"})
	if _, err := s.PrepareDefinitionDocument(ctx, "another-org", documentDraft(t, def)); !errors.Is(err, catalog.ErrInvalid) {
		t.Fatalf("unknown scoped Type: %v", err)
	}
	if spy.calls != 0 {
		t.Fatal("unknown scoped Type reached Git")
	}
	def = setValue(def, "output_mapping", map[string]any{"host": "host"})
	if _, err := s.PrepareDefinitionDocument(ctx, c.opts.OrganizationKey, documentDraft(t, def)); !errors.Is(err, catalog.ErrInvalid) {
		t.Fatalf("missing required outputs: %v", err)
	}
	defs, _ := c.st.ListResourceDefinitions(ctx, c.opts.OrganizationKey)
	for _, d := range defs {
		if d.Key == def.Key {
			t.Fatal("invalid mapping created Definition")
		}
	}
}
