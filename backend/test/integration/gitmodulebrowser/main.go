// Command gitmodulebrowser verifies only Git module authoring against the real
// HTTP/catalog/registry/Git adapters. It owns a temporary HTTPS Git fixture and
// an in-memory catalog; no Terraform, cloud, Kubernetes or database is invoked.
package main

import (
	"context"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"net/http/cgi"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/adapters/terraform"
	"orchestrator/internal/adapters/terraform/registry"
	"orchestrator/internal/application/authentication"
	delivery "orchestrator/internal/delivery/http"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/seed"
)

type trustedFixture struct {
	*registry.GitCLI
	ca string
}

func (g trustedFixture) Open(ctx context.Context, s registry.GitSource, commit string) (registry.Snapshot, error) {
	s.CAFile = g.ca
	return g.GitCLI.Open(ctx, s, commit)
}
func (g trustedFixture) Resolve(ctx context.Context, s registry.GitSource, ref string) (string, error) {
	s.CAFile = g.ca
	return g.GitCLI.Resolve(ctx, s, ref)
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	ui := flag.String("ui-dir", "../frontend/dist", "built Console")
	scenario := flag.String("scenario", "../frontend/test/e2e/git-module-local.mjs", "focused browser scenario")
	flag.Parse()
	dir, err := os.MkdirTemp("", "git-module-browser-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	git := func(work string, args ...string) (string, error) {
		cmd := exec.Command("git", args...)
		cmd.Dir = work
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=fixture", "GIT_AUTHOR_EMAIL=fixture@example.com", "GIT_COMMITTER_NAME=fixture", "GIT_COMMITTER_EMAIL=fixture@example.com", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
		out, err := cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("fixture git: %v %s", err, out)
		}
		return strings.TrimSpace(string(out)), nil
	}
	repo := filepath.Join(dir, "repo.git")
	if _, err := git(dir, "init", "--bare", repo); err != nil {
		return err
	}
	work := filepath.Join(dir, "work")
	if err := os.Mkdir(work, 0o700); err != nil {
		return err
	}
	if _, err := git(work, "init", "-b", "main"); err != nil {
		return err
	}
	if err := os.Mkdir(filepath.Join(work, "modules"), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(work, "modules", "main.tf"), []byte("variable \"label\" {\n type = string\n default = \"hello\"\n}\noutput \"name\" { value = var.label }\n"), 0o600); err != nil {
		return err
	}
	for _, args := range [][]string{{"add", "."}, {"commit", "-m", "fixture"}, {"tag", "-a", "v1.0.0", "-m", "release"}, {"push", repo, "main", "--tags"}} {
		if _, err := git(work, args...); err != nil {
			return err
		}
	}
	commit, err := git(work, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	execPath, err := git(work, "--exec-path")
	if err != nil {
		return err
	}
	source := httptest.NewTLSServer(&cgi.Handler{Path: filepath.Join(execPath, "git-http-backend"), Stderr: io.Discard, Env: []string{"GIT_PROJECT_ROOT=" + dir, "GIT_HTTP_EXPORT_ALL=1", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null"}})
	defer source.Close()
	ca := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: source.Certificate().Raw}), 0o600); err != nil {
		return err
	}
	cache, err := registry.OpenCache(filepath.Join(dir, "cache"))
	if err != nil {
		return err
	}
	cli, err := registry.NewGitCLI("git", filepath.Join(dir, "cache", "tmp"))
	if err != nil {
		return err
	}
	release, err := registry.DefaultRelease()
	if err != nil {
		return err
	}
	st := store.New()
	ctx := context.Background()
	opts := seed.Defaults()
	if err := seed.Apply(ctx, st, opts); err != nil {
		return err
	}
	if err := st.CreateResourceType(ctx, opts.OrganizationKey, resource.Type{Key: "git-demo", Outputs: []resource.OutputField{{Name: "name", Type: "string", Required: true}}}); err != nil {
		return err
	}
	modules, err := registry.New(registry.Config{Release: release, Cache: cache, Store: st, Packaged: terraform.PackagedModules(), Git: trustedFixture{GitCLI: cli, ca: ca}})
	if err != nil {
		return err
	}
	if err := modules.Prepare(ctx); err != nil {
		return err
	}
	server := httptest.NewServer(delivery.NewServer(delivery.Config{Store: st, Authentication: authentication.NewService(st), ModuleRegistry: modules, SeedOptions: opts, UIDir: *ui}))
	defer server.Close()
	node := exec.Command("node", *scenario)
	node.Env = append(os.Environ(), "GIT_MODULE_CONSOLE_URL="+server.URL, "GIT_MODULE_SOURCE_URL="+source.URL+"/repo.git", "GIT_MODULE_COMMIT="+commit)
	node.Stdout = os.Stdout
	node.Stderr = os.Stderr
	return node.Run()
}
