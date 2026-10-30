package persistence

import (
	"context"
	"orchestrator/internal/domain/drift"
)

// DriftScanRepository stores only value-free reports. Baselines, saved plans,
// private change digests and credentials belong to the State runtime repository.
type DriftScanRepository interface {
	QueueDriftScan(context.Context, drift.Scan) (drift.Scan, error)
	GetDriftScan(context.Context, string, string, string) (drift.Scan, error)
	ListDriftScans(context.Context, string, string, string, int) ([]drift.Scan, error)
	ListQueuedDriftScans(context.Context, int) ([]drift.Scan, error)
	StartDriftScan(context.Context, string, string, int64) (drift.Scan, error)
	FinishDriftScan(context.Context, drift.Scan, int64) error
	SkipQueuedDriftScan(context.Context, string, string, int64, string) error
}
