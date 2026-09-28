package pending

import (
	"context"
	"testing"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/domain/configuration"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/planning/score"
)

func TestValidateImageRegistryForFleetPreview(t *testing.T) {
	doc := &score.Document{Containers: map[string]environment.Container{"main": {Image: "10.96.91.170:80/library/app:v1"}}}
	if err := validateImageRegistry(doc, "10.96.91.170:80"); err != nil {
		t.Fatal(err)
	}
	doc.Containers["main"] = environment.Container{Image: "docker.io/library/app:v1"}
	if err := validateImageRegistry(doc, "10.96.91.170:80"); err == nil {
		t.Fatal("foreign registry accepted")
	}
	if err := validateImageRegistry(doc, ""); err != nil {
		t.Fatalf("direct delivery must stay unchanged: %v", err)
	}
}

func TestChangedReferencedConfigurationStillUpdatesNoopModule(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	if err := st.SaveEnvironment(ctx, environment.Environment{Key: "staging", ApplicationKey: "app", NamespaceIdentity: "app-staging"}); err != nil {
		t.Fatal(err)
	}
	if err := st.CommitConfigurationRevision(ctx, 0, configuration.Revision{ID: "applied", ApplicationKey: "app", EnvironmentKey: "staging", Version: 1, Entries: map[string]configuration.Entry{"API_KEY": {Kind: configuration.Variable, ValueRef: "old"}}}); err != nil {
		t.Fatal(err)
	}
	module := environment.Module{Spec: environment.ModuleSpec{Containers: map[string]environment.Container{"main": {Variables: map[string]string{"API_KEY": "${context.uc12.API_KEY}"}}}}}
	desired := configuration.Revision{ID: "desired", Entries: map[string]configuration.Entry{"API_KEY": {Kind: configuration.Variable, ValueRef: "new"}}}
	changed, err := usesChangedConfiguration(ctx, st, module, desired, "applied")
	if err != nil || !changed {
		t.Fatalf("referenced configuration revision was ignored: %v", err)
	}
}
