// Package deployment holds Deployment lifecycle, plan snapshot and workload state.
package deployment

import (
	"regexp"
	"time"
)

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
	// ActionProvision pre-provisions destination resources of a target
	// transition before any destination workload starts.
	ActionProvision Action = "PROVISION"
)

// Deployment is one plan-and-execute run against an Environment.
type Deployment struct {
	ID               string `json:"id"`
	EnvironmentID    string `json:"environmentId"`
	OrganizationKey  string `json:"organizationKey"`
	ApplicationKey   string `json:"applicationKey"`
	EnvironmentKey   string `json:"environmentKey"`
	ExecutionProfile string `json:"executionProfile"`
	// ConnectionKey and TargetGeneration record the actual execution target
	// of this run (ADR-012); history never derives them from the current binding.
	ConnectionKey          string     `json:"connectionKey,omitempty"`
	TargetGeneration       int64      `json:"targetGeneration,omitempty"`
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

// ResourceAttempt is the safe progress projection of one persisted Terraform
// attempt of a Deployment (schema `terraform_resource_attempts.deployment_id`).
// It carries only allowlisted codes and the public module pin: never the module
// or target snapshots, state, physical IDs, recovery handles or resolved inputs.
type ResourceAttempt struct {
	Descriptor     string    `json:"descriptor"`
	ScopeType      string    `json:"scopeType"`
	Number         int       `json:"attempt"`
	Action         string    `json:"action"`
	Ownership      string    `json:"ownership"`
	Stage          string    `json:"stage"`
	Status         string    `json:"status"`
	SafeErrorCode  string    `json:"safeErrorCode,omitempty"`
	ModuleID       string    `json:"moduleId"`
	ModuleRevision string    `json:"moduleRevision"`
	StartedAt      time.Time `json:"startedAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// Unknown markers a projection shows for a persisted value outside the allowlists.
const (
	AttemptUnknown      = "UNKNOWN"
	AttemptUnknownError = "UNKNOWN_ERROR"
)

func member(set []string, v string) bool {
	for _, s := range set {
		if s == v {
			return true
		}
	}
	return false
}

var (
	attemptStages    = []string{"INTENT", "INIT", "PLANNING", "PLANNED", "APPLYING", "APPLIED", "OUTPUTS", "READINESS", "READY", "NO_CHANGE", "CHECKING", "DESTROYING", "VERIFYING", "DESTROYED", "RECOVERY"}
	attemptStatuses  = []string{"RUNNING", "SUCCEEDED", "FAILED", "INTERRUPTED", "NEEDS_ATTENTION"}
	attemptActions   = []string{"CREATE", "UPDATE", "DESTROY"}
	attemptOwners    = []string{"CREATED_BY_RUN", "PREEXISTING", "UPDATED_BY_RUN"}
	attemptScopes    = []string{"APPLICATION", "ENVIRONMENT", "WORKLOAD", "SHARED"}
	attemptErrorCode = []string{"DESTRUCTIVE_PLAN", "OUTPUT_CONTRACT", "NOT_READY", "READINESS_UNAVAILABLE", "OUTCOME_UNKNOWN", "STATE_NOT_ACKNOWLEDGED", "STATE_BACKEND", "TRANSIENT", "AUTH", "INPUT", "TIMEOUT", "CANCELLED", "FAILED", "PHYSICAL_RESOURCE_REMAINS", "SECRET_UNAVAILABLE"}
	moduleToken      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	descriptorShape  = regexp.MustCompile(`^[a-z0-9-]+\.[a-z0-9-]+#[a-z0-9@._-]{1,200}$`)
)

// Safe returns the API-boundary projection: persisted strings are free text as
// far as the database is concerned, so every field is checked against an
// explicit allowlist or shape. A value outside it is replaced by a generic
// marker and its original text is never returned.
func (a ResourceAttempt) Safe() ResourceAttempt {
	pick := func(set []string, v, unknown string) string {
		if member(set, v) {
			return v
		}
		return unknown
	}
	out := a
	out.Stage = pick(attemptStages, a.Stage, AttemptUnknown)
	out.Status = pick(attemptStatuses, a.Status, AttemptUnknown)
	out.Action = pick(attemptActions, a.Action, AttemptUnknown)
	out.Ownership = pick(attemptOwners, a.Ownership, AttemptUnknown)
	out.ScopeType = pick(attemptScopes, a.ScopeType, AttemptUnknown)
	out.SafeErrorCode = ""
	if a.SafeErrorCode != "" {
		out.SafeErrorCode = pick(attemptErrorCode, a.SafeErrorCode, AttemptUnknownError)
	}
	if !descriptorShape.MatchString(a.Descriptor) {
		out.Descriptor = AttemptUnknown
	}
	if !moduleToken.MatchString(a.ModuleID) {
		out.ModuleID = AttemptUnknown
	}
	if !moduleToken.MatchString(a.ModuleRevision) {
		out.ModuleRevision = AttemptUnknown
	}
	return out
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
	ID             string `json:"id"`
	EnvironmentKey string `json:"environmentKey"`
	// Generation is the target generation owning this row. Rows of different
	// generations coexist so a replacement target never overwrites the source.
	Generation              int64          `json:"generation,omitempty"`
	WorkloadID              string         `json:"workloadId"`
	LastDeploymentID        string         `json:"lastDeploymentId"`
	AppliedConfigRevisionID string         `json:"appliedConfigRevisionId,omitempty"`
	TargetRef               map[string]any `json:"targetRef"`
	ManifestDigest          string         `json:"manifestDigest"`
	Status                  InstanceStatus `json:"status"`
	ObservedAt              time.Time      `json:"observedAt"`
}

// WorkloadSnapshot is the state observed for one workload during one
// Deployment. Unlike WorkloadInstance, it is never advanced by a later run.
type WorkloadSnapshot struct {
	DeploymentID            string         `json:"deploymentId"`
	WorkloadID              string         `json:"workloadId"`
	AppliedConfigRevisionID string         `json:"appliedConfigRevisionId,omitempty"`
	TargetRef               map[string]any `json:"targetRef"`
	ManifestDigest          string         `json:"manifestDigest"`
	Status                  InstanceStatus `json:"status"`
	ObservedAt              time.Time      `json:"observedAt"`
}
