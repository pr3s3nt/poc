package delivery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"time"

	domain "orchestrator/internal/domain/delivery"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/platform/canon"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/ports/persistence"
)

var ErrBaselineRequired = errors.New("delivery: admitted artifact and incarnation baseline required")

type transferEnvelope struct {
	SourceIntentID  string
	Source          port.TransferSource
	ProviderPayload []byte
}
type transferUnit struct {
	Source      persistence.DeliveryExecution
	Destination persistence.DeliveryExecution
	Envelope    transferEnvelope
}

// Transfer imports exact admitted artifacts under one Environment fence. All
// destination intents exist before the first release; no renderer or Terraform
// executor participates. Interrupted external outcomes retain the claim.
func (c *Coordinator) Transfer(ctx context.Context, orgKey string, env environment.Environment, stage func(string, map[string]any) error) error {
	if c == nil || c.Artifacts == nil {
		return ErrUnavailable
	}
	owner, ok := persistence.OwnerFrom(ctx)
	if !ok {
		return persistence.ErrOperationLost
	}
	org, err := c.Store.GetOrganization(ctx, orgKey)
	if err != nil {
		return err
	}
	binding, destination, err := c.binding(ctx, orgKey, env)
	if err != nil {
		return err
	}
	adopter, ok := destination.(port.OwnershipTransfer)
	if !ok {
		return domain.ErrUnsupported
	}
	records, err := c.Store.ListDeliveryExecutions(ctx, org.ID, env.ID)
	if err != nil {
		return err
	}
	workloadIDs := map[string]bool{}
	for _, e := range records {
		if e.Revision.Scope.TargetGeneration == env.TargetGeneration && e.Revision.Scope.Epoch == env.DeliveryEpoch {
			workloadIDs[e.WorkloadID] = true
		}
	}
	sources, recovering, err := c.recoverySources(ctx, records)
	if err != nil {
		return err
	}
	if !recovering {

		for workload := range workloadIDs {
			scoped := []persistence.DeliveryExecution{}
			for _, e := range records {
				if e.Revision.Scope.TargetGeneration == env.TargetGeneration && e.Revision.Scope.Epoch == env.DeliveryEpoch {
					scoped = append(scoped, e)
				}
			}
			active, err := activeExecutions(scoped, workload)
			if err != nil {
				return err
			}
			sources = append(sources, active...)
		}

	}
	if len(sources) == 0 {
		return ErrBaselineRequired
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].Revision.Scope.UnitKey < sources[j].Revision.Scope.UnitKey })
	// Match every current workload, including routes as separate source units.
	instances, err := c.Store.ListWorkloadInstancesFor(ctx, env.ApplicationKey+"/"+env.Key, env.TargetGeneration)
	if err != nil {
		return err
	}
	covered := map[string]bool{}
	for _, source := range sources {
		covered[source.WorkloadID] = true
	}
	for _, instance := range instances {
		if string(instance.Status) != "REMOVED" && !covered[instance.WorkloadID] {
			return ErrBaselineRequired
		}
	}
	units := []transferUnit{}
	for _, source := range sources {
		if len(source.Inventory) == 0 {
			return ErrBaselineRequired
		}
		sourceBinding, sourceProvider, err := c.sourceProvider(ctx, source)
		if err != nil {
			return err
		}
		exporter, ok := sourceProvider.(port.ArtifactExporter)
		if !ok {
			return domain.ErrUnsupported
		}
		validator, ok := sourceProvider.(port.OwnershipTransfer)
		if !ok {
			return domain.ErrUnsupported
		}
		sourceIntent := port.Intent{ID: source.Mutation.ID, OrganizationKey: orgKey, Owner: port.Owner{OperationID: owner.OperationID, Instance: owner.Owner, Fence: owner.Fence}, Binding: sourceBinding, Revision: source.Revision, Target: source.Target}
		if err := func() error {
			if recovering {
				return nil
			}
			return validator.ValidateTransfer(ctx, sourceIntent, source.Receipt, source.Inventory)
		}(); err != nil {
			return err
		}
		raw, err := c.providerArtifact(ctx, source)
		if err != nil {
			return err
		}
		manifests, err := exporter.ExportArtifact(ctx, raw)
		if err != nil {
			return err
		}
		if len(manifests) != len(source.Inventory) {
			return ErrBaselineRequired
		}
		request := DeployRequest{OrganizationKey: orgKey, ApplicationKey: env.ApplicationKey, Environment: env, WorkloadID: source.WorkloadID, DeploymentID: owner.OperationID, Target: source.Target}
		request.Environment.DeliveryEpoch = env.DeliveryEpoch + 1
		intent, unitID := phaseIntent(request, org.ID, owner, binding, source.Revision.Phase)
		intent.ID = stableID("delivery-adopt", owner.OperationID, unitID)
		// Adopt with self-heal off. Requested healing is enabled only once every unit
		// has an acknowledged destination, by a subsequent policy submission.
		intent.Revision.Selection.SelfHeal = false
		prepared, err := destination.Prepare(ctx, intent, manifests)
		if err != nil {
			return err
		}
		envelope := transferEnvelope{SourceIntentID: source.Mutation.ID, Source: port.TransferSource{Revision: source.Revision, Receipt: source.Receipt, Inventory: source.Inventory}, ProviderPayload: prepared.Payload}
		payload, err := json.Marshal(envelope)
		if err != nil {
			return err
		}
		id := intent.ID
		record, err := c.Store.GetDeliveryExecution(ctx, org.ID, id)
		if errors.Is(err, persistence.ErrNotFound) {
			ref, artifactErr := c.Artifacts.Put(ctx, org.ID, unitID, payload)
			if artifactErr != nil {
				return artifactErr
			}
			revision := prepared.Revision
			revision.ArtifactRef = ref
			digest := sha256.Sum256(payload)
			now := time.Now().UTC()
			record, err = c.Store.PrepareDelivery(ctx, persistence.DeliveryExecution{UnitID: unitID, OrganizationID: org.ID, EnvironmentID: env.ID, WorkloadID: source.WorkloadID, Revision: revision, Target: source.Target, Version: 1, CreatedAt: now, UpdatedAt: now, Mutation: domain.Mutation{ID: id, Scope: revision.Scope, RevisionID: revision.ID, OperationID: owner.OperationID, RequestID: id, RequestDigest: hex.EncodeToString(digest[:]), Action: domain.Adopt, Status: domain.IntentPending, Owner: owner.Owner, Fence: owner.Fence, Version: 1}})
		}
		if err != nil {
			return err
		}
		units = append(units, transferUnit{Source: source, Destination: record, Envelope: envelope})
	}
	detail := map[string]any{"deliveryTransfer": true, "units": len(units), "epoch": env.DeliveryEpoch + 1}
	if err := stage("RELEASING", detail); err != nil {
		return err
	}
	effects := true
	defer func() {
		if effects {
			persistence.MarkHold(ctx)
		}
	}()
	for _, unit := range units {
		if err := c.releaseExecution(ctx, orgKey, unit.Source); err != nil {
			return err
		}
	}
	if err := stage("ADOPTING", detail); err != nil {
		return err
	}
	for index := range units {
		unit := &units[index]
		if err := c.adoptUnit(ctx, orgKey, binding, adopter, destination, unit); err != nil {
			return err
		}
	}
	if err := stage("VERIFYING", detail); err != nil {
		return err
	}
	if env.DesiredDelivery.SelfHeal {
		grouped := map[string]DeployRequest{}
		for _, unit := range units {
			_, sourceProvider, err := c.sourceProvider(ctx, unit.Source)
			if err != nil {
				return err
			}
			exporter := sourceProvider.(port.ArtifactExporter)
			raw, err := c.providerArtifact(ctx, unit.Source)
			if err != nil {
				return err
			}
			manifests, err := exporter.ExportArtifact(ctx, raw)
			if err != nil {
				return err
			}
			request, exists := grouped[unit.Source.WorkloadID]
			if !exists {
				nextEnv := env
				nextEnv.DeliveryEpoch++
				request = DeployRequest{PolicyOnly: true, OrganizationKey: orgKey, ApplicationKey: env.ApplicationKey, Environment: nextEnv, WorkloadID: unit.Source.WorkloadID, DeploymentID: owner.OperationID + "-policy", Target: unit.Source.Target}
			}
			request.Manifests = append(request.Manifests, manifests...)
			grouped[unit.Source.WorkloadID] = request
		}
		ids := []string{}
		for id := range grouped {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			if err := c.Deploy(ctx, grouped[id]); err != nil {
				return err
			}
		}
	}

	if err := stage("COMMITTING", detail); err != nil {
		return err
	}
	repo, ok := c.Store.(persistence.DeliveryActivationRepository)
	if !ok {
		return ErrUnavailable
	}
	if err := repo.ActivateDelivery(ctx, persistence.DeliveryActivation{ApplicationKey: env.ApplicationKey, EnvironmentKey: env.Key, ConfigurationRevision: env.DeliveryConfigRevision, Selection: env.DesiredDelivery, CheckEpoch: true, ExpectedEpoch: env.DeliveryEpoch, NextEpoch: env.DeliveryEpoch + 1}); err != nil {
		return err
	}

	effects = false
	return stage("COMPLETED", detail)
}
func (c *Coordinator) sourceProvider(ctx context.Context, e persistence.DeliveryExecution) (domain.BindingVersion, port.Provider, error) {
	var b domain.BindingVersion
	if e.Revision.Selection.Effective().Mode == domain.CD {
		var err error
		b, err = c.Store.GetDeliveryBinding(ctx, e.OrganizationID, e.Revision.Selection.BindingID, e.Revision.Selection.BindingVersion)
		if err != nil {
			return b, nil, err
		}
	}
	p, err := c.Registry.Resolve(providerKind(e.Revision.Selection, b))
	return b, p, err
}
func (c *Coordinator) adoptUnit(ctx context.Context, orgKey string, b domain.BindingVersion, adopter port.OwnershipTransfer, provider port.Provider, unit *transferUnit) error {
	owner, _ := persistence.OwnerFrom(ctx)
	record := unit.Destination
	if record.Mutation.Status == domain.IntentAcknowledged {
		return nil
	}
	if record.Mutation.Status != domain.IntentPending {
		return domain.ErrOutcomeUnknown
	}
	var publication *persistence.GitPublicationLease
	if publisher, ok := provider.(port.PublicationIdentity); ok {
		destination, err := publisher.PublicationDestination(b)
		if err != nil {
			return err
		}
		lease := persistence.GitPublicationLease{Destination: destination, OrganizationID: record.OrganizationID, Owner: owner}
		if err := c.Store.ClaimGitPublication(ctx, lease); err != nil {
			return err
		}
		publication = &lease
	}
	old := record.Mutation.Version
	var err error
	record.Mutation, err = record.Mutation.Advance(domain.IntentRunning, false)
	if err != nil {
		return err
	}
	record, err = c.Store.UpdateDeliveryExecution(ctx, record, old)
	if err != nil {
		return err
	}
	i := port.Intent{ID: record.Mutation.ID, OrganizationKey: orgKey, Owner: port.Owner{OperationID: owner.OperationID, Instance: owner.Owner, Fence: owner.Fence}, Binding: b, Revision: record.Revision, Target: record.Target}
	raw, err := c.Artifacts.Get(ctx, record.OrganizationID, record.UnitID, record.Revision.ArtifactRef)
	if err != nil {
		return err
	}
	var envelope transferEnvelope
	if json.Unmarshal(raw, &envelope) != nil {
		return ErrArtifact
	}
	receipt, err := adopter.Adopt(ctx, i, port.Prepared{Revision: record.Revision, Payload: envelope.ProviderPayload}, envelope.Source)
	if err != nil {
		return err
	}
	record.Receipt = receipt
	record.UpdatedAt = time.Now().UTC()
	record, err = c.Store.UpdateDeliveryExecution(ctx, record, record.Mutation.Version)
	if err != nil {
		return err
	}
	poll := c.PollInterval
	if poll <= 0 {
		poll = 2 * time.Second
	}
	deadline := c.Deadline
	if deadline <= 0 {
		deadline = 5 * time.Minute
	}
	wait, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	for {
		observed, err := provider.Observe(wait, i, receipt)
		if err != nil {
			return err
		}
		if observed.Ready(i.Revision, time.Now().UTC(), 30*time.Second) {
			record.Inventory = []persistence.DeliveryObject{}
			expected := map[string]string{}
			for _, o := range envelope.Source.Inventory {
				expected[o.Key()] = o.UID
			}
			for _, o := range observed.Objects {
				ref := execution.ObjectRef{APIVersion: o.APIVersion, Kind: o.Kind, Name: o.Name}
				if expected[ref.Key()] != o.UID {
					return domain.ErrStale
				}
				record.Inventory = append(record.Inventory, persistence.DeliveryObject{ObjectRef: ref, UID: o.UID, Release: receipt.Handle["release"]})
			}
			if len(record.Inventory) != len(expected) {
				return ErrBaselineRequired
			}
			break
		}
		select {
		case <-wait.Done():
			return domain.ErrOutcomeUnknown
		case <-ticker.C:
		}
	}
	old = record.Mutation.Version
	record.Mutation, err = record.Mutation.Advance(domain.IntentAcknowledged, true)
	if err != nil {
		return err
	}
	record.UpdatedAt = time.Now().UTC()
	record, err = c.Store.UpdateDeliveryExecution(ctx, record, old)
	if err != nil {
		return err
	}
	if publication != nil {
		if err := c.Store.ReleaseGitPublication(ctx, *publication); err != nil {
			return err
		}
	}
	unit.Destination = record
	return nil
}

// TransferDigest binds the safe checkpoint projection without exposing artifacts.
func TransferDigest(records []persistence.DeliveryExecution) (string, error) {
	return canon.Hash(records)
}

func (c *Coordinator) providerArtifact(ctx context.Context, e persistence.DeliveryExecution) ([]byte, error) {
	raw, err := c.Artifacts.Get(ctx, e.OrganizationID, e.UnitID, e.Revision.ArtifactRef)
	if err != nil {
		return nil, err
	}
	if e.Mutation.Action == domain.Adopt {
		var envelope transferEnvelope
		if json.Unmarshal(raw, &envelope) != nil || len(envelope.ProviderPayload) == 0 {
			return nil, ErrArtifact
		}
		return envelope.ProviderPayload, nil
	}
	return raw, nil
}
