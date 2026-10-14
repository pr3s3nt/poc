// Package terraformstate defines the ports of the Terraform v2 durable state
// backend (OC-18): the restricted PostgreSQL repository of instances, attempts,
// dependency edges, executor capabilities, state locks and encrypted state
// blobs, and the state cipher whose key lives outside the database. Only the
// PostgreSQL adapter implements Repository; no JSON or memory store may stand
// in for durable state.
package terraformstate

import (
	"context"
	"errors"
	"time"

	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/ports/persistence"
)

// Errors. Messages never carry state bytes, tokens or lock diagnostics.
var (
	// ErrUnauthorized means the presented capability is unknown.
	ErrUnauthorized = errors.New("terraformstate: capability is not valid")
	// ErrForbidden means the capability exists but does not authorize this
	// request: wrong state or scope, expired, stale fence, operation no
	// longer current or an action that may not write.
	ErrForbidden = errors.New("terraformstate: capability does not authorize this request")
	// ErrLockConflict means another owner, fence or lock ID holds the state lock.
	ErrLockConflict = errors.New("terraformstate: state is locked by another owner")
	// ErrLockMismatch means a write or unlock did not present the held lock.
	ErrLockMismatch = errors.New("terraformstate: request does not hold the state lock")
	// ErrStateConflict rejects a lineage change, serial regression or
	// different bytes at the current serial.
	ErrStateConflict = errors.New("terraformstate: state lineage or serial conflict")
	// ErrInvalidState rejects a body that is not a Terraform state document.
	ErrInvalidState = errors.New("terraformstate: request body is not a Terraform state")
	// ErrUndecryptable means stored ciphertext cannot be opened with the
	// configured key. It is never treated as an empty state.
	ErrUndecryptable = errors.New("terraformstate: stored state cannot be decrypted with the configured key")
	// ErrNoAttempt means a state write has no running write-ahead attempt.
	ErrNoAttempt = errors.New("terraformstate: no running attempt for this state write")
	// ErrCleanupSelectionChanged means the deletion request was already
	// admitted with a different selected set (409 STALE_PREVIEW).
	ErrCleanupSelectionChanged = errors.New("terraformstate: deletion request was admitted with a different selection")
	// ErrOwnerRequired means an owner-only write was made without a claim owner.
	ErrOwnerRequired = errors.New("terraformstate: write needs the current Environment claim owner")
)

// Invocation is a verified capability bound to the current claim owner. It is
// produced inside the transaction that holds a share lock on the operation.
type Invocation struct {
	InstanceID     string
	OrganizationID string
	StateKey       string
	OperationID    string
	Owner          string
	Fence          int64
	Action         tfresource.CapabilityAction
	ExpiresAt      time.Time
}

// NewInstance is the write-ahead registration of an instance in the
// Environment owned by the claim owner in ctx. Organization and Application
// are taken from that Environment, never from the caller.
type NewInstance struct {
	TargetGeneration int64
	Descriptor       string
	ScopeType        string
	ScopeID          string
	ActiveResourceID string
	ModuleBundleID   string
	TargetRef        map[string]any
	PhysicalIntent   map[string]any
	CreatedByRunID   string
}

