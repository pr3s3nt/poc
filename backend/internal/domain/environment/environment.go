package environment

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"

	"orchestrator/internal/domain/application"
)

var namespacePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// InfrastructureScope says where an aws-eks Environment keeps VPC/EKS (ADR-011).
type InfrastructureScope string

// Infrastructure scopes.
const (
	// ScopeEnvironment gives the Environment its own VPC/EKS identity.
	ScopeEnvironment InfrastructureScope = "ENVIRONMENT"
	// ScopeLegacyApplication keeps the Application-scoped VPC/EKS identity of
	// bindings migrated from the former Application target.
	ScopeLegacyApplication InfrastructureScope = "LEGACY_APPLICATION"
)

// Valid reports whether the scope is a known value.
func (s InfrastructureScope) Valid() bool {
	return s == ScopeEnvironment || s == ScopeLegacyApplication
}

// Environment is a deployment target that belongs to exactly one Application.
// ConnectionKey, Profile, Region, RuntimeStatus, InfrastructureScope and
// TargetGeneration are the execution binding: empty/UNCONFIGURED until
// selected, then replaceable only through the versioned dedicated commands
// (ADR-012). SecretStoreKey is the independent workload secret-store
// selection.
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

	ConnectionKey       string                       `json:"connectionKey,omitempty"`
	Profile             application.ExecutionProfile `json:"executionProfile,omitempty"`
	Region              string                       `json:"region,omitempty"`
	RuntimeStatus       application.RuntimeStatus    `json:"runtimeStatus,omitempty"`
	InfrastructureScope InfrastructureScope          `json:"infrastructureScope,omitempty"`

	// TargetGeneration is the immutable generation of the current execution
	// binding. Generation 0 keeps the legacy resource/namespace/state identity.
	TargetGeneration int64 `json:"targetGeneration,omitempty"`
	// GenerationHigh is the highest generation ever allocated to this
	// Environment, including failed transition attempts.
	GenerationHigh int64 `json:"generationHigh,omitempty"`
	// SecretStoreKey selects the workload Secret Store Connection.
	SecretStoreKey string `json:"secretStoreKey,omitempty"`
	// ActiveOperationID is the persisted claim that excludes concurrent
	// execution, settings, configuration and draft writes.
	ActiveOperationID string `json:"activeOperationId,omitempty"`
}

// Configured reports whether the execution binding was set.
func (e Environment) Configured() bool { return e.ConnectionKey != "" }

// Status reports the runtime status, UNCONFIGURED while the binding is unset.
func (e Environment) Status() application.RuntimeStatus {
	if !e.Configured() || e.RuntimeStatus == "" {
		return application.RuntimeUnconfigured
	}
	return e.RuntimeStatus
}

// InfrastructureScopeValid reports whether the stored scope is a known value;
// an unset scope reads as ENVIRONMENT.
func (e Environment) InfrastructureScopeValid() bool { return e.Scope().Valid() }

// Scope reports the infrastructure scope; unset bindings read as ENVIRONMENT.
func (e Environment) Scope() InfrastructureScope {
	if e.InfrastructureScope == "" {
		return ScopeEnvironment
	}
	return e.InfrastructureScope
}

// Binding is the nonsecret execution target pinned in snapshots and hashes.
// Generation is omitted at generation 0 so legacy hashes stay unchanged.
type Binding struct {
	ConnectionKey string                       `json:"connectionKey"`
	Profile       application.ExecutionProfile `json:"executionProfile"`
	Region        string                       `json:"region"`
	Scope         InfrastructureScope          `json:"infrastructureScope"`
	Generation    int64                        `json:"generation,omitempty"`
}

// Binding returns the pinned target of a configured Environment.
func (e Environment) Binding() Binding {
	return Binding{ConnectionKey: e.ConnectionKey, Profile: e.Profile, Region: e.Region, Scope: e.Scope(), Generation: e.TargetGeneration}
}

// WithBinding returns a copy of the Environment executing against binding b.
func (e Environment) WithBinding(b Binding, status application.RuntimeStatus) Environment {
	e.ConnectionKey, e.Profile, e.Region = b.ConnectionKey, b.Profile, b.Region
	e.InfrastructureScope, e.TargetGeneration, e.RuntimeStatus = b.Scope, b.Generation, status
	return e
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
