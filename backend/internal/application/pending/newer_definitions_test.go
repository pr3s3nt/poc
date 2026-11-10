package pending

import (
	"context"
	"testing"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/application/definitionversions"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/resource"
)

// UC-03 DV-04 (issue #68): Preview names a pinned revision older than Active.
func TestNewerDefinitionsNamesOnlyOlderPinnedRevisions(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	org := "acme"
	if err := st.SaveOrganization(ctx, application.Organization{ID: "org-1", Key: org, Name: "Acme"}); err != nil {
		t.Fatal(err)
	}
	def := resource.Definition{Key: "pg", Name: "bad image", ResourceTypeKey: "postgres"}
	r1, err := definitionversions.Record(ctx, st, org, def, true)
	if err != nil {
		t.Fatal(err)
	}
	def.Name = "fixed image"
	r2, err := definitionversions.Record(ctx, st, org, def, true)
	if err != nil {
		t.Fatal(err)
	}
	unversioned := resource.Definition{Key: "legacy"}
	got, err := newerDefinitions(ctx, st, org, []resource.Definition{r1, unversioned})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != (NewerDefinition{Key: "pg", SelectedNumber: 1, ActiveNumber: 2}) {
		t.Fatalf("got %+v", got)
	}
	if got, _ := newerDefinitions(ctx, st, org, []resource.Definition{r2}); len(got) != 0 {
		t.Fatalf("Active revision reported as outdated: %+v", got)
	}
}
