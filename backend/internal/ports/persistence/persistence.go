// Package persistence defines the repository and UnitOfWork ports.
package persistence

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/configuration"
	"orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/identity"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/secretstore"
	"orchestrator/internal/domain/tfmodule"
)

// ErrNotFound is returned when an aggregate does not exist.
var ErrNotFound = errors.New("persistence: not found")

// ErrImmutable is returned when a write would change an immutable record or
// association.
var ErrImmutable = errors.New("persistence: immutable record")

// ErrVersionConflict is returned when an optimistic version check fails.
var ErrVersionConflict = errors.New("persistence: version conflict")

// ErrCommitUncertain is returned by Transact when the database may or may not
// have committed (for example a lost commit acknowledgement or a cancelled
// context during COMMIT). Every other Transact error means the transaction
// rolled back. Callers that wrote an external object for the transaction must
// keep it on ErrCommitUncertain and must not retry the mutation.
var ErrCommitUncertain = errors.New("persistence: commit outcome unknown")

// ErrDuplicate is returned by insert-only Create operations when the logical
// key already exists; the existing record is left unchanged.
var ErrDuplicate = errors.New("persistence: duplicate key")

// ErrEnvironmentBusy is returned when a write or claim meets an Environment
// owned by another active, interrupted or recovering operation (ADR-012).
var ErrEnvironmentBusy = errors.New("persistence: environment has an active operation")

// RecoveryWindow bounds one recovery attempt: recovery gets a fresh deadline so
// an expired original operation can still be compensated.
const RecoveryWindow = 30 * time.Minute

// ErrRecoveryUnconfirmed is returned when recovery lacks an explicit
// confirmation that the prior execution has stopped.
var ErrRecoveryUnconfirmed = errors.New("persistence: recovery needs confirmation that the prior execution stopped")

// ErrOperationLost is returned to an operation owner whose claim was taken
// over by recovery; the owner must stop executing.
var ErrOperationLost = errors.New("persistence: operation claim was lost")

// ErrStateLocksHeld refuses to release an Environment claim whose operation
// still holds a Terraform state lock: the state lock is released first.
var ErrStateLocksHeld = errors.New("persistence: operation still holds a Terraform state lock")

// BusyError is ErrEnvironmentBusy for one Environment, so delivery can show
// the owning operation without parsing text.
type BusyError struct{ ApplicationKey, EnvironmentKey string }

func (e *BusyError) Error() string {
	return "persistence: environment " + e.ApplicationKey + "/" + e.EnvironmentKey + " has an active operation"
}

// Is reports ErrEnvironmentBusy.
func (e *BusyError) Is(target error) bool { return target == ErrEnvironmentBusy }

// Busy returns the busy error of one Environment.
func Busy(applicationKey, environmentKey string) error {
	return &BusyError{ApplicationKey: applicationKey, EnvironmentKey: environmentKey}
}

type ownerKey struct{}

// Owner is the fenced identity of an Environment claim holder: the operation,
// the process-level owner string and the fence incremented by each recovery.
type Owner struct {
	OperationID string
	Owner       string
	Fence       int64
}

// WithOwner marks ctx as carrying the claim owner. Owner-only repository
// writes succeed only for the current (Owner, Fence) of the operation, so a
// paused former owner cannot mutate state after recovery took the claim.
func WithOwner(ctx context.Context, owner Owner) context.Context {
	return context.WithValue(ctx, ownerKey{}, owner)
}

// OwnerFrom returns the claim owner carried by ctx.
func OwnerFrom(ctx context.Context) (Owner, bool) {
	owner, ok := ctx.Value(ownerKey{}).(Owner)
	return owner, ok
}

type holdKey struct{}

// Hold is the shared flag of one claim: an executor that reaches an unknown
// outcome (killed process, failed state push, unconfirmed stop) marks it, and
// the claim holder must then suspend the Environment instead of releasing it,
// whatever status the caller would otherwise record.
type Hold struct{ marked atomic.Bool }

// WithHold returns ctx carrying a fresh Hold flag.
func WithHold(ctx context.Context) context.Context {
	return context.WithValue(ctx, holdKey{}, &Hold{})
}

