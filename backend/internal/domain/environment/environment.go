package environment

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var namespacePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// InfrastructureScope pins resource identity scope; it never requests infrastructure or selects a deployment target.
type InfrastructureScope string

// Infrastructure scopes.
const (
	// ScopeEnvironment gives explicitly requested resources an Environment identity scope.
	ScopeEnvironment InfrastructureScope = "ENVIRONMENT"
)

// ErrInvalidInfrastructureScope reports a stored or submitted scope other than
// ENVIRONMENT. It never carries the rejected value.
var ErrInvalidInfrastructureScope = errors.New("environment: infrastructure scope must be ENVIRONMENT")

// Valid reports whether the scope is the only supported value.
func (s InfrastructureScope) Valid() bool { return s == ScopeEnvironment }

// Environment is a deployment scope that belongs to exactly one Application.
// It carries no deployment Connection, execution profile or region (ADR-017):
// placement comes from Resource Definitions, accounts and typed references.
// InfrastructureScope and TargetGeneration pin the identity of the
// Environment's resources. SecretStoreKey is the independent workload
// secret-store selection.
type Environment struct {
	ID                     string `json:"id"`
	Key                    string `json:"key"`
	ApplicationID          string `json:"applicationId"`
	ApplicationKey         string `json:"applicationKey"`
	Name                   string `json:"name"`
	Type                   string `json:"environmentType"`
	NamespaceIdentity      string `json:"namespaceIdentity"`
	CurrentDeploymentSetID string `json:"currentDeploymentSetId"`
	Version                int64  `json:"version"`
	DraftVersion           int64  `json:"draftVersion,omitempty"`
	PublicRoutesPending    bool   `json:"publicRoutesPending,omitempty"`

	InfrastructureScope InfrastructureScope `json:"infrastructureScope,omitempty"`

	// TargetGeneration is the immutable identity generation of the Environment's resources.
	// Generation 0 keeps the legacy resource/namespace/state identity.
	TargetGeneration int64 `json:"targetGeneration,omitempty"`
	// SecretStoreKey selects the workload Secret Store Connection.
	SecretStoreKey string `json:"secretStoreKey,omitempty"`
	// ActiveOperationID is the persisted claim that excludes concurrent
	// execution, settings, configuration and draft writes.
	ActiveOperationID string `json:"activeOperationId,omitempty"`
}

// InfrastructureScopeValid reports whether the stored scope is unset (reads as
// ENVIRONMENT) or ENVIRONMENT. Any other value is rejected, never normalized.
func (e Environment) InfrastructureScopeValid() bool {
	return e.InfrastructureScope == "" || e.InfrastructureScope.Valid()
}

// Scope reports the infrastructure scope; an unset scope reads as ENVIRONMENT.
// A stored invalid value is returned unchanged so callers cannot mistake it
// for ENVIRONMENT; reject it with InfrastructureScopeValid first.
func (e Environment) Scope() InfrastructureScope {
	if e.InfrastructureScope == "" {
		return ScopeEnvironment
	}
	return e.InfrastructureScope
}

// Binding is the nonsecret identity pin of an Environment's resources in snapshots and hashes.
// Generation is omitted at generation 0 so existing hashes stay unchanged.
type Binding struct {
	Scope      InfrastructureScope `json:"infrastructureScope"`
	Generation int64               `json:"generation,omitempty"`
}

// Binding returns the pinned identity of the Environment.
func (e Environment) Binding() Binding {
	return Binding{Scope: e.Scope(), Generation: e.TargetGeneration}
}

// Busy reports whether an operation currently owns the Environment.
func (e Environment) Busy() bool { return e.ActiveOperationID != "" }

// Namespace is the physical namespace of the current target generation.
func (e Environment) Namespace() string { return NamespaceFor(e.NamespaceIdentity, e.TargetGeneration) }

// NamespaceFor derives the physical namespace of one target generation.
// Generation 0 is the legacy identity. Later generations keep the bounded
// readable identity prefix and end with "-g<N>-<hash>", where the hash covers
// the full identity and generation, so truncation never removes the part that
// makes generations unique. The result is a DNS-1123 label of at most 63 bytes.
func NamespaceFor(identity string, generation int64) string {
	if generation <= 0 {
		return identity
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00g%d", identity, generation)))
	suffix := fmt.Sprintf("-g%d-%s", generation, hex.EncodeToString(sum[:])[:8])
	prefix := identity
	if limit := 63 - len(suffix); len(prefix) > limit {
		prefix = strings.TrimRight(prefix[:limit], "-")
	}
	return prefix + suffix
}

// GenerationScopeSuffix is appended to Environment-scoped resource scope IDs
// and descriptor paths of generation >= 1; generation 0 appends nothing.
func GenerationScopeSuffix(generation int64) string {
	if generation <= 0 {
		return ""
	}
	return fmt.Sprintf(".g%d", generation)
}

// Validate reports whether the Environment can be used as a deployment target.
func (e Environment) Validate() error {
	if e.Key == "" || e.ApplicationKey == "" {
		return fmt.Errorf("environment: key and application key are required")
	}
	if !e.InfrastructureScopeValid() {
		return ErrInvalidInfrastructureScope
	}
	if !namespacePattern.MatchString(e.NamespaceIdentity) {
		return fmt.Errorf("environment: namespace identity %q is not a valid DNS-1123 label", e.NamespaceIdentity)
	}
	return nil
}

// DeploymentSet is an immutable desired-state snapshot of an Environment.
type DeploymentSet struct {
	ID                    string    `json:"id"`
	EnvironmentID         string    `json:"environmentId"`
	EnvironmentKey        string    `json:"environmentKey"`
	CreatedByDeploymentID string    `json:"createdByDeploymentId,omitempty"`
	Document              Document  `json:"document"`
	DocumentHash          string    `json:"documentHash"`
	CreatedAt             time.Time `json:"createdAt"`
	// LiteralProofs is private provenance of accepted ordinary variables
	// (UC-16 BR-23). It is outside Document and DocumentHash, so planning and
	// Delta semantics are unchanged; absent means every literal is unknown.
	LiteralProofs LiteralProofs `json:"literalProofs,omitempty"`
	// ResourceAliases preserves Score shared names outside planning semantics.
	ResourceAliases ResourceAliases `json:"resourceAliases,omitempty"`
}

// ScopeID is the Environment resource scope identifier of one target
// generation. Generation 0 keeps the legacy "<app>.<env>" identity; later
// generations append "~g<N>", a character no key can contain, so a workload
// scope of generation 0 can never collide with a later generation's scope.
func ScopeID(applicationKey, environmentKey string, generation int64) string {
	base := applicationKey + "." + environmentKey
	if generation <= 0 {
		return base
	}
	return fmt.Sprintf("%s~g%d", base, generation)
}
