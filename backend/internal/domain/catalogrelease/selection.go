package catalogrelease

import (
	"encoding/json"
	"golang.org/x/mod/semver"
	"orchestrator/internal/domain/resource"
	"sort"
	"strings"
)

// Selection is an immutable Environment intent including platform-owned bindings.
type Selection struct {
	SourceID    string            `json:"sourceId"`
	Tag         string            `json:"tag"`
	Fingerprint string            `json:"fingerprint"`
	Accounts    map[string]string `json:"accounts"`
}

func (s Selection) Validate() error {
	if !ValidID(s.SourceID) || s.Tag == "" || !ValidDigest(s.Fingerprint) {
		return ErrInvalid
	}
	for slot, account := range s.Accounts {
		if !ValidID(slot) || account == "" {
			return ErrInvalid
		}
	}
	return nil
}
func (s Selection) Digest() string { raw, _ := json.Marshal(s); return Digest(raw) }

// Definitions materializes only this release, preserving stable Definition keys.
func (r Release) Definitions(selection Selection) ([]resource.Definition, error) {
	if r.Validate() != nil || selection.Validate() != nil || selection.SourceID != r.SourceID || selection.Tag != r.Tag || selection.Fingerprint != r.Fingerprint() {
		return nil, ErrIncomplete
	}
	out := []resource.Definition{}
	for _, c := range r.Components {
		if c.Kind != Definition {
			continue
		}
		raw, err := json.Marshal(c.Snapshot)
		if err != nil {
			return nil, ErrIncomplete
		}
		var d resource.Definition
		if json.Unmarshal(raw, &d) != nil {
			return nil, ErrIncomplete
		}
		if c.Binding != "" {
			account := selection.Accounts[c.Binding]
			if account == "" {
				return nil, ErrIncomplete
			}
			d.DriverAccount = account
		}
		out = append(out, d)
	}
	resource.SortDefinitions(out)
	return out, nil
}

func IsDowngrade(before, after string) bool {
	if !strings.HasPrefix(before, "v") {
		before = "v" + before
	}
	if !strings.HasPrefix(after, "v") {
		after = "v" + after
	}
	return semver.IsValid(before) && semver.IsValid(after) && semver.Compare(after, before) < 0
}

// SortReleases presents numeric SemVer order, newest first, with a stable tie.
func SortReleases(releases []Release) {
	sort.Slice(releases, func(i, j int) bool {
		a, b := releases[i].Tag, releases[j].Tag
		if !strings.HasPrefix(a, "v") {
			a = "v" + a
		}
		if !strings.HasPrefix(b, "v") {
			b = "v" + b
		}
		if c := semver.Compare(a, b); c != 0 {
			return c > 0
		}
		return releases[i].Tag < releases[j].Tag
	})
}
