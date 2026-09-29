// Package persistence defines the repository and UnitOfWork ports.
package persistence

import (
	"context"
	"errors"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/configuration"
	"orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/identity"
	"orchestrator/internal/domain/resource"
)

// ErrNotFound is returned when an aggregate does not exist.
var ErrNotFound = errors.New("persistence: not found")

// ErrImmutable is returned when a write would change an immutable record or
// association.
var ErrImmutable = errors.New("persistence: immutable record")

// ErrVersionConflict is returned when an optimistic version check fails.
var ErrVersionConflict = errors.New("persistence: version conflict")

// ApplicationRepository owns Organization, Application and Connection records.
type ApplicationRepository interface {
	GetOrganization(ctx context.Context, key string) (application.Organization, error)
	SaveOrganization(ctx context.Context, org application.Organization) error
	ListApplications(ctx context.Context) ([]application.Application, error)
	GetApplication(ctx context.Context, key string) (application.Application, error)
	SaveApplication(ctx context.Context, app application.Application) error
	GetConnection(ctx context.Context, key string) (application.Connection, error)
	ListConnections(ctx context.Context) ([]application.Connection, error)
	SaveConnection(ctx context.Context, conn application.Connection) error
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
	SaveEnvironment(ctx context.Context, env environment.Environment) error
	GetDeploymentSet(ctx context.Context, id string) (environment.DeploymentSet, error)
	SaveDeploymentSet(ctx context.Context, set environment.DeploymentSet) error
	// CompareVersionAndSetCurrent performs the optimistic final commit of UC-06 MS-12.
	CompareVersionAndSetCurrent(ctx context.Context, applicationKey, environmentKey string, expectedVersion int64, setID string) error
}

// CatalogRepository owns Resource Types and Resource Definitions.
type CatalogRepository interface {
	ListResourceTypes(ctx context.Context, organizationKey string) ([]resource.Type, error)
	ListResourceDefinitions(ctx context.Context, organizationKey string) ([]resource.Definition, error)
	SaveResourceType(ctx context.Context, organizationKey string, t resource.Type) error
	SaveResourceDefinition(ctx context.Context, organizationKey string, d resource.Definition) error
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
// Snapshot is written once and referenced by exactly one Deployment through
// Deployment.DeltaSnapshotID (deployments.delta_snapshot_id UNIQUE).
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
	UpsertWorkloadInstance(ctx context.Context, w deployment.WorkloadInstance) error
	ListWorkloadInstances(ctx context.Context, environmentKey string) ([]deployment.WorkloadInstance, error)
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
	UnitOfWork
}
