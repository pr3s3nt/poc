// Package deployment holds Deployment lifecycle, plan snapshot and workload state.
package deployment

import "time"

// Status is the Deployment state machine value.
type Status string

// Deployment states (architecture/state-machines/deployment.puml).
const (
	StatusPlanning     Status = "PLANNING"
	StatusProvisioning Status = "PROVISIONING"
	StatusDeploying    Status = "DEPLOYING"
	StatusSucceeded    Status = "SUCCEEDED"
	StatusFailed       Status = "FAILED"
)

// Action records why the deployment was created.
type Action string

// Deployment actions.
const (
	ActionDeploy Action = "DEPLOY"
	ActionUpdate Action = "UPDATE"
	ActionRemove Action = "REMOVE"
)

// Deployment is one plan-and-execute run against an Environment.
type Deployment struct {
	ID                     string     `json:"id"`
	OrganizationKey        string     `json:"organizationKey"`
	ApplicationKey         string     `json:"applicationKey"`
	EnvironmentKey         string     `json:"environmentKey"`
	ExecutionProfile       string     `json:"executionProfile"`
	Action                 Action     `json:"action"`
	WorkloadID             string     `json:"workloadId"`
	ActorRef               string     `json:"actorRef"`
	Status                 Status     `json:"status"`
	BaseEnvironmentVersion int64      `json:"baseEnvironmentVersion"`
	BaseDeploymentSetID    string     `json:"baseDeploymentSetId"`
	CandidateDeploymentSet string     `json:"candidateDeploymentSetId"`
	DeltaSnapshotID        string     `json:"deltaSnapshotId,omitempty"`
	FailureReason          string     `json:"failureReason,omitempty"`
	StartedAt              time.Time  `json:"startedAt"`
	FinishedAt             *time.Time `json:"finishedAt,omitempty"`
}

// ResourceStatus is the per-node execution status recorded for a Deployment.
type ResourceStatus string

// Deployment resource statuses.
const (
	ResourcePending ResourceStatus = "PENDING"
	ResourceRunning ResourceStatus = "RUNNING"
	ResourceReady   ResourceStatus = "READY"
	ResourceFailed  ResourceStatus = "FAILED"
)

// Resource is the progress record of one resource node within a Deployment.
type Resource struct {
	DeploymentID     string         `json:"deploymentId"`
	NodeDescriptor   string         `json:"nodeDescriptor"`
	ActiveResourceID string         `json:"activeResourceId,omitempty"`
	DefinitionKey    string         `json:"definitionKey,omitempty"`
	ResourceTypeKey  string         `json:"resourceType,omitempty"`
	Status           ResourceStatus `json:"status"`
	BatchIndex       int            `json:"batchIndex"`
	ResolvedInputs   map[string]any `json:"resolvedInputs,omitempty"`
	OutputSnapshot   map[string]any `json:"outputSnapshot,omitempty"`
	StartedAt        time.Time      `json:"startedAt"`
	FinishedAt       *time.Time     `json:"finishedAt,omitempty"`
}

// InstanceStatus is the Workload Instance state machine value.
type InstanceStatus string

// Workload instance states.
const (
	InstanceApplying InstanceStatus = "APPLYING"
	InstanceReady    InstanceStatus = "READY"
	InstanceRemoving InstanceStatus = "REMOVING"
	InstanceRemoved  InstanceStatus = "REMOVED"
	InstanceFailed   InstanceStatus = "FAILED"
)

// WorkloadInstance is the applied state of one workload inside an Environment.
type WorkloadInstance struct {
	ID                      string         `json:"id"`
	EnvironmentKey          string         `json:"environmentKey"`
	WorkloadID              string         `json:"workloadId"`
	LastDeploymentID        string         `json:"lastDeploymentId"`
	AppliedConfigRevisionID string         `json:"appliedConfigRevisionId,omitempty"`
	TargetRef               map[string]any `json:"targetRef"`
	ManifestDigest          string         `json:"manifestDigest"`
	Status                  InstanceStatus `json:"status"`
	ObservedAt              time.Time      `json:"observedAt"`
}
