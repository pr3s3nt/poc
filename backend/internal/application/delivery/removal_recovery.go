package delivery

import (
	"context"
	"reflect"

	domain "orchestrator/internal/domain/delivery"
	"orchestrator/internal/domain/environment"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/persistence"
)

// RecoverRemovals resumes only the original UID-guarded withdrawal. It cannot
// turn a release-only handoff into a delete, invent inventory or replay Submit.
func (c *Coordinator) RecoverRemovals(ctx context.Context, orgKey string, env environment.Environment) error {
	owner, ok := persistence.OwnerFrom(ctx)
	if !ok {
		return persistence.ErrOperationLost
	}
	repo, ok := c.Store.(persistence.DeliveryRecoveryRepository)
	if !ok {
		return ErrUnavailable
	}
	org, err := c.Store.GetOrganization(ctx, orgKey)
	if err != nil {
		return err
	}
	records, err := c.Store.ListDeliveryExecutions(ctx, org.ID, env.ID)
	if err != nil {
		return err
	}
	for _, record := range records {
		if record.Mutation.OperationID != owner.OperationID || record.Mutation.Action != domain.Withdraw || record.Mutation.Status == domain.IntentAcknowledged {
			continue
		}
		if record.Receipt.IntentID == "" || len(record.Inventory) == 0 {
			return domain.ErrOutcomeUnknown
		}
		source, err := c.Store.GetDeliveryExecution(ctx, org.ID, record.Receipt.IntentID)
		if err != nil {
			return err
		}
		if source.Mutation.Status != domain.IntentAcknowledged || (source.Mutation.Action != domain.Submit && source.Mutation.Action != domain.Adopt) || source.EnvironmentID != env.ID || source.UnitID != record.UnitID || source.Revision.ID != record.Receipt.RevisionID || !reflect.DeepEqual(source.Receipt, record.Receipt) || !reflect.DeepEqual(source.Inventory, record.Inventory) || record.Mutation.ID != stableID("delivery-withdraw", owner.OperationID, source.UnitID, source.Revision.ID) {
			return domain.ErrOutcomeUnknown
		}
		binding, provider, err := c.sourceProvider(ctx, source)
		if err != nil {
			return err
		}
		if publisher, ok := provider.(port.PublicationIdentity); ok {
			destination, err := publisher.PublicationDestination(binding)
			if err != nil {
				return err
			}
			if err := repo.RecoverGitPublication(ctx, org.ID, destination); err != nil {
				return err
			}
		}
		if _, err := repo.ResumeDeliveryExecution(ctx, org.ID, record.Mutation.ID, record.Mutation.Version); err != nil {
			return err
		}
		if err := c.withdrawExecution(ctx, orgKey, source, true); err != nil {
			return err
		}
	}
	return nil
}
