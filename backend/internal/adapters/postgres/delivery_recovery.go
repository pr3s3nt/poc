package postgres

import (
	"context"
	domain "orchestrator/internal/domain/delivery"
	"orchestrator/internal/ports/persistence"
)

func (s *Store) ResumeDeliveryExecution(ctx context.Context, org, id string, expected int64) (out persistence.DeliveryExecution, err error) {
	err = s.fenced(ctx, func(ctx context.Context) error {
		owner, ok := persistence.OwnerFrom(ctx)
		if !ok {
			return persistence.ErrOperationLost
		}
		var recovering bool
		if err := s.q(ctx).QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM environment_operations WHERE id=$1 AND status='RECOVERING' AND owner_instance=$2 AND fence=$3)`, owner.OperationID, owner.Owner, owner.Fence).Scan(&recovering); err != nil {
			return translate(err)
		}
		if !recovering {
			return persistence.ErrOperationLost
		}
		e, err := scanExecution(s.q(ctx).QueryRow(ctx, executionSelect+` WHERE u.organization_id=$1 AND i.id=$2 FOR UPDATE OF i,u`, org, id))
		if err != nil {
			return err
		}
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
		if err := s.validateDeliveryOwner(ctx, e); err != nil {
			return err
		}
		_, err = s.q(ctx).Exec(ctx, `UPDATE delivery_intents SET owner=$2,fence=$3,status='PENDING',version=version+1 WHERE id=$1`, id, owner.Owner, owner.Fence)
		if err != nil {
			return translate(err)
		}
		out, err = s.GetDeliveryExecution(ctx, org, id)
		return err
	})
	return
}
func (s *Store) RecoverGitPublication(ctx context.Context, org, destination string) error {
	return s.fenced(ctx, func(ctx context.Context) error {
		owner, ok := persistence.OwnerFrom(ctx)
		if !ok {
			return persistence.ErrOperationLost
		}
		var recovering bool
		if err := s.q(ctx).QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM environment_operations op JOIN environments e ON e.active_operation_id=op.id JOIN applications a ON a.id=e.application_id WHERE op.id=$1 AND op.status='RECOVERING' AND op.owner_instance=$2 AND op.fence=$3 AND a.organization_id=$4)`, owner.OperationID, owner.Owner, owner.Fence, org).Scan(&recovering); err != nil {
			return translate(err)
		}
		if !recovering {
			return persistence.ErrOperationLost
		}
		var same bool
		err := s.q(ctx).QueryRow(ctx, `SELECT organization_id=$2 AND environment_operation_id=$3 AND fence<=$4 FROM git_publication_leases WHERE destination_digest=$1 FOR UPDATE`, destination, org, owner.OperationID, owner.Fence).Scan(&same)
		if translate(err) == persistence.ErrNotFound {
			return nil
		}
		if err != nil {
			return translate(err)
		}
		if !same {
			return persistence.ErrEnvironmentBusy
		}
		_, err = s.q(ctx).Exec(ctx, `UPDATE git_publication_leases SET owner=$2,fence=$3 WHERE destination_digest=$1`, destination, owner.Owner, owner.Fence)
		return translate(err)
	})
}

func (s *Store) RebindDeliverySubmission(ctx context.Context, org, id string, expected int64) (out persistence.DeliveryExecution, err error) {
	err = s.fenced(ctx, func(ctx context.Context) error {
		owner, ok := persistence.OwnerFrom(ctx)
		if !ok {
			return persistence.ErrOperationLost
		}
		var recovering bool
		if err := s.q(ctx).QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM environment_operations WHERE id=$1 AND status='RECOVERING' AND owner_instance=$2 AND fence=$3)`, owner.OperationID, owner.Owner, owner.Fence).Scan(&recovering); err != nil {
			return translate(err)
		}
		if !recovering {
			return persistence.ErrOperationLost
		}
		e, err := scanExecution(s.q(ctx).QueryRow(ctx, executionSelect+` WHERE u.organization_id=$1 AND i.id=$2 FOR UPDATE OF i,u`, org, id))
		if err != nil {
			return err
		}
		if e.Mutation.OperationID != owner.OperationID || e.Mutation.Action != domain.Submit || (e.Mutation.Status != domain.IntentRunning && e.Mutation.Status != domain.IntentUnknown) {
			return domain.ErrUnsupported
		}
		if e.Mutation.Version != expected {
			return persistence.ErrVersionConflict
		}
		e.Mutation.Owner = owner.Owner
		e.Mutation.Fence = owner.Fence
		e.Mutation.Version++
		if err := s.validateDeliveryOwner(ctx, e); err != nil {
			return err
		}
		_, err = s.q(ctx).Exec(ctx, `UPDATE delivery_intents SET owner=$2,fence=$3,version=version+1 WHERE id=$1`, id, owner.Owner, owner.Fence)
		if err != nil {
			return translate(err)
		}
		out, err = s.GetDeliveryExecution(ctx, org, id)
		return err
	})
	return
}
