package resource

import "fmt"

// ScopeType names the ownership boundary of a resource instance.
type ScopeType string

// Scope types defined by the architecture baseline.
const (
	ScopeApplication ScopeType = "APPLICATION"
	ScopeEnvironment ScopeType = "ENVIRONMENT"
	ScopeWorkload    ScopeType = "WORKLOAD"
	ScopeShared      ScopeType = "SHARED"
)

// Scope is the second half of an Active Resource logical identity.
type Scope struct {
	Type ScopeType `json:"type"`
	ID   string    `json:"id"`
}

// Validate reports whether the scope is usable as a logical identity.
func (s Scope) Validate() error {
	switch s.Type {
	case ScopeApplication, ScopeEnvironment, ScopeWorkload, ScopeShared:
	default:
		return fmt.Errorf("resource: unknown scope type %q", s.Type)
	}
	if s.ID == "" {
		return fmt.Errorf("resource: scope id is empty")
	}
	return nil
}

// Key renders a comparable scope key.
func (s Scope) Key() string { return string(s.Type) + ":" + s.ID }
