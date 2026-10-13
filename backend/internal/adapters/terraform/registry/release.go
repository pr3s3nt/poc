package registry

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"orchestrator/internal/domain/tfmodule"
)

// releaseJSON is the platform release manifest shipped with the binary: the
// exact Terraform version and binary digest, the trusted provider versions
// and lock hashes, the executor-owned variables and the trusted packaged
// modules with their fixed revision and content digests. It is reviewed
// release data, not Admin or PE input.
//
//go:embed release.json
var releaseJSON []byte

// Release is the parsed platform release manifest.
type Release struct {
	Terraform         ReleaseTerraform  `json:"terraform"`
	Providers         []ReleaseProvider `json:"providers"`
	ExecutorVariables []string          `json:"executorVariables"`
	Packaged          PackagedSource    `json:"packaged"`
}

// ReleaseTerraform pins the Terraform CLI.
type ReleaseTerraform struct {
	Version      string `json:"version"`
	BinarySHA256 string `json:"binarySha256"`
}

// ReleaseProvider pins one trusted provider. Binding says which platform
// target configures it; "none" providers take no binding.
type ReleaseProvider struct {
	Source  string   `json:"source"`
	Version string   `json:"version"`
	Binding string   `json:"binding"`
	Hashes  []string `json:"hashes"`
}

// PackagedSource is the trusted packaged module source of the release.
type PackagedSource struct {
	SourceID string           `json:"sourceId"`
	Revision string           `json:"revision"`
	Modules  []PackagedModule `json:"modules"`
}

// PackagedModule pins one packaged module.
type PackagedModule struct {
	ID              string   `json:"id"`
	Subdirectory    string   `json:"subdirectory"`
	ContentSHA256   string   `json:"contentSha256"`
	ResourceTypes   []string `json:"resourceTypes"`
	ExecutorOutputs []string `json:"executorOutputs,omitempty"`
}

// Ref returns the source pin of a packaged module.
func (r Release) Ref(m PackagedModule) tfmodule.SourceRef {
	return tfmodule.SourceRef{SourceID: r.Packaged.SourceID, ModuleID: m.ID, Revision: r.Packaged.Revision, Subdirectory: m.Subdirectory, ContentSHA256: m.ContentSHA256}
}

// Clone returns a deep copy: the registry owns its release pins, so no
// caller can change provider hashes, packaged digests, resource types or
// executor-owned names after validation.
func (r Release) Clone() Release {
	out := r
	out.ExecutorVariables = append([]string(nil), r.ExecutorVariables...)
	out.Providers = make([]ReleaseProvider, len(r.Providers))
	for i, p := range r.Providers {
		p.Hashes = append([]string(nil), p.Hashes...)
		out.Providers[i] = p
	}
	out.Packaged.Modules = make([]PackagedModule, len(r.Packaged.Modules))
	for i, m := range r.Packaged.Modules {
		m.ResourceTypes = append([]string(nil), m.ResourceTypes...)
		m.ExecutorOutputs = append([]string(nil), m.ExecutorOutputs...)
		out.Packaged.Modules[i] = m
	}
	return out
}

// DefaultRelease returns the embedded platform release manifest.
func DefaultRelease() (Release, error) { return parseRelease(releaseJSON) }

// PackagedRefs returns the source pins of every packaged module of the
// embedded release, keyed by module ID. Seeds reference these pins.
func PackagedRefs() (map[string]tfmodule.SourceRef, error) {
	release, err := DefaultRelease()
	if err != nil {
		return nil, err
	}
	out := map[string]tfmodule.SourceRef{}
	for _, m := range release.Packaged.Modules {
		out[m.ID] = release.Ref(m)
	}
	return out, nil
}

func strictDecode(data []byte, into any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		return errors.New("malformed or unknown fields")
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return errors.New("trailing data")
	}
	return nil
}

func parseRelease(data []byte) (Release, error) {
	var r Release
	if err := strictDecode(data, &r); err != nil {
		return Release{}, fmt.Errorf("registry: release manifest: %w", err)
	}
	if err := r.validate(); err != nil {
		return Release{}, fmt.Errorf("registry: release manifest: %w", err)
	}
	return r, nil
}

