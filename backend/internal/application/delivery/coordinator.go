package delivery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	domain "orchestrator/internal/domain/delivery"
	"orchestrator/internal/domain/environment"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/ports/persistence"
)

type executionStore interface {
	persistence.Store
	persistence.DeliveryBindingRepository
	persistence.DeliveryExecutionRepository
	persistence.GitPublicationRepository
	persistence.OrganizationIdentityReader
}
type Coordinator struct {
	Store        executionStore
	Registry     *port.Registry
	Artifacts    *Artifacts
	PollInterval time.Duration
	Deadline     time.Duration
}
type DeployRequest struct {
	PolicyOnly      bool
	OrganizationKey string
	ApplicationKey  string
	Environment     environment.Environment
	WorkloadID      string
	DeploymentID    string
	Target          execution.Target
	Manifests       []execution.Manifest
}

func stableID(parts ...string) string {
	raw, _ := json.Marshal(parts)
	sum := sha256.Sum256(raw)
	sum[6] = (sum[6] & 15) | 80
	sum[8] = (sum[8] & 63) | 128
	h := hex.EncodeToString(sum[:16])
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}
func (c *Coordinator) Guard(ctx context.Context, i port.Intent) error {
	owner, ok := persistence.OwnerFrom(ctx)
	if !ok || owner.OperationID != i.Owner.OperationID || owner.Owner != i.Owner.Instance || owner.Fence != i.Owner.Fence {
		return persistence.ErrOperationLost
	}
	op, err := c.Store.GetOperation(ctx, owner.OperationID)
	if err != nil || op.Owner != owner.Owner || op.Fence != owner.Fence || !op.Status.Holds() {
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
	if e.Mutation.Status != domain.IntentRunning || e.Mutation.OperationID != owner.OperationID {
		return domain.ErrOutcomeUnknown
	}
	if e.Mutation.Action == domain.Submit || e.Mutation.Action == domain.Withdraw || e.Mutation.Action == domain.Adopt {
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
	}
	return nil
}
func (c *Coordinator) binding(ctx context.Context, orgKey string, env environment.Environment) (domain.BindingVersion, port.Provider, error) {
	selection := env.DesiredDelivery.Effective()
	if selection.Mode == domain.Direct {
		provider, err := c.Registry.Resolve("direct")
		return domain.BindingVersion{}, provider, err
	}
	org, err := c.Store.GetOrganization(ctx, orgKey)
	if err != nil {
		return domain.BindingVersion{}, nil, err
	}
	b, err := c.Store.GetDeliveryBinding(ctx, org.ID, selection.BindingID, selection.BindingVersion)
	if err != nil {
		return b, nil, err
	}
	if b.Status != domain.BindingReady {
		return b, nil, domain.ErrUnsupported
	}
	if err := b.Capabilities.ValidateSelection(selection); err != nil {
		return b, nil, err
	}
	provider, err := c.Registry.Resolve(b.ProviderKind)
	return b, provider, err
}

func providerKind(selection domain.Selection, binding domain.BindingVersion) string {
	if selection.Effective().Mode == domain.Direct {
		return "direct"
	}
	return binding.ProviderKind
}

// Deploy persists the entire replayable phase before calling any provider. It
// never falls back to direct on unsupported CD behavior or unknown outcomes.
func (c *Coordinator) Deploy(ctx context.Context, req DeployRequest) error {
	if c == nil || c.Store == nil || c.Registry == nil || c.Artifacts == nil {
		return ErrUnavailable
	}
	owner, ok := persistence.OwnerFrom(ctx)
	if !ok {
		return persistence.ErrOperationLost
	}
	b, provider, err := c.binding(ctx, req.OrganizationKey, req.Environment)
	if err != nil {
		return err
	}
	org, err := c.Store.GetOrganization(ctx, req.OrganizationKey)
	if err != nil {
		return err
	}
	phases := map[domain.Phase][]execution.Manifest{}
	for _, m := range req.Manifests {
		if m.Auxiliary || m.Secret {
			continue
		}
		phase := domain.Prerequisite
		switch m.Kind {
		case "Job":
			phase = domain.Job
		case "Deployment", "StatefulSet", "DaemonSet", "HorizontalPodAutoscaler":
			// Keep an autoscaler with its rollout target so the provider can
			// admit one replica owner and preserve delegation during upgrades.
			phase = domain.Rollout
		}
		if req.WorkloadID == "_routes" {
			phase = domain.Route
		}
		phases[phase] = append(phases[phase], m)
	}
	// Deployment labels are artifact provenance, not cluster target identity.
	target := req.Target
	target.Extra = map[string]string{}
	for k, v := range req.Target.Extra {
		if k != "deployment" {
			target.Extra[k] = v
		}
	}
	if len(target.Extra) == 0 {
		target.Extra = nil
	}
	req.Target = target
	if len(phases) == 0 {
		return domain.ErrInvalid
	}
	// Validate the complete proposed artifact locally before the first external
	// write, including a phase whose prior completion will later be reused.
	for phase, manifests := range phases {
		intent, _ := phaseIntent(req, org.ID, owner, b, phase)
		if _, err := provider.Prepare(ctx, intent, manifests); err != nil {
			return err
		}
	}
	existing, err := c.Store.ListDeliveryExecutions(ctx, org.ID, req.Environment.ID)
	if err != nil {
		return err
	}
	active, err := activeExecutions(existing, req.WorkloadID)
	if err != nil {
		persistence.MarkHold(ctx)
		return err
	}
	reuseJob := false
	for _, previous := range active {
		if previous.Revision.Phase != domain.Job || len(phases[domain.Job]) == 0 {
			continue
		}
		policy, ok := provider.(port.ImmutableExecution)
		if !ok {
			return domain.ErrUnsupported
		}
		raw, err := c.providerArtifact(ctx, previous)
		if err != nil {
			return err
		}
		same, err := policy.SameExecution(ctx, raw, phases[domain.Job])
		if err != nil {
			return err
		}
		if !same {
			return domain.ErrUnsupported
		}
		reuseJob = true
	}
	// A phase absent from the new complete artifact still has a live controller.
	// Retire it with the same retained/UID protocol as workload removal.
	for _, previous := range active {
		if len(phases[previous.Revision.Phase]) == 0 {
			if err := c.removeExecution(ctx, req.OrganizationKey, previous); err != nil {
				return err
			}
		}
	}
	for _, phase := range []domain.Phase{domain.Prerequisite, domain.Job, domain.Rollout, domain.Route} {
		manifests := phases[phase]
		if len(manifests) == 0 {
			continue
		}
		if phase == domain.Job && reuseJob {
			continue
		}
		intent, unit := phaseIntent(req, org.ID, owner, b, phase)
		existing, err := c.Store.ListDeliveryExecutions(ctx, org.ID, req.Environment.ID)
		if err != nil {
			return err
		}
		var newest time.Time
		for _, e := range existing {
			if e.UnitID == unit && e.Mutation.Status == domain.IntentAcknowledged && (e.Mutation.Action == domain.Submit || e.Mutation.Action == domain.Adopt) && e.CreatedAt.After(newest) {
				intent.ExpectedRevision = e.Revision.ID
				intent.SourceInventory = append([]port.AppliedObject(nil), e.Inventory...)
				newest = e.CreatedAt
			}
		}

		if err := c.runPhase(ctx, provider, intent, unit, req.WorkloadID, manifests); err != nil {
			return err
		}
	}
	return nil
}

func phaseIntent(req DeployRequest, orgID string, owner persistence.Owner, b domain.BindingVersion, phase domain.Phase) (port.Intent, string) {
	selection := req.Environment.DesiredDelivery.Effective()
	if phase == domain.Job {
		selection.SelfHeal = false
	}
	scope := domain.Scope{OrganizationID: orgID, EnvironmentID: req.Environment.ID, TargetGeneration: req.Environment.TargetGeneration, Epoch: req.Environment.DeliveryEpoch, UnitKey: req.WorkloadID + "/" + string(phase)}
	unit := stableID("delivery-unit", scope.OrganizationID, scope.EnvironmentID, fmt.Sprint(scope.TargetGeneration), fmt.Sprint(scope.Epoch), scope.UnitKey)
	revision := domain.Revision{ID: stableID("delivery-revision", owner.OperationID, req.DeploymentID, unit), Scope: scope, Phase: phase, Selection: selection, PayloadDigest: strings.Repeat("0", 64), ArtifactRef: "preparing"}
	return port.Intent{ID: stableID("delivery-intent", revision.ID, string(domain.Submit)), OrganizationKey: req.OrganizationKey, Owner: port.Owner{OperationID: owner.OperationID, Instance: owner.Owner, Fence: owner.Fence}, Binding: b, Revision: revision, Target: req.Target, PolicyOnly: req.PolicyOnly}, unit
}
func (c *Coordinator) runPhase(ctx context.Context, provider port.Provider, i port.Intent, unit, workload string, manifests []execution.Manifest) error {
	record, err := c.Store.GetDeliveryExecution(ctx, i.Revision.Scope.OrganizationID, i.ID)
	if errors.Is(err, persistence.ErrNotFound) {
		prepared, prepareErr := provider.Prepare(ctx, i, manifests)
		if prepareErr != nil {
			return prepareErr
		}
		ref, artifactErr := c.Artifacts.Put(ctx, i.Revision.Scope.OrganizationID, unit, prepared.Payload)
		if artifactErr != nil {
			return artifactErr
		}
		i.Revision = prepared.Revision
		i.Revision.ArtifactRef = ref
		digest := sha256.Sum256(prepared.Payload)
		now := time.Now().UTC()
		record, err = c.Store.PrepareDelivery(ctx, persistence.DeliveryExecution{UnitID: unit, OrganizationID: i.Revision.Scope.OrganizationID, EnvironmentID: i.Revision.Scope.EnvironmentID, WorkloadID: workload, Revision: i.Revision, Target: i.Target, Version: 1, CreatedAt: now, UpdatedAt: now, Mutation: domain.Mutation{ID: i.ID, Scope: i.Revision.Scope, RevisionID: i.Revision.ID, OperationID: i.Owner.OperationID, RequestID: i.ID, RequestDigest: hex.EncodeToString(digest[:]), Action: domain.Submit, Status: domain.IntentPending, Owner: i.Owner.Instance, Fence: i.Owner.Fence, Version: 1}})
	}
	if err != nil {
		return err
	}
	i.Revision = record.Revision
	if record.Mutation.Status == domain.IntentAcknowledged {
		return nil
	}
	if record.Mutation.Status != domain.IntentPending {
		persistence.MarkHold(ctx)
		return domain.ErrOutcomeUnknown
	}
	raw, err := c.Artifacts.Get(ctx, record.OrganizationID, record.UnitID, record.Revision.ArtifactRef)
	if err != nil {
		return err
	}
	var publication *persistence.GitPublicationLease
	if publishes, ok := provider.(port.PublicationIdentity); ok {
		destination, err := publishes.PublicationDestination(i.Binding)
		if err != nil {
			return err
		}
		owner, _ := persistence.OwnerFrom(ctx)
		l := persistence.GitPublicationLease{Destination: destination, OrganizationID: record.OrganizationID, Owner: owner}
		if err := c.Store.ClaimGitPublication(ctx, l); err != nil {
			return err
		}
		publication = &l
	}
	previousVersion := record.Mutation.Version
	record.Mutation, err = record.Mutation.Advance(domain.IntentRunning, false)
	if err != nil {
		return err
	}
	record.UpdatedAt = time.Now().UTC()
	record, err = c.Store.UpdateDeliveryExecution(ctx, record, previousVersion)
	if err != nil {
		return err
	}
	effectPending := true
	defer func() {
		if effectPending {
			persistence.MarkHold(ctx)
		}
	}()
	receipt, err := provider.Submit(ctx, i, port.Prepared{Revision: record.Revision, Payload: raw})
	if err != nil {
		// Even a failed push can have reached the remote. Preserve mutation and lease
		// for explicit observation/recovery, never automatically replay.
		old := record.Mutation.Version
		record.Mutation, _ = record.Mutation.Advance(domain.IntentUnknown, false)
		record.UpdatedAt = time.Now().UTC()
		_, _ = c.Store.UpdateDeliveryExecution(ctx, record, old)
		persistence.MarkHold(ctx)
		return err
	}
	record.Receipt = receipt
	// Persist the receipt before polling. Same-state updates keep its version and
	// are guarded by the same owner; no external effect is repeated by observation.
	record.UpdatedAt = time.Now().UTC()
	record, err = c.Store.UpdateDeliveryExecution(ctx, record, record.Mutation.Version)
	if err != nil {
		persistence.MarkHold(ctx)
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
			for _, object := range observed.Objects {
				record.Inventory = append(record.Inventory, persistence.DeliveryObject{ObjectRef: execution.ObjectRef{APIVersion: object.APIVersion, Kind: object.Kind, Name: object.Name}, UID: object.UID, Release: receipt.Handle["release"]})
			}
			break
		}
		if observed.Health == domain.Degraded {
			return errors.New("delivery: admitted workload is degraded")
		}
		select {
		case <-wait.Done():
			return errors.New("delivery: exact revision readiness deadline exceeded")
		case <-ticker.C:
		}
	}
	// A kept object omitted by the new release still needs UID-guarded prune.
	prior, err := c.Store.ListDeliveryExecutions(ctx, record.OrganizationID, record.EnvironmentID)
	if err != nil {
		return err
	}
	currentKeys := map[string]bool{}
	for _, o := range record.Inventory {
		currentKeys[o.Key()] = true
	}
	staleByKey := map[string]port.AppliedObject{}
	var priorActive *persistence.DeliveryExecution
	for index := range prior {
		previous := &prior[index]
		if previous.UnitID == record.UnitID && previous.Mutation.ID != record.Mutation.ID && (previous.Mutation.Action == domain.Submit || previous.Mutation.Action == domain.Adopt) && previous.Mutation.Status == domain.IntentAcknowledged && (priorActive == nil || previous.CreatedAt.After(priorActive.CreatedAt)) {
			priorActive = previous
		}
	}
	if priorActive != nil {
		for _, o := range priorActive.Inventory {
			if !currentKeys[o.Key()] {
				staleByKey[o.Key()] = o
			}
		}
	}

	if len(staleByKey) > 0 {
		pruner, ok := provider.(port.GuardedPrune)
		if !ok {
			return domain.ErrUnsupported
		}
		stale := []port.AppliedObject{}
		for _, o := range staleByKey {
			stale = append(stale, o)
		}
		if err := pruner.PruneReleasedObjects(ctx, i, receipt, stale); err != nil {
			persistence.MarkHold(ctx)
			return err
		}
	}
	old := record.Mutation.Version
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

// Activate commits the applied delivery selection only after all admitted work
// has completed. Selection alone never creates this acknowledgement.
func (c *Coordinator) Activate(ctx context.Context, app string, env environment.Environment) error {
	repo, ok := c.Store.(persistence.DeliveryActivationRepository)
	if !ok {
		return ErrUnavailable
	}
	return repo.ActivateDelivery(ctx, persistence.DeliveryActivation{ApplicationKey: app, EnvironmentKey: env.Key, ConfigurationRevision: env.DeliveryConfigRevision, Selection: env.DesiredDelivery.Effective()})
}

// CheckAvailable is local preflight; Preview never contacts a controller.
func (c *Coordinator) CheckAvailable(ctx context.Context, orgKey string, env environment.Environment) error {
	_, _, err := c.binding(ctx, orgKey, env)
	return err
}

// ValidateForPreview verifies stored binding/capability pins only, with no network
// calls. It refuses switching an occupied Environment until handoff is admitted.
func (c *Coordinator) ValidateForPreview(ctx context.Context, orgKey string, env environment.Environment) error {
	if env.DesiredDelivery.Effective().Mode != domain.CD {
		return nil
	}
	_, _, err := c.binding(ctx, orgKey, env)
	return err
}
