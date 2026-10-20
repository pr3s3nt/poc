package registry

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/adapters/terraform"
	"orchestrator/internal/ports/moduleregistry"
)

func TestInspectGitComputesPinAndSurvivesRegistryRestart(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t, map[string]string{"new-directory/main.tf": rootModule})
	// No repository/module allowlist is required for this preparation flow.
	f.svc.cfg.Policy = Policy{}
	request := moduleregistry.GitInspection{ResourceType: "redis", URL: repoURL, Path: "new-directory", Ref: commitA}
	bundle, err := f.svc.InspectGit(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Ref.Revision != commitA || bundle.Ref.ContentSHA256 == "" || bundle.Contract.SourceURL != repoURL {
		t.Fatalf("bad inspected pin: %+v", bundle.Ref)
	}
	if _, err = f.svc.Verify(ctx, bundle.Ref); err != nil {
		t.Fatal(err)
	}
	tampered := bundle.Ref
	tampered.ContentSHA256 = strings.Repeat("0", 64)
	if _, err = f.svc.Verify(ctx, tampered); !errors.Is(err, moduleregistry.ErrDrift) {
		t.Fatalf("digest tampering: %v", err)
	}
	restarted, err := New(Config{Release: f.release, Policy: Policy{}, Packaged: terraform.PackagedModules(), Cache: f.cache, Store: f.store, Git: f.git})
	if err != nil {
		t.Fatal(err)
	}
	before := f.git.opens
	got, err := restarted.Verify(ctx, bundle.Ref)
	if err != nil || got.ID != bundle.ID || f.git.opens != before {
		t.Fatalf("restart re-resolved or lost pin: %v", err)
	}
	// A corrupt cached artifact remains an error; inspection is not a repair bypass.
	dir := sourceDir(f.cache.bundleDir(bundle.Ref), rootDir(bundle.Ref))
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte("drift"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.InspectGit(ctx, request); !errors.Is(err, moduleregistry.ErrDrift) {
		t.Fatalf("drift repaired: %v", err)
	}
}

func TestInspectGitRefResolutionAndInputGuards(t *testing.T) {
	g := newGitServer(t)
	commit := g.commit(t, map[string]string{"modules/custom/main.tf": rootModule}, nil, nil)
	runGit(t, g.work, "tag", "-a", "v1.2.0", "-m", "release")
	runGit(t, g.work, "push", filepath.Join(filepath.Dir(g.work), "repo.git"), "refs/tags/v1.2.0")
	cli, err := NewGitCLI("git", privateDir(t))
	if err != nil {
		t.Fatal(err)
	}
	source := GitSource{URL: g.url, CAFile: g.caFile}
	for _, ref := range []string{"v1.2.0", "refs/tags/v1.2.0", "main"} {
		got, err := cli.Resolve(context.Background(), source, ref)
		if err != nil || got != commit {
			t.Fatalf("%s resolved %s: %v", ref, got, err)
		}
	}
	// CA is platform TLS configuration only, not a repository/module permission.
	release, _ := DefaultRelease()
	cache, err := OpenCache(privateDir(t))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := New(Config{Release: release, Cache: cache, Store: store.New(), Git: cli, Packaged: terraform.PackagedModules(), Policy: Policy{Sources: []GitSource{{ID: "tls", Kind: "git", URL: g.url, CAFile: g.caFile, Modules: []GitModule{{ID: "unrelated", Subdirectory: "other", ResourceTypes: []string{"other"}}}}}}})
	if err != nil {
		t.Fatal(err)
	}
	req := moduleregistry.GitInspection{ResourceType: "redis", URL: g.url, Path: "modules/custom", Ref: "v1.2.0"}
	got, err := svc.InspectGit(context.Background(), req)
	if err != nil || got.Ref.Revision != commit {
		t.Fatalf("inspect annotated tag: %v", err)
	}
	for name, change := range map[string]func(*moduleregistry.GitInspection){
		"http":       func(r *moduleregistry.GitInspection) { r.URL = "http://example.com/repo" },
		"credential": func(r *moduleregistry.GitInspection) { r.URL = "https://secret:token@example.com/repo" },
		"traversal":  func(r *moduleregistry.GitInspection) { r.Path = "../module" },
		"option":     func(r *moduleregistry.GitInspection) { r.Ref = "--upload-pack=bad" },
		"refspec":    func(r *moduleregistry.GitInspection) { r.Ref = "main:refs/heads/x" },
	} {
		t.Run(name, func(t *testing.T) {
			bad := req
			change(&bad)
			if _, err := svc.InspectGit(context.Background(), bad); !errors.Is(err, moduleregistry.ErrNotAllowed) {
				t.Fatalf("unsafe source: %v", err)
			}
		})
	}
}

func TestInspectGitPinsUnlistedRemoteChildren(t *testing.T) {
	f := newFixture(t, map[string]string{"main.tf": rootModule + "\nmodule \"nested\" { source = \"git::" + childURL + "//net?ref=" + commitB + "\" }\n"})
	f.svc.cfg.Policy = Policy{}
	f.git.repos[childURL] = map[string]map[string]string{commitB: {"net/main.tf": "output \"value\" { value = \"ok\" }"}}
	got, err := f.svc.InspectGit(context.Background(), moduleregistry.GitInspection{ResourceType: "redis", URL: repoURL, Ref: commitA})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Contract.Transitive) != 1 || got.Contract.Transitive[0].Revision != commitB {
		t.Fatalf("child not pinned: %+v", got.Contract.Transitive)
	}
}
