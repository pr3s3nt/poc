package persistence

import (
	"context"
	"orchestrator/internal/domain/application"
)

// InstallationSetup provides installation-wide enumeration and serialization.
// LockInstallationSetup must be called inside Transact before checking emptiness.
type InstallationSetup interface {
	ListOrganizations(context.Context) ([]application.Organization, error)
	LockInstallationSetup(context.Context) error
}
