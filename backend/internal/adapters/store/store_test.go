package store

import (
	"context"
	"errors"
	"testing"

	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/ports/persistence"
)

func TestCompareVersionAndSetCurrentRejectsStaleVersion(t *testing.T) {
	ctx := context.Background()
	s := New()
	set := environment.DeploymentSet{ID: "set-1", EnvironmentKey: "app/dev", Document: environment.NewDocument()}
	if err := s.SaveDeploymentSet(ctx, set); err != nil {
		t.Fatalf("save set: %v", err)
	}
	env := environment.Environment{Key: "dev", ApplicationKey: "app", NamespaceIdentity: "app-dev", Version: 3}
	if err := s.SaveEnvironment(ctx, env); err != nil {
		t.Fatalf("save env: %v", err)
	}
	if err := s.CompareVersionAndSetCurrent(ctx, "app", "dev", 2, "set-1"); !errors.Is(err, persistence.ErrVersionConflict) {
		t.Fatalf("expected a version conflict, got %v", err)
	}
	if err := s.CompareVersionAndSetCurrent(ctx, "app", "dev", 3, "set-1"); err != nil {
		t.Fatalf("expected the commit to succeed: %v", err)
	}
	updated, err := s.GetEnvironment(ctx, "app", "dev")
	if err != nil {
		t.Fatalf("get env: %v", err)
	}
	if updated.CurrentDeploymentSetID != "set-1" || updated.Version != 4 {
		t.Fatalf("unexpected environment after commit: %#v", updated)
	}
}

func TestUpsertActiveResourceKeepsLogicalIdentity(t *testing.T) {
	ctx := context.Background()
	s := New()
	descriptor, err := resource.NewDescriptor("postgres", "default", "acceptance-db")
	if err != nil {
		t.Fatalf("descriptor: %v", err)
	}
	scope := resource.Scope{Type: resource.ScopeShared, ID: "app.dev"}
	first, err := s.UpsertActiveResource(ctx, resource.ActiveResource{
		OrganizationKey: "acme", Descriptor: descriptor, Scope: scope, Status: resource.StatusReady,
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	second, err := s.UpsertActiveResource(ctx, resource.ActiveResource{
		OrganizationKey: "acme", Descriptor: descriptor, Scope: scope, Status: resource.StatusReady,
	})
	if err != nil {
		t.Fatalf("upsert again: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("logical identity must keep one row: %s vs %s", first.ID, second.ID)
	}
	if second.Version != first.Version+1 {
		t.Fatalf("expected the version to advance: %d -> %d", first.Version, second.Version)
	}
	list, err := s.ListActiveResources(ctx, "acme")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected one active resource, got %d", len(list))
	}
}

func TestTransactRollsBackOnError(t *testing.T) {
	ctx := context.Background()
	s := New()
	wantErr := errors.New("boom")
	err := s.Transact(ctx, func(ctx context.Context) error {
		if err := s.SaveDeploymentSet(ctx, environment.DeploymentSet{ID: "set-x", Document: environment.NewDocument()}); err != nil {
			return err
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := s.GetDeploymentSet(ctx, "set-x"); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("expected the write to be rolled back, got %v", err)
	}
}

func TestCatalogIsScopedByOrganization(t *testing.T) {
	ctx := context.Background()
	s := New()
	for _, org := range []string{"acme", "other"} {
		if err := s.SaveResourceType(ctx, org, resource.Type{Key: "postgres", Outputs: []resource.OutputField{{Name: "host", Type: "string"}}}); err != nil {
			t.Fatal(err)
		}
		if err := s.SaveResourceDefinition(ctx, org, resource.Definition{Key: "postgres-kind", ResourceTypeKey: "postgres", DriverType: resource.DriverKubernetes, Criteria: []resource.Criterion{{}}}); err != nil {
			t.Fatal(err)
		}
	}
	for _, org := range []string{"acme", "other"} {
		types, err := s.ListResourceTypes(ctx, org)
		if err != nil || len(types) != 1 || types[0].Key != "postgres" {
			t.Fatalf("%s types: %#v, %v", org, types, err)
		}
		defs, err := s.ListResourceDefinitions(ctx, org)
		if err != nil || len(defs) != 1 || defs[0].Key != "postgres-kind" {
			t.Fatalf("%s definitions: %#v, %v", org, defs, err)
		}
	}
	types, err := s.ListResourceTypes(ctx, "unknown")
	if err != nil || len(types) != 0 {
		t.Fatalf("unknown organization could read catalog: %#v, %v", types, err)
	}
}
