package delivery

import (
	"context"
	"encoding/json"
	domain "orchestrator/internal/domain/delivery"
	"orchestrator/internal/domain/environment"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/persistence"
)

func (c *Coordinator) ResumeTransfer(ctx context.Context, orgKey string, env environment.Environment, stage func(string, map[string]any) error) error {
	owner, ok := persistence.OwnerFrom(ctx)
	if !ok {
		return persistence.ErrOperationLost
	}
	op, err := c.Store.GetOperation(ctx, owner.OperationID)
	if err != nil || op.Status != environment.OpRecovering || op.Fence != owner.Fence || op.Owner != owner.Owner {
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
	transferRecords := []persistence.DeliveryExecution{}
	destinations := map[string]bool{}
	hasSubmissions := false
	transferComplete := true
	for _, record := range records {
		if record.Mutation.OperationID != owner.OperationID {
			continue
		}
		if record.Mutation.Action == domain.Submit {
			hasSubmissions = true
			continue
		}
		if record.Mutation.Action != domain.Adopt && record.Mutation.Action != domain.Withdraw {
			return domain.ErrUnsupported
		}
		if record.Mutation.Status != domain.IntentAcknowledged {
			transferComplete = false
		}
		if record.Mutation.Action == domain.Adopt {
			transferRecords = append(transferRecords, record)
		}
		b, p, err := c.sourceProvider(ctx, record)
		if err != nil {
			return err
		}
		if publisher, ok := p.(port.PublicationIdentity); ok {
			destination, err := publisher.PublicationDestination(b)
			if err != nil {
				return err
			}
			destinations[destination] = true
		}
	}
	if len(transferRecords) == 0 {
		return ErrBaselineRequired
	}
	// Healing is published only after every ownership transfer acknowledges.
	// Recover its exact receipts without resetting SUBMIT to pending or replaying
	// a completed Job. An incomplete handoff cannot coexist with this phase.
	if hasSubmissions {
		if !transferComplete {
			return domain.ErrOutcomeUnknown
		}
		if err := c.RecoverSubmissions(ctx, orgKey, env); err != nil {
			return err
		}
	}
	if env.AppliedDelivery.Effective() == env.DesiredDelivery.Effective() {
		for _, record := range transferRecords {
			if record.Mutation.Status != domain.IntentAcknowledged || record.Revision.Scope.Epoch != env.DeliveryEpoch {
				return domain.ErrOutcomeUnknown
			}
		}
		return nil
	}
	for destination := range destinations {
		if err := repo.RecoverGitPublication(ctx, org.ID, destination); err != nil {
			return err
		}
	}
	for _, record := range records {
		if record.Mutation.OperationID == owner.OperationID && record.Mutation.Action != domain.Submit && record.Mutation.Status != domain.IntentAcknowledged {
			if _, err := repo.ResumeDeliveryExecution(ctx, org.ID, record.Mutation.ID, record.Mutation.Version); err != nil {
				return err
			}
		}
	}
	return c.Transfer(ctx, orgKey, env, stage)
}
func (c *Coordinator) recoverySources(ctx context.Context, records []persistence.DeliveryExecution) ([]persistence.DeliveryExecution, bool, error) {
	owner, ok := persistence.OwnerFrom(ctx)
	if !ok {
		return nil, false, persistence.ErrOperationLost
	}
	op, err := c.Store.GetOperation(ctx, owner.OperationID)
	if err != nil {
		return nil, false, err
	}
	if op.Status != environment.OpRecovering {
		return nil, false, nil
	}
	sources := []persistence.DeliveryExecution{}
	for _, record := range records {
		if record.Mutation.OperationID != owner.OperationID || record.Mutation.Action != domain.Adopt {
			continue
		}
		raw, err := c.Artifacts.Get(ctx, record.OrganizationID, record.UnitID, record.Revision.ArtifactRef)
		if err != nil {
			return nil, false, err
		}
		var envelope transferEnvelope
		if json.Unmarshal(raw, &envelope) != nil {
			return nil, false, ErrArtifact
		}
		source, err := c.Store.GetDeliveryExecution(ctx, record.OrganizationID, envelope.SourceIntentID)
		if err != nil {
			return nil, false, err
		}
		if source.Revision.Scope.Epoch+1 != record.Revision.Scope.Epoch {
			return nil, false, ErrArtifact
		}
		sources = append(sources, source)
	}
	return sources, len(sources) > 0, nil
}
