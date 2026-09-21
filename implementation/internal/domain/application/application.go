// Package application holds Organization, Application and Connection aggregates.
package application

import (
	"fmt"
	"regexp"
)

var keyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,40}$`)

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
	Key  string `json:"key"`
	Name string `json:"name"`
}

// Application owns exactly one Execution Profile and, for aws-eks, the VPC/EKS scope.
type Application struct {
	Key             string           `json:"key"`
	OrganizationKey string           `json:"organizationKey"`
	Name            string           `json:"name"`
	Profile         ExecutionProfile `json:"executionProfile"`
	ConnectionKey   string           `json:"connectionKey"`
	Region          string           `json:"region,omitempty"`
	RuntimeStatus   RuntimeStatus    `json:"runtimeStatus"`
	Version         int64            `json:"version"`
}

// Validate reports whether the Application satisfies UC-01 business rules.
func (a Application) Validate() error {
	if !keyPattern.MatchString(a.Key) {
		return fmt.Errorf("application: invalid application key %q", a.Key)
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

// Connection is a verified driver account or registered cluster.
// Only the opaque secret reference is persisted; credential values never are.
type Connection struct {
	Key             string           `json:"key"`
	OrganizationKey string           `json:"organizationKey"`
	Kind            ConnectionKind   `json:"kind"`
	Config          map[string]any   `json:"config"`
	SecretRef       string           `json:"secretRef"`
	Status          ConnectionStatus `json:"status"`
	Verification    map[string]any   `json:"verification"`
}

// ConfigString reads a non-secret configuration string.
func (c Connection) ConfigString(key string) string {
	if v, ok := c.Config[key].(string); ok {
		return v
	}
	return ""
}