// Repository is the restricted persistence of Terraform state and resource
// instances. Owner-only methods require persistence.WithOwner in ctx and
// verify the current (owner, fence) and the Environment's active claim in the
// same transaction. Methods ending in ForUpdate must run inside Transact.
type Repository interface {
	persistence.UnitOfWork

	// RegisterInstance is idempotent on the identity tuple: an existing
	// instance is returned unchanged.
	RegisterInstance(ctx context.Context, in NewInstance) (tfresource.Instance, error)
	// FindInstance returns the instance of the claim owner's Environment with
	// this identity, or found=false. It never creates one (owner-only).
	FindInstance(ctx context.Context, generation int64, descriptor, scopeType, scopeID string) (inst tfresource.Instance, found bool, err error)
	GetInstance(ctx context.Context, id string) (tfresource.Instance, error)
	ListInstances(ctx context.Context, environmentID string) ([]tfresource.Instance, error)
	// SetInstanceStatus is an owner-only CAS on the instance version.
	SetInstanceStatus(ctx context.Context, id string, expectedVersion int64, status tfresource.InstanceStatus) (tfresource.Instance, error)

	// RecordDependencies inserts consumer -> provider edges with the resource
	// intent, before execution (owner-only). It is a CAS on the consumer
	// version; when an edge is added the consumer version advances and the
	// new edge carries that version. Existing edges are kept.
	RecordDependencies(ctx context.Context, consumerID string, expectedConsumerVersion int64, providerIDs []string) (tfresource.Instance, error)
	// RetireDependencies removes edges once the consumer no longer holds the
	// provider: it is READY after an applied update without them, or
	// DESTROYED with absence proven. A failed, interrupted or unknown consumer
	// keeps its edges. CAS on and advance of the consumer version (owner-only).
	RetireDependencies(ctx context.Context, consumerID string, expectedConsumerVersion int64, providerIDs []string) (tfresource.Instance, error)
	// Dependencies returns every edge in which the instance takes part.
	Dependencies(ctx context.Context, instanceID string) ([]tfresource.Dependency, error)

	// BeginAttempt writes an attempt before side effects (owner-only). The
	// attempt number is assigned and ownership is classified atomically.
	BeginAttempt(ctx context.Context, a tfresource.Attempt) (tfresource.Attempt, error)
	UpdateAttempt(ctx context.Context, id string, p tfresource.AttemptProgress) (tfresource.Attempt, error)
	ListAttempts(ctx context.Context, instanceID string) ([]tfresource.Attempt, error)

	// InsertCapability stores a hash-only capability for the owner's
	// operation and current fence (owner-only).
	InsertCapability(ctx context.Context, c tfresource.Capability) error
	// AuthorizeForUpdate resolves a capability by hash, share-locks its
	// operation row and verifies scope, fence, status and current claim.
	AuthorizeForUpdate(ctx context.Context, tokenHash, stateKey string, now time.Time) (Invocation, error)

	LockForUpdate(ctx context.Context, instanceID string) (*tfresource.Lock, error)
	// InsertLock claims the state lock; false when a lock row already exists.
	InsertLock(ctx context.Context, l tfresource.Lock) (bool, error)
	DeleteLock(ctx context.Context, instanceID, lockID, operationID string, fence int64) error
	StateForUpdate(ctx context.Context, instanceID string) (*tfresource.StateBlob, error)
	// PutState inserts (expectedVersion 0) or replaces the blob at
	// expectedVersion; ErrVersionConflict otherwise.
	PutState(ctx context.Context, b tfresource.StateBlob, expectedVersion int64) error
	// RecordStateSerial records the committed serial on the running attempt
	// of the operation; ErrNoAttempt when there is none.
	RecordStateSerial(ctx context.Context, operationID, instanceID string, serial int64) error
	Lock(ctx context.Context, instanceID string) (*tfresource.Lock, error)
	// ReleaseRecoveredLock removes a lock left by an earlier fence of the
	// owner's own operation; only a RECOVERING owner may do so.
	ReleaseRecoveredLock(ctx context.Context, instanceID string) error
}

// AdmissionCheck runs inside the claim transaction of a resource cleanup,
// after the Environment row is locked and before the claim is written. It
// reads through ctx, which carries that transaction, so every role-independent
// condition it checks (token, versions, UNREFERENCED, ownership, consumers,
// busy, selected set) is atomic with the claim. A non-nil error aborts the
// whole admission with nothing written.
type AdmissionCheck func(ctx context.Context) error

// CleanupRepository persists the admitted authorization of a resource
// cleanup together with its Environment claim. It stores exactly the selected
// set; it never adds dependencies. The repository itself checks the selection
// shape, Environment membership and pinned item versions/refs; everything
// that depends on the session role, the preview token, UNREFERENCED status,
// consumers and ownership is the AdmissionCheck of the I04-08 admission
// service, which runs in the same transaction as the claim.
type CleanupRepository interface {
	// ClaimResourceCleanup claims the Environment with kind RESOURCE_CLEANUP
	// and the confirmed scope in one transaction. check, when non-nil, runs in
	// that transaction before anything is written. Every selected ID must be an
	// instance of the claimed Environment whose pinned version, state key and
	// module match. A replay of the same request hash by the same actor
	// returns the admitted operation (replayed=true) when the selection is
	// identical and ErrCleanupSelectionChanged otherwise; a replay performs no
	// check and writes nothing.
	ClaimResourceCleanup(ctx context.Context, claim persistence.OperationClaim, req tfresource.CleanupRequest, check AdmissionCheck) (op environment.Operation, replayed bool, err error)
	CleanupRequest(ctx context.Context, operationID string) (tfresource.CleanupRequest, error)
}

// ResourceReader is the Console's safe, read-only view of Terraform resource
// instances, their dependency edges and cleanup operations (UC-09 RES-01..03).
// It is served by the Console login: it can reach only the safe instance and
// attempt columns, so it never exposes target references, physical intent,
// snapshots, state, locks or recovery handles. Every method is scoped by the
// Application and Environment keys and returns ErrNotFound otherwise.
type ResourceReader interface {
	ListResourceRecords(ctx context.Context, applicationKey, environmentKey string) ([]tfresource.ResourceRecord, error)
	// ListResourceEdges returns every consumer -> provider edge of the Environment.
	ListResourceEdges(ctx context.Context, applicationKey, environmentKey string) ([]tfresource.Dependency, error)
	GetCleanupRecord(ctx context.Context, applicationKey, environmentKey, operationID string) (tfresource.CleanupRecord, error)
	// ListCleanupRecords returns the Environment's cleanup operations, newest first.
	ListCleanupRecords(ctx context.Context, applicationKey, environmentKey string, limit int) ([]tfresource.CleanupRecord, error)
}

// Cipher seals and opens state with a key held outside the database. Each
// Seal draws a fresh nonce. Open fails with ErrUndecryptable for an unknown
// key ID, wrong key or altered bytes/AAD.
type Cipher interface {
	KeyID() string
	Seal(plaintext, aad []byte) (nonce, ciphertext []byte, err error)
	Open(keyID string, nonce, ciphertext, aad []byte) ([]byte, error)
}
