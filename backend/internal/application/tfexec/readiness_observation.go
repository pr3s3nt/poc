package tfexec

import (
	"context"

	"orchestrator/internal/application/envops"
	"orchestrator/internal/ports/terraformexec"
)

// ReadinessDetailPrefix keys the safe readiness observation of one resource in
// the owning Environment operation detail (UC-08 BR-11, issue #68).
const ReadinessDetailPrefix = "readiness:"

func readinessRecorder(ctx context.Context, deploymentID, descriptor string) func(terraformexec.ReadinessObservation) {
	record := envops.WaitRecorder(ctx, ReadinessDetailPrefix+descriptor, deploymentID)
	return func(o terraformexec.ReadinessObservation) { record(o.Waiting, o.Replaced) }
}