// MarkHold flags the claim in ctx as held. It is a no-op without a Hold.
func MarkHold(ctx context.Context) {
	if h, ok := ctx.Value(holdKey{}).(*Hold); ok {
		h.marked.Store(true)
	}
}

// Held reports whether an executor marked the claim in ctx as held.
func Held(ctx context.Context) bool {
	h, ok := ctx.Value(holdKey{}).(*Hold)
	return ok && h.marked.Load()
}

// OperationFrom returns the operation ID carried by ctx, if any.
func OperationFrom(ctx context.Context) string {
	owner, _ := OwnerFrom(ctx)
	return owner.OperationID
}

// SecretStoreSelection is the versioned write of the Environment secret store.
type SecretStoreSelection struct {
	ApplicationKey, EnvironmentKey string
	StoreKey                       string
	ExpectedVersion                int64
}

// OperationClaim asks to own an Environment after verifying its pins.
type OperationClaim struct {
	ID, ApplicationKey, EnvironmentKey, Owner string
	ReviewRequestID                           string
	Kind                                      environment.OperationKind
	Pins                                      environment.OperationPins
	Detail                                    map[string]any
	Deadline                                  time.Time
}

// ApplicationRepository owns Organization, Application and Connection records.
type ApplicationRepository interface {
	GetOrganization(ctx context.Context, key string) (application.Organization, error)
	SaveOrganization(ctx context.Context, org application.Organization) error
	ListApplications(ctx context.Context) ([]application.Application, error)
	GetApplication(ctx context.Context, key string) (application.Application, error)
	SaveApplication(ctx context.Context, app application.Application) error
	GetConnection(ctx context.Context, organizationKey, key string) (application.Connection, error)
	ListConnections(ctx context.Context, organizationKey string) ([]application.Connection, error)
	SaveConnection(ctx context.Context, conn application.Connection) error
	// CreateConnection is insert-only registration (UC-04): ErrDuplicate for
	// an existing (Organization, key), ErrNotFound for a missing Organization.
	// SaveConnection remains the seed/test upsert.
	CreateConnection(ctx context.Context, conn application.Connection) error
	// UpdateConnection is the V01 account compare-and-set. It replaces name,
	// config, secret reference, status and verification of an existing
	// Connection of the Organization if and only if its stored version equals
	// expectedVersion, increments the version and returns the stored record.
	// ErrNotFound for a missing or foreign key, ErrVersionConflict when stale.
	// Identity, kind and authentication type never change.
	UpdateConnection(ctx context.Context, organizationKey, key string, expectedVersion int64, next application.Connection) (application.Connection, error)
}

type IdentityRepository interface {
	GetUserAccountByUsername(ctx context.Context, username string) (identity.UserAccount, error)
	GetUserAccount(ctx context.Context, id string) (identity.UserAccount, error)
	SaveUserAccount(ctx context.Context, account identity.UserAccount) error
	SaveSession(ctx context.Context, session identity.Session) error
	GetSessionByTokenHash(ctx context.Context, tokenHash string) (identity.Session, error)
}

// EnvironmentRepository owns Environment records and immutable Deployment Sets.
type EnvironmentRepository interface {
	ListEnvironments(ctx context.Context, applicationKey string) ([]environment.Environment, error)
	GetEnvironment(ctx context.Context, applicationKey, environmentKey string) (environment.Environment, error)
	// SaveEnvironment is the seed/creation upsert. It never writes the
	// store selection, generation or claim of an existing Environment;
	// dedicated commands own them.
	SaveEnvironment(ctx context.Context, env environment.Environment) error
	// SelectSecretStore replaces the secret-store selection with one versioned
	// write; ErrVersionConflict when ExpectedVersion is stale, ErrEnvironmentBusy when another operation owns the Environment.
	SelectSecretStore(ctx context.Context, s SecretStoreSelection) (environment.Environment, error)
	// SetPublicRoutesPending changes only the route flag, without a version bump.
	SetPublicRoutesPending(ctx context.Context, applicationKey, environmentKey string, pending bool) error
	GetDeploymentSet(ctx context.Context, id string) (environment.DeploymentSet, error)
	SaveDeploymentSet(ctx context.Context, set environment.DeploymentSet) error
	// CompareVersionAndSetCurrent performs the optimistic final commit of UC-06 MS-12.
	CompareVersionAndSetCurrent(ctx context.Context, applicationKey, environmentKey string, expectedVersion int64, setID string) error
}

