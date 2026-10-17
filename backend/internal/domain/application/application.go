// Package application holds Organization, Application and Connection aggregates.
package application

import (
	"errors"
	"fmt"
	"regexp"
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
	// Version is the optimistic-concurrency guard of V01 account updates. A
	// stored record is always >= 1; the stores assign 1 only to a new record.
	Version      int64            `json:"version"`
	Config       map[string]any   `json:"config"`
	SecretRef    string           `json:"secretRef"`
	Status       ConnectionStatus `json:"status"`
	Verification map[string]any   `json:"verification"`
}

// ErrInvalidConnection reports a Connection whose explicit metadata is missing
// or inconsistent. It never carries the rejected values.
var ErrInvalidConnection = errors.New("application: connection needs a name and an authentication type compatible with its kind")

// Validate requires the explicit current metadata: a display Name and an
// AuthenticationType compatible with Kind. Nothing is inferred or defaulted;
// the process-configured AWS seed supplies both explicitly.
func (c Connection) Validate() error {
	if c.Name == "" || !c.AuthenticationType.Compatible(c.Kind) {
		return ErrInvalidConnection
	}
	return nil
}

// CredentialBacked reports whether execution must resolve the credential from
// the Connection credential store; no host credential exists.
func (c Connection) CredentialBacked() bool { return c.AuthenticationType == AuthKubeconfig }

// ConfigStrings reads a non-secret list of strings. A list that went through
// JSON storage is []any, an in-process one is []string; any other shape or a
// non-string member reads as absent.
func (c Connection) ConfigStrings(key string) []string {
	switch values := c.Config[key].(type) {
	case []string:
		return append([]string(nil), values...)
	case []any:
		out := make([]string, 0, len(values))
		for _, value := range values {
			text, ok := value.(string)
			if !ok {
				return nil
			}
			out = append(out, text)
		}
		return out
	}
	return nil
}

// AllowedRegions are the AWS regions the Connection was verified for, in the
// order the registrant gave them (the first is the default). A historical
// record without the allowedRegions field allows exactly its single "region".
// When the field exists it is authoritative: a malformed, empty, duplicated or
// non-string list yields no region at all (fail closed), never a silent
// fallback to "region".
func (c Connection) AllowedRegions() []string {
	if _, present := c.Config["allowedRegions"]; present {
		regions := c.ConfigStrings("allowedRegions")
		seen := map[string]bool{}
		for _, region := range regions {
			if region == "" || seen[region] {
				return nil
			}
			seen[region] = true
		}
		return regions
	}
	if region := c.ConfigString("region"); region != "" {
		return []string{region}
	}
	return nil
}

// AllowsRegion reports whether region is one of the Connection's allowed regions.
func (c Connection) AllowsRegion(region string) bool {
	for _, allowed := range c.AllowedRegions() {
		if region != "" && allowed == region {
			return true
		}
	}
	return false
}

// ConfigString reads a non-secret configuration string.
func (c Connection) ConfigString(key string) string {
	if v, ok := c.Config[key].(string); ok {
		return v
	}
	return ""
}
