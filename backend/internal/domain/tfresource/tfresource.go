// Package tfresource holds the Terraform v2 resource-instance model (ADR-014):
// the stable instance identity and its opaque state key, write-ahead attempts,
// the persisted dependency graph, state locks, executor capabilities and the
// explicit-selection groundwork of resource cleanup. It holds no state bytes,
// tokens or credentials.
package tfresource

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"orchestrator/internal/domain/resource"
)

// ErrInvalid reports a malformed instance, attempt, capability or cleanup
// request. Messages name the field, never a submitted value.
var ErrInvalid = errors.New("tfresource: invalid request")

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrInvalid}, args...)...)
}

// Identity is the canonical identity tuple of one resource instance. It never
// contains a run ID or a Definition version; a different Environment or target
// generation is always a different instance.
type Identity struct {
	OrganizationID   string
	ApplicationID    string
	EnvironmentID    string
	TargetGeneration int64
	Descriptor       string
	Scope            resource.Scope
}

// Validate reports whether every component of the tuple is present.
func (i Identity) Validate() error {
	if i.OrganizationID == "" || i.ApplicationID == "" || i.EnvironmentID == "" {
		return invalid("identity needs Organization, Application and Environment")
	}
	if i.TargetGeneration < 0 {
		return invalid("identity target generation is negative")
	}
	if d, err := resource.ParseDescriptor(i.Descriptor); err != nil || d.String() != i.Descriptor {
		return invalid("identity descriptor is not canonical")
	}
	if err := i.Scope.Validate(); err != nil {
		return invalid("identity scope is invalid")
	}
	return nil
}

// StateKey is the opaque backend state ID: a digest of the canonical tuple.
// Every component is length-prefixed so no two tuples share an encoding.
func (i Identity) StateKey() string {
	h := sha256.New()
	h.Write([]byte("orchestrator-terraform-state-v1"))
	for _, part := range []string{i.OrganizationID, i.ApplicationID, i.EnvironmentID, strconv.FormatInt(i.TargetGeneration, 10), i.Descriptor, string(i.Scope.Type), i.Scope.ID} {
		h.Write([]byte(strconv.Itoa(len(part))))
		h.Write([]byte{':'})
		h.Write([]byte(part))
	}
	return hex.EncodeToString(h.Sum(nil))
}

var stateKeyPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// ValidStateKey reports whether s has the shape of an opaque state key.
func ValidStateKey(s string) bool { return stateKeyPattern.MatchString(s) }

// InstanceStatus is the persisted lifecycle status of an instance.
type InstanceStatus string

// Instance statuses (schema `terraform_resource_instances.status`).
const (
	InstancePending        InstanceStatus = "PENDING"
	InstanceProvisioning   InstanceStatus = "PROVISIONING"
	InstanceReady          InstanceStatus = "READY"
	InstanceFailed         InstanceStatus = "FAILED"
	InstanceUnreferenced   InstanceStatus = "UNREFERENCED"
	InstanceInterrupted    InstanceStatus = "INTERRUPTED"
	InstanceNeedsAttention InstanceStatus = "NEEDS_ATTENTION"
	InstanceDestroying     InstanceStatus = "DESTROYING"
	InstanceDestroyed      InstanceStatus = "DESTROYED"
)

// Valid reports whether s is a known instance status.
func (s InstanceStatus) Valid() bool {
	switch s {
	case InstancePending, InstanceProvisioning, InstanceReady, InstanceFailed, InstanceUnreferenced,
		InstanceInterrupted, InstanceNeedsAttention, InstanceDestroying, InstanceDestroyed:
		return true
	}
	return false
}

