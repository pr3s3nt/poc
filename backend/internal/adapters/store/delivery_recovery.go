package store

import (
	"context"
	domain "orchestrator/internal/domain/delivery"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/ports/persistence"
)

func (s *Store) ResumeDeliveryExecution(ctx context.Context, org, id string, expected int64) (out persistence.DeliveryExecution, err error) {
	err = s.Transact(ctx, func(ctx context.Context) error {
		owner, ok := persistence.OwnerFrom(ctx)
		if !ok {
			return persistence.ErrOperationLost
		}
		op, ok := s.state.Operations[owner.OperationID]
		if !ok || op.Status != environment.OpRecovering || op.Owner != owner.Owner || op.Fence != owner.Fence {
			return persistence.ErrOperationLost
		}
		record, ok := s.state.DeliveryExecutions[id]
		if !ok || record.Execution.OrganizationID != org {
			return persistence.ErrNotFound
		}
		e := cloneDelivery(record.Execution)
		e.Revision.ArtifactRef = record.ArtifactRef
		if e.Mutation.OperationID != owner.OperationID || (e.Mutation.Action != domain.Adopt && e.Mutation.Action != domain.Withdraw) {
			return domain.ErrUnsupported
		}
		if e.Mutation.Version != expected {
			return persistence.ErrVersionConflict
		}
		if e.Mutation.Status == domain.IntentAcknowledged {
			out = e
			return nil
		}
		e.Mutation.Owner = owner.Owner
		e.Mutation.Fence = owner.Fence
		e.Mutation.Status = domain.IntentPending
		e.Mutation.Version++
		if err := s.deliveryOwner(ctx, e); err != nil {
			return err
		}
		s.state.DeliveryExecutions[id] = deliveryExecutionRecord{Execution: cloneDelivery(e), ArtifactRef: e.Revision.ArtifactRef}
		out = e
		return nil
	})
	return
}
func (s *Store) RecoverGitPublication(ctx context.Context, org, destination string) error {
	return s.Transact(ctx, func(ctx context.Context) error {
		owner, ok := persistence.OwnerFrom(ctx)
		if !ok {
			return persistence.ErrOperationLost
		}
		op, ok := s.state.Operations[owner.OperationID]
		if !ok || op.Status != environment.OpRecovering {
			return persistence.ErrOperationLost
		}
		next := persistence.GitPublicationLease{Destination: destination, OrganizationID: org, Owner: owner}
		if err := s.publicationOwner(ctx, next); err != nil {
			return err
		}
		old, exists := s.state.PublicationLeases[destination]
		if !exists {
			return nil
		}
		if old.OrganizationID != org || old.Owner.OperationID != owner.OperationID || old.Owner.Fence > owner.Fence {
			return persistence.ErrEnvironmentBusy
		}
		s.state.PublicationLeases[destination] = next
		return nil
	})
}

func (s *Store) RebindDeliverySubmission(ctx context.Context, org, id string, expected int64) (out persistence.DeliveryExecution, err error) {
	err = s.Transact(ctx, func(ctx context.Context) error {
		owner, ok := persistence.OwnerFrom(ctx)
		if !ok {
			return persistence.ErrOperationLost
		}
		op, ok := s.state.Operations[owner.OperationID]
		if !ok || op.Status != environment.OpRecovering || op.Owner != owner.Owner || op.Fence != owner.Fence {
			return persistence.ErrOperationLost
		}
		record, ok := s.state.DeliveryExecutions[id]
		if !ok || record.Execution.OrganizationID != org {
			return persistence.ErrNotFound
		}
		e := cloneDelivery(record.Execution)
		e.Revision.ArtifactRef = record.ArtifactRef
		if e.Mutation.OperationID != owner.OperationID || e.Mutation.Action != domain.Submit || (e.Mutation.Status != domain.IntentRunning && e.Mutation.Status != domain.IntentUnknown) {
			return domain.ErrUnsupported
		}
		if e.Mutation.Version != expected {
			return persistence.ErrVersionConflict
		}
		e.Mutation.Owner = owner.Owner
		e.Mutation.Fence = owner.Fence
		e.Mutation.Version++
		if err := s.deliveryOwner(ctx, e); err != nil {
			return err
		}
		s.state.DeliveryExecutions[id] = deliveryExecutionRecord{Execution: cloneDelivery(e), ArtifactRef: e.Revision.ArtifactRef}
		out = e
		return nil
	})
	return
}
