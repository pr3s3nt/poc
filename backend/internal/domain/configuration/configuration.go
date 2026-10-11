// Package configuration models UC-12 desired Application values: Variables
// carry ordinary values in metadata, Secrets are external immutable refs.
package configuration

import "fmt"

type Kind string

const (
	Variable Kind = "VARIABLE"
	Secret   Kind = "SECRET"
)

// Entry is one revision key. A Variable carries only a nonempty ordinary Value
// in Orchestrator metadata. A Secret carries only its explicit owning StoreKey
// plus an opaque immutable ValueRef. Every field is a string, so entries stay
// comparable.
type Entry struct {
	Kind     Kind   `json:"kind"`
	ValueRef string `json:"valueRef,omitempty"`
	StoreKey string `json:"storeKey,omitempty"`
	Value    string `json:"value,omitempty"`
}

// Validate rejects entries that mix Variable and Secret fields, including old
// store-backed Variables. There is no read or conversion fallback.
func (e Entry) Validate() error {
	switch e.Kind {
	case Variable:
		if e.Value == "" || e.ValueRef != "" || e.StoreKey != "" {
			return fmt.Errorf("variable must carry only a metadata value")
		}
	case Secret:
		if e.ValueRef == "" || e.StoreKey == "" || e.Value != "" {
			return fmt.Errorf("secret must carry only an owning store and value ref")
		}
	default:
		return fmt.Errorf("unknown kind")
	}
	return nil
}

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
		if name == "" {
			return fmt.Errorf("configuration: invalid entry %q", name)
		}
		if err := entry.Validate(); err != nil {
			return fmt.Errorf("configuration: invalid entry %q: %w", name, err)
		}
	}
	return nil
}
