// Package observability defines the read-only cluster collection boundary.
package observability

import (
	"context"
	"orchestrator/internal/domain/telemetry"
	"orchestrator/internal/ports/execution"
)

type Source struct {
	telemetry.Source
	Target     execution.Target
	Controller *execution.ObjectRef
}

type Reader interface {
	Collect(context.Context, Source, telemetry.State) (telemetry.Batch, error)
}
