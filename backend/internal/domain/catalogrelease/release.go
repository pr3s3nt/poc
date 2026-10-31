package catalogrelease

import (
	"encoding/json"
	"sort"
	"time"

	"orchestrator/internal/domain/resource"
)

type Source struct {
	AccountBindings    map[string]string `json:"accountBindings,omitempty"`
	Organization       string            `json:"organization"`
	ID                 string            `json:"id"`
	Repository         string            `json:"repository"`
	Branch             string            `json:"branch"`
	TagPattern         string            `json:"tagPattern"`
	ManifestPath       string            `json:"manifestPath"`
	IncludePrereleases bool              `json:"includePrereleases"`
	Version            int64             `json:"version"`
}

func (s Source) Validate() error {
	for slot, account := range s.AccountBindings {
		if !ValidID(slot) || !ValidID(account) {
			return ErrInvalid
		}
	}
	if s.Organization == "" || !ValidID(s.ID) || !ValidRepository(s.Repository) || len(s.Repository) > 2048 || len(s.ManifestPath) > 1024 || !ValidBranch(s.Branch) || !ValidPath(s.ManifestPath) || len(s.TagPattern) == 0 || len(s.TagPattern) > 100 || s.Version < 1 {
		return ErrInvalid
	}
	for _, r := range s.TagPattern {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '*' || r == '?' || r == '.' || r == '-' || r == '_') {
			return ErrInvalid
		}
	}
	return nil
}

type VerifiedComponent struct {
	Component
	SHA256   string `json:"sha256"`
	BundleID string `json:"bundleId,omitempty"`
	// Snapshot excludes account bindings; materialization overlays platform slots.
	Snapshot *resource.Definition `json:"snapshot,omitempty"`
	Binding  string               `json:"binding,omitempty"`
}
type Release struct {
	Organization   string              `json:"organization"`
	SourceID       string              `json:"sourceId"`
	Tag            string              `json:"tag"`
	Commit         string              `json:"commit"`
	BranchTip      string              `json:"branchTip"`
	Manifest       []byte              `json:"manifest"`
	ManifestSHA256 string              `json:"manifestSha256"`
	Components     []VerifiedComponent `json:"components"`
	ObservedAt     time.Time           `json:"observedAt"`
}

func (r Release) Validate() error {
	if r.Organization == "" || !ValidID(r.SourceID) || r.Tag == "" || !ValidCommit(r.Commit) || !ValidCommit(r.BranchTip) || r.ObservedAt.IsZero() || Digest(r.Manifest) != r.ManifestSHA256 {
		return ErrIncomplete
	}
	m, err := ParseManifest(r.Manifest)
	if err != nil || len(m.Components) != len(r.Components) {
		return ErrIncomplete
	}
	byID := map[string]VerifiedComponent{}
	for _, c := range r.Components {
		if _, ok := byID[c.ID]; ok {
			return ErrIncomplete
		}
		byID[c.ID] = c
	}
	for _, c := range m.Components {
		v, ok := byID[c.ID]
		if !ok || v.Component != c || !ValidDigest(v.SHA256) {
			return ErrIncomplete
		}
		switch c.Kind {
		case Definition:
			if v.Snapshot == nil || v.Snapshot.Validate() != nil || v.Snapshot.DriverAccount != "" || v.BundleID != "" {
				return ErrIncomplete
			}
		default:
			if v.BundleID == "" || v.Snapshot != nil || v.Binding != "" {
				return ErrIncomplete
			}
		}
	}
	return nil
}

// Fingerprint excludes observation metadata and includes all execution pins.
func (r Release) Fingerprint() string {
	components := append([]VerifiedComponent(nil), r.Components...)
	sort.Slice(components, func(i, j int) bool { return components[i].ID < components[j].ID })
	raw, _ := json.Marshal(struct {
		Commit, Manifest string
		Components       []VerifiedComponent
	}{r.Commit, r.ManifestSHA256, components})
	return Digest(raw)
}

type Difference struct {
	ID     string `json:"id"`
	Change string `json:"change"`
}

func Compare(before, after Release) []Difference {
	old, next := map[string]VerifiedComponent{}, map[string]VerifiedComponent{}
	keys := map[string]bool{}
	for _, c := range before.Components {
		old[c.ID] = c
		keys[c.ID] = true
	}
	for _, c := range after.Components {
		next[c.ID] = c
		keys[c.ID] = true
	}
	result := make([]Difference, 0, len(keys))
	for id := range keys {
		a, aok := old[id]
		b, bok := next[id]
		change := "no-change"
		switch {
		case !aok:
			change = "added"
		case !bok:
			change = "removed"
		case a.Kind != b.Kind || a.SHA256 != b.SHA256:
			change = "update"
		}
		result = append(result, Difference{id, change})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}
