// Package application holds Organization, Application and Connection aggregates.
package application

import (
	"fmt"
	"regexp"
	"strings"
)

var keyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,40}$`)
var dnsLabelPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// ExecutionProfile is the execution policy bound to an Application (UC-01).
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

// RuntimeStatus tracks whether application-scoped infrastructure exists.
type RuntimeStatus string

// Application runtime statuses.
const (
	RuntimePending RuntimeStatus = "PENDING"
	RuntimeReady   RuntimeStatus = "READY"
)

// Organization is the ownership boundary of every other aggregate.
type Organization struct {
	ID                   string `json:"id"`
	Key                  string `json:"key"`
	Name                 string `json:"name"`
	DefaultConnectionKey string `json:"defaultConnectionKey"`
}

// Application owns exactly one Execution Profile and, for aws-eks, the VPC/EKS scope.
type Application struct {
	ID                    string           `json:"id"`
	Key                   string           `json:"key"`
	OrganizationKey       string           `json:"organizationKey"`
	Name                  string           `json:"name"`
	Subdomain             string           `json:"subdomain"`
	Profile               ExecutionProfile `json:"executionProfile"`
	ConnectionKey         string           `json:"connectionKey"`
	Region                string           `json:"region,omitempty"`
	RuntimeStatus         RuntimeStatus    `json:"runtimeStatus"`
	Version               int64            `json:"version"`
	ConfigurationProvider string           `json:"configurationProvider,omitempty"`
}

// Validate reports whether the Application satisfies UC-01 business rules.
func (a Application) Validate() error {
	if !keyPattern.MatchString(a.Key) {
		return fmt.Errorf("application: invalid application key %q", a.Key)
	}
	// Legacy seeded planning fixtures predate UC-01 self-service creation and
	// have no public endpoint. UC-01's creation service always requires it.
	if a.Subdomain != "" && !dnsLabelPattern.MatchString(a.Subdomain) {
		return fmt.Errorf("application: invalid subdomain %q", a.Subdomain)
	}
	if !a.Profile.Valid() {
		return fmt.Errorf("application: invalid execution profile %q", a.Profile)
	}
	if a.ConnectionKey == "" {
		return fmt.Errorf("application: application %q has no connection", a.Key)
	}
	if a.Profile == ProfileAWSEKS && a.Region == "" {
		return fmt.Errorf("application: aws-eks application %q needs a region", a.Key)
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
	// AuthHostContext is the legacy Kubernetes kube context on the backend host.
	AuthHostContext AuthenticationType = "HOST_CONTEXT"
	// AuthKubeconfig is an uploaded, normalized selected-context kubeconfig
	// held by the Connection credential store.
	AuthKubeconfig AuthenticationType = "KUBECONFIG"
	// AuthAWSAccessKey is the AWS account identity of ADR-009.
	AuthAWSAccessKey AuthenticationType = "AWS_ACCESS_KEY"
)

// Compatible reports whether the authentication type belongs to the kind.
func (t AuthenticationType) Compatible(kind ConnectionKind) bool {
	switch t {
	case AuthHostContext, AuthKubeconfig:
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

// Legacy reference shapes written before authentication types existed.
const (
	legacyHostContextRef = "host-kube-context://"
	legacySeedRef        = "secret://connections/"
)

// WithLegacyDefaults fills Name and AuthenticationType of records written
// before those fields existed. A missing name reads as the key; a missing
// authentication type is inferred only from the known legacy reference
// shapes. Unknown or kind-incompatible types fail closed.
func (c Connection) WithLegacyDefaults() (Connection, error) {
	if c.Name == "" {
		c.Name = c.Key
	}
	if c.AuthenticationType == "" {
		switch {
		case c.Kind == ConnectionKubernetes && (strings.HasPrefix(c.SecretRef, legacyHostContextRef) || strings.HasPrefix(c.SecretRef, legacySeedRef)):
			c.AuthenticationType = AuthHostContext
		case c.Kind == ConnectionAWS && strings.HasPrefix(c.SecretRef, legacySeedRef):
			c.AuthenticationType = AuthAWSAccessKey
		default:
			return c, fmt.Errorf("application: connection %q has an unknown authentication type", c.Key)
		}
	}
	if !c.AuthenticationType.Compatible(c.Kind) {
		return c, fmt.Errorf("application: connection %q has an unsupported authentication type", c.Key)
	}
	return c, nil
}

// CredentialBacked reports whether execution must resolve the credential from
// the Connection credential store instead of the backend host.
func (c Connection) CredentialBacked() bool { return c.AuthenticationType == AuthKubeconfig }

// ConfigString reads a non-secret configuration string.
func (c Connection) ConfigString(key string) string {
	if v, ok := c.Config[key].(string); ok {
		return v
	}
	return ""
}
