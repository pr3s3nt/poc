// Package catalogrelease models immutable, Organization-scoped catalog sets.
package catalogrelease

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

var (
	ErrInvalid     = errors.New("catalog: invalid catalog document")
	ErrConflict    = errors.New("catalog: observed tag changed commit")
	ErrIncomplete  = errors.New("catalog: incomplete verified release")
	ErrUnavailable = errors.New("catalog: source unavailable")
)

const MaxManifestBytes = 1 << 20
const MaxComponents = 256
const APIVersion = "orchestrator.dev/catalog/v1"
const (
	TerraformModule = "terraform-module"
	RenderBundle    = "render-bundle"
	Definition      = "definition"
)

var identifier = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
var branchPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,199}$`)

type Component struct {
	ID           string `yaml:"id" json:"id"`
	Kind         string `yaml:"kind" json:"kind"`
	Path         string `yaml:"path" json:"path"`
	ResourceType string `yaml:"resourceType,omitempty" json:"resourceType,omitempty"`
}
type Manifest struct {
	APIVersion string      `yaml:"apiVersion" json:"apiVersion"`
	Components []Component `yaml:"components" json:"components"`
}

func Digest(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }
func ValidDigest(s string) bool {
	b, err := hex.DecodeString(s)
	return err == nil && len(b) == 32 && s == strings.ToLower(s)
}
func ValidCommit(s string) bool {
	b, err := hex.DecodeString(s)
	return err == nil && (len(b) == 20 || len(b) == 32) && s == strings.ToLower(s)
}
func ValidID(s string) bool { return identifier.MatchString(s) }
func ValidPath(s string) bool {
	if s == "" || s == "." || !utf8.ValidString(s) || strings.Contains(s, "\\") || strings.HasPrefix(s, "/") || path.Clean(s) != s {
		return false
	}
	for _, part := range strings.Split(s, "/") {
		if part == ".." || part == "." || part == ".git" || part == "" {
			return false
		}
		for _, r := range part {
			if r < 32 || r == 127 {
				return false
			}
		}
	}
	return true
}
func ValidBranch(s string) bool {
	if !branchPattern.MatchString(s) || strings.HasPrefix(s, "refs/") || strings.Contains(s, "..") || strings.Contains(s, "//") || strings.HasSuffix(s, "/") || strings.HasSuffix(s, ".") {
		return false
	}
	for _, p := range strings.Split(s, "/") {
		if strings.HasPrefix(p, ".") || strings.HasSuffix(p, ".lock") {
			return false
		}
	}
	return true
}
func ValidRepository(s string) bool {
	u, err := url.Parse(s)
	return err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && u.Opaque == "" && !strings.ContainsAny(s, "\r\n\t ")
}

// ParseManifest retains byte provenance separately from canonical component order.
func ParseManifest(raw []byte) (Manifest, error) {
	var m Manifest
	if len(raw) == 0 || len(raw) > MaxManifestBytes || !utf8.Valid(raw) {
		return m, ErrInvalid
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&m); err != nil {
		return Manifest{}, ErrInvalid
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		return Manifest{}, ErrInvalid
	}
	// Reject aliases/anchors explicitly rather than allowing expansion semantics.
	var node yaml.Node
	if yaml.Unmarshal(raw, &node) != nil || !plainYAML(&node) {
		return Manifest{}, ErrInvalid
	}
	if m.APIVersion != APIVersion || len(m.Components) == 0 || len(m.Components) > MaxComponents {
		return Manifest{}, ErrInvalid
	}
	ids, paths := map[string]bool{}, map[string]bool{}
	definitions := 0
	for _, c := range m.Components {
		if !ValidID(c.ID) || !ValidPath(c.Path) || ids[c.ID] || paths[c.Path] {
			return Manifest{}, ErrInvalid
		}
		ids[c.ID] = true
		paths[c.Path] = true
		switch c.Kind {
		case TerraformModule:
			if !ValidID(c.ResourceType) {
				return Manifest{}, ErrInvalid
			}
		case RenderBundle:
			if c.ResourceType != "" {
				return Manifest{}, ErrInvalid
			}
		case Definition:
			if c.ResourceType != "" {
				return Manifest{}, ErrInvalid
			}
			definitions++
		default:
			return Manifest{}, ErrInvalid
		}
	}
	if definitions == 0 {
		return Manifest{}, ErrInvalid
	}
	sort.Slice(m.Components, func(i, j int) bool { return m.Components[i].ID < m.Components[j].ID })
	return m, nil
}
func plainYAML(n *yaml.Node) bool {
	if n.Kind == yaml.AliasNode || n.Anchor != "" {
		return false
	}
	for _, c := range n.Content {
		if !plainYAML(c) {
			return false
		}
	}
	return true
}
