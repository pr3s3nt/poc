// Package gitx wraps the git operations the supervisor needs.
package gitx

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"issueflow/internal/runner"
)

type Git struct{ R runner.Runner }

func (g Git) run(ctx context.Context, dir string, args ...string) (string, error) {
	res, err := g.R.Run(ctx, dir, []string{"GIT_TERMINAL_PROMPT=0", "LC_ALL=C"}, "git", args...)
	if err != nil {
		return "", err
	}
	if res.Exit != 0 {
		return res.Stdout, fmt.Errorf("git %s: exit %d: %s", strings.Join(args, " "), res.Exit, strings.TrimSpace(res.Stderr))
	}
	return res.Stdout, nil
}

func (g Git) Fetch(ctx context.Context, repo, branch string) error {
	_, err := g.run(ctx, repo, "fetch", "origin", branch)
	return err
}

func (g Git) RevParse(ctx context.Context, dir, rev string) (string, error) {
	out, err := g.run(ctx, dir, "rev-parse", "--verify", rev)
	return strings.TrimSpace(out), err
}

// AddWorktree creates a new branch in a new worktree. It never touches the
// main checkout's files or index.
func (g Git) AddWorktree(ctx context.Context, repo, path, branch, start string) error {
	_, err := g.run(ctx, repo, "worktree", "add", "-b", branch, path, start)
	return err
}

func (g Git) WorktreeExists(path string) bool {
	_, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil
}

func (g Git) Head(ctx context.Context, dir string) (string, error) {
	return g.RevParse(ctx, dir, "HEAD")
}

func (g Git) Branch(ctx context.Context, dir string) (string, error) {
	out, err := g.run(ctx, dir, "rev-parse", "--abbrev-ref", "HEAD")
	return strings.TrimSpace(out), err
}

// RemoteSHA reads a remote branch head without fetching; "" when absent.
func (g Git) RemoteSHA(ctx context.Context, dir, branch string) (string, error) {
	out, err := g.run(ctx, dir, "ls-remote", "--heads", "origin", branch)
	if err != nil {
		return "", err
	}
	for _, l := range strings.Split(out, "\n") {
		f := strings.Fields(l)
		if len(f) == 2 && f[1] == "refs/heads/"+branch {
			return f[0], nil
		}
	}
	return "", nil
}

func (g Git) DiffStat(ctx context.Context, dir, base string) string {
	out, _ := g.run(ctx, dir, "diff", "--stat", base)
	return strings.TrimSpace(out)
}

