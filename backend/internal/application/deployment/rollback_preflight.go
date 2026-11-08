package deployment

import (
	"context"
	"time"

	domain "orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/ports/renderregistry"
)

// ValidateRollbackRenderers verifies the original retained successful plans
// before claiming the Environment or changing any desired configuration.
func (s *Service) ValidateRollbackRenderers(ctx context.Context, appKey, envKey string, target environment.DeploymentSet, cutoff time.Time) error {
	validator, ok := s.renderer.(execution.RenderPreflight)
	if !ok {
		return nil
	}
	records, err := s.store.ListDeployments(ctx, appKey, envKey)
	if err != nil {
		return err
	}
	for id := range target.Document.Modules {
		found := false
		for _, record := range records {
			if record.WorkloadID != id || record.Status != domain.StatusSucceeded || record.FinishedAt == nil || record.FinishedAt.After(cutoff) {
				continue
			}
			raw, err := s.store.GetPlan(ctx, record.ID)
			if err != nil {
				return err
			}
			plan := decodePlan(raw)
			if plan == nil {
				return renderregistry.ErrBundleUnavailable
			}
			selection, ok := plan.Rendering[id]
			if !ok {
				return renderregistry.ErrBundleUnavailable
			}
			if err := validator.ValidateSelection(ctx, selection); err != nil {
				return err
			}
			found = true
			break
		}
		if !found {
			return renderregistry.ErrBundleUnavailable
		}
	}
	return nil
}