// Instance is the write-ahead record of one Terraform-managed resource
// instance. Identity, state key, target and physical intent are immutable.
// TargetRef and PhysicalIntent hold stable names, tags and references only.
type Instance struct {
	ID               string
	Identity         Identity
	StateKey         string
	ActiveResourceID string
	ModuleBundleID   string
	TargetRef        map[string]any
	PhysicalIntent   map[string]any
	CreatedByRunID   string
	Status           InstanceStatus
	Version          int64
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Action is the kind of an attempt (schema `terraform_resource_attempts.action`).
type Action string

// Attempt actions.
const (
	ActionCreate  Action = "CREATE"
	ActionUpdate  Action = "UPDATE"
	ActionDestroy Action = "DESTROY"
)

// Valid reports whether a is a known attempt action.
func (a Action) Valid() bool { return a == ActionCreate || a == ActionUpdate || a == ActionDestroy }

// Ownership is the provenance classification of an attempt.
type Ownership string

// Ownership classifications. CREATED_BY_RUN needs evidence that the run
// created the instance; prior state or record is PREEXISTING or UPDATED_BY_RUN.
const (
	CreatedByRun Ownership = "CREATED_BY_RUN"
	Preexisting  Ownership = "PREEXISTING"
	UpdatedByRun Ownership = "UPDATED_BY_RUN"
)

// Valid reports whether o is a known classification.
func (o Ownership) Valid() bool { return o == CreatedByRun || o == Preexisting || o == UpdatedByRun }

// AttemptStatus is the safe status of an attempt.
type AttemptStatus string

// Attempt statuses. Only RUNNING attempts accept state writes.
const (
	AttemptRunning        AttemptStatus = "RUNNING"
	AttemptSucceeded      AttemptStatus = "SUCCEEDED"
	AttemptFailed         AttemptStatus = "FAILED"
	AttemptInterrupted    AttemptStatus = "INTERRUPTED"
	AttemptNeedsAttention AttemptStatus = "NEEDS_ATTENTION"
)

// Valid reports whether s is a known attempt status.
func (s AttemptStatus) Valid() bool {
	switch s {
	case AttemptRunning, AttemptSucceeded, AttemptFailed, AttemptInterrupted, AttemptNeedsAttention:
		return true
	}
	return false
}

var safeCode = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)

// ValidSafeCode reports whether s is an allowlisted-shape stage or error code.
func ValidSafeCode(s string) bool { return safeCode.MatchString(s) }

