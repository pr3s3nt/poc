// Package resourcecleanup is the scoped, safe read side and the confirmed
// deletion flow of Terraform-managed resources (UC-07 DEL-*, UC-09 RES-01..03,
// OC-19, ADR-014): resource detail, deletion preview, the atomic admission of
// the explicitly selected set and the follow-up of the admitted operation.
//
// Execution is the existing tfexec cleanup engine; this package only decides
// what may be admitted and projects persisted, allowlisted results. It never
// returns target references, snapshots, resolved inputs, state, recovery paths
// or CLI diagnostics.
package resourcecleanup

import (
	"errors"
	"time"
)

// Scope is the session-derived tenant scope of every call.
type Scope struct {
	OrganizationKey string
	ApplicationKey  string
	EnvironmentKey  string
}

// Actor is the authenticated caller. The role gates the mutations; the
// username is the persisted actor reference.
type Actor struct {
	Username string
	// MayDelete is true for the Platform Engineer and Admin roles only.
	MayDelete bool
}

// Allowlisted refusal reasons (OC-19) and the extra busy reason of the
// eligibility projection. Public responses never carry any other text.
const (
	ReasonRetainedData      = "RETAINED_DATA"
	ReasonNotUnreferenced   = "NOT_UNREFERENCED"
	ReasonHasConsumers      = "HAS_CONSUMERS"
	ReasonPlatformResource  = "PLATFORM_RESOURCE"
	ReasonOwnershipUnproven = "OWNERSHIP_UNVERIFIED"
	ReasonTargetUnavailable = "ORIGINAL_TARGET_UNAVAILABLE"
	ReasonEnvironmentBusy   = "ENVIRONMENT_BUSY"
	// ReasonMetadataSyncPending: the Active Resource is UNREFERENCED but the State
	// projection (the Terraform instance status, owned by the State login) was not
	// confirmed. Deletion stays refused (fail closed); nothing is repaired automatically.
	ReasonMetadataSyncPending = "METADATA_SYNC_PENDING"
	// ReasonInfrastructureDown is shown in the eligibility of a resource when
	// the state, module, credential or engine services needed to delete it
	// are not configured: a snapshot without the means is not eligible.
	ReasonInfrastructureDown = "INFRASTRUCTURE_UNAVAILABLE"
)

var refusalReasons = map[string]bool{
	ReasonRetainedData: true, ReasonNotUnreferenced: true, ReasonHasConsumers: true, ReasonPlatformResource: true,
	ReasonOwnershipUnproven: true, ReasonTargetUnavailable: true, ReasonEnvironmentBusy: true, ReasonMetadataSyncPending: true,
}

// Consumer kinds of the safe consumer projection.
const (
	KindResource        = "RESOURCE"
	KindWorkload        = "WORKLOAD"
	KindPendingWorkload = "PENDING_WORKLOAD"
	KindDeploymentSet   = "DEPLOYMENT_SET"
)

// Errors. Messages are fixed; none carries a submitted value.
var (
	// ErrUnavailable means the state, module or credential infrastructure the
	// flow needs is not configured (503).
	ErrUnavailable = errors.New("resourcecleanup: the resource infrastructure is not available")
	// ErrForbidden means the caller may read but not delete (403).
	ErrForbidden = errors.New("resourcecleanup: only a Platform Engineer or Admin may delete resources")
	// ErrInvalid is a malformed request (400, field selectedResourceIds/token).
	ErrInvalid = errors.New("resourcecleanup: invalid request")
	// ErrConfirmationMismatch is a wrong confirmation name (400). It never
	// carries the submitted or the expected name.
	ErrConfirmationMismatch = errors.New("resourcecleanup: the confirmation name does not match")
	// ErrStalePreview means the preview no longer describes the persisted
	// state, or the request replays a token with a different selected set (409).
	ErrStalePreview = errors.New("resourcecleanup: the preview is stale; review the deletion scope again")
)

// FieldError is a malformed request field (400). It unwraps to ErrInvalid and
// names only the fixed request field, never a submitted value.
type FieldError struct{ Field string }

func (e *FieldError) Error() string { return "resourcecleanup: invalid request field" }
func (e *FieldError) Unwrap() error { return ErrInvalid }

func invalid(field string) error { return &FieldError{Field: field} }

// RefusedError is an ineligible root or selection (409 CLEANUP_REFUSED). The
// reason is one of the allowlisted codes.
type RefusedError struct{ Reason string }

func (e *RefusedError) Error() string { return "resourcecleanup: the deletion is refused" }

// RefusalReason returns the allowlisted reason of the error.
func (e *RefusedError) RefusalReason() string {
	if refusalReasons[e.Reason] {
		return e.Reason
	}
	return ReasonHasConsumers
}

// ConsumerView is one safe consumer of a resource.
type ConsumerView struct {
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	ResourceID string `json:"resourceId,omitempty"`
	Status     string `json:"status,omitempty"`
}

// Eligibility is the snapshot decision whether the resource could be deleted.
// It is not a grant: the admission re-checks everything.
type Eligibility struct {
	Eligible bool     `json:"eligible"`
	Reasons  []string `json:"reasons"`
}

// TargetView is the safe original target of a resource: the Connection and
// binding of the persisted run that last executed it, never the Environment's
// current binding and never a private target reference. Known is false when
// the persisted public records do not identify it (then nothing is shown).
type TargetView struct {
	Known          bool   `json:"known"`
	ConnectionKey  string `json:"connectionKey,omitempty"`
	ConnectionName string `json:"connectionName,omitempty"`
	ConnectionKind string `json:"connectionKind,omitempty"`
	AccountID      string `json:"accountId,omitempty"`
	Region         string `json:"region,omitempty"`
	Generation     int64  `json:"generation"`
}

