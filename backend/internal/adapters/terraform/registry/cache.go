package registry

import (
	"orchestrator/internal/ports/moduleregistry"

	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"orchestrator/internal/domain/tfmodule"
)

// ErrArtifact means a verified artifact is missing from the private cache or
// its content no longer matches the recorded digests. Execution stops; the
// registry never substitutes another revision or refetches silently.
var ErrArtifact = fmt.Errorf("%w: the verified module artifact is missing or has drifted from its digests", moduleregistry.ErrDrift)

// Cache is the private, digest-addressed module cache. It lives outside any
// Git work tree with owner-only permissions. Each bundle is materialized as
// <dir>/bundles/<key>/sources/<source>@<revision>/<source-relative paths>,
// so relative module calls resolve exactly as in their pinned revision and
// remote children are present before execution.
type Cache struct {
	dir string
}

// OpenCache creates or opens the cache directory. It refuses a directory
// inside a Git work tree and one other users can read or write.
func OpenCache(dir string) (*Cache, error) {
	if dir == "" || !filepath.IsAbs(dir) {
		return nil, errors.New("registry: the module cache needs an absolute directory")
	}
	dir = filepath.Clean(dir)
	for parent := dir; ; parent = filepath.Dir(parent) {
		if _, err := os.Lstat(filepath.Join(parent, ".git")); err == nil {
			return nil, errors.New("registry: the module cache must be outside a Git work tree")
		}
		if filepath.Dir(parent) == parent {
			break
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, errors.New("registry: create the module cache directory")
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("registry: the module cache is not a directory")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("registry: the module cache directory must be private (0700)")
	}
	for _, sub := range []string{"bundles", "tmp"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o700); err != nil {
			return nil, errors.New("registry: prepare the module cache")
		}
	}
	return &Cache{dir: dir}, nil
}

// TempDir is the private scratch directory for fetches.
func (c *Cache) TempDir() string { return filepath.Join(c.dir, "tmp") }

func bundleKey(ref tfmodule.SourceRef) string {
	sum := sha256.Sum256([]byte(ref.Key()))
	return hex.EncodeToString(sum[:])
}

func (c *Cache) bundleDir(ref tfmodule.SourceRef) string {
	return filepath.Join(c.dir, "bundles", bundleKey(ref))
}

// sourceDir is the materialized directory of one module directory below a
// bundle root.
func sourceDir(bundleRoot string, dir moduleDir) string {
	base := filepath.Join(bundleRoot, "sources", dir.sourceID+"@"+dir.revision)
	if dir.path == "." {
		return base
	}
	return filepath.Join(base, filepath.FromSlash(dir.path))
}

func rootDir(ref tfmodule.SourceRef) moduleDir {
	return moduleDir{sourceID: ref.SourceID, revision: ref.Revision, path: ref.Subdirectory}
}

// store materializes verified trees atomically. An existing entry is kept
// only if it still verifies; a drifted entry fails closed.
func (c *Cache) store(ref tfmodule.SourceRef, contract tfmodule.Contract, trees map[moduleDir]Tree) error {
	final := c.bundleDir(ref)
	if _, err := os.Lstat(final); err == nil {
		_, err := c.verify(ref, contract)
		return err
	}
	staging, err := os.MkdirTemp(c.TempDir(), "stage-")
	if err != nil {
		return fmt.Errorf("registry: stage module cache entry")
	}
	defer os.RemoveAll(staging)
	for dir, tree := range trees {
		if err := tree.write(sourceDir(staging, dir)); err != nil {
			return fmt.Errorf("registry: write module cache entry")
		}
	}
	if err := os.Rename(staging, final); err != nil {
		if _, statErr := os.Lstat(final); statErr == nil {
			// A concurrent writer won; its entry must verify too.
			_, err := c.verify(ref, contract)
			return err
		}
		return fmt.Errorf("registry: publish module cache entry")
	}
	return nil
}

// verify recomputes the root and every transitive digest from disk and
// returns the bundle root directory.
func (c *Cache) verify(ref tfmodule.SourceRef, contract tfmodule.Contract) (string, error) {
	bundleRoot := c.bundleDir(ref)
	check := func(dir moduleDir, want string) error {
		tree, err := readTree(sourceDir(bundleRoot, dir))
		if err != nil || tree.Digest() != want {
			return ErrArtifact
		}
		return nil
	}
	if err := check(rootDir(ref), ref.ContentSHA256); err != nil {
		return "", err
	}
	for _, module := range contract.Transitive {
		if err := check(moduleDir{sourceID: module.SourceID, revision: module.Revision, path: module.Path}, module.ContentSHA256); err != nil {
			return "", err
		}
	}
	return bundleRoot, nil
}

// materialize re-reads the root and every transitive module directory of one
// bundle from the shared cache, checks each digest against the bytes just
// read and writes exactly those bytes below dest. The returned copy is the
// only content an execution uses, so content verified here cannot be swapped
// afterwards by a writer of the shared cache.
func (c *Cache) materialize(ref tfmodule.SourceRef, contract tfmodule.Contract, dest string) error {
	bundleRoot := c.bundleDir(ref)
	copyDir := func(dir moduleDir, want string) error {
		tree, err := readTree(sourceDir(bundleRoot, dir))
		if err != nil || tree.Digest() != want {
			return ErrArtifact
		}
		if err := tree.write(sourceDir(dest, dir)); err != nil {
			return errors.New("registry: write the private module copy")
		}
		return nil
	}
	if err := copyDir(rootDir(ref), ref.ContentSHA256); err != nil {
		return err
	}
	for _, module := range contract.Transitive {
		if err := copyDir(moduleDir{sourceID: module.SourceID, revision: module.Revision, path: module.Path}, module.ContentSHA256); err != nil {
			return err
		}
	}
	return nil
}
