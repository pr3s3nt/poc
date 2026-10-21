package registry

import (
	"context"
	"errors"
	"io/fs"
	"orchestrator/internal/domain/tfmodule"
	"os"
	"path/filepath"
	"reflect"
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

// Published modules retain trusted lifecycle semantics only for exact release
// content and the corresponding Type; a Git path never grants those semantics.
func TestInspectGitReleaseCopiesKeepLifecycleIdentity(t *testing.T) {
	ctx := context.Background()
	release, err := DefaultRelease()
	if err != nil {
		t.Fatal(err)
	}
	for _, module := range release.Packaged.Modules {
		t.Run(module.ID, func(t *testing.T) {
			repo := map[string]string{}
			err := fs.WalkDir(terraform.PackagedModules(), module.Subdirectory, func(path string, entry fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if entry.IsDir() {
					return nil
				}
				data, err := fs.ReadFile(terraform.PackagedModules(), path)
				repo["published/"+strings.TrimPrefix(path, module.Subdirectory+"/")] = string(data)
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			f := newFixture(t, repo)
			request := moduleregistry.GitInspection{ResourceType: module.ResourceTypes[0], URL: repoURL, Path: "published", Ref: commitA}
			bundle, err := f.svc.InspectGit(ctx, request)
			if err != nil {
				t.Fatal(err)
			}
			if bundle.Ref.ModuleID != module.ID || bundle.Ref.SourceID == release.Packaged.SourceID || bundle.Ref.Revision != commitA || bundle.Ref.Subdirectory != "published" || bundle.Ref.ContentSHA256 != module.ContentSHA256 || bundle.Contract.SourceKind != tfmodule.SourceKindGit {
				t.Fatalf("lost Git pin or release identity: %+v", bundle.Ref)
			}
			if strings.Join(bundle.Contract.ExecutorOutputs, ",") != strings.Join(module.ExecutorOutputs, ",") {
				t.Fatalf("lost executor outputs: %v", bundle.Contract.ExecutorOutputs)
			}
			restarted, err := New(Config{Release: release, Cache: f.cache, Store: f.store, Packaged: terraform.PackagedModules(), Git: f.git})
			if err != nil {
				t.Fatal(err)
			}
			verified, err := restarted.Verify(ctx, bundle.Ref)
			if err != nil || !reflect.DeepEqual(verified.Contract, bundle.Contract) {
				t.Fatalf("release Git pin did not survive restart: %v", err)
			}

			request.ResourceType = "other-type"
			other, err := f.svc.InspectGit(ctx, request)
			if err != nil {
				t.Fatal(err)
			}
			if other.Ref.ModuleID == module.ID || len(other.Contract.ExecutorOutputs) != 0 {
				t.Fatal("content match granted semantics to a different Type")
			}
			request.ResourceType = module.ResourceTypes[0]
			f.git.repos[repoURL][commitB] = map[string]string{}
			for path, content := range repo {
				f.git.repos[repoURL][commitB][path] = content + "\n# changed release content\n"
			}
			request.Ref = commitB
			changed, err := f.svc.InspectGit(ctx, request)
			if err != nil {
				t.Fatal(err)
			}
			if changed.Ref.ModuleID == module.ID || len(changed.Contract.ExecutorOutputs) != 0 {
				t.Fatal("modified Git content inherited release semantics")
			}
		})
	}
}