// SecretStoreRepository owns Organization-scoped workload Secret Store records.
type SecretStoreRepository interface {
	// CreateSecretStore is insert-only: ErrDuplicate for an existing
	// (Organization, key), ErrNotFound for a missing Organization.
	CreateSecretStore(ctx context.Context, store secretstore.Store) error
	GetSecretStore(ctx context.Context, organizationKey, key string) (secretstore.Store, error)
	ListSecretStores(ctx context.Context, organizationKey string) ([]secretstore.Store, error)
	// AdmitManagedSecretStore is the only non-insert write of a store record. It
	// atomically creates the record (ExpectedID empty) or replaces the credential
	// reference of a managed record, after checking the prior state. See ManagedStoreAdmission.
	AdmitManagedSecretStore(ctx context.Context, admission ManagedStoreAdmission) (secretstore.Store, error)
}

// ManagedStoreAdmission is the compare-and-set admission of the explicitly
// configured Compose bootstrap store. Store is the complete READY
// record to publish, with its already persisted CredentialRef. The Expected*
// fields describe the prior state the caller verified against:
//
//   - ExpectedID empty: no record may exist (ErrDuplicate otherwise);
//   - otherwise the record must still have that ID and ExpectedCredentialRef
//     (ErrVersionConflict otherwise), be Compose-managed in both prior and
//     published form (ErrImmutable for an unmarked user registration) and have
//     the same endpoint identity as Store (ErrImmutable otherwise).
//
// Admission keeps the record ID, key, creation time and endpoints, so every
// Environment selection, revision entry and bundle reference is unchanged.
type ManagedStoreAdmission struct {
	Store                 secretstore.Store
	ExpectedID            string
	ExpectedCredentialRef string
}

// OperationRepository owns the persisted Environment operation claims and
// transition records (ADR-012).
type OperationRepository interface {
	// ClaimEnvironment verifies pins and claims the Environment in one local
	// transaction: ErrEnvironmentBusy when claimed, ErrVersionConflict when a
	// pinned version or identity differs.
	ClaimEnvironment(ctx context.Context, claim OperationClaim) (environment.Operation, error)
	GetOperation(ctx context.Context, id string) (environment.Operation, error)
	GetReviewOperation(ctx context.Context, applicationKey, environmentKey, requestID string) (environment.Operation, error)
	// ActiveOperation returns the operation holding the Environment, if any.
	ActiveOperation(ctx context.Context, applicationKey, environmentKey string) (environment.Operation, bool, error)
	ListOperations(ctx context.Context, applicationKey, environmentKey string, limit int) ([]environment.Operation, error)
	// HeartbeatOperation extends a live claim of owner; ErrOperationLost when
	// the claim no longer belongs to the fenced owner.
	HeartbeatOperation(ctx context.Context, owner Owner, stage string, detail map[string]any) error
	// ReleaseOperation moves a claim to a terminal status and frees the
	// Environment atomically.
	ReleaseOperation(ctx context.Context, owner Owner, status environment.OperationStatus, failure string) error
	// MarkInterrupted flags ACTIVE claims whose heartbeat is older than
	// staleBefore as INTERRUPTED. The claim stays held; nothing is released.
	MarkInterrupted(ctx context.Context, staleBefore time.Time) (int, error)
	// SuspendOperation returns a held claim to INTERRUPTED with a visible failure,
	// without releasing the Environment. The current fenced owner uses it when a
	// compensation or recovery step failed, so the claim keeps demanding operator
	// action instead of being freed under a false safe outcome.
	SuspendOperation(ctx context.Context, owner Owner, failure string) error
	// BeginRecovery moves an INTERRUPTED claim to RECOVERING for newOwner and
	// increments its fence. confirmedBy records who confirmed that the prior
	// execution is stopped; an empty value is rejected. Stale heartbeat alone
	// never authorizes recovery.
	BeginRecovery(ctx context.Context, id, newOwner, confirmedBy string) (environment.Operation, error)
}