// ModuleView is the pinned module identity of a resource.
type ModuleView struct {
	ID       string `json:"id"`
	Revision string `json:"revision"`
}

// AttemptView is the safe summary of the last execution attempt.
type AttemptView struct {
	Number    int       `json:"number"`
	Action    string    `json:"action"`
	Stage     string    `json:"stage"`
	Status    string    `json:"status"`
	ErrorCode string    `json:"errorCode,omitempty"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// CleanupSummary links a resource to the latest cleanup operation touching it.
type CleanupSummary struct {
	OperationID string `json:"operationId"`
	Status      string `json:"status"`
	Stage       string `json:"stage"`
}

// ScopeView is the ownership scope of a resource.
type ScopeView struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// ResourceView is the safe resource detail (UC-09 RES-01).
type ResourceView struct {
	ID          string          `json:"id"`
	Application string          `json:"applicationId"`
	Environment string          `json:"environment"`
	Descriptor  string          `json:"descriptor"`
	Type        string          `json:"type"`
	Class       string          `json:"class"`
	ResourceID  string          `json:"resourceId"`
	Status      string          `json:"status"`
	Scope       ScopeView       `json:"scope"`
	Owner       string          `json:"owner"`
	Generation  int64           `json:"generation"`
	Module      ModuleView      `json:"module"`
	Target      TargetView      `json:"target"`
	Version     int64           `json:"version"`
	Outputs     map[string]any  `json:"outputs"`
	Consumers   []ConsumerView  `json:"consumers"`
	Eligibility Eligibility     `json:"eligibility"`
	LastAttempt *AttemptView    `json:"lastAttempt,omitempty"`
	Cleanup     *CleanupSummary `json:"cleanup,omitempty"`
}

// CandidateView is the root or one dependency candidate of a deletion preview.
type CandidateView struct {
	ID         string `json:"id"`
	Descriptor string `json:"descriptor"`
	Type       string `json:"type"`
	Class      string `json:"class"`
	Status     string `json:"status"`
	Version    int64  `json:"version"`
	// Selectable is false when a static blocker permanently excludes the
	// item from any selection; Reasons then names the allowlisted causes.
	Selectable bool     `json:"selectable"`
	Reasons    []string `json:"reasons"`
	// RequiredBy lists the IDs of in-scope consumers that must be selected
	// before this item may be selected.
	RequiredBy []string       `json:"requiredBy"`
	Consumers  []ConsumerView `json:"consumers"`
}

// PreviewView is the deletion preview (OC-19 PrepareDeletion).
type PreviewView struct {
	Token            string          `json:"token"`
	ExpiresAt        time.Time       `json:"expiresAt"`
	Application      string          `json:"applicationId"`
	Environment      string          `json:"environment"`
	Generation       int64           `json:"generation"`
	ConfirmationName string          `json:"confirmationName"`
	Root             CandidateView   `json:"root"`
	Candidates       []CandidateView `json:"candidates"`
}

// AdmissionView is the accepted operation of a deletion request.
type AdmissionView struct {
	OperationID string `json:"operationId"`
	Status      string `json:"status"`
	Stage       string `json:"stage"`
	Replayed    bool   `json:"replayed"`
}

// CleanupResourceView is the per-resource progress and verification result.
type CleanupResourceView struct {
	ID         string `json:"id"`
	Descriptor string `json:"descriptor"`
	// CurrentStatus is the resource's status right now. It is context only: a
	// later operation may have changed it. Outcome and Verified are what THIS
	// operation recorded.
	CurrentStatus    string   `json:"currentStatus"`
	Outcome          string   `json:"outcome"`
	Verified         bool     `json:"verified"`
	Remaining        []string `json:"remaining"`
	RemainingUnknown bool     `json:"remainingUnknown"`
	ReasonCode       string   `json:"reasonCode,omitempty"`
}

// CleanupView is the safe persisted view of one cleanup operation (RES-03).
type CleanupView struct {
	OperationID string `json:"operationId"`
	Application string `json:"applicationId"`
	Environment string `json:"environment"`
	// Status is the persisted claim status (ACTIVE, SUCCEEDED, FAILED,
	// INTERRUPTED, RECOVERING). Stage is the allowlisted REQUESTED, CHECKING,
	// DESTROYING or VERIFYING.
	Status string `json:"status"`
	Stage  string `json:"stage"`
	// Terminal is the engine result once known: SUCCEEDED, FAILED or
	// NEEDS_ATTENTION. AllVerified is true only when every confirmed resource
	// is persisted as DESTROYED.
	Terminal    string                `json:"terminal,omitempty"`
	AllVerified bool                  `json:"allVerified"`
	Mode        string                `json:"authorizationMode"`
	Actor       string                `json:"actor"`
	RootID      string                `json:"rootId"`
	Resources   []CleanupResourceView `json:"resources"`
	RemainingID []string              `json:"remainingIds"`
	FailureCode string                `json:"failureCode,omitempty"`
	StartedAt   time.Time             `json:"startedAt"`
	UpdatedAt   time.Time             `json:"updatedAt"`
	FinishedAt  *time.Time            `json:"finishedAt,omitempty"`
}

// SummaryView is one row of the Environment's resource list (UC-09: the
// navigation entry to a resource detail, including UNREFERENCED resources whose
// Workload was removed). It carries identity and status only.
type SummaryView struct {
	ID         string `json:"id"`
	Descriptor string `json:"descriptor"`
	Type       string `json:"type"`
	Class      string `json:"class"`
	ResourceID string `json:"resourceId"`
	Status     string `json:"status"`
	Scope      string `json:"scope"`
	Generation int64  `json:"generation"`
	Module     string `json:"module"`
}
