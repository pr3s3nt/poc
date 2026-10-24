// Package renderbundles is the Git render bundle registry adapter (ADR-021):
// hardened HTTPS Git inspection shared with the Terraform registry, the
// durable digest-verified content cache and the verified-content loader the
// renderer uses. Planning, Preview and execution never fetch Git.
package renderbundles

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"orchestrator/internal/adapters/terraform/registry"
	"orchestrator/internal/domain/renderbundle"
	"orchestrator/internal/ports/renderregistry"
)

// Cache is the durable, private, digest-addressed content cache: one
// directory per content SHA-256 under <dir>/bundles. Content is shared by
// Organizations; records stay scoped. Nothing is garbage collected in v1.
type Cache struct{ dir string }

// OpenCache creates or opens the cache. It refuses a relative directory, one
// inside a Git work tree and one other users can read or write.
func OpenCache(dir string) (*Cache, error) {
	if dir == "" || !filepath.IsAbs(dir) {
		return nil, errors.New("renderbundles: the render bundle cache needs an absolute directory")
	}
	dir = filepath.Clean(dir)
	for parent := dir; ; parent = filepath.Dir(parent) {
		if _, err := os.Lstat(filepath.Join(parent, ".git")); err == nil {
			return nil, errors.New("renderbundles: the render bundle cache must be outside a Git work tree")
		}
		if filepath.Dir(parent) == parent {
			break
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, errors.New("renderbundles: create the render bundle cache directory")
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("renderbundles: the render bundle cache is not a directory")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("renderbundles: the render bundle cache directory must be private (0700)")
	}
	for _, sub := range []string{"bundles", "tmp"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o700); err != nil {
			return nil, errors.New("renderbundles: prepare the render bundle cache")
		}
	}
	return &Cache{dir: dir}, nil
}

// TempDir is the private scratch directory for Git fetches.
func (c *Cache) TempDir() string { return filepath.Join(c.dir, "tmp") }

func (c *Cache) path(digest string) string { return filepath.Join(c.dir, "bundles", digest) }

// Digest is the canonical content digest shared with the Terraform registry:
// SHA-256 over sorted "<path>\x00<hex sha256>\n" lines.
func Digest(files map[string][]byte) string { return registry.Tree(files).Digest() }

// Store materializes files under their digest atomically. A verified entry is
// kept. A corrupt entry of the same digest is replaced by the freshly
// verified content: the digest is the identity, so this is the explicit
// same-pin repair (RB-C02), never a different version.
func (c *Cache) Store(digest string, files map[string][]byte) error {
	if Digest(files) != digest {
		return renderregistry.ErrDrift
	}
	suffix := make([]byte, 8)
	if _, err := rand.Read(suffix); err != nil {
		return err
	}
	if _, err := os.Lstat(c.path(digest)); err == nil {
		if _, err := c.Load(digest); err == nil {
			return nil
		}
		corrupt := filepath.Join(c.TempDir(), "corrupt-"+hex.EncodeToString(suffix))
		if err := os.Rename(c.path(digest), corrupt); err != nil {
			return fmt.Errorf("%w: cache repair", renderregistry.ErrBundleUnavailable)
		}
		_ = os.RemoveAll(corrupt)
	}
	staging := filepath.Join(c.TempDir(), "stage-"+hex.EncodeToString(suffix))
	if err := os.Mkdir(staging, 0o700); err != nil {
		return fmt.Errorf("%w: cache staging", renderregistry.ErrBundleUnavailable)
	}
	defer os.RemoveAll(staging)
	for rel, content := range files {
		target := filepath.Join(staging, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return fmt.Errorf("%w: cache write", renderregistry.ErrBundleUnavailable)
		}
		if err := os.WriteFile(target, content, 0o600); err != nil {
			return fmt.Errorf("%w: cache write", renderregistry.ErrBundleUnavailable)
		}
	}
	if err := os.Rename(staging, c.path(digest)); err != nil {
		// A concurrent inspection may have stored the same content first.
		if _, statErr := os.Lstat(c.path(digest)); statErr == nil {
			_, err := c.Load(digest)
			return err
		}
		return fmt.Errorf("%w: cache commit", renderregistry.ErrBundleUnavailable)
	}
	_, err := c.Load(digest)
	return err
}

// Load reads one cached bundle without following links and verifies its
// digest. Missing or corrupt content is ErrBundleUnavailable (fail closed).
func (c *Cache) Load(digest string) (map[string][]byte, error) {
	root := c.path(digest)
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("%w: the verified content is not cached", renderregistry.ErrBundleUnavailable)
	}
	files := map[string][]byte{}
	total := 0
	err = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == root {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		info, err := os.Lstat(p)
		if err != nil {
			return err
		}
		switch mode := info.Mode(); {
		case mode&os.ModeSymlink != 0, !mode.IsDir() && !mode.IsRegular():
			return errors.New("special file")
		case mode.IsDir():
			return nil
		}
		if !renderbundle.ValidFilePath(rel) || len(files) >= renderbundle.MaxFiles || info.Size() > renderbundle.MaxFileBytes {
			return errors.New("unexpected file")
		}
		content, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		total += len(content)
		if total > renderbundle.MaxTreeBytes {
			return errors.New("too large")
		}
		files[rel] = content
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("%w: the cached content is not readable", renderregistry.ErrBundleUnavailable)
	}
	if Digest(files) != digest {
		return nil, fmt.Errorf("%w: the cached content changed", renderregistry.ErrBundleUnavailable)
	}
	return files, nil
}
