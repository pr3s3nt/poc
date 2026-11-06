package delivery

import (
	"context"
	"log"
	domain "orchestrator/internal/domain/delivery"
	"orchestrator/internal/domain/environment"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/ports/persistence"
	"time"
)

func (c *Coordinator) GuardRecovery(ctx context.Context, i port.Intent) error {
	owner, ok := persistence.OwnerFrom(ctx)
	if !ok || owner.OperationID != i.Owner.OperationID || owner.Owner != i.Owner.Instance || owner.Fence != i.Owner.Fence {
		return persistence.ErrOperationLost
	}
	op, err := c.Store.GetOperation(ctx, owner.OperationID)
	if err != nil || op.Status != environment.OpRecovering || op.Owner != owner.Owner || op.Fence != owner.Fence {
		return persistence.ErrOperationLost
	}
	env, err := c.Store.GetEnvironment(ctx, op.ApplicationKey, op.EnvironmentKey)
	if err != nil || env.ActiveOperationID != op.ID || env.ID != i.Revision.Scope.EnvironmentID {
		return persistence.ErrOperationLost
	}
	e, err := c.Store.GetDeliveryExecution(ctx, i.Revision.Scope.OrganizationID, i.ID)
	if err != nil {
		return err
	}
	if e.Mutation.OperationID != owner.OperationID || e.Mutation.Owner != owner.Owner || e.Mutation.Fence != owner.Fence || e.Mutation.Action != domain.Submit || (e.Mutation.Status != domain.IntentRunning && e.Mutation.Status != domain.IntentUnknown) {
		return domain.ErrOutcomeUnknown
	}
	provider, err := c.Registry.Resolve(providerKind(i.Revision.Selection, i.Binding))
	if err != nil {
		return err
	}
	if publication, ok := provider.(port.PublicationIdentity); ok {
		destination, err := publication.PublicationDestination(i.Binding)
		if err != nil {
			return err
		}
		return c.Store.CheckGitPublication(ctx, persistence.GitPublicationLease{Destination: destination, OrganizationID: i.Binding.OrganizationID, Owner: owner})
	}
	return nil
}
func (c *Coordinator) RecoverSubmissions(ctx context.Context, orgKey string, env environment.Environment) (resultErr error) {
	stage := "initialize"
	defer func() {
		if resultErr != nil {
			code := "UNCLASSIFIED"
			// Only fixed messages are eligible; arbitrary provider output stays private.
			known := map[string]string{
				"fleet: controller ownership or incarnation mismatch":          "OWNERSHIP",
				"fleet: controller operation could not be confirmed":           "CONTROLLER",
				"fleet: invalid binding configuration":                         "CONFIG",
				"fleet: controller or registered target could not be verified": "TARGET",
				"delivery: prior mutation outcome must be resolved":            "UNKNOWN",
				"gitpublication: operation failed":                             "GIT",
				"gitpublication: generated path changed concurrently":          "GIT_CONFLICT",
				"gitpublication: invalid generated publication":                "GIT_INVALID",
			}
			if v, ok := known[resultErr.Error()]; ok {
				code = v
			}
			log.Printf("delivery submission recovery stopped: stage=%s code=%s type=%T", stage, code, resultErr)
		}
	}()

	repo, ok := c.Store.(persistence.DeliveryRecoveryRepository)
	if !ok {
		return ErrUnavailable
	}
	org, err := c.Store.GetOrganization(ctx, orgKey)
	if err != nil {
		return err
	}
	owner, ok := persistence.OwnerFrom(ctx)
	if !ok {
		return persistence.ErrOperationLost
	}
	op, err := c.Store.GetOperation(ctx, owner.OperationID)
	if err != nil {
		return err
	}
	policyOnly, _ := op.Detail["deliveryPolicyOnly"].(bool)
	transfer, _ := op.Detail["deliveryTransfer"].(bool)
	policyOnly = policyOnly || transfer || op.Stage == "DELIVERY_POLICY"
	records, err := c.Store.ListDeliveryExecutions(ctx, org.ID, env.ID)
	if err != nil {
		return err
	}
	for _, record := range records {
		if record.Mutation.OperationID != owner.OperationID || (record.Mutation.Status == domain.IntentAcknowledged || record.Mutation.Status == domain.IntentFailed) {
			continue
		}
		if record.Mutation.Action != domain.Submit || (record.Receipt.IntentID != "" && record.Receipt.IntentID != record.Mutation.ID) {
			return domain.ErrOutcomeUnknown
		}
		stage = "binding"
		binding, provider, err := c.sourceProvider(ctx, record)
		if err != nil {
			return err
		}
		recoverer, ok := provider.(port.SubmissionRecovery)
		if !ok {
			return domain.ErrUnsupported
		}
		var destination string
		if publication, ok := provider.(port.PublicationIdentity); ok {
			destination, err = publication.PublicationDestination(binding)
			if err != nil {
				return err
			}
			stage = "publication-lease"
			if err = repo.RecoverGitPublication(ctx, org.ID, destination); err != nil {
				return err
			}
		}
		stage = "rebind"
		record, err = repo.RebindDeliverySubmission(ctx, org.ID, record.Mutation.ID, record.Mutation.Version)
		if err != nil {
			return err
		}
		stage = "artifact"
		raw, err := c.providerArtifact(ctx, record)
		if err != nil {
			return err
		}
		var previous *persistence.DeliveryExecution
		for index := range records {
			source := &records[index]
			if source.UnitID == record.UnitID && source.Mutation.Status == domain.IntentAcknowledged && (previous == nil || source.CreatedAt.After(previous.CreatedAt)) {
				previous = source
			}
		}
		source := port.TransferSource{}
		if previous != nil && (previous.Mutation.Action == domain.Submit || previous.Mutation.Action == domain.Adopt) {
			source = port.TransferSource{Revision: previous.Revision, Receipt: previous.Receipt, Inventory: previous.Inventory}
		}
		intent := port.Intent{ID: record.Mutation.ID, OrganizationKey: orgKey, Owner: port.Owner{OperationID: owner.OperationID, Instance: owner.Owner, Fence: owner.Fence}, Binding: binding, Revision: record.Revision, Target: record.Target, PolicyOnly: policyOnly}
		intent.SourceInventory = append([]port.AppliedObject(nil), source.Inventory...)
		intent.ExpectedRevision = source.Revision.ID
		if record.Receipt.IntentID == "" {
			if absentObserver, ok := provider.(port.SubmissionAbsenceRecovery); ok {
				stage = "absence"
				absent, err := absentObserver.ObserveSubmissionAbsent(ctx, intent, port.Prepared{Revision: record.Revision, Payload: raw})
				if err != nil {
					return err
				}
				if absent {
					old := record.Mutation.Version
					record.Mutation, err = record.Mutation.Advance(domain.IntentFailed, true)
					if err != nil {
						return err
					}
					record.UpdatedAt = time.Now().UTC()
					if err = c.Store.Transact(ctx, func(tx context.Context) error {
						if _, err := c.Store.UpdateDeliveryExecution(tx, record, old); err != nil {
							return err
						}
						if destination != "" {
							return c.Store.ReleaseGitPublication(tx, persistence.GitPublicationLease{Destination: destination, OrganizationID: org.ID, Owner: owner})
						}
						return nil
					}); err != nil {
						return err
					}
					continue
				}
			}
			observer, ok := provider.(port.SubmissionReceiptRecovery)
			if !ok {
				return domain.ErrOutcomeUnknown
			}
			stage = "receipt"
			receipt, err := observer.RecoverReceipt(ctx, intent, port.Prepared{Revision: record.Revision, Payload: raw})
			if err != nil {
				return err
			}
			record.Receipt = receipt
			record.UpdatedAt = time.Now().UTC()
			record, err = c.Store.UpdateDeliveryExecution(ctx, record, record.Mutation.Version)
			if err != nil {
				return err
			}
		}
		stage = "submission"
		receipt, err := recoverer.RecoverSubmission(ctx, intent, port.Prepared{Revision: record.Revision, Payload: raw}, record.Receipt, source)
		if err != nil {
			return err
		}
		// A previously failing controller can still be in exponential backoff
		// after its ownership conflict is resolved. Recovery only observes the
		// receipted revision; allow that retry without forcing a new deployment.
		deadline := c.Deadline
		if deadline <= 0 {
			deadline = 20 * time.Minute
		}
		wait, cancel := context.WithTimeout(ctx, deadline)
		for {
			stage = "observe"
			observed, observeErr := provider.Observe(wait, intent, receipt)
			if observeErr != nil {
				cancel()
				return observeErr
			}
			if err := c.prunePending(wait, provider, intent, receipt, observed); err != nil {
				cancel()
				return err
			}
			if observed.Ready(intent.Revision, time.Now().UTC(), ObservationInterval) {
				expected := map[string]string{}
				for _, o := range source.Inventory {
					expected[o.Key()] = o.UID
				}
				inventory := []persistence.DeliveryObject{}
				for _, o := range observed.Objects {
					ref := execution.ObjectRef{APIVersion: o.APIVersion, Kind: o.Kind, Name: o.Name}
					if uid, exists := expected[ref.Key()]; exists && uid != o.UID {
						cancel()
						return domain.ErrStale
					}
					inventory = append(inventory, persistence.DeliveryObject{ObjectRef: ref, UID: o.UID, Release: receipt.Handle["release"]})
				}
				old := record.Mutation.Version
				record.Mutation, err = record.Mutation.Advance(domain.IntentAcknowledged, true)
				if err != nil {
					cancel()
					return err
				}
				record.Inventory = inventory
				record.Receipt = receipt
				record.UpdatedAt = time.Now().UTC()
				_, err = c.Store.UpdateDeliveryExecution(ctx, record, old)
				cancel()
				if err != nil {
					return err
				}
				break
			}
			timer := time.NewTimer(2 * time.Second)
			select {
			case <-wait.Done():
				timer.Stop()
				cancel()
				return domain.ErrOutcomeUnknown
			case <-timer.C:
			}
		}
		if destination != "" {
			if err := c.Store.ReleaseGitPublication(ctx, persistence.GitPublicationLease{Destination: destination, OrganizationID: org.ID, Owner: owner}); err != nil {
				return err
			}
		}
	}
	return nil
}
