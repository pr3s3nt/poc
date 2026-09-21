// Package persistence defines the repository and UnitOfWork ports.
package persistence

import (
	"context"
	"errors"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
)

// ErrNotFound is returned when an aggregate does not exist.
var ErrNotFound = errors.New("persistence: not found")

// ErrVersionConflict is returned when an optimistic version check fails.
var ErrVersionConflict = errors.New("persistence: version conflict")

// ApplicationRepository owns Organization, Application and Connection records.
type ApplicationRepository interface {
	ListApplications(ctx context.Context) ([]application.Application, error)
	GetApplication(ctx context.Context, key string) (application.Application, error)
	SaveApplication(ctx context.Context, app application.Application) error
	GetConnection(ctx context.Context, key string) (application.Connection, error)
	ListConnections(ctx context.Context) ([]application.Connection, error)
	SaveConnection(ctx context.Context, conn application.Connection) error
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
	ListResourceTypes(ctx context.Context) ([]resource.Type, error)
	ListResourceDefinitions(ctx context.Context) ([]resource.Definition, error)
	SaveResourceType(ctx context.Context, t resource.Type) error
	SaveResourceDefinition(ctx context.Context, d resource.Definition) error
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

// UnitOfWork runs a function inside one store transaction.
type UnitOfWork interface {
	Transact(ctx context.Context, fn func(ctx context.Context) error) error
}

// Store aggregates every repository port behind one adapter.
type Store interface {
	ApplicationRepository
	EnvironmentRepository
	CatalogRepository
	DeploymentRepository
	ActiveResourceRepository
	WorkloadInstanceRepository
	UnitOfWork
}
