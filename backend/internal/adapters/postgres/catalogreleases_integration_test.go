package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/catalogrelease"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
)

func TestCatalogReleasePersistencePrivilegesAndCAS(t *testing.T) {
	db := freshDatabase(t)
	ctx := context.Background()
	st, err := Open(ctx, db.Console)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	org := application.Organization{ID: ids.New(), Key: "catalog-org", Name: "Catalog"}
	if err := st.SaveOrganization(ctx, org); err != nil {
		t.Fatal(err)
	}
	source := persistence.CatalogSource{Source: catalogrelease.Source{Organization: org.Key, ID: "source", Repository: "https://example.com/catalog.git", Branch: "main", TagPattern: "v*", ManifestPath: "catalog.yaml", Version: 1}}
	if err := st.SaveCatalogSource(ctx, 0, source); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveCatalogSource(ctx, 0, source); !errors.Is(err, persistence.ErrVersionConflict) {
		t.Fatal("CAS not enforced", err)
	}
	raw := []byte("apiVersion: orchestrator.dev/catalog/v1\ncomponents:\n- id: cluster\n  kind: definition\n  path: cluster.yaml\n")
	d := resource.Definition{Key: "cluster", Name: "Cluster", ResourceTypeKey: resource.ClusterTargetType, DriverType: resource.DriverExistingCluster, Version: 1, Criteria: []resource.Criterion{{}}}
	r := catalogrelease.Release{Organization: org.Key, SourceID: "source", Tag: "v1.0.0", Commit: strings.Repeat("a", 40), BranchTip: strings.Repeat("a", 40), Manifest: raw, ManifestSHA256: catalogrelease.Digest(raw), ObservedAt: time.Now().UTC(), Components: []catalogrelease.VerifiedComponent{{Component: catalogrelease.Component{ID: "cluster", Kind: catalogrelease.Definition, Path: "cluster.yaml"}, SHA256: catalogrelease.Digest([]byte("cluster")), Snapshot: &d, Binding: "internal"}}}
	if err := st.InsertCatalogRelease(ctx, r); err != nil {
		t.Fatal(err)
	}
	if err := st.InsertCatalogRelease(ctx, r); err != nil {
		t.Fatal("idempotent insert", err)
	}
	r.Commit = strings.Repeat("b", 40)
	if err := st.InsertCatalogRelease(ctx, r); !errors.Is(err, catalogrelease.ErrConflict) {
		t.Fatal("immutable release overwritten", err)
	}
	reopened, err := Open(ctx, db.Console)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err := reopened.GetCatalogRelease(ctx, org.Key, "source", "v1.0.0")
	if err != nil || got.Commit != strings.Repeat("a", 40) {
		t.Fatal("restart lost pin", err)
	}
	other, err := reopened.ListCatalogReleases(ctx, "other-org", "source")
	if err != nil || len(other) != 0 {
		t.Fatal("cross-org release leak", err)
	}
	descriptor := resource.Descriptor{Type: resource.ClusterTargetType, Class: "default", ID: "catalog-only"}
	active, err := st.UpsertActiveResource(ctx, resource.ActiveResource{OrganizationKey: org.Key, Descriptor: descriptor, Scope: resource.Scope{Type: resource.ScopeEnvironment, ID: "catalog-env"}, DefinitionKey: "cluster", CatalogSource: "source", CatalogTag: "v1.0.0", Status: resource.StatusReady, Outputs: map[string]any{}, ExecutorState: map[string]any{}})
	if err != nil {
		t.Fatal("catalog-only Definition could not acknowledge a resource", err)
	}
	loaded, err := st.FindByLogicalIdentity(ctx, org.Key, descriptor, active.Scope)
	if err != nil || loaded.ID != active.ID || loaded.CatalogSource != "source" || loaded.DefinitionKey != "cluster" {
		t.Fatal("catalog provenance lost", loaded, err)
	}
	active.DefinitionKey = "absent"
	if _, err := st.UpsertActiveResource(ctx, active); err == nil {
		t.Fatal("Definition absent from release accepted")
	}
	denied(t, connectAs(t, db.Console), "immutable release update", `UPDATE catalog_releases SET fingerprint=repeat('b',64)`)
	denied(t, connectAs(t, db.Console), "immutable release delete", `DELETE FROM catalog_releases`)
	denied(t, connectAs(t, db.State), "catalog source read", `SELECT document FROM catalog_sources`)
	denied(t, connectAs(t, db.State), "catalog release read", `SELECT document FROM catalog_releases`)
	denied(t, connectAs(t, db.Console), "catalog DDL", `ALTER TABLE catalog_sources ADD COLUMN unsafe text`)
}
