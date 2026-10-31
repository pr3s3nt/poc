package registry

import (
	"context"
	"orchestrator/internal/adapters/store"
	appcatalog "orchestrator/internal/application/catalog"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/catalogrelease"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/ports/catalogsource"
	"orchestrator/internal/ports/gitaccess"
	"orchestrator/internal/ports/moduleregistry"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func catalogFixtureSource(g gitServer) catalogrelease.Source {
	return catalogrelease.Source{Organization: "test", ID: "catalog", Repository: g.url, Branch: "main", TagPattern: "v*", ManifestPath: "catalog.yaml", Version: 1}
}
func TestCatalogDiscoveryAncestryAnnotatedTagsAndSemver(t *testing.T) {
	g := newGitServer(t, true)
	first := g.commit(t, map[string]string{"file": "one"}, nil, nil)
	runGit(t, g.work, "tag", "-a", "v1.9.0", "-m", "release")
	runGit(t, g.work, "tag", "v1.10.0-rc.1")
	tip := g.commit(t, map[string]string{"file": "two"}, nil, nil)
	runGit(t, g.work, "tag", "v1.10.0")
	runGit(t, g.work, "checkout", "--orphan", "unrelated")
	runGit(t, g.work, "commit", "-q", "--allow-empty", "-m", "outside")
	outside := runGit(t, g.work, "rev-parse", "HEAD")
	runGit(t, g.work, "tag", "v2.0.0")
	runGit(t, g.work, "push", filepath.Join(filepath.Dir(g.work), "repo.git"), "--tags")
	cli, err := NewGitCLI("git", privateDir(t))
	if err != nil {
		t.Fatal(err)
	}
	pem, err := os.ReadFile(g.caFile)
	if err != nil {
		t.Fatal(err)
	}
	cli.Access = draftAccess{repository: g.url, value: gitaccess.Access{Username: "reader", Token: "fixture-token", CAPEM: string(pem)}}
	source := catalogFixtureSource(g)
	observation, err := cli.Discover(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	if observation.BranchTip != tip || len(observation.Tags) != 3 || observation.Tags[0].Name != "v2.0.0" || observation.Tags[0].Commit != outside || observation.Tags[0].InBranch || observation.Tags[1].Name != "v1.10.0" || !observation.Tags[1].InBranch || observation.Tags[2].Commit != first {
		t.Fatalf("unexpected discovery: %+v", observation)
	}
	source.IncludePrereleases = true
	observation, err = cli.Discover(context.Background(), source)
	if err != nil || len(observation.Tags) != 4 {
		t.Fatal(observation, err)
	}
	entries, err := os.ReadDir(cli.TempDir)
	if err != nil || len(entries) != 0 {
		t.Fatal("private discovery snapshots not removed", err)
	}
}
func TestCatalogFileRejectsSymlink(t *testing.T) {
	g := newGitServer(t)
	commit := g.commit(t, map[string]string{"safe.yaml": "value"}, nil, map[string]string{"catalog.yaml": "safe.yaml"})
	cli, _ := NewGitCLI("git", privateDir(t))
	snap, err := cli.Open(context.Background(), GitSource{URL: g.url, CAFile: g.caFile}, commit)
	if err != nil {
		t.Fatal(err)
	}
	defer snap.Close()
	if _, err := snap.(*gitSnapshot).catalogFile("catalog.yaml"); err == nil {
		t.Fatal("symlink manifest accepted")
	}
	if raw, err := snap.(*gitSnapshot).catalogFile("safe.yaml"); err != nil || string(raw) != "value" {
		t.Fatal("safe file rejected", err)
	}
}

func TestCatalogInspectorPinsDefinitionsWithoutGlobalWrites(t *testing.T) {
	ctx := context.Background()
	g := newGitServer(t, true)
	manifest := "apiVersion: orchestrator.dev/catalog/v1\ncomponents:\n- id: cluster\n  kind: definition\n  path: definitions/cluster.yaml\n"
	definition := `{"binding":"internal","definition":{"key":"catalog-cluster","name":"Catalog cluster","resourceType":"k8s-cluster","driverType":"existing-cluster","driverInputs":{"values":{"variables":{}}},"version":1,"criteria":[{}]}}`
	commit := g.commit(t, map[string]string{"catalog.yaml": manifest, "definitions/cluster.yaml": definition}, nil, nil)
	cli, _ := NewGitCLI("git", privateDir(t))
	pem, err := os.ReadFile(g.caFile)
	if err != nil {
		t.Fatal(err)
	}
	cli.Access = draftAccess{repository: g.url, value: gitaccess.Access{Username: "reader", Token: "fixture-token", CAPEM: string(pem)}}
	st := store.New()
	opts := struct{ OrganizationKey, ConnectionKey string }{"catalog-org", "internal"}
	if err := st.SaveOrganization(ctx, application.Organization{Key: opts.OrganizationKey, Name: "Catalog"}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveResourceType(ctx, opts.OrganizationKey, resource.Type{Key: resource.ClusterTargetType, Outputs: []resource.OutputField{{Name: resource.ClusterTargetOutput, Type: "string"}}}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveConnection(ctx, application.Connection{OrganizationKey: opts.OrganizationKey, Key: opts.ConnectionKey, Name: "Internal", Kind: application.ConnectionKubernetes, AuthenticationType: application.AuthKubeconfig, Status: application.ConnectionReady}); err != nil {
		t.Fatal(err)
	}
	source := catalogFixtureSource(g)
	source.Organization = opts.OrganizationKey
	source.AccountBindings = map[string]string{"internal": opts.ConnectionKey}
	f := newFixture(t, map[string]string{"main.tf": rootModule})
	validator := appcatalog.NewService(st)
	inspector := CatalogInspector{Store: st, Git: cli, Modules: f.svc, Definitions: validator}
	observation := catalogsource.Observation{BranchTip: commit, ObservedAt: time.Now().UTC()}
	release, err := inspector.InspectRelease(ctx, source, catalogsource.Tag{Name: "v1.0.0", Commit: commit, InBranch: true}, observation)
	if err != nil || release.Validate() != nil || len(release.Components) != 1 {
		t.Fatal(release, err)
	}
	if release.Components[0].Snapshot.DriverAccount != "" || release.Components[0].Binding != "internal" {
		t.Fatal("platform binding leaked into Git snapshot")
	}
	definitions, err := st.ListResourceDefinitions(ctx, opts.OrganizationKey)
	if err != nil {
		t.Fatal(err)
	}
	for _, def := range definitions {
		if def.Key == "catalog-cluster" {
			t.Fatal("inspection modified global Definitions")
		}
	}
	source.AccountBindings = map[string]string{"internal": "does-not-exist"}
	if _, err := inspector.InspectRelease(ctx, source, catalogsource.Tag{Name: "v1.0.0", Commit: commit, InBranch: true}, observation); err == nil {
		t.Fatal("unresolvable platform account accepted")
	}
}
func TestCatalogModuleReuseChecksTransitiveAndPlatformPins(t *testing.T) {
	f := newFixture(t, map[string]string{"main.tf": rootModule})
	b, err := f.svc.InspectGit(context.Background(), moduleregistry.GitInspection{URL: repoURL, Path: ".", Ref: commitA, ResourceType: "redis"})
	if err != nil {
		t.Fatal(err)
	}
	next := b.Clone()
	next.Ref.Revision = commitB
	if !reuseCatalogModule(b, next) {
		t.Fatal("identical component unnecessarily repinned")
	}
	next.ProviderLockDigest = strings.Repeat("f", 64)
	if reuseCatalogModule(b, next) {
		t.Fatal("changed provider release reused old bundle")
	}
	next = b.Clone()
	next.Ref.Revision = commitB
	next.Contract.Transitive = append(next.Contract.Transitive, tfmodule.TransitiveModule{SourceID: "child", Revision: commitB, ContentSHA256: strings.Repeat("e", 64)})
	if reuseCatalogModule(b, next) {
		t.Fatal("changed transitive content reused old bundle")
	}
}
