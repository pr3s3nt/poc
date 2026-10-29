// Package appdeletion implements UC-18 Xóa Application (ADR-025): a read-only
// deletion preview of both Environments, the typed-name admission that moves
// the Application to DELETING, and the staged run that removes every
// workload, destroys every owned resource, purges the Application's Vault
// values and finally marks the Application DELETED.
//
// It composes the existing flows: workload removal is the UC-07 pending
// Delete -> Preview -> Deploy path, resource destruction is the confirmed
// cleanup engine of UC-07 DEL. It never destroys platform or PREEXISTING
// resources and never returns provider text, targets or secret values.
package appdeletion

import (
	"errors"
	"time"

	"orchestrator/internal/application/resourcecleanup"
	appdomain "orchestrator/internal/domain/application"
)

// PreviewTTL bounds how long a deletion preview may be confirmed.
const PreviewTTL = 15 * time.Minute

// Blocker codes. Public responses carry no other text.
const (
	BlockerEnvironmentBusy      = "ENVIRONMENT_BUSY"
	BlockerScheduleNotSuspended = "SCHEDULE_NOT_SUSPENDED"
	BlockerDeletionRunning      = "DELETION_RUNNING"
	BlockerResourceHeld         = resourcecleanup.ReasonInstanceHeld
	BlockerOwnership            = resourcecleanup.ReasonOwnershipUnproven
)

// Failure codes of a stage.
const (
	FailWorkloadRemoval = "WORKLOAD_REMOVAL_FAILED"
	FailCleanup         = "CLEANUP_INCOMPLETE"
	FailCleanupRefused  = "CLEANUP_REFUSED"
	FailVerification    = "VERIFICATION_FAILED"
	FailSecretPurge     = "SECRET_PURGE_FAILED"
	FailEnvironmentBusy = "ENVIRONMENT_BUSY"
	FailInterrupted     = "INTERRUPTED"
	FailInternal        = "INTERNAL_ERROR"
)

// Warning codes of a preview.
const (
	WarningMatchingCriteria = "MATCHING_CRITERIA_TARGET_APPLICATION"
)

// Errors. Messages are fixed and never carry submitted values.
var (
	ErrUnavailable          = errors.New("appdeletion: the deletion services are not available")
	ErrForbidden            = errors.New("appdeletion: the role may not delete Applications")
	ErrInvalid              = errors.New("appdeletion: invalid request")
	ErrConfirmationMismatch = errors.New("appdeletion: the confirmation name does not match")
	ErrStalePreview         = errors.New("appdeletion: the preview is stale; review the deletion again")
	ErrDeleting             = errors.New("appdeletion: the Application is being deleted")
)

// BlockedError refuses an admission because the preview has blockers.
type BlockedError struct{ Codes []string }

func (e *BlockedError) Error() string { return "appdeletion: the deletion is blocked" }

// Actor is the authenticated caller.
type Actor struct {
	Username string
	// MayDelete is true for ADMIN, PLATFORM_ENGINEER and DEVELOPER (ADR-025).
	MayDelete bool
}

// Blocker is one reason the deletion cannot be admitted now.
type Blocker struct {
	Code        string `json:"code"`
	ResourceID  string `json:"resourceId,omitempty"`
	Descriptor  string `json:"descriptor,omitempty"`
	OperationID string `json:"operationId,omitempty"`
}

// EnvironmentPreview is the deletion scope of one Environment.
type EnvironmentPreview struct {
	Key           string                          `json:"key"`
	Workloads     []string                        `json:"workloads"`
	PendingDrafts []string                        `json:"pendingDrafts"`
	Resources     []resourcecleanup.InventoryItem `json:"resources"`
	SecretValues  int                             `json:"secretValues"`
	Deployments   int                             `json:"deployments"`
	Blockers      []Blocker                       `json:"blockers"`
}

// Kept lists what the deletion never touches.
type Kept struct {
	SecretStores      []string `json:"secretStores"`
	PlatformResources int      `json:"platformResources"`
	Definitions       []string `json:"definitions"`
}

// PreviewView is the deletion preview (UC-18 MS-01..02).
type PreviewView struct {
	Token            string               `json:"token,omitempty"`
	ExpiresAt        *time.Time           `json:"expiresAt,omitempty"`
	Application      string               `json:"applicationId"`
	Status           string               `json:"status"`
	ConfirmationName string               `json:"confirmationName"`
	Environments     []EnvironmentPreview `json:"environments"`
	Kept             Kept                 `json:"kept"`
	Warnings         []string             `json:"warnings"`
	Blocked          bool                 `json:"blocked"`
	// Progress is the persisted progress of a previous attempt, if any.
	Progress *DeletionView `json:"progress,omitempty"`
}

// AdmissionView is the accepted deletion.
type AdmissionView struct {
	OperationID string `json:"operationId"`
	Status      string `json:"status"`
	Replayed    bool   `json:"replayed"`
}

// DeletionView is the safe persisted progress (UC-18 UI API).
type DeletionView struct {
	OperationID       string                          `json:"operationId"`
	Application       string                          `json:"applicationId"`
	ApplicationStatus string                          `json:"applicationStatus"`
	Status            string                          `json:"status"`
	FailureCode       string                          `json:"failureCode,omitempty"`
	Actor             string                          `json:"actor"`
	Attempt           int                             `json:"attempt"`
	Running           bool                            `json:"running"`
	RequestedAt       time.Time                       `json:"requestedAt"`
	UpdatedAt         time.Time                       `json:"updatedAt"`
	FinishedAt        *time.Time                      `json:"finishedAt,omitempty"`
	Environments      []appdomain.DeletionEnvironment `json:"environments"`
}
