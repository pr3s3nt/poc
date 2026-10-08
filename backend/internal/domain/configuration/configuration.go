// Package configuration models UC-12 desired Application values without
// storing the values themselves in orchestrator state.
package configuration

import "fmt"

type Kind string

const (
	Variable Kind = "VARIABLE"
	Secret   Kind = "SECRET"
)

// Entry is one revision key. A Variable carries its ordinary Value in
// Orchestrator metadata; a Secret carries the owning StoreKey plus an opaque
// ValueRef. A legacy Variable keeps its original ValueRef/StoreKey with an
// empty Value until a successful edit or store transition materializes it.
// Every field is a string, so entries stay comparable.
type Entry struct {
	Kind     Kind   `json:"kind"`
	ValueRef string `json:"valueRef,omitempty"`
	StoreKey string `json:"storeKey,omitempty"`
	Value    string `json:"value,omitempty"`
}

// LegacyVariable reports a Variable that still reads through a store ref.
func (e Entry) LegacyVariable() bool { return e.Kind == Variable && e.Value == "" && e.ValueRef != "" }

// UsesStore reports whether the entry bytes live in a secret store.
func (e Entry) UsesStore() bool { return e.Kind == Secret || e.LegacyVariable() }

type Revision struct {
	ID             string           `json:"id"`
	ApplicationKey string           `json:"applicationKey"`
	EnvironmentKey string           `json:"environmentKey"`
	Version        int64            `json:"version"`
	Entries        map[string]Entry `json:"entries"`
}

type Scope struct {
	ApplicationKey    string `json:"applicationKey"`
	EnvironmentKey    string `json:"environmentKey"`
	DesiredRevisionID string `json:"desiredRevisionId,omitempty"`
	Version           int64  `json:"version"`
}

func (r Revision) Validate() error {
	if r.ID == "" || r.ApplicationKey == "" || r.EnvironmentKey == "" || r.Version <= 0 {
		return fmt.Errorf("configuration: incomplete revision identity")
	}
	for name, entry := range r.Entries {
		if name == "" || (entry.Kind != Variable && entry.Kind != Secret) {
			return fmt.Errorf("configuration: invalid entry %q", name)
		}
		if entry.Kind == Secret && (entry.ValueRef == "" || entry.Value != "") {
			return fmt.Errorf("configuration: invalid secret entry %q", name)
		}
		if entry.Kind == Variable && entry.Value == "" && entry.ValueRef == "" {
			return fmt.Errorf("configuration: invalid variable entry %q", name)
		}
	}
	return nil
}
