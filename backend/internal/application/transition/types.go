// Package transition implements explicit Environment target transitions
// (ADR-012): DEPLOY_NEW and MIGRATE_POSTGRES with downtime, persisted staged
// progress, source preservation, recovery and explicit source cleanup.
package transition

import (
	"errors"
	"fmt"

	"orchestrator/internal/domain/environment"
)

// Errors mapped by delivery. Texts are fixed and safe.
var (
	ErrNoRuntime          = errors.New("transition: the environment has no runtime resources; change its connection in Settings instead")
	ErrSameDestination    = errors.New("transition: choose a destination different from the current connection")
	ErrAcknowledge        = errors.New("transition: migrating data stops the application; acknowledge the downtime")
	ErrStaleToken         = errors.New("transition: the preview is stale; preview the transition again")
	ErrRecoveryIncomplete = errors.New("transition: recovery did not complete; the environment stays held until it succeeds")
	ErrNotRecoverable     = errors.New("transition: the operation cannot be recovered now")
	ErrCleanupRefused     = errors.New("transition: the source generation cannot be cleaned up")
	ErrDestinationFailed  = errors.New("transition: the destination is not available")
)

// UnsupportedError reports a capability the transition cannot honor. It is
// raised during preflight, before any workload is stopped.
type UnsupportedError struct{ Reason string }

func (e *UnsupportedError) Error() string { return "transition: unsupported: " + e.Reason }

func unsupported(format string, args ...any) error {
	return &UnsupportedError{Reason: fmt.Sprintf(format, args...)}
}

// Request selects the destination and mode.
type Request struct {
	OrganizationKey, ApplicationKey, EnvironmentKey string
	DestinationKey                                  string
	Mode                                            environment.TransitionMode
	Mappings                                        []environment.ResourceMapping
}

// ExecuteRequest carries the server-issued token and the downtime acknowledgement.
type ExecuteRequest struct {
	Request
	Token               string
	AcknowledgeDowntime bool
	Actor               string
}

// Endpoint is the safe projection of one side of a transition.
type Endpoint struct {
	ConnectionKey  string `json:"connectionKey"`
	ConnectionName string `json:"connectionName,omitempty"`
	Kind           string `json:"kind,omitempty"`
	Profile        string `json:"executionProfile"`
	Region         string `json:"region,omitempty"`
	Generation     int64  `json:"generation"`
	Namespace      string `json:"namespace,omitempty"`
}

// WorkloadImpact names a workload the transition redeploys.
type WorkloadImpact struct {
	WorkloadID string `json:"workloadId"`
	// Action is UNCHANGED, UPDATED or ADDED (pending draft) or REMOVED (absent at
	// the destination; the source is never touched).
	Action        string `json:"action"`
	ConfigChanged bool   `json:"configChanged,omitempty"`
}

// Preview is what the Developer reviews before executing.
type Preview struct {
	Token            string                        `json:"token"`
	ApplicationKey   string                        `json:"applicationKey"`
	EnvironmentKey   string                        `json:"environmentKey"`
	Mode             environment.TransitionMode    `json:"mode"`
	Source           Endpoint                      `json:"source"`
	Destination      Endpoint                      `json:"destination"`
	Workloads        []WorkloadImpact              `json:"workloads"`
	Mappings         []environment.ResourceMapping `json:"mappings"`
	DowntimeRequired bool                          `json:"downtimeRequired"`
	Notes            []string                      `json:"notes"`
}

// Detail is the safe, refresh-stable projection of a transition record.
type Detail struct {
	ID          string                       `json:"id"`
	OperationID string                       `json:"operationId"`
	Mode        environment.TransitionMode   `json:"mode"`
	Stage       environment.TransitionStage  `json:"stage"`
	Status      environment.TransitionStatus `json:"status"`
	Source      Endpoint                     `json:"source"`
	Destination Endpoint                     `json:"destination"`
	SourceState environment.SourceState      `json:"sourceState"`
	// Authority names which generation is the live, recorded target: derived from
	// the persisted binding, never from the transition status alone.
	Authority          string                        `json:"authority"`
	Mappings           []environment.ResourceMapping `json:"mappings,omitempty"`
	Stages             []environment.StageResult     `json:"stages"`
	Failure            string                        `json:"failure,omitempty"`
	Compensation       []string                      `json:"compensation,omitempty"`
	CompensationFailed []string                      `json:"compensationFailed,omitempty"`
	BackupRetained     bool                          `json:"backupRetained,omitempty"`
	CanCleanupSource   bool                          `json:"canCleanupSource"`
	CreatedAt          string                        `json:"createdAt"`
	UpdatedAt          string                        `json:"updatedAt"`
}
