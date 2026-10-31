package catalogrelease

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/application/envops"
	"orchestrator/internal/domain/application"
	domain "orchestrator/internal/domain/catalogrelease"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/ports/catalogsource"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/seed/seedtest"
)

type discoverFunc func(context.Context, domain.Source) (catalogsource.Observation, error)

func (f discoverFunc) Discover(c context.Context, s domain.Source) (catalogsource.Observation, error) {
	return f(c, s)
}

type inspectFunc func(context.Context, domain.Source, catalogsource.Tag, catalogsource.Observation) (domain.Release, error)

func (f inspectFunc) InspectRelease(c context.Context, s domain.Source, t catalogsource.Tag, o catalogsource.Observation) (domain.Release, error) {
	return f(c, s, t, o)
}
func fixtureRelease(source domain.Source, tag catalogsource.Tag, o catalogsource.Observation) domain.Release {
	raw := []byte("apiVersion: orchestrator.dev/catalog/v1\ncomponents:\n- id: cluster\n  kind: definition\n  path: definitions/cluster.yaml\n")
	d := resource.Definition{Key: "cluster", Name: "Cluster", ResourceTypeKey: resource.ClusterTargetType, DriverType: resource.DriverExistingCluster, Version: 1, Criteria: []resource.Criterion{{}}}
	return domain.Release{Organization: source.Organization, SourceID: source.ID, Tag: tag.Name, Commit: tag.Commit, BranchTip: o.BranchTip, Manifest: raw, ManifestSHA256: domain.Digest(raw), ObservedAt: o.ObservedAt, Components: []domain.VerifiedComponent{{Component: domain.Component{ID: "cluster", Kind: domain.Definition, Path: "definitions/cluster.yaml"}, SHA256: domain.Digest([]byte("definition")), Snapshot: &d, Binding: "internal"}}}
}
func setupService(t *testing.T) (*Service, *store.Store, domain.Source) {
	t.Helper()
	st := store.New()
	opts := seedtest.Defaults()
	if err := seedtest.Apply(context.Background(), st, opts); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveApplication(context.Background(), application.Application{Key: opts.ApplicationKey, OrganizationKey: opts.OrganizationKey, Name: "Catalog test", Subdomain: "catalog-test"}); err != nil {
		t.Fatal(err)
	}
	for _, env := range []string{"staging", "production"} {
		if err := st.SaveEnvironment(context.Background(), environment.Environment{Key: env, ApplicationKey: opts.ApplicationKey, NamespaceIdentity: "catalog-" + env}); err != nil {
			t.Fatal(err)
		}
	}
	source := domain.Source{Organization: opts.OrganizationKey, ID: "catalog", Repository: "https://example.com/catalog.git", Branch: "main", TagPattern: "v*", ManifestPath: "catalog.yaml", Version: 1, AccountBindings: map[string]string{"internal": "internal"}}
	service := &Service{Store: st, Inspector: inspectFunc(func(_ context.Context, s domain.Source, t catalogsource.Tag, o catalogsource.Observation) (domain.Release, error) {
		return fixtureRelease(s, t, o), nil
	})}
	if _, err := service.Save(context.Background(), 0, source); err != nil {
		t.Fatal(err)
	}
	return service, st, source
}
func TestRefreshPublishesCompleteSetsAndRejectsMovedTags(t *testing.T) {
	s, st, source := setupService(t)
	ctx := context.Background()
	commit := strings.Repeat("a", 40)
	s.Discovery = discoverFunc(func(context.Context, domain.Source) (catalogsource.Observation, error) {
		return catalogsource.Observation{BranchTip: commit, ObservedAt: time.Now().UTC(), Tags: []catalogsource.Tag{{Name: "v1.0.0", Commit: commit, InBranch: true}, {Name: "v2.0.0", Commit: commit, InBranch: false}}}, nil
	})
	got, err := s.Refresh(ctx, source.Organization, source.ID)
	if err != nil || got.Code != "TAG_OUTSIDE_BRANCH" {
		t.Fatal(got, err)
	}
	releases, _ := st.ListCatalogReleases(ctx, source.Organization, source.ID)
	if len(releases) != 1 {
		t.Fatal(releases)
	}
	defsBefore, _ := st.ListResourceDefinitions(ctx, source.Organization)
	commit = strings.Repeat("b", 40)
	got, err = s.Refresh(ctx, source.Organization, source.ID)
	if err != nil || got.Code != "TAG_CONFLICT" {
		t.Fatal(got, err)
	}
	old, _ := st.GetCatalogRelease(ctx, source.Organization, source.ID, "v1.0.0")
	if old.Commit != strings.Repeat("a", 40) {
		t.Fatal("release overwritten")
	}
	s.Discovery = discoverFunc(func(context.Context, domain.Source) (catalogsource.Observation, error) {
		return catalogsource.Observation{}, errors.New("secret transport diagnostic")
	})
	got, err = s.Refresh(ctx, source.Organization, source.ID)
	if err != nil || got.Code != "SOURCE_UNAVAILABLE" {
		t.Fatal(got, err)
	}
	releases, _ = st.ListCatalogReleases(ctx, source.Organization, source.ID)
	if len(releases) != 1 {
		t.Fatal("remote failure deleted release")
	}
	defsAfter, _ := st.ListResourceDefinitions(ctx, source.Organization)
	if len(defsBefore) != len(defsAfter) {
		t.Fatal("discovery changed global registry")
	}
	if other, _ := st.ListCatalogReleases(ctx, "different-org", source.ID); len(other) != 0 {
		t.Fatal("cross-org release read")
	}
}
func TestFailedInspectionIsNotPublishedAndTagIdentityIsRetained(t *testing.T) {
	s, st, source := setupService(t)
	ctx := context.Background()
	commit := strings.Repeat("a", 40)
	s.Discovery = discoverFunc(func(context.Context, domain.Source) (catalogsource.Observation, error) {
		return catalogsource.Observation{BranchTip: commit, ObservedAt: time.Now().UTC(), Tags: []catalogsource.Tag{{Name: "v1.0.0", Commit: commit, InBranch: true}}}, nil
	})
	s.Inspector = inspectFunc(func(context.Context, domain.Source, catalogsource.Tag, catalogsource.Observation) (domain.Release, error) {
		return domain.Release{}, domain.ErrIncomplete
	})
	got, err := s.Refresh(ctx, source.Organization, source.ID)
	if err != nil || got.Code != "RELEASE_INVALID" || got.ObservedTags["v1.0.0"] != commit {
		t.Fatal(got, err)
	}
	releases, _ := st.ListCatalogReleases(ctx, source.Organization, source.ID)
	if len(releases) != 0 {
		t.Fatal("partial release published")
	}
}
func TestRefreshConcurrentSourceEditCannotPublish(t *testing.T) {
	s, st, source := setupService(t)
	ctx := context.Background()
	s.Discovery = discoverFunc(func(context.Context, domain.Source) (catalogsource.Observation, error) {
		newer := source
		newer.Version++
		newer.TagPattern = "v1*"
		if _, err := s.Save(ctx, 1, newer); err != nil {
			t.Fatal(err)
		}
		return catalogsource.Observation{BranchTip: strings.Repeat("a", 40), ObservedAt: time.Now().UTC(), Tags: []catalogsource.Tag{{Name: "v1.0.0", Commit: strings.Repeat("a", 40), InBranch: true}}}, nil
	})
	if _, err := s.Refresh(ctx, source.Organization, source.ID); !errors.Is(err, persistence.ErrVersionConflict) {
		t.Fatal(err)
	}
	releases, _ := st.ListCatalogReleases(ctx, source.Organization, source.ID)
	if len(releases) != 0 {
		t.Fatal("stale refresh published")
	}
}

