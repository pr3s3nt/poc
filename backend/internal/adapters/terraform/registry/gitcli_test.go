package registry

import (
	"context"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/http/cgi"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/adapters/terraform"
	"orchestrator/internal/domain/tfmodule"
	access "orchestrator/internal/ports/gitaccess"
	"orchestrator/internal/ports/moduleregistry"
)

// gitServer is a task-owned local HTTPS Git server (git http-backend behind
// an httptest TLS server) holding one fixture repository.
type gitServer struct {
	url    string
	caFile string
	work   string
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1", "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func newGitServer(t *testing.T, authenticated ...bool) gitServer {
	t.Helper()
	backend, err := exec.Command("git", "--exec-path").Output()
	if err != nil {
		t.Fatalf("git is required for the HTTPS fixture: %v", err)
	}
	httpBackend := filepath.Join(strings.TrimSpace(string(backend)), "git-http-backend")
	if _, err := os.Stat(httpBackend); err != nil {
		t.Fatalf("git-http-backend is required for the HTTPS fixture: %v", err)
	}
	root := t.TempDir()
	work := filepath.Join(root, "work")
	if err := os.MkdirAll(work, 0o700); err != nil {
		t.Fatal(err)
	}
	runGit(t, work, "init", "-q", "-b", "main")
	runGit(t, root, "init", "-q", "--bare", "repo.git")
	runGit(t, filepath.Join(root, "repo.git"), "config", "uploadpack.allowAnySHA1InWant", "true")
	runGit(t, filepath.Join(root, "repo.git"), "config", "http.receivepack", "false")
	handler := &cgi.Handler{Path: httpBackend, Stderr: io.Discard, Env: []string{"GIT_PROJECT_ROOT=" + root, "GIT_HTTP_EXPORT_ALL=1", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null"}}
	var endpoint http.Handler = handler
	if len(authenticated) > 0 && authenticated[0] {
		endpoint = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, token, ok := r.BasicAuth()
			if !ok || user != "reader" || token != "fixture-token" {
				w.Header().Set("WWW-Authenticate", `Basic realm="git"`)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			handler.ServeHTTP(w, r)
		})
	}
	server := httptest.NewTLSServer(endpoint)
	t.Cleanup(server.Close)
	caFile := filepath.Join(root, "ca.pem")
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	if err := os.WriteFile(caFile, certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	return gitServer{url: server.URL + "/repo.git", caFile: caFile, work: work}
}

// commit writes files (mode 0600 unless listed executable), optional
// symlinks, commits and pushes; it returns the full commit ID.
func (g gitServer) commit(t *testing.T, files map[string]string, executable []string, symlinks map[string]string) string {
	t.Helper()
	for p, content := range files {
		target := filepath.Join(g.work, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range executable {
		if err := os.Chmod(filepath.Join(g.work, filepath.FromSlash(p)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for p, dest := range symlinks {
		if err := os.Symlink(dest, filepath.Join(g.work, filepath.FromSlash(p))); err != nil {
			t.Fatal(err)
		}
	}
	runGit(t, g.work, "add", "-A")
	runGit(t, g.work, "commit", "-q", "-m", "fixture")
	runGit(t, g.work, "push", "-q", filepath.Join(filepath.Dir(g.work), "repo.git"), "HEAD:refs/heads/main", "--force")
	return runGit(t, g.work, "rev-parse", "HEAD")
}

func TestGitCLIFetchesExactCommitOverHTTPS(t *testing.T) {
	g := newGitServer(t)
	good := g.commit(t, map[string]string{"modules/redis/main.tf": rootModule, "docs/README.md": "x"}, nil, nil)
	symlink := g.commit(t, nil, nil, map[string]string{"modules/redis/link.tf": "../../etc/passwd"})
	_ = os.Remove(filepath.Join(g.work, "modules/redis/link.tf"))
	exe := g.commit(t, map[string]string{"modules/redis/hook.sh": "#!/bin/sh\nid\n"}, []string{"modules/redis/hook.sh"}, nil)

	cli, err := NewGitCLI("git", privateDir(t))
	if err != nil {
		t.Fatal(err)
	}
	source := GitSource{ID: "acme", Kind: tfmodule.SourceKindGit, URL: g.url, CAFile: g.caFile, Modules: []GitModule{{ID: "redis", Subdirectory: "modules/redis", ResourceTypes: []string{"redis"}}}}
	ctx := context.Background()
	snap, err := cli.Open(ctx, source, good)
	if err != nil {
		t.Fatalf("fetch the pinned commit: %v", err)
	}
	tree, err := snap.Tree("modules/redis")
	snap.Close()
	if err != nil || string(tree["main.tf"]) != rootModule || len(tree) != 1 {
		t.Fatalf("tree %v %v", tree, err)
	}
	for name, commit := range map[string]string{"symlink": symlink, "executable": exe} {
		snap, err := cli.Open(ctx, source, commit)
		if err != nil {
			t.Fatalf("%s: fetch: %v", name, err)
		}
		_, err = snap.Tree("modules/redis")
		snap.Close()
		if !errors.Is(err, ErrUnsafeContent) {
			t.Fatalf("%s: want unsafe content, got %v", name, err)
		}
	}
	if _, err := cli.Open(ctx, source, strings.Repeat("d", 40)); !errors.Is(err, moduleregistry.ErrUnavailable) {
		t.Fatalf("unknown commit: %v", err)
	}
	untrusted := source
	untrusted.CAFile = ""
	if _, err := cli.Open(ctx, untrusted, good); !errors.Is(err, moduleregistry.ErrUnavailable) {
		t.Fatalf("an untrusted TLS certificate must not be accepted: %v", err)
	}
	if _, err := cli.Open(ctx, GitSource{URL: strings.Replace(g.url, "https://", "http://", 1)}, good); !errors.Is(err, moduleregistry.ErrUnavailable) {
		t.Fatalf("plain http must be refused: %v", err)
	}
	entries, _ := os.ReadDir(cli.TempDir)
	if len(entries) != 0 {
		t.Fatalf("fetch scratch directories were not removed: %d", len(entries))
	}

	// End to end through the service: policy, fetch, digest, inspect, cache.
	release, _ := DefaultRelease()
	cache, err := OpenCache(privateDir(t))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := New(Config{Release: release, Policy: Policy{Sources: []GitSource{source}}, Packaged: terraform.PackagedModules(), Git: cli, Cache: cache, Store: store.New()})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Prepare(ctx); err != nil {
		t.Fatal(err)
	}
	ref := tfmodule.SourceRef{SourceID: "acme", ModuleID: "redis", Revision: good, Subdirectory: "modules/redis", ContentSHA256: tree.Digest()}
	bundle, err := svc.Verify(ctx, ref)
	if err != nil || bundle.Ref != ref {
		t.Fatalf("verify over HTTPS: %v", err)
	}
	raw := bundle.Contract
	for _, leak := range []string{g.url, cache.dir} {
		for _, call := range raw.ModuleCalls {
			if strings.Contains(call.Source, leak) {
				t.Fatalf("contract leaks %q", leak)
			}
		}
	}
	branch := ref
	branch.Revision = "main"
	if _, err := svc.Verify(ctx, branch); !errors.Is(err, moduleregistry.ErrNotAllowed) {
		t.Fatalf("branch name: %v", err)
	}
}

var _ http.Handler = (*cgi.Handler)(nil)

func TestGitDraftAuthenticationAndPrivateCA(t *testing.T) {
	g := newGitServer(t, true)
	commit := g.commit(t, map[string]string{"main.tf": "# fixture"}, nil, nil)
	temp := privateDir(t)
	cli, err := NewGitCLI("git", temp)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := os.ReadFile(g.caFile)
	if err != nil {
		t.Fatal(err)
	}
	value := access.Access{Username: "reader", Token: "fixture-token", CAPEM: string(ca)}
	if err = cli.TestGit(context.Background(), g.url, "main", value); err != nil {
		t.Fatal("authenticated ref read failed", err)
	}
	if err = cli.TestGit(context.Background(), g.url, commit, value); err != nil {
		t.Fatal("authenticated commit read failed", err)
	}
	value.Token = ""
	if err = cli.TestGit(context.Background(), g.url, "main", value); err == nil {
		t.Fatal("private Git accepted anonymous access")
	}
	entries, err := os.ReadDir(temp)
	if err != nil || len(entries) != 0 {
		t.Fatal("Git/CA temporary files remained")
	}
}

func TestGitCredentialsNeverFollowRedirect(t *testing.T) {
	var targetReads atomic.Int64
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetReads.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer target.Close()
	redirect := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+r.URL.Path, http.StatusFound)
	}))
	defer redirect.Close()
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: redirect.Certificate().Raw})
	ca = append(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: target.Certificate().Raw})...)
	cli, err := NewGitCLI("git", privateDir(t))
	if err != nil {
		t.Fatal(err)
	}
	if err = cli.TestGit(context.Background(), redirect.URL+"/repo.git", "main", access.Access{Username: "reader", Token: "fixture-token", CAPEM: string(ca)}); err == nil {
		t.Fatal("redirect was accepted")
	}
	if targetReads.Load() != 0 {
		t.Fatal("authenticated request followed redirect")
	}
}
