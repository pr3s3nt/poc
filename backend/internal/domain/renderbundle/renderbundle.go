// Package renderbundle is the domain of Git-owned workload render bundles
// (ADR-021): the version 1 directory manifest, the immutable verified source
// pin of one Organization and the typed literal render input contract. It does
// no I/O; inspection, caching and execution live in adapters.
package renderbundle

import (
	"bytes"
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Contract identifiers of version 1 (Git bundle contract RB-C01/RB-C02).
const (
	APIVersion      = "orchestrator.dev/render-bundle/v1"
	Kind            = "RenderBundle"
	InputContract   = "orchestrator.dev/render-input/v1"
	EngineContract  = "git-render-v1"
	RendererType    = "score-k8s"
	RendererVersion = "0.15.0"
	SourceKindGit   = "git"
	SourceKindZIP   = "zip"
	ManifestFile    = "bundle.yaml"
)

// Selected tree limits (RB-C02).
const (
	MaxFiles     = 256
	MaxTreeBytes = 4 << 20
	MaxFileBytes = 1 << 20
)

// Errors. Messages never quote submitted values or file content.
var (
	// ErrInvalid is a bundle that breaks the version 1 contract.
	ErrInvalid = errors.New("renderbundle: the bundle is not valid")
	// ErrUnsupported is an unsupported API, engine or renderer version.
	ErrUnsupported = errors.New("renderbundle: the bundle contract is not supported")
	// ErrInputInvalid is render_inputs that the bundle schema does not admit.
	ErrInputInvalid = errors.New("renderbundle: the render inputs are not valid")
)

// InvalidError is an ErrInvalid at a safe bundle-relative file name.
type InvalidError struct {
	File   string
	Reason string
}

func (e *InvalidError) Error() string {
	if e.File == "" {
		return "renderbundle: " + e.Reason
	}
	return "renderbundle: " + e.File + ": " + e.Reason
}
func (e *InvalidError) Is(target error) bool { return target == ErrInvalid }

func invalid(file, format string, args ...any) error {
	return &InvalidError{File: file, Reason: fmt.Sprintf(format, args...)}
}

// Manifest is the strict bundle.yaml of version 1.
type Manifest struct {
	APIVersion string `yaml:"apiVersion" json:"apiVersion"`
	Kind       string `yaml:"kind" json:"kind"`
	Metadata   struct {
		Name string `yaml:"name" json:"name"`
	} `yaml:"metadata" json:"metadata"`
	Renderer struct {
		Type    string `yaml:"type" json:"type"`
		Version string `yaml:"version" json:"version"`
	} `yaml:"renderer" json:"renderer"`
	InputContract string   `yaml:"inputContract" json:"inputContract"`
	InputsSchema  string   `yaml:"inputsSchema" json:"inputsSchema"`
	Defaults      string   `yaml:"defaults" json:"defaults"`
	Normalize     string   `yaml:"normalize" json:"normalize"`
	Provisioners  []string `yaml:"provisioners" json:"provisioners"`
	Patches       []string `yaml:"patches" json:"patches"`
	// Resources are optional templates returning additional Kubernetes
	// objects (ADR-024); omitted means none.
	Resources         []string           `yaml:"resources,omitempty" json:"resources,omitempty"`
	WorkloadTemplates []WorkloadTemplate `yaml:"workloadTemplates,omitempty" json:"workloadTemplates,omitempty"`
}

var namePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// requiredManifestKeys are the version 1 fields that must all be present;
// arrays may be empty but not absent.
var requiredManifestKeys = []string{"apiVersion", "kind", "metadata", "renderer", "inputContract", "inputsSchema", "defaults", "normalize", "provisioners", "patches"}

// ParseManifest decodes bundle.yaml strictly: unknown fields, duplicate keys,
// missing fields, unsupported versions and unsafe or repeated paths fail.
func ParseManifest(raw []byte) (Manifest, error) {
	var presence map[string]any
	if err := yaml.Unmarshal(raw, &presence); err != nil || presence == nil {
		return Manifest{}, invalid(ManifestFile, "the manifest is not a YAML object")
	}
	for _, key := range requiredManifestKeys {
		if _, ok := presence[key]; !ok {
			return Manifest{}, invalid(ManifestFile, "the field %s is required", key)
		}
	}
	var m Manifest
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	decoder.KnownFields(true)
	if err := decoder.Decode(&m); err != nil {
		return Manifest{}, invalid(ManifestFile, "the manifest has unknown, duplicate or mistyped fields")
	}
	if m.APIVersion != APIVersion || m.Kind != Kind {
		return Manifest{}, fmt.Errorf("%w: apiVersion/kind", ErrUnsupported)
	}
	if m.Renderer.Type != RendererType || m.Renderer.Version != RendererVersion {
		return Manifest{}, fmt.Errorf("%w: renderer", ErrUnsupported)
	}
	if m.InputContract != InputContract {
		return Manifest{}, fmt.Errorf("%w: input contract", ErrUnsupported)
	}
	if !namePattern.MatchString(m.Metadata.Name) {
		return Manifest{}, invalid(ManifestFile, "metadata.name must be a lowercase catalog ID")
	}
	for field, file := range map[string]string{"inputsSchema": m.InputsSchema, "defaults": m.Defaults, "normalize": m.Normalize} {
		if !ValidFilePath(file) {
			return Manifest{}, invalid(ManifestFile, "%s must be a clean relative file path", field)
		}
	}
	for field, list := range map[string][]string{"provisioners": m.Provisioners, "patches": m.Patches, "resources": m.Resources} {
		seen := map[string]bool{}
		for _, file := range list {
			if !ValidFilePath(file) {
				return Manifest{}, invalid(ManifestFile, "%s entries must be clean relative file paths", field)
			}
			if seen[file] {
				return Manifest{}, invalid(ManifestFile, "%s lists the same file twice", field)
			}
			seen[file] = true
		}
	}
	if err := validateWorkloadTemplates(m.WorkloadTemplates); err != nil {
		return Manifest{}, err
	}
	if m.Provisioners == nil {
		m.Provisioners = []string{}
	}
	if m.Patches == nil {
		m.Patches = []string{}
	}
	return m, nil
}

// ValidFilePath reports a clean relative slash path inside the bundle.
func ValidFilePath(p string) bool {
	if p == "" || len(p) > 512 || strings.HasPrefix(p, "/") || strings.Contains(p, "\\") || path.Clean(p) != p {
		return false
	}
	for _, segment := range strings.Split(p, "/") {
		if segment == "" || segment == "." || segment == ".." || strings.ContainsAny(segment, "%\x00") {
			return false
		}
		for _, r := range segment {
			if r < 0x20 || r == 0x7f {
				return false
			}
		}
	}
	return true
}

// Source is the immutable Git source pin of a bundle.
type Source struct {
	Kind          string `json:"kind"`
	URL           string `json:"url"`
	Path          string `json:"path"`
	Revision      string `json:"revision"`
	ContentSHA256 string `json:"contentSha256"`
}

// Contract is the validated, persisted bundle contract: the manifest, the
// input schema and the policy defaults. It never holds credentials or paths
// of the host cache.
type Contract struct {
	Manifest      Manifest       `json:"manifest"`
	InputsSchema  map[string]any `json:"inputsSchema"`
	Defaults      map[string]any `json:"defaults"`
	InputContract string         `json:"inputContract"`
}

// Bundle is one verified, immutable Git render bundle of an Organization.
type Bundle struct {
	ID              string    `json:"id"`
	OrganizationKey string    `json:"organizationKey"`
	Source          Source    `json:"source"`
	EngineContract  string    `json:"engineContract"`
	RendererVersion string    `json:"rendererVersion"`
	RendererBinary  string    `json:"rendererBinarySha256"`
	Contract        Contract  `json:"contract"`
	CreatedAt       time.Time `json:"createdAt"`
}

var (
	commitPattern = regexp.MustCompile(`^([0-9a-f]{40}|[0-9a-f]{64})$`)
	digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
	uuidPattern   = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

// ValidID reports a server-issued lowercase UUID.
func ValidID(id string) bool { return uuidPattern.MatchString(id) }

// ValidDirectory reports "." or a clean relative directory.
func ValidDirectory(p string) bool { return p == "." || ValidFilePath(p) }

// Validate checks the structural invariants of a stored record.
func (b Bundle) Validate() error {
	switch {
	case !ValidID(b.ID), b.OrganizationKey == "":
		return invalid("", "record identity is incomplete")
	case (b.Source.Kind != SourceKindGit && b.Source.Kind != SourceKindZIP) || b.Source.URL == "" || !ValidDirectory(b.Source.Path):
		return invalid("", "record source is incomplete")
	case b.Source.Kind == SourceKindZIP && (!digestPattern.MatchString(b.Source.Revision) || b.Source.URL != "zip:sha256:"+b.Source.Revision):
		return invalid("", "ZIP source identity is invalid")
	case !commitPattern.MatchString(b.Source.Revision) || !digestPattern.MatchString(b.Source.ContentSHA256):
		return invalid("", "record pin is incomplete")
	case b.EngineContract != EngineContract || b.RendererVersion != RendererVersion || !digestPattern.MatchString(b.RendererBinary):
		return invalid("", "record engine is not supported")
	case b.Contract.InputContract != InputContract || b.Contract.InputsSchema == nil || b.Contract.Defaults == nil:
		return invalid("", "record contract is incomplete")
	}
	return validateWorkloadTemplates(b.Contract.Manifest.WorkloadTemplates)
}

// SameVerifiedContent reports whether other records the identical verified
// content, contract and renderer as b (a repeated inspection is idempotent).
func (b Bundle) SameVerifiedContent(other Bundle) bool {
	left, err1 := canonical(b.Contract)
	right, err2 := canonical(other.Contract)
	return err1 == nil && err2 == nil && left == right && b.Source == other.Source &&
		b.EngineContract == other.EngineContract && b.RendererVersion == other.RendererVersion && b.RendererBinary == other.RendererBinary
}

// Name is the display name from the manifest; it is not an identity.
func (b Bundle) Name() string { return b.Contract.Manifest.Metadata.Name }

// ShortRevision is the display version: the first 12 characters of the commit.
func (b Bundle) ShortRevision() string {
	if len(b.Source.Revision) < 12 {
		return b.Source.Revision
	}
	return b.Source.Revision[:12]
}

// Clone returns an independent copy.
func (b Bundle) Clone() Bundle {
	out := b
	if b.Contract.Manifest.WorkloadTemplates != nil {
		out.Contract.Manifest.WorkloadTemplates = make([]WorkloadTemplate, len(b.Contract.Manifest.WorkloadTemplates))
		for i, t := range b.Contract.Manifest.WorkloadTemplates {
			out.Contract.Manifest.WorkloadTemplates[i] = t.Clone()
		}
	}
	out.Contract.Manifest.Provisioners = append([]string{}, b.Contract.Manifest.Provisioners...)
	out.Contract.Manifest.Patches = append([]string{}, b.Contract.Manifest.Patches...)
	if b.Contract.Manifest.Resources != nil {
		out.Contract.Manifest.Resources = append([]string{}, b.Contract.Manifest.Resources...)
	}
	out.Contract.InputsSchema, _ = deepCopy(b.Contract.InputsSchema).(map[string]any)
	out.Contract.Defaults, _ = deepCopy(b.Contract.Defaults).(map[string]any)
	return out
}
