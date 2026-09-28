package environment

import (
	"fmt"
	"regexp"
	"time"
)

var namespacePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// Environment is a deployment target that belongs to exactly one Application.
type Environment struct {
	Key                    string `json:"key"`
	ApplicationKey         string `json:"applicationKey"`
	Name                   string `json:"name"`
	Type                   string `json:"environmentType"`
	NamespaceIdentity      string `json:"namespaceIdentity"`
	CurrentDeploymentSetID string `json:"currentDeploymentSetId"`
	Version                int64  `json:"version"`
	DraftVersion           int64  `json:"draftVersion,omitempty"`
	PublicRoutesPending    bool   `json:"publicRoutesPending,omitempty"`
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
	EnvironmentKey        string    `json:"environmentKey"`
	CreatedByDeploymentID string    `json:"createdByDeploymentId,omitempty"`
	Document              Document  `json:"document"`
	DocumentHash          string    `json:"documentHash"`
	CreatedAt             time.Time `json:"createdAt"`
}