// ContentHash fingerprints the task tree: every non-ignored file in the
// working tree (tracked files that still exist plus untracked, non-ignored
// files) as path, type/exec bit, and content hash or symlink target. It is
// independent of the index and of commits, so `git add` and `git commit` of the
// same bytes never change it, while any edit, new file or deletion does.
// Revision facts (base ancestry, branch, head) are verified separately with
// IsAncestor/Branch/Head; the hash intentionally does not include them.
func (g Git) ContentHash(ctx context.Context, dir string) (string, error) {
	tracked, err := g.run(ctx, dir, "ls-files", "-z")
	if err != nil {
		return "", err
	}
	others, err := g.run(ctx, dir, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return "", err
	}
	set := map[string]bool{}
	for _, list := range []string{tracked, others} {
		for _, p := range strings.Split(list, "\x00") {
			if p != "" {
				set[p] = true
			}
		}
	}
	paths := make([]string, 0, len(set))
	for p := range set {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	h := sha256.New()
	for _, p := range paths {
		full := filepath.Join(dir, p)
		fi, err := os.Lstat(full)
		if os.IsNotExist(err) {
			continue // tracked but deleted in the working tree
		}
		if err != nil {
			return "", err
		}
		fh := sha256.New()
		kind := "f"
		switch {
		case fi.Mode()&os.ModeSymlink != 0:
			kind = "l"
			t, _ := os.Readlink(full)
			fh.Write([]byte(t))
		case fi.Mode().IsRegular():
			if fi.Mode().Perm()&0o111 != 0 {
				kind = "x"
			}
			f, err := os.Open(full)
			if err != nil {
				return "", err
			}
			_, err = io.Copy(fh, f)
			f.Close()
			if err != nil {
				return "", err
			}
		default:
			kind = "o" // directory (submodule) or special file: path only
		}
		fmt.Fprintf(h, "%s\x00%s\x00%s\x00", p, kind, hex.EncodeToString(fh.Sum(nil)))
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// IsAncestor reports whether anc is an ancestor of (or equal to) rev.
func (g Git) IsAncestor(ctx context.Context, dir, anc, rev string) (bool, error) {
	res, err := g.R.Run(ctx, dir, []string{"LC_ALL=C"}, "git", "merge-base", "--is-ancestor", anc, rev)
	if err != nil {
		return false, err
	}
	switch res.Exit {
	case 0:
		return true, nil
	case 1:
		return false, nil
	}
	return false, fmt.Errorf("git merge-base: exit %d: %s", res.Exit, strings.TrimSpace(res.Stderr))
}

func (g Git) IsClean(ctx context.Context, dir string) (bool, error) {
	out, err := g.run(ctx, dir, "status", "--porcelain")
	return strings.TrimSpace(out) == "", err
}

// FetchPinned fetches exactly one remote branch into a private ref and returns
// the commit it points to now. Everything read afterwards uses that fixed sha.
func (g Git) FetchPinned(ctx context.Context, dir, branch, localRef string) (string, error) {
	if _, err := g.run(ctx, dir, "fetch", "--no-tags", "origin", "+refs/heads/"+branch+":"+localRef); err != nil {
		return "", err
	}
	return g.RevParse(ctx, dir, localRef)
}

// ShowBlob reads one file at a fixed commit without checking anything out.
// The size is checked before the content is read; exists=false only when the
// path is definitely absent at that commit.
func (g Git) ShowBlob(ctx context.Context, dir, sha, path string, max int) ([]byte, bool, error) {
	res, err := g.R.Run(ctx, dir, []string{"LC_ALL=C"}, "git", "cat-file", "-s", sha+":"+path)
	if err != nil {
		return nil, false, err
	}
	if res.Exit != 0 {
		if strings.Contains(res.Stderr, "does not exist") || strings.Contains(res.Stderr, "exists on disk, but not in") || strings.Contains(res.Stderr, "Not a valid object name") {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("git cat-file -s: exit %d: %s", res.Exit, strings.TrimSpace(res.Stderr))
	}
	var size int
	if _, err := fmt.Sscan(strings.TrimSpace(res.Stdout), &size); err != nil {
		return nil, true, fmt.Errorf("unparseable blob size %q", res.Stdout)
	}
	if size > max {
		return nil, true, fmt.Errorf("report is %d bytes, over the %d byte limit", size, max)
	}
	out, err := g.run(ctx, dir, "cat-file", "blob", sha+":"+path)
	if err != nil {
		return nil, true, err
	}
	return []byte(out), true, nil
}

// ChangedPaths lists paths that differ between two commits.
func (g Git) ChangedPaths(ctx context.Context, dir, a, b string) ([]string, error) {
	out, err := g.run(ctx, dir, "diff", "--name-only", "-z", a, b)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, p := range strings.Split(out, "\x00") {
		if p != "" {
			paths = append(paths, p)
		}
	}
	return paths, nil
}

// PathsUnder lists files under prefix that are tracked (index/HEAD) or
// untracked and not ignored in the working tree.
func (g Git) PathsUnder(ctx context.Context, dir, prefix string) ([]string, error) {
	var all []string
	for _, args := range [][]string{{"ls-files", "-z", "--", prefix}, {"ls-files", "--others", "--exclude-standard", "-z", "--", prefix}, {"ls-tree", "-r", "-z", "--name-only", "HEAD", "--", prefix}} {
		out, err := g.run(ctx, dir, args...)
		if err != nil {
			return nil, err
		}
		for _, p := range strings.Split(out, "\x00") {
			if p != "" {
				all = append(all, p)
			}
		}
	}
	return all, nil
}
