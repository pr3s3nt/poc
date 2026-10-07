package environment

import (
	"fmt"
	"regexp"
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
// ConnectionKey, Profile, Region, RuntimeStatus and InfrastructureScope are the
// execution binding: empty/UNCONFIGURED until set once, then immutable
// (runtime status excepted).
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
type Binding struct {
	ConnectionKey string                       `json:"connectionKey"`
	Profile       application.ExecutionProfile `json:"executionProfile"`
	Region        string                       `json:"region"`
	Scope         InfrastructureScope          `json:"infrastructureScope"`
}

// Binding returns the pinned target of a configured Environment.
func (e Environment) Binding() Binding {
	return Binding{ConnectionKey: e.ConnectionKey, Profile: e.Profile, Region: e.Region, Scope: e.Scope()}
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
