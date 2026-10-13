// Package registry is the Terraform v2 module registry adapter: the platform
// release manifest and Admin source policy, fetchers for trusted packaged
// modules and allowlisted HTTPS Git commits, the content digest, the static
// module inspector, the private verified cache and the registry service that
// registration, platform preparation, planning metadata and pre-execution
// verification use (Terraform module contract, UC-03 BR-09).
package registry

import (
	"orchestrator/internal/ports/moduleregistry"

	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// Size limits of one fetched module tree. They bound memory use of an
// allowlisted but oversized repository; they are not a trust decision.
const (
	maxTreeFiles = 2000
	maxTreeBytes = 32 << 20
)

// ErrUnsafeContent marks module content the registry refuses: symlinks,
// executable or special files, unsafe paths and files that would change how
// Terraform resolves configuration.
var ErrUnsafeContent = fmt.Errorf("%w: module content is not allowed", moduleregistry.ErrRejected)

func unsafeContent(reason string) error { return fmt.Errorf("%w: %s", ErrUnsafeContent, reason) }

// Tree is one module directory: slash paths relative to the directory mapped
// to regular, non-executable file content.
type Tree map[string][]byte

// checkPath rejects absolute, unclean or traversing relative paths and the
// file names that change Terraform resolution (override files, JSON syntax,
// CLI configuration, local .terraform data and a module-owned lock file).
func checkPath(rel string) error {
	if rel == "" || strings.HasPrefix(rel, "/") || strings.Contains(rel, "\\") || path.Clean(rel) != rel {
		return unsafeContent("a file path is not a clean relative path")
	}
	for _, segment := range strings.Split(rel, "/") {
		if segment == ".." || segment == "." || segment == "" {
			return unsafeContent("a file path traverses outside the module")
		}
		if segment == ".terraform" || segment == ".git" {
			return unsafeContent("a module contains local Terraform or Git data")
		}
		for _, r := range segment {
			if r < 0x20 || r == 0x7f {
				return unsafeContent("a file path contains control characters")
			}
		}
	}
	base := path.Base(rel)
	switch {
	case strings.HasSuffix(base, ".tf.json"):
		return unsafeContent("JSON Terraform configuration is not supported")
	case base == "override.tf" || strings.HasSuffix(base, "_override.tf"):
		return unsafeContent("Terraform override files are not allowed")
	case base == ".terraformrc" || base == "terraform.rc" || strings.HasSuffix(base, ".tfrc"):
		return unsafeContent("Terraform CLI configuration is not allowed in a module")
	case base == ".terraform.lock.hcl":
		return unsafeContent("a module lock file is not allowed; the platform release owns provider locks")
	}
	return nil
}

// add inserts one file after path and limit checks.
func (t Tree) add(rel string, content []byte, total *int) error {
	if err := checkPath(rel); err != nil {
		return err
	}
	if len(t) >= maxTreeFiles {
		return unsafeContent("the module has too many files")
	}
	*total += len(content)
	if *total > maxTreeBytes {
		return unsafeContent("the module is too large")
	}
	t[rel] = content
	return nil
}

// Digest is the content SHA-256 of a tree: the SHA-256 over, in path order,
// "<path>\x00<hex sha256 of content>\n" for every file. It covers names and
// content; file modes are fixed (regular, non-executable).
func (t Tree) Digest() string {
	paths := make([]string, 0, len(t))
	for rel := range t {
		paths = append(paths, rel)
	}
	sort.Strings(paths)
	h := sha256.New()
	for _, rel := range paths {
		sum := sha256.Sum256(t[rel])
		h.Write([]byte(rel))
		h.Write([]byte{0})
		h.Write([]byte(hex.EncodeToString(sum[:])))
		h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Sub returns the files under dir ("." is the whole tree) relative to dir.
func (t Tree) Sub(dir string) Tree {
	out := Tree{}
	if dir == "." {
		for rel, content := range t {
			out[rel] = content
		}
		return out
	}
	prefix := dir + "/"
	for rel, content := range t {
		if strings.HasPrefix(rel, prefix) {
			out[strings.TrimPrefix(rel, prefix)] = content
		}
	}
	return out
}

// readTree reads a materialized directory without following any link. A
// symlink, device, executable or other special file fails closed.
func readTree(root string) (Tree, error) {
	out := Tree{}
	total := 0
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == root {
			if !d.IsDir() {
				return unsafeContent("the module root is not a directory")
			}
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
		case mode&os.ModeSymlink != 0:
			return unsafeContent("a module contains a symbolic link")
		case mode.IsDir():
			if err := checkPath(rel); err != nil {
				return err
			}
			return nil
		case !mode.IsRegular():
			return unsafeContent("a module contains a special file")
		case mode.Perm()&0o111 != 0:
			return unsafeContent("a module contains an executable file")
		}
		content, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return out.add(rel, content, &total)
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// write materializes the tree below dir with private, non-executable modes.
func (t Tree) write(dir string) error {
	for rel, content := range t {
		target := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(target, content, 0o600); err != nil {
			return err
		}
	}
	return nil
}
