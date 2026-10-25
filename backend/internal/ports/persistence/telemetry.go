package persistence

import (
	"context"
	"time"

	"orchestrator/internal/domain/telemetry"
)

// TelemetryRepository is observational storage, separate from Environment
// operation claims and Terraform state. A stale collector can never commit.
type TelemetryRepository interface {
	LeaseTelemetry(context.Context, telemetry.Source, string, time.Time) (telemetry.State, bool, error)
	CommitTelemetry(context.Context, telemetry.Batch) error
	ReadTelemetry(context.Context, telemetry.Query) (telemetry.Data, error)
	PruneTelemetry(context.Context, time.Time) error
}
