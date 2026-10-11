// Package application holds Organization, Application and Connection aggregates.
package application

import (
	"fmt"
	"regexp"
	"strings"
)

var keyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,40}$`)
var dnsLabelPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// ExecutionProfile is the execution policy resolved from an Environment target
// (UC-01).
type ExecutionProfile string

// Execution profiles supported by the MVP.
const (
	ProfileAWSEKS      ExecutionProfile = "aws-eks"
	ProfileInternalK8s ExecutionProfile = "internal-k8s"
)

// Valid reports whether the profile is one of the two approved values.
func (p ExecutionProfile) Valid() bool {
	return p == ProfileAWSEKS || p == ProfileInternalK8s
}

// RuntimeStatus tracks whether Environment-scoped infrastructure exists.
type RuntimeStatus string

// Runtime statuses.
const (
	// RuntimeUnconfigured is an Environment whose target is not set yet.
	RuntimeUnconfigured RuntimeStatus = "UNCONFIGURED"
	RuntimePending      RuntimeStatus = "PENDING"
	RuntimeReady        RuntimeStatus = "READY"
)

// Organization is the ownership boundary of every other aggregate.
type Organization struct {
	ID                   string `json:"id"`
	Key                  string `json:"key"`
	Name                 string `json:"name"`
	DefaultConnectionKey string `json:"defaultConnectionKey"`
}

// Application owns identity and current metadata only. Every execution and
// store binding belongs to an Environment (ADR-011/ADR-012).
type Application struct {
	ID              string `json:"id"`
	Key             string `json:"key"`
	OrganizationKey string `json:"organizationKey"`
	Name            string `json:"name"`
	Subdomain       string `json:"subdomain"`
	Version         int64  `json:"version"`
}

// Validate reports whether the Application satisfies UC-01 business rules.
func (a Application) Validate() error {
	if !keyPattern.MatchString(a.Key) {
		return fmt.Errorf("application: invalid application key %q", a.Key)
	}
	// Seeded planning fixtures predate UC-01 self-service creation and have no
	// public endpoint. UC-01's creation service always requires it.
	if a.Subdomain != "" && !dnsLabelPattern.MatchString(a.Subdomain) {
		return fmt.Errorf("application: invalid subdomain %q", a.Subdomain)
	}
	return nil
}

// ConnectionKind distinguishes cloud accounts from registered clusters.
type ConnectionKind string

// Connection kinds registered in UC-04.
const (
	ConnectionAWS        ConnectionKind = "AWS"
	ConnectionKubernetes ConnectionKind = "KUBERNETES"
)

// ConnectionStatus is the Connection state machine value.
type ConnectionStatus string

// Connection states.
const (
	ConnectionVerifying ConnectionStatus = "VERIFYING"
	ConnectionReady     ConnectionStatus = "READY"
	ConnectionRejected  ConnectionStatus = "REJECTED"
)

// AuthenticationType says how a Connection's credential is resolved. It is
// separate from the destination kind (UC-04 BR-14).
type AuthenticationType string

// Authentication types of the shared credential design.
const (
	// AuthKubeconfig is an uploaded, normalized selected-context kubeconfig
	// held by the Connection credential store.
	AuthKubeconfig AuthenticationType = "KUBECONFIG"
	// AuthAWSAccessKey is the AWS account identity of ADR-009.
	AuthAWSAccessKey AuthenticationType = "AWS_ACCESS_KEY"
)

// Compatible reports whether the authentication type belongs to the kind.
func (t AuthenticationType) Compatible(kind ConnectionKind) bool {
	switch t {
	case AuthKubeconfig:
		return kind == ConnectionKubernetes
	case AuthAWSAccessKey:
		return kind == ConnectionAWS
	}
	return false
}

// Connection is a verified driver account or registered cluster.
// Only the opaque secret reference is persisted; credential values never are.
type Connection struct {
	ID                 string             `json:"id"`
	Key                string             `json:"key"`
	Name               string             `json:"name,omitempty"`
	OrganizationKey    string             `json:"organizationKey"`
	Kind               ConnectionKind     `json:"kind"`
	AuthenticationType AuthenticationType `json:"authenticationType,omitempty"`
	Config             map[string]any     `json:"config"`
	SecretRef          string             `json:"secretRef"`
	Status             ConnectionStatus   `json:"status"`
	Verification       map[string]any     `json:"verification"`
}

// legacySeedRef is the process-configured AWS seed reference shape.
const legacySeedRef = "secret://connections/"

// WithLegacyDefaults fills Name of records written before it existed. A
// Kubernetes Connection must declare KUBECONFIG explicitly: a missing type or
// any type other than KUBECONFIG is never inferred. Only the process-configured
// AWS seed reference infers its type. Unknown or kind-incompatible types fail
// closed.
func (c Connection) WithLegacyDefaults() (Connection, error) {
	if c.Name == "" {
		c.Name = c.Key
	}
	if c.AuthenticationType == "" {
		if c.Kind == ConnectionAWS && strings.HasPrefix(c.SecretRef, legacySeedRef) {
			c.AuthenticationType = AuthAWSAccessKey
		} else {
			return c, fmt.Errorf("application: connection %q has an unknown authentication type", c.Key)
		}
	}
	if !c.AuthenticationType.Compatible(c.Kind) {
		return c, fmt.Errorf("application: connection %q has an unsupported authentication type", c.Key)
	}
	return c, nil
}

// CredentialBacked reports whether execution must resolve the credential from
// the Connection credential store; no host credential exists.
func (c Connection) CredentialBacked() bool { return c.AuthenticationType == AuthKubeconfig }

// ConfigString reads a non-secret configuration string.
func (c Connection) ConfigString(key string) string {
	if v, ok := c.Config[key].(string); ok {
		return v
	}
	return ""
}