// Attempt is the write-ahead execution record of one action on one instance
// under one Environment operation. Action, ownership, run and the module and
// target snapshots are immutable once written.
type Attempt struct {
	ID                  string
	InstanceID          string
	OperationID         string
	DeploymentID        string
	RunID               string
	Number              int
	Action              Action
	Ownership           Ownership
	Stage               string
	Status              AttemptStatus
	ModuleSnapshot      map[string]any
	TargetSnapshot      map[string]any
	ObservedPhysicalIDs []string
	StateSerial         *int64
	SafeErrorCode       string
	RecoveryHandle      string
	Deadline            time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// ValidateNew checks a write-ahead attempt before it is persisted.
func (a Attempt) ValidateNew() error {
	if a.InstanceID == "" {
		return invalid("attempt needs an instance")
	}
	if !a.Action.Valid() || !a.Ownership.Valid() {
		return invalid("attempt action or ownership is unknown")
	}
	if strings.TrimSpace(a.RunID) == "" {
		return invalid("attempt needs a run id")
	}
	if !ValidSafeCode(a.Stage) {
		return invalid("attempt stage is not a safe code")
	}
	if a.ModuleSnapshot == nil || a.TargetSnapshot == nil {
		return invalid("attempt needs module and target snapshots")
	}
	if a.Deadline.IsZero() {
		return invalid("attempt needs a deadline")
	}
	return nil
}

// AttemptProgress is an owner-only update after one execution boundary.
type AttemptProgress struct {
	Stage               string
	Status              AttemptStatus
	ObservedPhysicalIDs []string
	SafeErrorCode       string
	RecoveryHandle      string
}

// Validate checks the safe fields of a progress update.
func (p AttemptProgress) Validate() error {
	if !ValidSafeCode(p.Stage) || !p.Status.Valid() {
		return invalid("attempt progress stage or status")
	}
	if p.SafeErrorCode != "" && !ValidSafeCode(p.SafeErrorCode) {
		return invalid("attempt error code is not a safe code")
	}
	return nil
}

// Classify returns the ownership a new attempt must carry, given the run's
// previous attempt on the instance (if any) and whether any prior record or
// state exists. A retry keeps the original classification; CREATED_BY_RUN is
// refused when anything preexisted the run.
func Classify(requested Ownership, previousInRun *Ownership, instanceCreatedByThisRun, priorStateBeforeRun bool) error {
	if previousInRun != nil {
		if *previousInRun != requested {
			return invalid("attempt ownership must not change on retry")
		}
		return nil
	}
	if requested == CreatedByRun && (!instanceCreatedByThisRun || priorStateBeforeRun) {
		return invalid("CREATED_BY_RUN needs evidence that the run created the instance")
	}
	return nil
}

// Dependency is one persisted consumer -> provider edge. Edges are retained
// as audit until absence proof; only a DESTROYED consumer stops holding.
type Dependency struct {
	ConsumerID     string
	ProviderID     string
	ConsumerStatus InstanceStatus
	Version        int64
}

// Holds reports whether the consumer still needs the provider. A failed,
// interrupted or unknown consumer keeps holding it.
func (d Dependency) Holds() bool { return d.ConsumerStatus != InstanceDestroyed }

// CapabilityAction limits what one executor invocation may do with state.
type CapabilityAction string

// Capability actions. PLAN may read and lock but never write state.
const (
	CapabilityPlan    CapabilityAction = "PLAN"
	CapabilityCreate  CapabilityAction = "CREATE"
	CapabilityUpdate  CapabilityAction = "UPDATE"
	CapabilityDestroy CapabilityAction = "DESTROY"
)

// Valid reports whether a is a known capability action.
func (a CapabilityAction) Valid() bool {
	switch a {
	case CapabilityPlan, CapabilityCreate, CapabilityUpdate, CapabilityDestroy:
		return true
	}
	return false
}

// MayWrite reports whether the action permits state writes.
func (a CapabilityAction) MayWrite() bool { return a.Valid() && a != CapabilityPlan }

// Capability is the persisted, hash-only record of a short-lived executor
// invocation token. The plaintext token is never stored.
type Capability struct {
	TokenHash      string
	OrganizationID string
	InstanceID     string
	OperationID    string
	Fence          int64
	Action         CapabilityAction
	ExpiresAt      time.Time
}

// Lock is a held Terraform state lock. Metadata is safe: operation name,
// Terraform version and creation time only.
type Lock struct {
	InstanceID  string
	LockID      string
	OperationID string
	Owner       string
	Fence       int64
	Metadata    LockMetadata
	AcquiredAt  time.Time
	HeartbeatAt time.Time
	Deadline    time.Time
}

// LockMetadata is the allowlisted subset of a Terraform lock request.
type LockMetadata struct {
	Operation string `json:"operation,omitempty"`
	Version   string `json:"version,omitempty"`
	Created   string `json:"created,omitempty"`
}

// StateBlob is the restricted encrypted state row. It is never a public read model.
type StateBlob struct {
	InstanceID     string
	OrganizationID string
	Ciphertext     []byte
	Nonce          []byte
	KeyID          string
	AADRevision    int64
	Lineage        string
	Serial         int64
	Version        int64
	UpdatedAt      time.Time
}

// StateAAD binds a ciphertext to its Organization, opaque state ID, key ID and
// revision so a blob cannot be replayed into another state or revision.
func StateAAD(organizationID, stateKey, keyID string, revision int64) []byte {
	return []byte("orchestrator-terraform-state|org=" + organizationID + "|state=" + stateKey + "|key=" + keyID + "|rev=" + strconv.FormatInt(revision, 10))
}
