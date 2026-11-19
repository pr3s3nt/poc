// Package echo provides metadata outputs without infrastructure effects.
package echo

import (
	"context"
	"orchestrator/internal/ports/execution"
)

// Executor has no provider client, credentials or physical resource lifecycle.
type Executor struct{}

func (Executor) Provision(_ context.Context, req execution.ProvisionRequest) (execution.ProvisionResult, error) {
	return execution.ProvisionResult{Outputs: req.Inputs}, nil
}
