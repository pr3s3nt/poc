package gitops

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"orchestrator/internal/ports/execution"
)

func gitTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestGitRepoWritesOnlyScopedNonSecretManifestsAndRemovesThem(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	remote := filepath.Join(root, "remote.git")
	if err := os.Mkdir(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "init", "--bare", remote).CombinedOutput(); err != nil {
		t.Fatalf("init remote: %v: %s", err, out)
	}
	gitTest(t, repo, "init", "-b", "main")
	gitTest(t, repo, "config", "user.name", "Test Bot")
	gitTest(t, repo, "config", "user.email", "test@local.invalid")
	gitTest(t, repo, "remote", "add", "origin", remote)
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("gitops\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitTest(t, repo, "add", "README.md")
	gitTest(t, repo, "commit", "-m", "initial")
	gitTest(t, repo, "push", "origin", "main")
	d := &Deployer{opts: Options{RepoDir: repo, Branch: "main"}}
	path := filepath.Join("applications", "my-app", "staging", "frontend")
	manifest := execution.Manifest{Kind: "Deployment", Name: "frontend", Namespace: "app-my-app-staging", Object: map[string]any{
		"apiVersion": "apps/v1", "kind": "Deployment", "metadata": map[string]any{"name": "frontend", "namespace": "app-my-app-staging"},
	}}
	commit, err := d.writeAndPush(context.Background(), path, "app-my-app-staging", []execution.Manifest{manifest}, false)
	if err != nil {
		t.Fatal(err)
	}
	if commit == "" {
		t.Fatal("missing Git commit")
	}
	content, err := os.ReadFile(filepath.Join(repo, path, "fleet.yaml"))
	if err != nil || string(content) != "namespace: app-my-app-staging\n" {
		t.Fatalf("fleet config: %q %v", content, err)
	}
	files := gitTest(t, repo, "ls-files", path)
	if !strings.Contains(files, "deployment-frontend.json") || !strings.Contains(files, "fleet.yaml") {
		t.Fatalf("missing generated files: %s", files)
	}
	_, err = d.writeAndPush(context.Background(), path, "app-my-app-staging", []execution.Manifest{{Kind: "Secret", Name: "private", Secret: true, Object: map[string]any{"data": "raw-secret"}}}, false)
	if err == nil {
		t.Fatal("Secret must not be committed")
	}
	// The rejected write did not stage or commit secret material.
	if strings.Contains(gitTest(t, repo, "ls-files", path), "private") {
		t.Fatal("Secret path reached Git")
	}
	// The safe prior version remains recoverable; restore it for removal.
	if _, err := d.writeAndPush(context.Background(), path, "app-my-app-staging", []execution.Manifest{manifest}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := d.writeAndPush(context.Background(), path, "app-my-app-staging", nil, true); err != nil {
		t.Fatal(err)
	}
	if files := gitTest(t, repo, "ls-files", path); files != "" {
		t.Fatalf("removed bundle still tracked: %s", files)
	}
}

func TestScopeRejectsPathTraversalAndWrongCluster(t *testing.T) {
	d := &Deployer{opts: Options{KubeContext: "kind-idp-internal"}}
	target := execution.Target{Context: "kind-idp-internal", Namespace: "app-demo-staging", Extra: map[string]string{"application": "demo", "environment": "staging"}}
	if _, err := d.scope(target, "../escape"); err == nil {
		t.Fatal("path traversal accepted")
	}
	target.Context = "production"
	if _, err := d.scope(target, "frontend"); err == nil {
		t.Fatal("wrong cluster accepted")
	}
}
