package delivery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"orchestrator/internal/application/envops"
	"sort"
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
	Repositories *Repositories
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
	return c.resolveBinding(ctx, orgKey, env, true)
}
func (c *Coordinator) resolveBinding(ctx context.Context, orgKey string, env environment.Environment, provision bool) (domain.BindingVersion, port.Provider, error) {
	selection := env.DesiredDelivery.Effective()
	if selection.Mode == domain.Direct {
		return domain.BindingVersion{}, nil, ErrCDRequired
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
	if err == nil {
		if managed, ok := provider.(port.ApplicationRepositoryBinding); ok {
			policy, policyErr := managed.RepositoryPolicy(b)
			if policyErr != nil {
				return b, nil, policyErr
			}
			if policy.Enabled && provision {
				if c.Repositories == nil {
					return b, nil, ErrUnavailable
				}
				b, err = c.Repositories.Resolve(ctx, b, managed, env, true)
			}
		}
	}
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
	if validator, ok := provider.(port.ReceiptLayoutValidator); ok {
		environments, err := c.Store.ListEnvironments(ctx, req.ApplicationKey)
		if err != nil {
			return err
		}
		for _, env := range environments {
			executions, err := c.Store.ListDeliveryExecutions(ctx, org.ID, env.ID)
			if err != nil {
				return err
			}
			workloads := map[string]bool{}
			for _, record := range executions {
				workloads[record.WorkloadID] = true
			}
			for workload := range workloads {
				active, err := activeExecutions(executions, workload)
				if err != nil {
					return err
				}
				for _, record := range active {
					if err := validator.ValidateDeliveryReceipt(b, record.Receipt); err != nil {
						return err
					}
				}
			}
		}
	}
	phases := map[domain.Phase][]execution.Manifest{}
	for _, m := range req.Manifests {
		if m.Secret || (m.Kind == "Secret" && !execution.PublicCertificateSecret(m.Object)) {
			return domain.ErrUnsupported
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
		// Only the latest acknowledged, still-active unit owns a source.
		// A newer withdrawal must fence out older successful Submit receipts.
		for _, e := range active {
			if e.UnitID == unit {
				intent.ExpectedRevision = e.Revision.ID
				intent.SourceInventory = append([]port.AppliedObject(nil), e.Inventory...)
			}
		}
		// UC-06 CD-10: a newer determined failure that was published now holds
		// the provider pin. The next revision replaces it explicitly; the source
		// inventory stays that of the last acknowledged revision.
		if failed := publishedFailure(existing, unit, active); failed != nil {
			intent.ExpectedRevision = failed.Revision.ID
		}

		if err := c.runPhase(ctx, provider, intent, unit, req.WorkloadID, manifests); err != nil {
			return err
		}
	}
	return nil
}

// publishedFailure returns the newest Submit of unit when it is a determined
// failure with a persisted receipt and newer than the unit's acknowledged
// source, or nil. Failed submissions proven never published are skipped: they
// never held the provider pin.
func publishedFailure(executions []persistence.DeliveryExecution, unit string, active []persistence.DeliveryExecution) *persistence.DeliveryExecution {
	candidates := []*persistence.DeliveryExecution{}
	for index := range executions {
		if executions[index].UnitID == unit {
			candidates = append(candidates, &executions[index])
		}
	}
	sort.Slice(candidates, func(a, b int) bool { return candidates[a].CreatedAt.After(candidates[b].CreatedAt) })
	for _, e := range candidates {
		published := e.Receipt.IntentID == e.Mutation.ID && e.Receipt.RevisionID == e.Revision.ID
		if e.Mutation.Action == domain.Submit && e.Mutation.Status == domain.IntentFailed && !published {
			continue
		}
		if e.Mutation.Action != domain.Submit || e.Mutation.Status != domain.IntentFailed {
			return nil
		}
		for _, source := range active {
			if source.UnitID == unit && !e.CreatedAt.After(source.CreatedAt) {
				return nil
			}
		}
		return e
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
	return c.runPhaseAccepting(ctx, provider, i, unit, workload, manifests, nil)
}

// runPhaseAccepting is runPhase with an optional acceptance of a not fully
// Ready revision (ADR-042 secret preparation): accept reports whether the
// observation is sufficient for this revision's purpose.
func (c *Coordinator) runPhaseAccepting(ctx context.Context, provider port.Provider, i port.Intent, unit, workload string, manifests []execution.Manifest, accept func(domain.Observation) bool) error {
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
		deadline = DeployReadinessDeadline
	}
	wait, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	var last domain.Observation
	// UC-06 CD-11: wait reasons and stuck Pod replacements of this unit.
	recordWaits := envops.WaitRecorder(ctx, RolloutDetailPrefix+unit, "")
	recovery, _ := provider.(port.StuckPodRecovery)
	for {
		observed, err := provider.Observe(wait, i, receipt)
		if len(observed.Objects) > 0 {
			last = observed
		}
		deadlineFailure := func(err error) error {
			if wait.Err() != nil && ctx.Err() == nil {
				// The call was cut off by the expired readiness deadline.
				record.Inventory = observedInventory(last, receipt)
				return &observedFailure{cause: c.failRollout(ctx, i, &record, publication, &effectPending, ErrRolloutNotReady), observation: last}
			}
			return err
		}
		if err != nil {
			return deadlineFailure(err)
		}
		if observed.Ready(i.Revision, time.Now().UTC(), 30*time.Second) {
			record.Inventory = []persistence.DeliveryObject{}
			for _, object := range observed.Objects {
				record.Inventory = append(record.Inventory, persistence.DeliveryObject{ObjectRef: execution.ObjectRef{APIVersion: object.APIVersion, Kind: object.Kind, Name: object.Name}, UID: object.UID, Release: receipt.Handle["release"]})
			}
			break
		}
		if accept != nil && accept(observed) {
			// Keep the active inventory and add every observed identity, so
			// a later revision still prunes what this one did not observe.
			record.Inventory = mergeInventory(i.SourceInventory, observedInventory(observed, receipt))
			break
		}
		if observed.Health == domain.Degraded {
			// UC-06 CD-10: the receipted revision was observed failing. This
			// process is the only writer, so the outcome is known, not held.
			// Fleet's aggregate failure carries no object list; the last
			// object-level observation names the failing workload.
			observed.Objects = append(observed.Objects, last.Objects...)
			record.Inventory = observedInventory(observed, receipt)
			return &observedFailure{cause: c.failRollout(ctx, i, &record, publication, &effectPending, ErrRolloutDegraded), observation: observed}
		}
		if recovery != nil {
			if stuck, err := recovery.RecoverStuckPods(wait, i, receipt); err == nil {
				for k := range stuck.Replaced {
					recordWaits(stuck.Waiting, &stuck.Replaced[k])
				}
				recordWaits(stuck.Waiting, nil)
			}
		}
		if observed.Sync == domain.Pending && observed.Health == domain.Progressing {
			if convergence, ok := provider.(port.PendingConvergence); ok {
				if err := convergence.ConvergePending(wait, i, receipt); err != nil {
					return deadlineFailure(err)
				}
			}
		}
		if err := c.prunePending(wait, provider, i, receipt, observed); err != nil {
			return deadlineFailure(err)
		}
		select {
		case <-wait.Done():
			if ctx.Err() != nil {
				// The owning operation was cancelled or lost: outcome unknown.
				return ctx.Err()
			}
			record.Inventory = observedInventory(last, receipt)
			return &observedFailure{cause: c.failRollout(ctx, i, &record, publication, &effectPending, ErrRolloutNotReady), observation: last}
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
	// Objects that only a published, failed revision created (for example a
	// new versioned secret) are not in any acknowledged inventory; their
	// observed UIDs were recorded with the failure (ADR-042).
	for index := range prior {
		failed := &prior[index]
		if failed.UnitID != record.UnitID || failed.Mutation.ID == record.Mutation.ID || failed.Mutation.Action != domain.Submit || failed.Mutation.Status != domain.IntentFailed || !published(*failed) {
			continue
		}
		if priorActive != nil && !failed.CreatedAt.After(priorActive.CreatedAt) {
			continue
		}
		for _, o := range failed.Inventory {
			if _, known := staleByKey[o.Key()]; !known && !currentKeys[o.Key()] && o.UID != "" {
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
	// ADR-042: objects of this release that no recorded inventory covers (for
	// example created by a failed revision) are swept once this one is Ready.
	if sweeper, ok := provider.(port.ReleaseSweep); ok && accept == nil && (i.Revision.Scope.UnitKey == domain.EnvironmentAppUnit || i.Revision.Scope.UnitKey == domain.EnvironmentMigrateUnit) {
		if _, err := sweeper.SweepRelease(ctx, i, receipt); err != nil {
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

// DeployReadinessDeadline bounds waiting for an exact revision to become Ready
// in Deploy and in Deploy recovery (UC-06 CD-10).
const DeployReadinessDeadline = 5 * time.Minute

// Determined rollout failures (UC-06 CD-10). Their messages are unchanged so
// existing diagnostics and logs keep matching.
var (
	ErrRolloutDegraded = errors.New("delivery: admitted workload is degraded")
	// RolloutDetailPrefix keys the safe rollout wait state of one delivery
	// unit in the owning Environment operation detail (issue #68).
	RolloutDetailPrefix = "rollout:"
	ErrRolloutNotReady  = errors.New("delivery: exact revision readiness deadline exceeded")
)

// determinedFailureAllowed reports whether the owning operation is an ordinary
// Deploy. CD ownership transfer and policy-only delivery keep the hold, because
// they need every unit acknowledged before the transition can continue.
func (c *Coordinator) determinedFailureAllowed(ctx context.Context, i port.Intent) bool {
	if i.PolicyOnly {
		return false
	}
	owner, ok := persistence.OwnerFrom(ctx)
	if !ok {
		return false
	}
	op, err := c.Store.GetOperation(ctx, owner.OperationID)
	if err != nil || op.Kind != environment.OpDeploy || op.Stage == "DELIVERY_POLICY" {
		return false
	}
	policyOnly, _ := op.Detail["deliveryPolicyOnly"].(bool)
	transfer, _ := op.Detail["deliveryTransfer"].(bool)
	return !policyOnly && !transfer
}

// failRollout records a determined rollout failure: the receipted intent
// becomes FAILED and its Git publication lease is released, so the Environment
// can be released. Any persistence error leaves the hold in place.
func (c *Coordinator) failRollout(ctx context.Context, i port.Intent, record *persistence.DeliveryExecution, publication *persistence.GitPublicationLease, effectPending *bool, cause error) error {
	if !c.determinedFailureAllowed(ctx, i) {
		return cause
	}
	old := record.Mutation.Version
	next, err := record.Mutation.Advance(domain.IntentFailed, true)
	if err != nil {
		return cause
	}
	updated := *record
	updated.Mutation = next
	updated.UpdatedAt = time.Now().UTC()
	if err := c.Store.Transact(ctx, func(tx context.Context) error {
		if _, err := c.Store.UpdateDeliveryExecution(tx, updated, old); err != nil {
			return err
		}
		if publication != nil {
			return c.Store.ReleaseGitPublication(tx, *publication)
		}
		return nil
	}); err != nil {
		return cause
	}
	*record = updated
	*effectPending = false
	return cause
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
	_, _, err := c.resolveBinding(ctx, orgKey, env, false)
	return err
}

// observedInventory records the objects and UIDs a failing revision was
// observed with, so a later successful revision can prune what only it created.
func observedInventory(observed domain.Observation, receipt port.Receipt) []persistence.DeliveryObject {
	out := []persistence.DeliveryObject{}
	for _, object := range observed.Objects {
		if object.UID == "" {
			continue
		}
		out = append(out, persistence.DeliveryObject{ObjectRef: execution.ObjectRef{APIVersion: object.APIVersion, Kind: object.Kind, Name: object.Name}, UID: object.UID, Release: receipt.Handle["release"]})
	}
	return out
}

func mergeInventory(base, observed []persistence.DeliveryObject) []persistence.DeliveryObject {
	out := []persistence.DeliveryObject{}
	index := map[string]int{}
	for _, list := range [][]persistence.DeliveryObject{base, observed} {
		for _, o := range list {
			if at, ok := index[o.Key()]; ok {
				out[at] = o
				continue
			}
			index[o.Key()] = len(out)
			out = append(out, o)
		}
	}
	return out
}
