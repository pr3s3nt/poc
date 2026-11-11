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
		if previous != nil && source.Revision.ID != "" {
			priorRaw, err := c.providerArtifact(ctx, *previous)
			if err != nil {
				return err
			}
			intent.PriorSubmission = &port.PriorSubmission{Prepared: port.Prepared{Revision: source.Revision, Payload: priorRaw}, Receipt: source.Receipt}
		}
		// UC-06 CD-10: a newer published determined failure holds the provider
		// pin, as the Deploy that created this submission must name it. Recovery
		// observes against that pin; the acknowledged source inventory, if any,
		// stays the UID evidence.
		if failed := publishedFailure(otherExecutions(records, record.Mutation.ID), record.UnitID, acknowledged(records)); failed != nil {
			failedRaw, err := c.providerArtifact(ctx, *failed)
			if err != nil {
				return err
			}
			intent.ExpectedRevision = failed.Revision.ID
			intent.PriorSubmission = &port.PriorSubmission{Prepared: port.Prepared{Revision: failed.Revision, Payload: failedRaw}, Receipt: failed.Receipt, Failed: true}
		}
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
		// UC-06 CD-10: an ordinary Deploy whose receipted revision is observed
		// failing ends FAILED; transfer/policy recovery keeps the hold.
		determined := !policyOnly && op.Kind == environment.OpDeploy
		deadline := c.Deadline
		if deadline <= 0 {
			deadline = 20 * time.Minute
			if determined {
				// The Deploy readiness deadline. A crash-looping first rollout never
				// reports Degraded, and recovery runs inside one Console request,
				// so it must conclude well before the proxy timeout.
				deadline = DeployReadinessDeadline
			}
		}
		wait, cancel := context.WithTimeout(ctx, deadline)
		failed := false
		for {
			stage = "observe"
			observed, observeErr := provider.Observe(wait, intent, receipt)
			// A read cut off by the expired readiness deadline is the deadline
			// outcome, not a provider failure.
			if observeErr != nil && !(determined && wait.Err() != nil && ctx.Err() == nil) {
				cancel()
				return observeErr
			}
			if observeErr != nil {
				observed = domain.Observation{}
			}
			if err := c.prunePending(wait, provider, intent, receipt, observed); err != nil {
				cancel()
				return err
			}
			if determined && observed.Health == domain.Degraded {
				cancel()
				if err := c.failRecoveredSubmission(ctx, &record, destination, org.ID, owner); err != nil {
					return err
				}
				failed = true
				break
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
				if !determined || ctx.Err() != nil {
					return domain.ErrOutcomeUnknown
				}
				if err := c.failRecoveredSubmission(ctx, &record, destination, org.ID, owner); err != nil {
					return err
				}
				failed = true
			case <-timer.C:
			}
			if failed {
				break
			}
		}
		cancel()
		if failed {
			continue
		}
		if destination != "" {
			if err := c.Store.ReleaseGitPublication(ctx, persistence.GitPublicationLease{Destination: destination, OrganizationID: org.ID, Owner: owner}); err != nil {
				return err
			}
		}
	}
	return nil
}

// failRecoveredSubmission marks a receipted submission FAILED after recovery
// observed its rollout failing (UC-06 CD-10) and releases its Git publication.
func (c *Coordinator) failRecoveredSubmission(ctx context.Context, record *persistence.DeliveryExecution, destination, orgID string, owner persistence.Owner) error {
	old := record.Mutation.Version
	next, err := record.Mutation.Advance(domain.IntentFailed, true)
	if err != nil {
		return err
	}
	updated := *record
	updated.Mutation = next
	updated.UpdatedAt = time.Now().UTC()
	if err := c.Store.Transact(ctx, func(tx context.Context) error {
		if _, err := c.Store.UpdateDeliveryExecution(tx, updated, old); err != nil {
			return err
		}
		if destination != "" {
			return c.Store.ReleaseGitPublication(tx, persistence.GitPublicationLease{Destination: destination, OrganizationID: orgID, Owner: owner})
		}
		return nil
	}); err != nil {
		return err
	}
	*record = updated
	return nil
}

// otherExecutions returns records without the submission being recovered, so
// the in-flight attempt is never mistaken for the revision holding the pin.
func otherExecutions(records []persistence.DeliveryExecution, mutationID string) []persistence.DeliveryExecution {
	out := make([]persistence.DeliveryExecution, 0, len(records))
	for _, r := range records {
		if r.Mutation.ID != mutationID {
			out = append(out, r)
		}
	}
	return out
}

// acknowledged returns the latest acknowledged Submit/Adopt of each unit, the
// same authoritative sources a Deploy uses.
func acknowledged(records []persistence.DeliveryExecution) []persistence.DeliveryExecution {
	latest := map[string]persistence.DeliveryExecution{}
	for _, r := range records {
		if r.Mutation.Status != domain.IntentAcknowledged {
			continue
		}
		if prior, ok := latest[r.UnitID]; !ok || r.CreatedAt.After(prior.CreatedAt) {
			latest[r.UnitID] = r
		}
	}
	out := []persistence.DeliveryExecution{}
	for _, r := range latest {
		if r.Mutation.Action == domain.Submit || r.Mutation.Action == domain.Adopt {
			out = append(out, r)
		}
	}
	return out
}
