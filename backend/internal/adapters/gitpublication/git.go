// Package gitpublication implements fast-forward-only generated-manifest publication.
package gitpublication

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	port "orchestrator/internal/ports/gitpublication"
)

var ErrPublication = errors.New("gitpublication: operation failed")
var ErrConflict = errors.New("gitpublication: generated path changed concurrently")
var ErrInvalid = errors.New("gitpublication: invalid generated publication")
var branchShape = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,199}$`)
var segment = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,199}$`)

// Writer owns one dedicated clone. External durable leases are mandatory through
// Guard; this mutex only protects subprocess/worktree concurrency in one process.
type Writer struct {
	Dir            string
	DestinationKey string
	Env            []string
	mu             sync.Mutex
}

func New(dir, destination string, env []string) (*Writer, error) {
	if !filepath.IsAbs(dir) || destination == "" {
		return nil, ErrInvalid
	}
	info, err := os.Lstat(filepath.Join(dir, ".git"))
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, ErrInvalid
	}
	return &Writer{Dir: dir, DestinationKey: destination, Env: append([]string(nil), env...)}, nil
}
func validPath(p string) bool {
	if p == "" || filepath.IsAbs(p) || filepath.ToSlash(filepath.Clean(p)) != p || strings.Contains(p, "\\") {
		return false
	}
	for _, part := range strings.Split(p, "/") {
		if !segment.MatchString(part) || part == "." || part == ".." || part == ".git" {
			return false
		}
	}
	return true
}
func validate(i port.Intent) error {
	if i.ID == "" || strings.ContainsAny(i.ID, "\x00\r\n") || !validPath(i.Path) || !branchShape.MatchString(i.Branch) || strings.Contains(i.Branch, "..") || strings.Contains(i.Branch, "//") || strings.HasSuffix(i.Branch, ".lock") || strings.HasSuffix(i.Branch, "/") {
		return ErrInvalid
	}
	if (len(i.Files) == 0 && !i.Delete) || (i.Delete && len(i.Files) != 0) {
		return ErrInvalid
	}
	for name, data := range i.Files {
		if !validPath(name) || len(data) == 0 {
			return ErrInvalid
		}
	}
	if Digest(i.Files) != i.TreeDigest {
		return ErrInvalid
	}
	return nil
}

// Digest is retained as the writer package convenience entrypoint.
func Digest(files map[string][]byte) string { return port.Digest(files) }