func TestEnvironmentSelectionsAreIndependentAndDoNotReadGlobalDefinitions(t *testing.T) {
	s, st, source := setupService(t)
	ctx := context.Background()
	opts := seedtest.Defaults()
	o := catalogsource.Observation{BranchTip: strings.Repeat("a", 40), ObservedAt: time.Now().UTC()}
	for _, tag := range []string{"v1.0.0", "v2.0.0"} {
		r := fixtureRelease(source, catalogsource.Tag{Name: tag, Commit: o.BranchTip}, o)
		if err := st.Transact(ctx, func(ctx context.Context) error { return st.InsertCatalogRelease(ctx, r) }); err != nil {
			t.Fatal(err)
		}
	}
	staging, err := st.GetEnvironment(ctx, opts.ApplicationKey, "staging")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Select(ctx, source.Organization, opts.ApplicationKey, "staging", source.ID, "v1.0.0", staging.Version, 0); err != nil {
		t.Fatal(err)
	}
	defs, pin, err := ResolveDefinitions(ctx, st, source.Organization, opts.ApplicationKey, "staging", []resource.Definition{{Key: "must-not-leak"}})
	if err != nil || len(defs) != 1 || defs[0].Key != "cluster" || defs[0].DriverAccount != "internal" || pin == "" {
		t.Fatal(defs, pin, err)
	}
	// An unrelated global Definition cannot be mixed into a pinned Environment.
	if err := st.SaveResourceDefinition(ctx, source.Organization, resource.Definition{Key: "other", Name: "Other", ResourceTypeKey: resource.ClusterTargetType, DriverType: resource.DriverExistingCluster, DriverAccount: "changed", Version: 1, Criteria: []resource.Criterion{{}}}); err != nil {
		t.Fatal(err)
	}
	again, samePin, err := ResolveDefinitions(ctx, st, source.Organization, opts.ApplicationKey, "staging", []resource.Definition{{Key: "other"}})
	if err != nil || len(again) != 1 || samePin != pin {
		t.Fatal("global edit changed catalog", err)
	}
	legacy, err := st.GetCatalogEnvironment(ctx, opts.ApplicationKey, "production")
	if err != nil || legacy.Desired != nil {
		t.Fatal("staging selection changed production")
	}
	if _, err = s.Select(ctx, source.Organization, opts.ApplicationKey, "staging", source.ID, "v2.0.0", staging.Version, 1); !errors.Is(err, persistence.ErrVersionConflict) {
		t.Fatal("stale selection accepted", err)
	}
	usage, err := s.Usage(ctx, source.Organization, source.ID)
	if err != nil || len(usage) != 1 || usage[0].EnvironmentKey != "staging" || !usage[0].Desired || usage[0].Applied {
		t.Fatal("incorrect scoped release usage", usage, err)
	}
	foreign, err := s.Usage(ctx, "other-org", source.ID)
	if err != nil || len(foreign) != 0 {
		t.Fatal("cross-org release usage leak", err)
	}
	stored, _ := st.GetCatalogEnvironment(ctx, opts.ApplicationKey, "staging")
	if stored.Applied != nil || stored.Status != "PENDING" {
		t.Fatal("selection mutated applied release")
	}
	if err := st.ActivateCatalog(ctx, opts.ApplicationKey, "staging", 1, "APPLIED"); !errors.Is(err, persistence.ErrOperationLost) {
		t.Fatal("unfenced activation accepted", err)
	}
}

