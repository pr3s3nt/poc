package terraformexec

import (
	"context"
	"orchestrator/internal/domain/drift"
)

// DriftPlanSession is optional, separate from cleanup's disappeared-object plan
// contract. Saved plan bytes and the report's private digest remain inside the
// State runtime; Console receives only its value-free findings.
type DriftPlanSession interface {
	InspectDrift(context.Context) (drift.Report, error)
	ReadSavedPlan(context.Context) ([]byte, error)
}
