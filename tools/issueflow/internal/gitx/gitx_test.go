package gitx

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"issueflow/internal/runner"
)

func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func repo(t *testing.T) (Git, string, string) {
	d := t.TempDir()
	run(t, d, "init", "-b", "main", ".")
	os.WriteFile(filepath.Join(d, "a.txt"), []byte("a\n"), 0o644)
	os.WriteFile(filepath.Join(d, ".gitignore"), []byte("ignored/\n"), 0o644)
	run(t, d, "add", ".")
	run(t, d, "commit", "-m", "base")
	return Git{R: runner.OS{}}, d, run(t, d, "rev-parse", "HEAD")
}

func hash(t *testing.T, g Git, d string) string {
	h, err := g.ContentHash(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// The review finding: an accepted new file must keep its hash through
// git add and git commit, and any edit must change it.
func TestContentHashStableAcrossAddAndCommitOfSameBytes(t *testing.T) {
	g, d, _ := repo(t)
	os.WriteFile(filepath.Join(d, "new.go"), []byte("package x\n"), 0o644)
	os.MkdirAll(filepath.Join(d, "pkg"), 0o755)
	os.WriteFile(filepath.Join(d, "pkg", "run.sh"), []byte("#!/bin/sh\n"), 0o755)
	os.Symlink("a.txt", filepath.Join(d, "ln"))
	h0 := hash(t, g, d) // all untracked
	run(t, d, "add", "new.go")
	if hash(t, g, d) != h0 {
		t.Fatal("hash changed when an untracked file was staged")
	}
	run(t, d, "add", "-A")
	if hash(t, g, d) != h0 {
		t.Fatal("hash changed after staging everything")
	}
	run(t, d, "commit", "-m", "work")
	if hash(t, g, d) != h0 {
		t.Fatal("hash changed after commit")
	}
	if hash(t, g, d) != h0 {
		t.Fatal("unstable")
	}
}

func TestContentHashDetectsEveryKindOfEdit(t *testing.T) {
	g, d, _ := repo(t)
	os.WriteFile(filepath.Join(d, "new.txt"), []byte("v1\n"), 0o644)
	base := hash(t, g, d)
	mutate := map[string]func(){
		"edit untracked": func() { os.WriteFile(filepath.Join(d, "new.txt"), []byte("v2\n"), 0o644) },
		"edit tracked":   func() { os.WriteFile(filepath.Join(d, "a.txt"), []byte("changed\n"), 0o644) },
		"delete tracked": func() { os.Remove(filepath.Join(d, "a.txt")) },
		"new file":       func() { os.WriteFile(filepath.Join(d, "extra"), []byte("x"), 0o644) },
		"exec bit":       func() { os.Chmod(filepath.Join(d, "new.txt"), 0o755) },
		"rename":         func() { os.Rename(filepath.Join(d, "new.txt"), filepath.Join(d, "moved.txt")) },
		"symlink":        func() { os.Symlink("a.txt", filepath.Join(d, "l")) },
	}
	for name, f := range mutate {
		t.Run(name, func(t *testing.T) {
			g2, d2, _ := repo(t)
			os.WriteFile(filepath.Join(d2, "new.txt"), []byte("v1\n"), 0o644)
			before := hash(t, g2, d2)
			dd := d
			d = d2
			f()
			d = dd
			if hash(t, g2, d2) == before {
				t.Fatalf("%s did not change the hash", name)
			}
		})
	}
	_ = base
	_ = g
}

func TestContentHashIgnoresIgnoredFilesAndGitInternals(t *testing.T) {
	g, d, _ := repo(t)
	h0 := hash(t, g, d)
	os.MkdirAll(filepath.Join(d, "ignored"), 0o755)
	os.WriteFile(filepath.Join(d, "ignored", "junk"), []byte("x"), 0o644)
	if hash(t, g, d) != h0 {
		t.Fatal("ignored files changed the hash")
	}
	run(t, d, "branch", "other")
	if hash(t, g, d) != h0 {
		t.Fatal("refs changed the hash")
	}
}

func TestRevisionFactsAreSeparate(t *testing.T) {
	g, d, base := repo(t)
	ctx := context.Background()
	if ok, err := g.IsAncestor(ctx, d, base, "HEAD"); !ok || err != nil {
		t.Fatal("base is its own ancestor")
	}
	run(t, d, "checkout", "-b", "feature")
	os.WriteFile(filepath.Join(d, "f"), []byte("f"), 0o644)
	run(t, d, "add", "-A")
	run(t, d, "commit", "-m", "f")
	if ok, _ := g.IsAncestor(ctx, d, base, "HEAD"); !ok {
		t.Fatal("base should be an ancestor of the task branch")
	}
	run(t, d, "checkout", "--orphan", "rewritten")
	run(t, d, "commit", "-m", "unrelated history", "--allow-empty")
	if ok, err := g.IsAncestor(ctx, d, base, "HEAD"); ok || err != nil {
		t.Fatalf("rewritten history must not descend from base: %v %v", ok, err)
	}
	if b, _ := g.Branch(ctx, d); b != "rewritten" {
		t.Fatalf("branch %s", b)
	}
	if _, err := g.IsAncestor(ctx, d, "deadbeef", "HEAD"); err == nil {
		t.Fatal("an unknown revision must be an error, not 'false'")
	}
}

func TestRemoteSHAAndClean(t *testing.T) {
	g, d, base := repo(t)
	remote := t.TempDir()
	run(t, remote, "init", "--bare", "-b", "main", ".")
	run(t, d, "remote", "add", "origin", remote)
	run(t, d, "push", "origin", "main")
	ctx := context.Background()
	if s, err := g.RemoteSHA(ctx, d, "main"); err != nil || s != base {
		t.Fatalf("%q %v", s, err)
	}
	if s, err := g.RemoteSHA(ctx, d, "nope"); err != nil || s != "" {
		t.Fatalf("%q %v", s, err)
	}
	if ok, _ := g.IsClean(ctx, d); !ok {
		t.Fatal("clean expected")
	}
	os.WriteFile(filepath.Join(d, "u"), []byte("x"), 0o644)
	if ok, _ := g.IsClean(ctx, d); ok {
		t.Fatal("untracked file means not clean")
	}
}

func TestParseRemoteNeverLeaksCredentialsAndRejectsLocalPaths(t *testing.T) {
	good := map[string]string{
		"https://github.com/o/r.git":                    "o/r",
		"https://github.com/o/r":                        "o/r",
		"https://user:ghp_secret@github.com/o/r.git":    "o/r",
		"git@github.com:o/r.git":                        "o/r",
		"ssh://git@github.com/o/r.git":                  "o/r",
		"https://github.example.com/Org/Some-Repo.git/": "Org/Some-Repo",
	}
	for in, want := range good {
		got, ok := ParseRemote(in)
		if !ok || got != want {
			t.Errorf("%q: got %q ok=%v", in, got, ok)
		}
		if strings.Contains(got, "ghp_") || strings.Contains(got, "@") {
			t.Errorf("identity leaked credentials: %q", got)
		}
	}
	for _, in := range []string{"/tmp/remote.git", "../x", "https://github.com/onlyowner", "https://github.com/a/b/c", "", "file:///tmp/x.git"} {
		if id, ok := ParseRemote(in); ok {
			t.Errorf("%q parsed as %q", in, id)
		}
	}
}

func TestOriginRepoErrorsDoNotContainTheURL(t *testing.T) {
	g, d, _ := repo(t)
	run(t, d, "remote", "add", "origin", "https://user:supersecrettoken@example.com/justone")
	_, err := g.OriginRepo(context.Background(), d)
	if err == nil || strings.Contains(err.Error(), "supersecrettoken") || strings.Contains(err.Error(), "example.com") {
		t.Fatalf("%v", err)
	}
	run(t, d, "remote", "set-url", "origin", "https://user:supersecrettoken@github.com/o/r.git")
	if id, err := g.OriginRepo(context.Background(), d); err != nil || id != "o/r" {
		t.Fatalf("%q %v", id, err)
	}
}
