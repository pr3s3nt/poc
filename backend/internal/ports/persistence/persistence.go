// Package persistence defines the repository and UnitOfWork ports.
package persistence

import (
	"context"
	"errors"
	"time"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/configuration"
	"orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/identity"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/secretstore"
)

// ErrNotFound is returned when an aggregate does not exist.
var ErrNotFound = errors.New("persistence: not found")

// ErrImmutable is returned when a write would change an immutable record or
// association.
var ErrImmutable = errors.New("persistence: immutable record")

// ErrVersionConflict is returned when an optimistic version check fails.
var ErrVersionConflict = errors.New("persistence: version conflict")

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

// OperationFrom returns the operation ID carried by ctx, if any.
func OperationFrom(ctx context.Context) string {
	owner, _ := OwnerFrom(ctx)
	return owner.OperationID
}

// EnvironmentBinding is the versioned write of an Environment execution
// target. A write succeeds only at ExpectedVersion, advances the version and,
// when the Environment is claimed, only for the owning operation in ctx.
type EnvironmentBinding struct {
	ApplicationKey, EnvironmentKey string
	ConnectionKey                  string
	Profile                        application.ExecutionProfile
	Region                         string
	RuntimeStatus                  application.RuntimeStatus
	Scope                          environment.InfrastructureScope
	// Generation is the target generation written with the binding.
	Generation      int64
	ExpectedVersion int64
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
	// execution binding, store selection, generation or claim of an existing
	// Environment; dedicated commands own them.
	SaveEnvironment(ctx context.Context, env environment.Environment) error
	// BindEnvironment replaces the execution binding with one versioned write.
	// ErrVersionConflict when ExpectedVersion is stale, ErrEnvironmentBusy when
	// another operation owns the Environment, ErrNotFound for a missing
	// Environment or foreign/missing Connection.
	BindEnvironment(ctx context.Context, b EnvironmentBinding) (environment.Environment, error)
	// SelectSecretStore replaces the secret-store selection with one versioned
	// write; same errors as BindEnvironment.
	SelectSecretStore(ctx context.Context, s SecretStoreSelection) (environment.Environment, error)
	// UpdateRuntimeStatus advances PENDING/READY of a configured Environment.
	UpdateRuntimeStatus(ctx context.Context, applicationKey, environmentKey string, status application.RuntimeStatus) error
	// SetPublicRoutesPending changes only the route flag, without a version bump.
	SetPublicRoutesPending(ctx context.Context, applicationKey, environmentKey string, pending bool) error
	// AllocateGeneration reserves the next target generation of an Environment
	// for the owning operation and returns it.
	AllocateGeneration(ctx context.Context, applicationKey, environmentKey string) (int64, error)
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
	// BackfillLegacySecretStore stamps the explicit legacy store identity on
	// pre-ADR-012 configuration entries that carry a value ref but no store key
	// and selects it on Environments that already have configuration. Idempotent;
	// refs and revision identities are unchanged and no Environment version moves.
	BackfillLegacySecretStore(ctx context.Context, organizationKey, storeKey string) error
}

// OperationRepository owns the persisted Environment operation claims and
// transition records (ADR-012).
type OperationRepository interface {
	// ClaimEnvironment verifies pins and claims the Environment in one local
	// transaction: ErrEnvironmentBusy when claimed, ErrVersionConflict when a
	// pinned version or identity differs.
	ClaimEnvironment(ctx context.Context, claim OperationClaim) (environment.Operation, error)
	GetOperation(ctx context.Context, id string) (environment.Operation, error)
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
	SaveTransition(ctx context.Context, t environment.Transition) error
	GetTransition(ctx context.Context, id string) (environment.Transition, error)
	ListTransitions(ctx context.Context, applicationKey, environmentKey string) ([]environment.Transition, error)
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
