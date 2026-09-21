package resource

import "time"

// Status is the Active Resource state machine value.
type Status string

// Active Resource states (architecture/state-machines/active-resource.puml).
const (
	StatusProvisioning Status = "PROVISIONING"
	StatusReady        Status = "READY"
	StatusUnreferenced Status = "UNREFERENCED"
	StatusFailed       Status = "FAILED"
)

// ActiveResource is a provisioned resource instance recorded by the orchestrator.
type ActiveResource struct {
	ID               string         `json:"id"`
	OrganizationKey  string         `json:"organizationKey"`
	Descriptor       Descriptor     `json:"descriptor"`
	Scope            Scope          `json:"scope"`
	DefinitionKey    string         `json:"definitionKey"`
	ConnectionKey    string         `json:"connectionKey,omitempty"`
	Status           Status         `json:"status"`
	ExecutorState    map[string]any `json:"executorState,omitempty"`
	Outputs          map[string]any `json:"outputs,omitempty"`
	InputFingerprint string         `json:"inputFingerprint,omitempty"`
	LastDeploymentID string         `json:"lastDeploymentId,omitempty"`
	Version          int64          `json:"version"`
	CreatedAt        time.Time      `json:"createdAt"`
	UpdatedAt        time.Time      `json:"updatedAt"`
}

// LogicalKey is the unique Active Resource identity: organization + descriptor + scope.
func LogicalKey(org string, d Descriptor, s Scope) string {
	return org + "|" + d.String() + "|" + s.Key()
}

// LogicalKey returns the logical identity of this Active Resource.
func (a ActiveResource) LogicalKey() string {
	return LogicalKey(a.OrganizationKey, a.Descriptor, a.Scope)
}
