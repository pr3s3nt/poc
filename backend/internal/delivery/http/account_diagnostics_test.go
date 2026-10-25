package http

import (
	"context"
	"slices"
	"testing"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/domain/application"
	domain "orchestrator/internal/domain/diagnostics"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
)

// Old evidence is labelled stale for every pinned input, not only the account
// and Application versions: a changed draft, configuration or Definition makes
// an earlier success describe a state that no longer exists.
func TestStalenessCoversDraftsDefinitionsAndContext(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(st.SaveOrganization(ctx, application.Organization{Key: "acme", Name: "acme"}))
	must(st.SaveApplication(ctx, application.Application{ID: "00000000-0000-4000-8000-000000000001", Key: "shop", OrganizationKey: "acme", Name: "Shop", Version: 3}))
	must(st.SaveDeploymentSet(ctx, environment.DeploymentSet{ID: "set-1", EnvironmentKey: "staging", Document: environment.NewDocument()}))
	env := environment.Environment{ID: "00000000-0000-4000-8000-000000000002", Key: "staging", ApplicationID: "00000000-0000-4000-8000-000000000001", ApplicationKey: "shop", Name: "staging", NamespaceIdentity: "shop-staging", CurrentDeploymentSetID: "set-1", Version: 2, DraftVersion: 4}
	must(st.SaveEnvironment(ctx, env))
	must(st.SaveResourceType(ctx, "acme", resource.Type{Key: "postgres", Outputs: []resource.OutputField{{Name: "host", Type: "string"}}}))
	must(st.SaveResourceDefinition(ctx, "acme", resource.Definition{Key: "pg", Name: "pg", ResourceTypeKey: "postgres", DriverType: resource.DriverKubernetes, Criteria: []resource.Criterion{{}}, Version: 7}))
	s := &Server{store: st}

	scope := domain.Scope{AccountVersion: 5, Context: &domain.ContextRef{ApplicationKey: "shop", ApplicationVersion: 3, EnvironmentKey: "staging", EnvironmentVersion: 2, DraftVersion: 4, Definitions: []domain.DefinitionPin{{Key: "pg", Version: 7}}}}
	if got := s.newStaleness("acme").reasons(ctx, scope, 5); len(got) != 0 {
		t.Fatalf("unchanged state is current: %v", got)
	}
	if got := s.newStaleness("acme").reasons(ctx, scope, 6); !slices.Equal(got, []string{"ACCOUNT_CHANGED"}) {
		t.Fatalf("account: %v", got)
	}
	// A saved draft bumps the draft version.
	must(st.SaveWorkloadDraft(ctx, 4, environment.WorkloadDraft{ApplicationKey: "shop", EnvironmentKey: "staging", WorkloadID: "web", State: environment.DraftDelete}))
	if got := s.newStaleness("acme").reasons(ctx, scope, 5); !slices.Contains(got, "DRAFTS_CHANGED") {
		t.Fatalf("draft change: %v", got)
	}
	// A Definition edit changes a pinned version.
	must(st.SaveResourceDefinition(ctx, "acme", resource.Definition{Key: "pg", Name: "pg", ResourceTypeKey: "postgres", DriverType: resource.DriverKubernetes, Criteria: []resource.Criterion{{}}, Version: 8}))
	if got := s.newStaleness("acme").reasons(ctx, scope, 5); !slices.Contains(got, "DEFINITION_CHANGED") {
		t.Fatalf("definition change: %v", got)
	}
	// An Application of another Organization is unreadable: stale, not current.
	other := domain.Scope{AccountVersion: 5, Context: &domain.ContextRef{ApplicationKey: "ghost", ApplicationVersion: 1}}
	if got := s.newStaleness("acme").reasons(ctx, other, 5); !slices.Contains(got, "APPLICATION_CHANGED") {
		t.Fatalf("unreadable pinned object: %v", got)
	}
}
