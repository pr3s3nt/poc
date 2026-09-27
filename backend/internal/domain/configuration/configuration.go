// Package configuration models UC-12 desired Application values without
// storing the values themselves in orchestrator state.
package configuration

import "fmt"

type Kind string

const (
	Variable Kind = "VARIABLE"
	Secret   Kind = "SECRET"
)

type Entry struct {
	Kind     Kind   `json:"kind"`
	ValueRef string `json:"valueRef"`
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
		if name == "" || (entry.Kind != Variable && entry.Kind != Secret) || entry.ValueRef == "" {
			return fmt.Errorf("configuration: invalid entry %q", name)
		}
	}
	return nil
}
