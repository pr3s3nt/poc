package registry

import (
	"orchestrator/internal/ports/moduleregistry"

	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"
)

// Snapshot is one immutable source revision opened by a fetcher. Tree
// returns the files below dir (relative to the source root, "." for all).
type Snapshot interface {
	Tree(dir string) (Tree, error)
	Close()
}

// GitFetcher opens one exact commit of an allowlisted HTTPS repository.
type GitFetcher interface {
	Open(ctx context.Context, source GitSource, commit string) (Snapshot, error)
}

// ErrFetch is a safe fetch failure; no URL, stderr or credential detail is
// ever part of the message.
var ErrFetch = moduleregistry.ErrUnavailable

// packagedSnapshot reads the trusted packaged modules of the release.
type packagedSnapshot struct{ fsys fs.FS }

func (p packagedSnapshot) Close() {}

func (p packagedSnapshot) Tree(dir string) (Tree, error) {
	sub := p.fsys
	if dir != "." {
		var err error
		if sub, err = fs.Sub(p.fsys, dir); err != nil {
			return nil, err
		}
	}
	out := Tree{}
	total := 0
	err := fs.WalkDir(sub, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == "." {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch mode := info.Mode(); {
		case mode.IsDir():
			return checkPath(p)
		case !mode.IsRegular():
			return unsafeContent("a module contains a special file")
		}
		content, err := fs.ReadFile(sub, p)
		if err != nil {
			return err
		}
		return out.add(p, content, &total)
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// GitCLI fetches commits with the git binary under a fixed, hardened
// configuration: HTTPS only, no redirects, no credential helper, prompt or
// askpass, no hooks, no submodules and object fsck on transfer. It never
// checks out a work tree: file modes and content are read from the commit
// objects, so symlinks, executables and submodules are refused before any
// file is written.
type GitCLI struct {
	Binary  string
	TempDir string
	Timeout time.Duration
}

// NewGitCLI resolves the git binary once.
func NewGitCLI(binary, tempDir string) (*GitCLI, error) {
	if binary == "" {
		binary = "git"
	}
	resolved, err := exec.LookPath(binary)
	if err != nil {
		return nil, errors.New("registry: the git binary for allowlisted module sources was not found")
	}
	return &GitCLI{Binary: resolved, TempDir: tempDir, Timeout: 2 * time.Minute}, nil
}

type gitSnapshot struct {
	cli    *GitCLI
	ctx    context.Context
	dir    string
	commit string
	ca     string
}

// Open fetches exactly one commit (depth 1) into a private bare repository.
func (g *GitCLI) Open(ctx context.Context, source GitSource, commit string) (Snapshot, error) {
	if err := validateGitURL(source.URL); err != nil {
		return nil, ErrFetch
	}
	dir, err := os.MkdirTemp(g.TempDir, "fetch-")
	if err != nil {
		return nil, ErrFetch
	}
	s := &gitSnapshot{cli: g, ctx: ctx, dir: dir, commit: commit, ca: source.CAFile}
	if _, err := s.git("init", "-q", "--bare", "--object-format="+objectFormat(commit), "."); err != nil {
		s.Close()
		return nil, ErrFetch
	}
	if _, err := s.git("fetch", "-q", "--depth=1", "--no-tags", "--no-recurse-submodules", "--no-write-fetch-head", source.URL, commit+":refs/pinned"); err != nil {
		s.Close()
		return nil, ErrFetch
	}
	out, err := s.git("rev-parse", "--verify", "--end-of-options", "refs/pinned^{commit}")
	if err != nil || strings.TrimSpace(string(out)) != commit {
		s.Close()
		return nil, ErrFetch
	}
	return s, nil
}

func objectFormat(commit string) string {
	if len(commit) == 64 {
		return "sha256"
	}
	return "sha1"
}

func (s *gitSnapshot) Close() { _ = os.RemoveAll(s.dir) }

func (s *gitSnapshot) git(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(s.ctx, s.cli.Timeout)
	defer cancel()
	config := []string{
		"-c", "protocol.allow=never", "-c", "protocol.https.allow=always",
		"-c", "http.followRedirects=false", "-c", "http.sslVerify=true",
		"-c", "credential.helper=", "-c", "core.askPass=", "-c", "core.hooksPath=/dev/null",
		"-c", "core.fsmonitor=false", "-c", "fetch.recurseSubmodules=false",
		"-c", "transfer.fsckObjects=true", "-c", "submodule.recurse=false",
	}
	if s.ca != "" {
		config = append(config, "-c", "http.sslCAInfo="+s.ca)
	}
	cmd := exec.CommandContext(ctx, s.cli.Binary, append(append(config, "-C", s.dir), args...)...)
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"), "HOME=" + s.dir, "LC_ALL=C",
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_TERMINAL_PROMPT=0",
		"GIT_ASKPASS=", "SSH_ASKPASS=", "GIT_ALLOW_PROTOCOL=https", "GIT_PROTOCOL_FROM_USER=0",
		"GIT_NO_REPLACE_OBJECTS=1",
	}
	cmd.Stdin = nil
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	// stderr may echo the URL; it is discarded, never logged or returned.
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return stdout.Bytes(), nil
}

// Tree lists the commit's files below dir from the object database.
func (s *gitSnapshot) Tree(dir string) (Tree, error) {
	args := []string{"ls-tree", "-r", "-z", "--full-tree", "refs/pinned"}
	if dir != "." {
		args = append(args, "--", dir+"/")
	}
	listing, err := s.git(args...)
	if err != nil {
		return nil, ErrFetch
	}
	out := Tree{}
	total := 0
	for _, entry := range bytes.Split(listing, []byte{0}) {
		if len(entry) == 0 {
			continue
		}
		meta, name, ok := bytes.Cut(entry, []byte{'\t'})
		fields := strings.Fields(string(meta))
		if !ok || len(fields) != 3 {
			return nil, ErrFetch
		}
		mode, kind, object := fields[0], fields[1], fields[2]
		switch {
		case mode == "120000":
			return nil, unsafeContent("a module contains a symbolic link")
		case mode == "160000" || kind == "commit":
			return nil, unsafeContent("a module contains a Git submodule")
		case mode == "100755":
			return nil, unsafeContent("a module contains an executable file")
		case mode != "100644" || kind != "blob":
			return nil, unsafeContent("a module contains a special file")
		}
		rel := string(name)
		if dir != "." {
			rel = strings.TrimPrefix(rel, dir+"/")
		}
		if err := checkPath(rel); err != nil {
			return nil, err
		}
		size, err := s.git("cat-file", "-s", object)
		if err != nil {
			return nil, ErrFetch
		}
		if n, err := strconv.Atoi(strings.TrimSpace(string(size))); err != nil || n > maxTreeBytes {
			return nil, unsafeContent("the module is too large")
		}
		content, err := s.git("cat-file", "blob", object)
		if err != nil {
			return nil, ErrFetch
		}
		if err := out.add(path.Clean(rel), content, &total); err != nil {
			return nil, err
		}
	}
	return out, nil
}