func (w *Writer) git(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", w.Dir, "-c", "core.hooksPath=/dev/null", "-c", "credential.helper="}, args...)...)
	cmd.Env = append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME"), "GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null"}, w.Env...)
	out, err := cmd.Output()
	if err != nil {
		return nil, ErrPublication
	}
	return out, nil
}
func (w *Writer) fetch(ctx context.Context, branch string) error {
	_, err := w.git(ctx, "fetch", "--no-tags", "origin", branch)
	return err
}
func (w *Writer) treeMatches(ctx context.Context, commit string, i port.Intent) (bool, error) {
	data, err := w.git(ctx, "ls-tree", "-r", "-z", commit, "--", i.Path+"/")
	if err != nil {
		return false, err
	}
	found := map[string][]byte{}
	for _, entry := range bytes.Split(data, []byte{0}) {
		if len(entry) == 0 {
			continue
		}
		fields := bytes.SplitN(entry, []byte{'\t'}, 2)
		if len(fields) != 2 {
			return false, ErrPublication
		}
		meta := strings.Fields(string(fields[0]))
		if len(meta) != 3 || meta[0] != "100644" || meta[1] != "blob" {
			return false, ErrConflict
		}
		name, ok := strings.CutPrefix(string(fields[1]), i.Path+"/")
		if !ok {
			return false, ErrPublication
		}
		content, err := w.git(ctx, "cat-file", "blob", meta[2])
		if err != nil {
			return false, err
		}
		found[name] = content
	}
	return Digest(found) == i.TreeDigest, nil
}
func (w *Writer) observe(ctx context.Context, i port.Intent) (port.Receipt, bool, error) {
	// Check exact owned tree, not only a commit message marker. A later commit on
	// another path is acceptable only when this unit's complete tree still matches.
	commit, err := w.git(ctx, "rev-parse", "FETCH_HEAD")
	if err != nil {
		return port.Receipt{}, false, err
	}
	sha := strings.TrimSpace(string(commit))
	matched, err := w.treeMatches(ctx, sha, i)
	return port.Receipt{IntentID: i.ID, Commit: sha, TreeDigest: i.TreeDigest}, matched, err
}
func (w *Writer) ObservePublication(ctx context.Context, i port.Intent) (port.Receipt, bool, error) {
	if err := validate(i); err != nil {
		return port.Receipt{}, false, err
	}
	if i.DestinationKey != w.DestinationKey {
		return port.Receipt{}, false, ErrInvalid
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.fetch(ctx, i.Branch); err != nil {
		return port.Receipt{}, false, err
	}
	return w.observe(ctx, i)
}
func (w *Writer) Publish(ctx context.Context, i port.Intent, guard port.Guard) (port.Receipt, error) {
	if err := validate(i); err != nil {
		return port.Receipt{}, err
	}
	if guard == nil || i.DestinationKey != w.DestinationKey {
		return port.Receipt{}, ErrInvalid
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := guard(ctx); err != nil {
		return port.Receipt{}, err
	}
	status, err := w.git(ctx, "status", "--porcelain")
	if err != nil {
		return port.Receipt{}, err
	}
	if len(status) > 0 {
		return port.Receipt{}, ErrConflict
	}
	if err := w.fetch(ctx, i.Branch); err != nil {
		return port.Receipt{}, err
	}
	if receipt, found, err := w.observe(ctx, i); err != nil || found {
		return receipt, err
	}
	// Detached worktree state follows the exact fetched base. The clone is owned
	// solely by this writer; this never resets a user checkout.
	if _, err := w.git(ctx, "checkout", "--detach", "FETCH_HEAD"); err != nil {
		return port.Receipt{}, err
	}
	for _, part := range []string{i.Path} {
		current := w.Dir
		for _, piece := range strings.Split(part, "/") {
			current = filepath.Join(current, piece)
			if info, err := os.Lstat(current); err == nil && (info.Mode()&os.ModeSymlink != 0 || !info.IsDir()) {
				return port.Receipt{}, ErrInvalid
			}
		}
	}
	dest := filepath.Join(w.Dir, filepath.FromSlash(i.Path))
	// Delete only files previously tracked below this exact unit path, after
	// refusing unexpected/symlink entries. No wildcard staging or force push.
	if err := filepath.Walk(dest, func(p string, info os.FileInfo, err error) error {
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return ErrPublication
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return ErrInvalid
		}
		return nil
	}); err != nil {
		return port.Receipt{}, err
	}
	if err := os.RemoveAll(dest); err != nil {
		return port.Receipt{}, ErrPublication
	}
	names := make([]string, 0, len(i.Files))
	for name := range i.Files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		target := filepath.Join(dest, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return port.Receipt{}, ErrPublication
		}
		if err := os.WriteFile(target, i.Files[name], 0o644); err != nil {
			return port.Receipt{}, ErrPublication
		}
	}
	if _, err := w.git(ctx, "add", "--", i.Path); err != nil {
		return port.Receipt{}, err
	}
	if _, err := w.git(ctx, "-c", "user.name=Orchestrator", "-c", "user.email=orchestrator@localhost", "commit", "-m", "Publish "+i.ID); err != nil {
		return port.Receipt{}, err
	}
	if err := guard(ctx); err != nil {
		return port.Receipt{}, err
	}
	// A non-fast-forward rejection is a conflict. The caller must refetch/observe
	// under its persisted intent before retry, never replay with a force push.
	if _, err := w.git(ctx, "push", "origin", "HEAD:refs/heads/"+i.Branch); err != nil {
		return port.Receipt{}, ErrConflict
	}
	sha, err := w.git(ctx, "rev-parse", "HEAD")
	if err != nil {
		return port.Receipt{}, err
	}
	return port.Receipt{IntentID: i.ID, Commit: strings.TrimSpace(string(sha)), TreeDigest: i.TreeDigest}, nil
}

func (w *Writer) VerifyTree(ctx context.Context, branch, path, commit, digest string) (bool, error) {
	if !branchShape.MatchString(branch) || !validPath(path) || (len(commit) != 40 && len(commit) != 64) || len(digest) != 64 {
		return false, ErrInvalid
	}
	for _, v := range []string{commit, digest} {
		for _, r := range v {
			if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
				return false, ErrInvalid
			}
		}
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.fetch(ctx, branch); err != nil {
		return false, err
	}
	if _, err := w.git(ctx, "merge-base", "--is-ancestor", commit, "FETCH_HEAD"); err != nil {
		return false, ErrConflict
	}
	return w.treeMatches(ctx, commit, port.Intent{Path: path, TreeDigest: digest})
}