func (r Release) validate() error {
	if !tfmodule.ValidVersion(r.Terraform.Version) || !tfmodule.ValidDigest(r.Terraform.BinarySHA256) {
		return errors.New("terraform version and binary digest must be exact")
	}
	seen := map[string]bool{}
	for _, p := range r.Providers {
		if !strings.HasPrefix(p.Source, "registry.terraform.io/") || strings.Count(p.Source, "/") != 2 || seen[p.Source] {
			return errors.New("provider sources must be unique fully qualified registry addresses")
		}
		seen[p.Source] = true
		if !tfmodule.ValidVersion(p.Version) || len(p.Hashes) == 0 {
			return errors.New("providers need an exact version and lock hashes")
		}
		switch p.Binding {
		case tfmodule.BindingAWS, tfmodule.BindingKubernetes, tfmodule.BindingNone:
		default:
			return errors.New("provider binding kind is unknown")
		}
		for _, h := range p.Hashes {
			if !strings.HasPrefix(h, "h1:") && !strings.HasPrefix(h, "zh:") {
				return errors.New("provider hashes must be h1: or zh: lock hashes")
			}
		}
	}
	if !tfmodule.ValidIdentifier(r.Packaged.SourceID) || r.Packaged.Revision == "" || len(r.Packaged.Modules) == 0 {
		return errors.New("packaged source needs an identifier and revision")
	}
	ids := map[string]bool{}
	for _, m := range r.Packaged.Modules {
		ref := r.Ref(m)
		if err := ref.Validate(); err != nil || ids[m.ID] || len(m.ResourceTypes) == 0 {
			return errors.New("packaged modules need unique IDs, valid pins and resource types")
		}
		ids[m.ID] = true
	}
	return nil
}

// provider returns the release pin of a fully qualified provider source.
func (r Release) provider(source string) (ReleaseProvider, bool) {
	for _, p := range r.Providers {
		if p.Source == source {
			return p, true
		}
	}
	return ReleaseProvider{}, false
}

// LockFile renders the readonly Terraform dependency lock file for the given
// provider requirements. Its SHA-256 is the bundle provider_lock_digest; the
// executor writes exactly these bytes and runs init with -lockfile=readonly.
func LockFile(providers map[string]tfmodule.ProviderRequirement) []byte {
	bySource := map[string]tfmodule.ProviderRequirement{}
	for _, p := range providers {
		bySource[p.Source] = p
	}
	sources := make([]string, 0, len(bySource))
	for source := range bySource {
		sources = append(sources, source)
	}
	sort.Strings(sources)
	var b strings.Builder
	b.WriteString("# Generated from the platform release manifest. Do not edit.\n")
	for _, source := range sources {
		p := bySource[source]
		hashes := append([]string(nil), p.Hashes...)
		sort.Strings(hashes)
		fmt.Fprintf(&b, "\nprovider %q {\n  version     = %q\n  constraints = %q\n  hashes = [\n", source, p.Version, p.Version)
		for _, h := range hashes {
			fmt.Fprintf(&b, "    %q,\n", h)
		}
		b.WriteString("  ]\n}\n")
	}
	return []byte(b.String())
}

// LockDigest is the SHA-256 of LockFile(providers).
func LockDigest(providers map[string]tfmodule.ProviderRequirement) string {
	sum := sha256.Sum256(LockFile(providers))
	return hex.EncodeToString(sum[:])
}

// fileSHA256 hashes a file without following it into a directory.
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("not a regular file")
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// retainedFile is one entry of the retained-releases file: a trusted
// historical release manifest (the release.json shape) and the absolute path
// of its pinned Terraform binary.
type retainedFile struct {
	Release         json.RawMessage `json:"release"`
	TerraformBinary string          `json:"terraformBinary"`
}

// LoadRetained reads the platform-reviewed list of retained historical
// releases. An empty path retains none. The file is strict: unknown fields,
// relative binary paths and invalid manifests fail startup.
func LoadRetained(path string) ([]RetainedRelease, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("registry: the retained releases file cannot be read")
	}
	var entries []retainedFile
	if err := strictDecode(data, &entries); err != nil {
		return nil, fmt.Errorf("registry: retained releases: %w", err)
	}
	out := make([]RetainedRelease, 0, len(entries))
	for _, e := range entries {
		release, err := parseRelease(e.Release)
		if err != nil {
			return nil, err
		}
		if e.TerraformBinary == "" || e.TerraformBinary[0] != '/' {
			return nil, errors.New("registry: a retained release needs an absolute terraformBinary path")
		}
		out = append(out, RetainedRelease{Release: release, TerraformBinary: e.TerraformBinary})
	}
	return out, nil
}
