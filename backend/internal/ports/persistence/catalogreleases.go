package persistence

import (
	"context"
	"orchestrator/internal/domain/catalogrelease"
	"time"
)

type CatalogSource struct {
	Source       catalogrelease.Source `json:"source"`
	ObservedTags map[string]string     `json:"observedTags"`
	LastChecked  time.Time             `json:"lastChecked,omitempty"`
	Code         string                `json:"code,omitempty"`
}
type CatalogReleaseRepository interface {
	ListCatalogSources(context.Context, string) ([]CatalogSource, error)
	GetCatalogSource(context.Context, string, string) (CatalogSource, error)
	SaveCatalogSource(context.Context, int64, CatalogSource) error
	ListCatalogReleases(context.Context, string, string) ([]catalogrelease.Release, error)
	GetCatalogRelease(context.Context, string, string, string) (catalogrelease.Release, error)
	InsertCatalogRelease(context.Context, catalogrelease.Release) error
}

// CatalogDiscoveryOrganizations is internal worker enumeration, never a public query.
type CatalogDiscoveryOrganizations interface {
	ListCatalogSourceOrganizations(context.Context) ([]string, error)
}

// CatalogEnvironment records desired/applied sets without changing resource identity.
type CatalogEnvironment struct {
	ApplicationKey string                    `json:"applicationKey"`
	EnvironmentKey string                    `json:"environmentKey"`
	Revision       int64                     `json:"revision"`
	Desired        *catalogrelease.Selection `json:"desired,omitempty"`
	Applied        *catalogrelease.Selection `json:"applied,omitempty"`
	Status         string                    `json:"status"`
}
type CatalogSelectionCommand struct {
	Organization               string
	ApplicationKey             string
	EnvironmentKey             string
	ExpectedEnvironmentVersion int64
	ExpectedRevision           int64
	Selection                  catalogrelease.Selection
}
type CatalogEnvironmentRepository interface {
	GetCatalogEnvironment(context.Context, string, string) (CatalogEnvironment, error)
	SelectCatalog(context.Context, CatalogSelectionCommand) (CatalogEnvironment, error)
	ActivateCatalog(context.Context, string, string, int64, string) error
}
