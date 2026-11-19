package delivery

import (
	"context"
	"errors"
	"log"
	"maps"
	"sort"
	"time"

	domain "orchestrator/internal/domain/delivery"
	"orchestrator/internal/domain/environment"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/persistence"
)

const verifiedWithdrawalInventory = "verifiedWithdrawalInventory"

func (c *Coordinator) RemoveWorkload(ctx context.Context, orgKey string, envID, workload string) error {
	org, err := c.Store.GetOrganization(ctx, orgKey)
	if err != nil {
		return err
	}
	executions, err := c.Store.ListDeliveryExecutions(ctx, org.ID, envID)
	if err != nil {
		return err
	}
	latest, err := withdrawalExecutions(executions, workload, false)
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

// RemoveApplicationDelivery withdraws all remaining units for an Environment
// under the caller's removal claim, including units of failed workloads that
// never entered the applied Deployment Set.
func (c *Coordinator) RemoveApplicationDelivery(ctx context.Context, orgKey string, env environment.Environment) error {
	org, err := c.Store.GetOrganization(ctx, orgKey)
	if err != nil {
		return err
	}
	executions, err := c.Store.ListDeliveryExecutions(ctx, org.ID, env.ID)
	if err != nil {
		return err
	}
	latest, err := withdrawalExecutions(executions, "", true)
	if err != nil {
		persistence.MarkHold(ctx)
		return err
	}
	for _, execution := range latest {
		if err := c.removeExecution(ctx, orgKey, execution); err != nil {
			return err
		}
	}
	return nil
}

func activeExecutions(executions []persistence.DeliveryExecution, workload string) ([]persistence.DeliveryExecution, error) {
	return selectExecutions(executions, workload, false, false)
}

func withdrawalExecutions(executions []persistence.DeliveryExecution, workload string, entireEnvironment bool) ([]persistence.DeliveryExecution, error) {
	return selectExecutions(executions, workload, entireEnvironment, true)
}

func selectExecutions(executions []persistence.DeliveryExecution, workload string, entireEnvironment, includePublishedFailures bool) ([]persistence.DeliveryExecution, error) {
	// A failed submission with an exact publication receipt still controls the
	// unit. Unresolved publication or withdrawal keeps the Environment held.
	latest := map[string]persistence.DeliveryExecution{}
	failedPublications := map[string]persistence.DeliveryExecution{}
	for _, e := range executions {
		if !entireEnvironment && e.WorkloadID != workload {
			continue
		}
		if e.Mutation.Status == domain.IntentRunning || e.Mutation.Status == domain.IntentUnknown {
			return nil, domain.ErrOutcomeUnknown
		}
		failedPublication := includePublishedFailures && e.Mutation.Action == domain.Submit && e.Mutation.Status == domain.IntentFailed && published(e)
		if entireEnvironment && failedPublication {
			failedPublications[e.Mutation.ID] = e
		}
		if e.Mutation.Status == domain.IntentAcknowledged || failedPublication {
			if prior, exists := latest[e.UnitID]; !exists || e.CreatedAt.After(prior.CreatedAt) {
				latest[e.UnitID] = e
			}
		}
	}
	out := []persistence.DeliveryExecution{}
	for _, e := range latest {
		if e.Mutation.Action == domain.Submit || e.Mutation.Action == domain.Adopt {
			out = append(out, e)
			continue
		}
		// Older acknowledged removals of failed Fleet publications could
		// leave retained objects behind. Replay only the exact receipt of
		// the latest removal, under the new Environment fence.
		source, failed := failedPublications[e.Receipt.IntentID]
		if failed && source.UnitID == e.UnitID && source.Revision.ID == e.Receipt.RevisionID &&
			e.Receipt.Handle["inventory"] != "" && e.Receipt.Handle[verifiedWithdrawalInventory] == "" &&
			e.Mutation.Action == domain.Withdraw &&
			e.Mutation.ID == stableID("delivery-withdraw", e.Mutation.OperationID, e.UnitID, e.Receipt.RevisionID) {
			out = append(out, source)
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
	// Resolve the same durable per-Application repository/Environment binding
	// that admitted the source receipt; the platform policy has no clone URL.
	b, provider, err := c.sourceProvider(ctx, source)
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
		log.Printf("delivery withdrawal stopped: unit=%s remove=%t cause=%v", source.Revision.Scope.UnitKey, remove, err)
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
	if remove && record.Receipt.Handle["inventory"] != "" {
		record.Receipt.Handle = maps.Clone(record.Receipt.Handle)
		record.Receipt.Handle[verifiedWithdrawalInventory] = "true"
	}
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
