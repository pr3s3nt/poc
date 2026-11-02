package seedtest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"orchestrator/internal/application/envops"
	"orchestrator/internal/domain/catalogrelease"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/seed"
)

// PublishCatalog explicitly snapshots test Definitions. This is synthetic test
// provenance, never Git discovery and never called from product code. Tests
// changing Definitions must publish a new fixture release to select the change.
func PublishCatalog(ctx context.Context, st persistence.Store, o seed.Options) error {
	env, err := st.GetEnvironment(ctx, o.ApplicationKey, o.EnvironmentKey)
	if err != nil {
		return err
	}
	defs, err := st.ListResourceDefinitions(ctx, o.OrganizationKey)
	if err != nil {
		return err
	}
	if len(defs) == 0 {
		return nil
	}
	old, err := st.GetCatalogEnvironment(ctx, o.ApplicationKey, o.EnvironmentKey)
	if err != nil {
		return err
	}
	components := []catalogrelease.VerifiedComponent{}
	manifest := catalogrelease.Manifest{APIVersion: catalogrelease.APIVersion}
	accounts := map[string]string{}
	for _, d := range defs {
		binding := ""
		if d.DriverAccount != "" {
			binding = d.DriverAccount
			accounts[binding] = binding
			d.DriverAccount = ""
		}
		raw, _ := json.Marshal(d)
		component := catalogrelease.Component{ID: d.Key, Kind: catalogrelease.Definition, Path: "definitions/" + d.Key + ".json"}
		manifest.Components = append(manifest.Components, component)
		snapshot := d
		components = append(components, catalogrelease.VerifiedComponent{Component: component, SHA256: catalogrelease.Digest(raw), Snapshot: &snapshot, Binding: binding})
	}
	renders, err := persistence.RenderBundlesOf(ctx, st, o.OrganizationKey)
	if err != nil {
		return err
	}
	for _, b := range renders {
		c := catalogrelease.Component{ID: fmt.Sprintf("render-%d", len(components)), Kind: catalogrelease.RenderBundle, Path: fmt.Sprintf("renders/%d", len(components))}
		manifest.Components = append(manifest.Components, c)
		components = append(components, catalogrelease.VerifiedComponent{Component: c, SHA256: b.Source.ContentSHA256, BundleID: b.ID})
	}
	releases, err := st.ListCatalogReleases(ctx, o.OrganizationKey, "test-fixture")
	if err != nil {
		return err
	}
	raw, _ := json.Marshal(manifest)
	release := catalogrelease.Release{Organization: o.OrganizationKey, SourceID: "test-fixture", Tag: fmt.Sprintf("v1.0.%d", len(releases)), Commit: strings.Repeat("a", 40), BranchTip: strings.Repeat("a", 40), Manifest: raw, ManifestSHA256: catalogrelease.Digest(raw), Components: components, ObservedAt: time.Now().UTC()}
	if old.Desired != nil && old.Desired.Fingerprint == release.Fingerprint() {
		return nil
	}
	if err := release.Validate(); err != nil {
		return err
	}
	err = st.Transact(ctx, func(ctx context.Context) error {
		if _, err := st.GetCatalogSource(ctx, o.OrganizationKey, "test-fixture"); errors.Is(err, persistence.ErrNotFound) {
			if err := st.SaveCatalogSource(ctx, 0, persistence.CatalogSource{Source: catalogrelease.Source{Organization: o.OrganizationKey, ID: "test-fixture", Repository: "https://example.com/test-fixture.git", Branch: "main", TagPattern: "v*", ManifestPath: "catalog.yaml", Version: 1}}); err != nil {
				return err
			}
		}
		if err := st.InsertCatalogRelease(ctx, release); err != nil {
			return err
		}
		_, err := st.SelectCatalog(ctx, persistence.CatalogSelectionCommand{Organization: o.OrganizationKey, ApplicationKey: o.ApplicationKey, EnvironmentKey: o.EnvironmentKey, ExpectedEnvironmentVersion: env.Version, ExpectedRevision: old.Revision, Selection: catalogrelease.Selection{SourceID: release.SourceID, Tag: release.Tag, Fingerprint: release.Fingerprint(), Accounts: accounts}})
		if err != nil {
			return err
		}
		// Setup keeps the initial Environment version used by unrelated assertions.
		return st.SaveEnvironment(ctx, env)
	})
	if err != nil {
		return err
	}
	if old.Desired == nil {
		return ApplyCatalogFixture(ctx, st, o)
	}
	return nil
}

// ApplyCatalogFixture initializes an acknowledged synthetic baseline for tests
// using the internal single-workload executor. Public deployments use pending.
func ApplyCatalogFixture(ctx context.Context, st persistence.Store, o seed.Options) error {
	env, err := st.GetEnvironment(ctx, o.ApplicationKey, o.EnvironmentKey)
	if err != nil {
		return err
	}
	binding, err := st.GetCatalogEnvironment(ctx, o.ApplicationKey, o.EnvironmentKey)
	if err != nil {
		return err
	}
	lease, err := envops.NewManager(st).Begin(ctx, envops.Claim{ApplicationKey: o.ApplicationKey, EnvironmentKey: o.EnvironmentKey, Kind: environment.OpDeploy})
	if err != nil {
		return err
	}
	activateErr := st.ActivateCatalog(lease.Ctx, o.ApplicationKey, o.EnvironmentKey, binding.Revision, "APPLIED")
	endErr := lease.End(environment.OpSucceeded, "")
	if activateErr != nil {
		return activateErr
	}
	if endErr != nil {
		return endErr
	}
	return st.SaveEnvironment(ctx, env)
}
