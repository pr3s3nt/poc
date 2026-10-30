package delivery

import (
	"context"
	"errors"
	"sort"
	"time"

	domain "orchestrator/internal/domain/delivery"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/persistence"
)

func (c *Coordinator) RemoveWorkload(ctx context.Context, orgKey string, envID, workload string) error {
	org, err := c.Store.GetOrganization(ctx, orgKey)
	if err != nil {
		return err
	}
	executions, err := c.Store.ListDeliveryExecutions(ctx, org.ID, envID)
	if err != nil {
		return err
	}
	latest, err := activeExecutions(executions, workload)
	if err != nil {
		persistence.MarkHold(ctx)
		return err
	}
	for _, e := range latest {
		if err := c.removeExecution(ctx, orgKey, e); err != nil {
			return err
		}
	}
	return nil
}

func activeExecutions(executions []persistence.DeliveryExecution, workload string) ([]persistence.DeliveryExecution, error) {
	// Only the latest successful revision of each unit is authoritative. Unknown
	// earlier publication/removal keeps the Environment held rather than deleting.
	latest := map[string]persistence.DeliveryExecution{}
	for _, e := range executions {
		if e.WorkloadID != workload {
			continue
		}
		if e.Mutation.Status == domain.IntentRunning || e.Mutation.Status == domain.IntentUnknown {
			return nil, domain.ErrOutcomeUnknown
		}
		if e.Mutation.Status == domain.IntentAcknowledged {
			if prior, exists := latest[e.UnitID]; !exists || e.CreatedAt.After(prior.CreatedAt) {
				latest[e.UnitID] = e
			}
		}
	}
	out := []persistence.DeliveryExecution{}
	for _, e := range latest {
		if e.Mutation.Action == domain.Submit || e.Mutation.Action == domain.Adopt {
			out = append(out, e)
		}
	}
	// Reverse lifecycle order: stop rollout/Jobs before their prerequisites.
	order := map[domain.Phase]int{domain.Route: 0, domain.Rollout: 1, domain.Job: 2, domain.Prerequisite: 3}
	sort.Slice(out, func(i, j int) bool {
		if order[out[i].Revision.Phase] != order[out[j].Revision.Phase] {
			return order[out[i].Revision.Phase] < order[out[j].Revision.Phase]
		}
		return out[i].UnitID < out[j].UnitID
	})
	return out, nil
}
func (c *Coordinator) removeExecution(ctx context.Context, orgKey string, source persistence.DeliveryExecution) error {
	return c.withdrawExecution(ctx, orgKey, source, true)
}
func (c *Coordinator) releaseExecution(ctx context.Context, orgKey string, source persistence.DeliveryExecution) error {
	return c.withdrawExecution(ctx, orgKey, source, false)
}
func (c *Coordinator) withdrawExecution(ctx context.Context, orgKey string, source persistence.DeliveryExecution, remove bool) error {
	owner, ok := persistence.OwnerFrom(ctx)
	if !ok {
		return persistence.ErrOperationLost
	}
	selection := source.Revision.Selection.Effective()
	var b domain.BindingVersion
	var err error
	if selection.Mode == domain.CD {
		b, err = c.Store.GetDeliveryBinding(ctx, source.OrganizationID, selection.BindingID, selection.BindingVersion)
		if err != nil {
			return err
		}
	}
	provider, err := c.Registry.Resolve(providerKind(selection, b))
	if err != nil {
		return err
	}
	remover, ok := provider.(port.GuardedRemoval)
	if remove && !ok {
		return domain.ErrUnsupported
	}
	prefix := "delivery-release"
	if remove {
		prefix = "delivery-withdraw"
	}
	id := stableID(prefix, owner.OperationID, source.UnitID, source.Revision.ID)
	record, err := c.Store.GetDeliveryExecution(ctx, source.OrganizationID, id)
	if errors.Is(err, persistence.ErrNotFound) {
		// Withdrawal needs its own immutable revision/intent while referring to the
		// source controller receipt. It never reuses another operation's owner pin.
		sourceRevision := source.Revision
		source.Revision.ID = stableID("delivery-withdraw-revision", id)
		source.Mutation = domain.Mutation{ID: id, Scope: source.Revision.Scope, RevisionID: source.Revision.ID, OperationID: owner.OperationID, RequestID: id, RequestDigest: sourceRevision.PayloadDigest, Action: domain.Withdraw, Status: domain.IntentPending, Owner: owner.Owner, Fence: owner.Fence, Version: 1}
		source.Version = 1
		source.CreatedAt = time.Now().UTC()
		source.UpdatedAt = source.CreatedAt
		record, err = c.Store.PrepareDelivery(ctx, source)
	}
	if err != nil {
		return err
	}
	if record.Mutation.Status == domain.IntentAcknowledged {
		return nil
	}
	if record.Mutation.Status != domain.IntentPending {
		persistence.MarkHold(ctx)
		return domain.ErrOutcomeUnknown
	}
	var publication *persistence.GitPublicationLease
	if publisher, ok := provider.(port.PublicationIdentity); ok {
		destination, err := publisher.PublicationDestination(b)
		if err != nil {
			return err
		}
		lease := persistence.GitPublicationLease{Destination: destination, OrganizationID: source.OrganizationID, Owner: owner}
		if err := c.Store.ClaimGitPublication(ctx, lease); err != nil {
			return err
		}
		publication = &lease
	}
	effectPending := false
	defer func() {
		if effectPending {
			persistence.MarkHold(ctx)
		}
	}()
	old := record.Mutation.Version
	record.Mutation, err = record.Mutation.Advance(domain.IntentRunning, false)
	if err != nil {
		return err
	}
	record, err = c.Store.UpdateDeliveryExecution(ctx, record, old)
	if err != nil {
		return err
	}
	effectPending = true
	// The external controller still carries the source deployment revision.
	intentRevision := record.Revision
	intentRevision.ID = record.Receipt.RevisionID
	intent := port.Intent{ID: id, OrganizationKey: orgKey, Owner: port.Owner{OperationID: owner.OperationID, Instance: owner.Owner, Fence: owner.Fence}, Binding: b, Revision: intentRevision, Target: record.Target}
	var withdrawErr error
	if remove {
		withdrawErr = remover.RemoveRetainedObjects(ctx, intent, record.Receipt, record.Inventory)
	} else {
		proof, err := provider.Withdraw(ctx, intent, record.Receipt)
		withdrawErr = err
		if err == nil && !proof.ManagementStopped {
			withdrawErr = domain.ErrOutcomeUnknown
		}
	}
	if err := withdrawErr; err != nil {
		old := record.Mutation.Version
		record.Mutation, _ = record.Mutation.Advance(domain.IntentUnknown, false)
		record.UpdatedAt = time.Now().UTC()
		_, _ = c.Store.UpdateDeliveryExecution(ctx, record, old)
		persistence.MarkHold(ctx)
		return err
	}
	old = record.Mutation.Version
	record.Mutation, err = record.Mutation.Advance(domain.IntentAcknowledged, true)
	if err != nil {
		return err
	}
	record.UpdatedAt = time.Now().UTC()
	_, err = c.Store.UpdateDeliveryExecution(ctx, record, old)
	if err != nil {
		return err
	}
	if publication != nil {
		if err := c.Store.ReleaseGitPublication(ctx, *publication); err != nil {
			return err
		}
	}
	effectPending = false
	return nil
}