// CatalogRepository owns Resource Types and Resource Definitions.
type CatalogRepository interface {
	ListResourceTypes(ctx context.Context, organizationKey string) ([]resource.Type, error)
	ListResourceDefinitions(ctx context.Context, organizationKey string) ([]resource.Definition, error)
	SaveResourceType(ctx context.Context, organizationKey string, t resource.Type) error
	SaveResourceDefinition(ctx context.Context, organizationKey string, d resource.Definition) error
	// CreateResourceType and CreateResourceDefinition are insert-only
	// registration (UC-02/03): ErrDuplicate for an existing (Organization,
	// key) without touching it, ErrNotFound for a missing Organization or
	// Resource Type. Save* remain the seed/test upserts.
	CreateResourceType(ctx context.Context, organizationKey string, t resource.Type) error
	CreateResourceDefinition(ctx context.Context, organizationKey string, d resource.Definition) error
	// UpdateResourceDefinition is the UC-03 V02-06 compare-and-set. It
	// replaces the editable content and matching criteria of the Definition
	// named by d.Key when the stored version equals expected, increments the
	// version exactly once and returns the stored record. The key and the
	// Resource Type are immutable. ErrNotFound: missing Definition, Resource
	// Type change or missing Driver Account; ErrVersionConflict: stale
	// expected version, with the stored winner untouched.
	UpdateResourceDefinition(ctx context.Context, organizationKey string, expected int64, d resource.Definition) (resource.Definition, error)
}

// ModuleBundleRepository owns the verified Terraform module registry
// (terraform_module_bundles). Bundles are insert-only and immutable; one
// revision path (source, module, revision, subdirectory) has at most one
// verified digest.
type ModuleBundleRepository interface {
	// InsertModuleBundle inserts b unless a bundle with the same revision
	// path already exists, and returns the bundle stored for that path: b
	// itself, or the existing (possibly concurrently committed) bundle, which
	// callers compare to detect drift. It never updates a stored bundle.
	InsertModuleBundle(ctx context.Context, b tfmodule.Bundle) (tfmodule.Bundle, error)
	// GetModuleBundle returns the bundle of ref's revision path (the digest
	// of ref is not part of the lookup), or ErrNotFound.
	GetModuleBundle(ctx context.Context, ref tfmodule.SourceRef) (tfmodule.Bundle, error)
	// GetModuleBundleByID returns the bundle with this ID (the historical
	// module of a Terraform instance), or ErrNotFound.
	GetModuleBundleByID(ctx context.Context, id string) (tfmodule.Bundle, error)
	ListModuleBundles(ctx context.Context) ([]tfmodule.Bundle, error)
}

// DeploymentRepository owns Deployment records, plan snapshots and node progress.
type DeploymentRepository interface {
	SaveDeployment(ctx context.Context, d deployment.Deployment) error
	GetDeployment(ctx context.Context, id string) (deployment.Deployment, error)
	ListDeployments(ctx context.Context, applicationKey, environmentKey string) ([]deployment.Deployment, error)
	SavePlan(ctx context.Context, deploymentID string, plan map[string]any) error
	GetPlan(ctx context.Context, deploymentID string) (map[string]any, error)
	SaveDeploymentResource(ctx context.Context, r deployment.Resource) error
	ListDeploymentResources(ctx context.Context, deploymentID string) ([]deployment.Resource, error)
	// ListDeploymentAttempts is the Console's read-only, safe projection of the
	// Terraform attempts linked to one Deployment (ADR-014). It reads only the
	// safe attempt columns and the public module pin of the instance, scoped to
	// the Deployment's own Environment; it never returns snapshots, state,
	// recovery handles or target references.
	ListDeploymentAttempts(ctx context.Context, deploymentID string) ([]deployment.ResourceAttempt, error)
}

// DeltaSnapshotRepository owns immutable Deployment Delta Snapshots. A
// Snapshot is written once for exactly one Deployment. Its Application is
// derived through Deployment -> Environment -> Application.
type DeltaSnapshotRepository interface {
	SaveDeltaSnapshot(ctx context.Context, snapshot deployment.DeploymentDeltaSnapshot) error
	GetDeltaSnapshot(ctx context.Context, id string) (deployment.DeploymentDeltaSnapshot, error)
}

// ActiveResourceRepository owns Active Resource logical identity and state.
type ActiveResourceRepository interface {
	FindByLogicalIdentity(ctx context.Context, organizationKey string, descriptor resource.Descriptor, scope resource.Scope) (resource.ActiveResource, error)
	ListActiveResources(ctx context.Context, organizationKey string) ([]resource.ActiveResource, error)
	UpsertActiveResource(ctx context.Context, a resource.ActiveResource) (resource.ActiveResource, error)
}

