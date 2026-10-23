package persistence

import (
	"context"
	"time"

	"orchestrator/internal/domain/effectiveness"
)

// EffectivenessBaselineRepository owns the append-only UC-17 baseline
// versions (effectiveness_baselines). It is optional on a Store: a store
// without it cannot persist baselines.
type EffectivenessBaselineRepository interface {
	// ListEffectivenessBaselines returns every version of one metric and scope
	// (empty applicationKey = Organization), highest version first.
	ListEffectivenessBaselines(ctx context.Context, organizationKey, metric, applicationKey string) ([]effectiveness.Baseline, error)
	// AppendEffectivenessBaseline inserts b as version expectedVersion+1 of its
	// scope and returns the stored record. ErrVersionConflict when the scope's
	// current version is not expectedVersion; ErrNotFound for a missing
	// Organization, Application of another Organization or unknown author.
	// Existing versions are never changed.
	AppendEffectivenessBaseline(ctx context.Context, b effectiveness.Baseline, expectedVersion int64) (effectiveness.Baseline, error)
}

// CleanupRun is the safe projection of one RESOURCE_CLEANUP operation that
// UC-17 reads: identity, scope, authorization mode, actor, status and times.
type CleanupRun struct {
	OperationID    string
	ApplicationKey string
	EnvironmentKey string
	RootID         string
	Mode           string
	ActorRef       string
	Status         string
	StartedAt      time.Time
	FinishedAt     *time.Time
}

// CleanupHistory lists the cleanup operations of an Organization. It is
// optional: only stores that persist cleanup authorization provide it.
type CleanupHistory interface {
	ListCleanupRuns(ctx context.Context, organizationKey string) ([]CleanupRun, error)
}
