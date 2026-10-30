package drift

import (
	"context"
	"time"

	domain "orchestrator/internal/domain/drift"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/ports/persistence"
)

// RecoverStoppedScan is used only after BeginRecovery's explicit stopped-process
// confirmation. It never runs a provider, restarts a scan or adopts refreshed
// state. Only original locks of the confirmed operation may be released.
func (s *Service) RecoverStoppedScan(ctx context.Context, orgKey, appKey, envKey, scanID string) error {
	owner, ok := persistence.OwnerFrom(ctx)
	if !ok {
		return persistence.ErrOperationLost
	}
	op, err := s.Store.GetOperation(ctx, owner.OperationID)
	if err != nil || op.Kind != environment.OpDriftScan || op.Status != environment.OpRecovering || op.Owner != owner.Owner || op.Fence != owner.Fence || op.ApplicationKey != appKey || op.EnvironmentKey != envKey || op.Detail["driftScanId"] != scanID {
		return persistence.ErrOperationLost
	}
	row, err := s.Get(ctx, orgKey, appKey, envKey, scanID)
	if err != nil {
		return err
	}
	if row.OperationID != op.ID || (row.Status != domain.Running && row.Status != domain.Interrupted) {
		return persistence.ErrOperationLost
	}
	recovery, ok := s.Instances.(interface {
		Lock(context.Context, string) (*tfresource.Lock, error)
		ReleaseRecoveredLock(context.Context, string) error
	})
	if !ok {
		return persistence.ErrOperationLost
	}
	for _, pin := range row.Scope {
		lock, err := recovery.Lock(ctx, pin.InstanceID)
		if err != nil {
			return err
		}
		if lock == nil {
			continue
		}
		if lock.OperationID != owner.OperationID || lock.Fence >= owner.Fence {
			return persistence.ErrOperationLost
		}
		if err := recovery.ReleaseRecoveredLock(ctx, pin.InstanceID); err != nil {
			return err
		}
	}
	if row.Status == domain.Interrupted {
		return nil
	}
	now := time.Now().UTC()
	row.Status, row.Result, row.ReasonCode, row.FinishedAt = domain.Interrupted, domain.Unknown, "PRIOR_SCAN_STOPPED", &now
	return s.Store.FinishDriftScan(ctx, row, row.Version)
}