func TestAppliedSelectionRequiresTheSameEnvironmentFenceAndPreservesPartial(t *testing.T) {
	s, st, source := setupService(t)
	ctx := context.Background()
	opts := seedtest.Defaults()
	observation := catalogsource.Observation{BranchTip: strings.Repeat("a", 40), ObservedAt: time.Now().UTC()}
	for _, tag := range []string{"v1.0.0", "v2.0.0"} {
		r := fixtureRelease(source, catalogsource.Tag{Name: tag, Commit: observation.BranchTip}, observation)
		if err := st.InsertCatalogRelease(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	current, _ := st.GetEnvironment(ctx, opts.ApplicationKey, "staging")
	if _, err := s.Select(ctx, source.Organization, opts.ApplicationKey, "staging", source.ID, "v1.0.0", current.Version, 0); err != nil {
		t.Fatal(err)
	}
	manager := envops.NewManager(st)
	first, err := manager.Begin(ctx, envops.Claim{ApplicationKey: opts.ApplicationKey, EnvironmentKey: "staging", Kind: environment.OpDeploy})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ActivateCatalog(first.Ctx, opts.ApplicationKey, "staging", 1, "APPLIED"); err != nil {
		t.Fatal(err)
	}
	if err := st.ActivateCatalog(first.Ctx, opts.ApplicationKey, "production", 1, "APPLIED"); !errors.Is(err, persistence.ErrOperationLost) {
		t.Fatal("another Environment accepted a foreign fence", err)
	}
	if err := first.End(environment.OpSucceeded, ""); err != nil {
		t.Fatal(err)
	}
	current, _ = st.GetEnvironment(ctx, opts.ApplicationKey, "staging")
	if _, err := s.Select(ctx, source.Organization, opts.ApplicationKey, "staging", source.ID, "v2.0.0", current.Version, 1); err != nil {
		t.Fatal(err)
	}
	next, err := manager.Begin(ctx, envops.Claim{ApplicationKey: opts.ApplicationKey, EnvironmentKey: "staging", Kind: environment.OpDeploy})
	if err != nil {
		t.Fatal(err)
	}
	defer next.End(environment.OpFailed, "fixture partial")
	if err := st.ActivateCatalog(next.Ctx, opts.ApplicationKey, "staging", 2, "PARTIAL"); err != nil {
		t.Fatal(err)
	}
	stored, _ := st.GetCatalogEnvironment(ctx, opts.ApplicationKey, "staging")
	if stored.Applied == nil || stored.Applied.Tag != "v1.0.0" || stored.Desired.Tag != "v2.0.0" || stored.Status != "PARTIAL" {
		t.Fatal("partial upgrade changed applied provenance", stored)
	}
	if err := st.ActivateCatalog(first.Ctx, opts.ApplicationKey, "staging", 2, "APPLIED"); !errors.Is(err, persistence.ErrOperationLost) {
		t.Fatal("released fence advanced applied release", err)
	}
}