// WorkloadInstanceRepository owns applied workload state per Environment.
type WorkloadInstanceRepository interface {
	// UpsertWorkloadProgress atomically updates the current Environment state
	// and the snapshot owned by w.LastDeploymentID while that Deployment runs.
	UpsertWorkloadProgress(ctx context.Context, w deployment.WorkloadInstance) error
	UpsertWorkloadInstance(ctx context.Context, w deployment.WorkloadInstance) error
	// ListWorkloadInstances returns the instances of the Environment's current
	// target generation; ListWorkloadInstancesFor returns one chosen generation.
	ListWorkloadInstances(ctx context.Context, environmentKey string) ([]deployment.WorkloadInstance, error)
	ListWorkloadInstancesFor(ctx context.Context, environmentKey string, generation int64) ([]deployment.WorkloadInstance, error)
	ListDeploymentWorkloads(ctx context.Context, deploymentID string) ([]deployment.WorkloadSnapshot, error)
}

// ConfigurationRepository owns immutable UC-12 revision metadata, not values.
type ConfigurationRepository interface {
	GetConfigurationScope(ctx context.Context, applicationKey, environmentKey string) (configuration.Scope, error)
	GetConfigurationRevision(ctx context.Context, id string) (configuration.Revision, error)
	CommitConfigurationRevision(ctx context.Context, expectedVersion int64, revision configuration.Revision) error
	// CommitDesiredRename atomically commits one configuration revision and the
	// desired workload drafts that reference its renamed key (T30).
	CommitDesiredRename(ctx context.Context, command DesiredRename) error
}

// DesiredRename is one atomic UC-12 rename with reference rewrite. Every
// Expected* field is checked inside the same transaction as the writes.
// Revision.Version must be ExpectedConfigVersion+1 and Drafts hold only
// PENDING_UPSERT drafts with a Score. The command never changes the
// Environment version, current Deployment Set, applied revisions or pending
// deletes; it advances the draft version once when Drafts is not empty.
// Failures are ErrEnvironmentBusy, ErrVersionConflict or ErrNotFound and leave
// all metadata unchanged.
type DesiredRename struct {
	ApplicationKey, EnvironmentKey string
	ExpectedConfigVersion          int64
	ExpectedEnvironmentVersion     int64
	ExpectedDraftVersion           int64
	ExpectedCurrentSetID           string
	Revision                       configuration.Revision
	Drafts                         []environment.WorkloadDraft
}

// WorkloadDraftRepository owns UC-16 pending desired Score documents.
type WorkloadDraftRepository interface {
	ListWorkloadDrafts(ctx context.Context, applicationKey, environmentKey string) ([]environment.WorkloadDraft, error)
	GetWorkloadDraft(ctx context.Context, applicationKey, environmentKey, workloadID string) (environment.WorkloadDraft, error)
	SaveWorkloadDraft(ctx context.Context, expectedDraftVersion int64, draft environment.WorkloadDraft) error
	DeleteWorkloadDraft(ctx context.Context, applicationKey, environmentKey, workloadID string, expectedDraftVersion int64) error
}

// UnitOfWork runs a function inside one store transaction.
type UnitOfWork interface {
	Transact(ctx context.Context, fn func(ctx context.Context) error) error
}

// ReadSnapshot runs read-only queries against one consistent view of state.
// The view passed to fn must only be read: writes through it are rejected or
// discarded and never persisted. Nested inside Transact, fn reads the current
// transaction.
type ReadSnapshot interface {
	ReadSnapshot(ctx context.Context, fn func(ctx context.Context, view Store) error) error
}

// Store aggregates every repository port behind one adapter.
type Store interface {
	ApplicationRepository
	IdentityRepository
	EnvironmentRepository
	CatalogRepository
	ModuleBundleRepository
	DeploymentRepository
	DeltaSnapshotRepository
	ActiveResourceRepository
	WorkloadInstanceRepository
	ConfigurationRepository
	WorkloadDraftRepository
	SecretStoreRepository
	OperationRepository
	UnitOfWork
	ReadSnapshot
}
