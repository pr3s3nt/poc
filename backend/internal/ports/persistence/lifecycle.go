package persistence

import (
	"context"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/configuration"
)

// ApplicationLifecycleRepository persists the UC-18 Application lifecycle
// (ADR-025). Both product stores implement it; callers type-assert.
type ApplicationLifecycleRepository interface {
	// TransitionApplication replaces the lifecycle status and deletion
	// progress of an Application if and only if its stored version equals
	// expectedVersion, increments the version and returns the stored record.
	// ErrNotFound for a missing key, ErrVersionConflict when stale. Name,
	// subdomain and identity never change. A DELETED Application stops
	// reserving its name and subdomain.
	TransitionApplication(ctx context.Context, key string, expectedVersion int64, status application.LifecycleStatus, deletion *application.Deletion) (application.Application, error)
	// ListConfigurationRevisions returns every configuration revision of one
	// Environment, oldest first. It reads metadata and references only.
	ListConfigurationRevisions(ctx context.Context, applicationKey, environmentKey string) ([]configuration.Revision, error)
}
