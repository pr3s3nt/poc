package gitpublication

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	port "orchestrator/internal/ports/gitpublication"
)

func run(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git failed: %v %s", err, b)
	}
	return strings.TrimSpace(string(b))
}
func TestPublicationIsIdempotentAndPreservesUnrelatedPaths(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()
	remote := filepath.Join(base, "remote.git")
	clone := filepath.Join(base, "writer")
	run(t, "init", "--bare", "--initial-branch=main", remote)
	run(t, "clone", remote, clone)
	if err := os.WriteFile(filepath.Join(clone, "preexisting.txt"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	run(t, "-C", clone, "add", "preexisting.txt")
	run(t, "-C", clone, "-c", "user.name=Test", "-c", "user.email=test@localhost", "commit", "-m", "initial")
	run(t, "-C", clone, "push", "origin", "main")
	writer, err := New(clone, "destination", nil)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{"deployment.json": []byte(`{"kind":"Deployment"}`), "fleet.yaml": []byte("namespace: app\n")}
	i := port.Intent{ID: "intent-1", DestinationKey: "destination", Branch: "main", Path: "org/a/env/b/unit", Files: files, TreeDigest: Digest(files)}
	guards := 0
	guard := func(context.Context) error { guards++; return nil }
	first, err := writer.Publish(ctx, i, guard)
	if err != nil {
		t.Fatal(err)
	}
	second, err := writer.Publish(ctx, i, guard)
	if err != nil || second.Commit != first.Commit {
		t.Fatal("duplicate publication", err)
	}
	if got := run(t, "--git-dir", remote, "show", "main:preexisting.txt"); got != "keep" {
		t.Fatal("foreign file changed")
	}
	if guards != 3 {
		t.Fatalf("owner guard omitted: %d", guards)
	}
	observed, ok, err := writer.ObservePublication(ctx, i)
	if err != nil || !ok || observed.Commit != first.Commit {
		t.Fatal("lost acknowledgement not recoverable", err)
	}
	changed := map[string][]byte{"service.json": []byte(`{"kind":"Service"}`)}
	i.ID = "intent-2"
	i.Files = changed
	i.TreeDigest = Digest(changed)
	_, err = writer.Publish(ctx, i, guard)
	if err != nil {
		t.Fatal(err)
	}
	tree := run(t, "--git-dir", remote, "ls-tree", "-r", "--name-only", "main")
	if strings.Contains(tree, "deployment.json") || !strings.Contains(tree, "service.json") || !strings.Contains(tree, "preexisting.txt") {
		t.Fatal("exact unit tree not replaced", tree)
	}
	denied := errors.New("owner lost")
	i.ID = "intent-3"
	i.Files = files
	i.TreeDigest = Digest(files)
	before := run(t, "--git-dir", remote, "rev-parse", "main")
	if _, err = writer.Publish(ctx, i, func(context.Context) error { return denied }); !errors.Is(err, denied) {
		t.Fatal("lost owner accepted", err)
	}
	if after := run(t, "--git-dir", remote, "rev-parse", "main"); after != before {
		t.Fatal("write after owner loss")
	}
	// Removing one unit in an existing GitRepo must preserve every sibling path.
	i.ID = "withdraw-unit"
	i.Files = map[string][]byte{}
	i.TreeDigest = Digest(nil)
	i.Delete = true
	removed, err := writer.Publish(ctx, i, guard)
	if err != nil {
		t.Fatal("guarded unit withdrawal failed", err)
	}
	if tree := run(t, "--git-dir", remote, "ls-tree", "-r", "--name-only", "main"); tree != "preexisting.txt" {
		t.Fatal("withdrawal changed sibling content", tree)
	}
	if matched, err := writer.VerifyTree(ctx, i.Branch, i.Path, removed.Commit, i.TreeDigest); err != nil || !matched {
		t.Fatal("withdrawal commit not verifiable", err)
	}
	if repeated, err := writer.Publish(ctx, i, guard); err != nil || repeated.Commit != removed.Commit {
		t.Fatal("withdrawal replay created another commit", err)
	}
}
func TestPublicationRejectsEscapingPathsAndWrongDigests(t *testing.T) {
	for _, p := range []string{"../foreign", "/absolute", "generated/.git/hooks", "a/../b", "a\\b"} {
		i := port.Intent{ID: "i", Branch: "main", Path: p, Files: map[string][]byte{"a.json": []byte("{}")}}
		i.TreeDigest = Digest(i.Files)
		if validate(i) == nil {
			t.Fatalf("accepted path %s", p)
		}
	}
}
