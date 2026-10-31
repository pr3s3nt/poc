// Package deployment implements UC-06 (deploy workload) and UC-09 (deployment view).
package deployment

import (
	"context"
	"errors"
	"fmt"
	"log"
	catalogapp "orchestrator/internal/application/catalogrelease"
	"sort"
	"strings"

	deliveryapp "orchestrator/internal/application/delivery"
	"orchestrator/internal/application/deploymentreview"
	"orchestrator/internal/application/envops"
	"orchestrator/internal/application/provisioning"
	"orchestrator/internal/application/workloadconfig"
	appdomain "orchestrator/internal/domain/application"
	"orchestrator/internal/domain/configuration"
	deliverydomain "orchestrator/internal/domain/delivery"
	domain "orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning"
	"orchestrator/internal/planning/placeholder"
	"orchestrator/internal/planning/score"
	"orchestrator/internal/platform/canon"
	"orchestrator/internal/platform/clock"
	"orchestrator/internal/platform/ids"
	configport "orchestrator/internal/ports/configuration"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/ports/persistence"
)

// DeployCommand is the UC-06 system operation input (OC-08).
type DeployCommand struct {
	OrganizationKey  string
	ApplicationKey   string
	EnvironmentKey   string
	WorkloadID       string
	ScoreBefore      map[string]any
	ScoreAfter       map[string]any
	Actor            string
	Action           domain.Action
	RunID            string
	ReviewRequestID  string
	ExpectedPlanHash string
	ConfigRevisionID string
	// DeferPublicRoutes is set only by the UC-16 pending batch, whose final
	// Environment state Preview validated: routes reconcile after the batch
	// and this step may plan an intermediate Environment state.
	DeferPublicRoutes bool
	// AcceptLiterals is set only by the UC-16 pending batch when ScoreAfter is
	// a saved, validated draft: its ordinary variable values are then recorded
	// as proven in the resulting set (UC-16 BR-23). A direct Score deploy never
	// sets it, so a literal it introduces or changes stays unreadable.
	AcceptLiterals bool
}

// DeployResult is returned to the actor at UC-06 MS-13.
type DeployResult struct {
	DeploymentID string        `json:"deploymentId"`
	Status       domain.Status `json:"status"`
	PlanHash     string        `json:"planHash"`
	WorkloadID   string        `json:"workloadId"`
}

// Service orchestrates plan, provision, render, apply and commit.
type Service struct {
	delivery        *deliveryapp.Coordinator
	store           persistence.Store
	planner         *planning.Service
	provisioning    *provisioning.Service
	renderer        execution.WorkloadRenderer
	deployer        execution.WorkloadDeployer
	terraform       planning.ModuleInspector
	clock           clock.Clock
	registry        configport.Registry
	ops             *envops.Manager
	imagePullSecret string
	configSync      execution.ConfigSecretSynchronizer
	publicRoutes    execution.PublicRouteManager
	baseDomain      string
	instanceMarker  InstanceMarker
	reviewer        deploymentreview.Assembler
}

func (s *Service) SetDelivery(c *deliveryapp.Coordinator) { s.delivery = c }
func (s *Service) UpdateDeliveryPolicy(ctx context.Context, orgKey string, env environment.Environment, stage func(string, map[string]any) error) error {
	if s.delivery == nil {
		return deliveryapp.ErrUnavailable
	}
	return s.delivery.UpdatePolicy(ctx, orgKey, env, stage)
}
func (s *Service) ResumeDeliveryPolicy(ctx context.Context, orgKey string, env environment.Environment, stage func(string, map[string]any) error) error {
	if s.delivery == nil {
		return deliveryapp.ErrUnavailable
	}
	if err := s.delivery.RecoverSubmissions(ctx, orgKey, env); err != nil {
		return err
	}
	return s.delivery.UpdatePolicy(ctx, orgKey, env, stage)
}
func (s *Service) TransferDelivery(ctx context.Context, orgKey string, env environment.Environment, stage func(string, map[string]any) error) error {
	if s.delivery == nil {
		return deliveryapp.ErrUnavailable
	}
	return s.delivery.Transfer(ctx, orgKey, env, stage)
}

func (s *Service) ResumeDeliveryTransfer(ctx context.Context, orgKey string, env environment.Environment, stage func(string, map[string]any) error) error {
	if s.delivery == nil {
		return deliveryapp.ErrUnavailable
	}
	return s.delivery.ResumeTransfer(ctx, orgKey, env, stage)
}

// ActivateDelivery is called by the pending batch only after routes and every
// workload completed; a partially successful batch retains the applied manager.
func (s *Service) ActivateDelivery(ctx context.Context, appKey, envKey string) error {
	env, err := s.store.GetEnvironment(ctx, appKey, envKey)
	if err != nil {
		return err
	}
	if s.delivery == nil {
		return deliveryapp.ErrUnavailable
	}
	return s.delivery.Activate(ctx, appKey, env)
}

func (s *Service) SetReviewer(reviewer deploymentreview.Assembler) { s.reviewer = reviewer }

// InstanceMarker mirrors a committed UNREFERENCED Active Resource onto the
// Terraform resource instance of the same logical identity (implemented by
// the Terraform executor). It is optional: without Terraform instances there
// is nothing to mirror.
type InstanceMarker interface {
	MarkUnreferenced(ctx context.Context, generation int64, descriptor, scopeType, scopeID string) error
}

// SetInstanceMarker enables the instance-side UNREFERENCED mirror.
func (s *Service) SetInstanceMarker(m InstanceMarker) { s.instanceMarker = m }

func (s *Service) SetStoreRegistry(registry configport.Registry) { s.registry = registry }

// SetOperations shares the Environment claim manager with the other services.
func (s *Service) SetOperations(ops *envops.Manager) { s.ops = ops }

func (s *Service) operations() *envops.Manager {
	if s.ops == nil {
		s.ops = envops.NewManager(s.store)
	}
	return s.ops
}
func (s *Service) SetImagePullSecret(name string) { s.imagePullSecret = name }
func (s *Service) SetConfigSecretSynchronizer(sync execution.ConfigSecretSynchronizer) {
	s.configSync = sync
}
func (s *Service) SetPublicRouteManager(manager execution.PublicRouteManager, baseDomain string) {
	s.publicRoutes, s.baseDomain = manager, baseDomain
}

// NewService wires UC-06 with its collaborators.
func NewService(
	store persistence.Store,
	planner *planning.Service,
	prov *provisioning.Service,
	renderer execution.WorkloadRenderer,
	deployer execution.WorkloadDeployer,
	terraform planning.ModuleInspector,
	c clock.Clock,
) *Service {
	return &Service{
		store: store, planner: planner, provisioning: prov,
		renderer: renderer, deployer: deployer, terraform: terraform, clock: c,
	}
}

// DeployWorkload runs the full UC-06 main success scenario. A caller that
// already holds the Environment claim passes a context carrying it (batch,
// transition, recovery); a direct deploy claims the Environment itself,
// atomically with its snapshot pins and before any side effect.
func (s *Service) DeployWorkload(ctx context.Context, cmd DeployCommand) (*DeployResult, error) {
	if persistence.OperationFrom(ctx) != "" {
		return s.deployWorkload(ctx, cmd)
	}
	snapshot, err := LoadPlanningSnapshot(ctx, s.store, cmd.OrganizationKey, cmd.ApplicationKey, cmd.EnvironmentKey)
	if err != nil {
		return nil, err
	}
	lease, err := s.operations().Begin(ctx, envops.Claim{
		ApplicationKey: cmd.ApplicationKey, EnvironmentKey: cmd.EnvironmentKey, Kind: environment.OpDeploy,
		ReviewRequestID: cmd.ReviewRequestID,
		Pins: environment.OperationPins{
			CheckEnvVersion: true, EnvVersion: snapshot.Env.Version,
			CheckSet: true, CurrentSetID: snapshot.Env.CurrentDeploymentSetID,
			CheckBinding: true, Binding: snapshot.Env.Binding(),
		},
		Detail: map[string]any{"workload": cmd.WorkloadID},
	})
	if err != nil {
		return nil, err
	}
	result, err := s.deployWorkload(lease.Ctx, cmd)
	if err != nil {
		_ = lease.End(environment.OpFailed, PublicFailure(err))
		return nil, err
	}
	if endErr := lease.End(environment.OpSucceeded, ""); endErr != nil {
		return result, endErr
	}
	return result, nil
}

func (s *Service) deployWorkload(ctx context.Context, cmd DeployCommand) (*DeployResult, error) {
	if opID := persistence.OperationFrom(ctx); opID != "" {
		op, err := s.store.GetOperation(ctx, opID)
		if err != nil {
			return nil, err
		}
		if op.ApplicationKey != cmd.ApplicationKey || op.EnvironmentKey != cmd.EnvironmentKey || op.Kind != environment.OpDeploy || (cmd.ReviewRequestID != "" && !strings.EqualFold(cmd.ReviewRequestID, op.ReviewRequestID)) {
			return nil, persistence.ErrOperationLost
		}
		cmd.ReviewRequestID = op.ReviewRequestID
	}
	// MS-01: read the current Deployment Set, connection, catalog and Active
	// Resources from one consistent snapshot shared with UC-05 Preview.
	snapshot, err := LoadPlanningSnapshotWithBaselines(ctx, s.store, cmd.OrganizationKey, cmd.ApplicationKey, cmd.EnvironmentKey)
	if err != nil {
		return nil, err
	}
	app, env := snapshot.App, snapshot.Env
	baseSet, baseSetID := snapshot.BaseSet, snapshot.BaseSetID

	after, err := parseScore(cmd.ScoreAfter)
	if err != nil {
		return nil, err
	}
	before, err := parseScore(cmd.ScoreBefore)
	if err != nil {
		return nil, err
	}
	workloadID := cmd.WorkloadID
	if workloadID == "" && after != nil {
		workloadID = after.Metadata.Name
	}
	action := cmd.Action
	if action == "" {
		if after == nil {
			action = domain.ActionRemove
		} else if _, exists := baseSet.Modules[workloadID]; exists {
			action = domain.ActionUpdate
		} else {
			action = domain.ActionDeploy
		}
	}

	now := s.clock.Now()
	record := domain.Deployment{
		ID:                     ids.New(),
		ReviewRequestID:        cmd.ReviewRequestID,
		EnvironmentID:          env.ID,
		OrganizationKey:        cmd.OrganizationKey,
		ApplicationKey:         app.Key,
		EnvironmentKey:         env.Key,
		TargetGeneration:       env.TargetGeneration,
		Action:                 action,
		WorkloadID:             workloadID,
		ActorRef:               cmd.Actor,
		Status:                 domain.StatusPlanning,
		BaseEnvironmentVersion: env.Version,
		BaseDeploymentSetID:    baseSetID,
		StartedAt:              now,
	}
	if err := s.store.SaveDeployment(ctx, record); err != nil {
		return nil, err
	}

	planCtx := planning.Context{
		OrganizationKey: cmd.OrganizationKey,
		App:             app,
		Env:             env,
		DeploymentID:    record.ID,
		RunID:           cmd.RunID,
	}

	catalog, types, definitions, active := snapshot.Catalog, snapshot.Types, snapshot.Definitions, snapshot.Active

	// MS-02..MS-08: planning pipeline.
	plan, err := s.planner.Plan(planning.Request{
		OrganizationKey: cmd.OrganizationKey,
		App:             app,
		Env:             env,
		BaseSet:         baseSet,
		Before:          before,
		After:           after,
		WorkloadID:      workloadID,
		RunID:           cmd.RunID,
		Action:          action,
		Catalog:         catalog,
		Active:          active,
		Terraform:       s.terraform,
		// Only the pending batch sets DeferPublicRoutes, after validating its
		// final Environment state; direct deploys keep full validation.
		AllowIntermediateEnvironmentState: cmd.DeferPublicRoutes,
	})
	if err != nil {
		return nil, s.fail(ctx, record, err)
	}
	if snapshot.CatalogPending {
		if err := catalogapp.ValidateDataUpdates(plan, snapshot.Baselines.Reader(), snapshot.Types); err != nil {
			return nil, s.fail(ctx, record, err)
		}
	}
	if cmd.ExpectedPlanHash != "" && plan.PlanHash != cmd.ExpectedPlanHash {
		return nil, s.fail(ctx, record, ErrStalePlan)
	}
	plan.Review = s.reviewer.Assemble(ctx, plan, plan.ResourceIntents(snapshot.Baselines.Reader()), snapshot.ReviewBaselines)

	if validator, ok := s.renderer.(execution.RenderPreflight); ok {
		for _, selection := range plan.Rendering {
			if err := validator.ValidateSelection(ctx, selection); err != nil {
				return nil, s.fail(ctx, record, err)
			}
		}
	} else if len(plan.Rendering) > 0 {
		return nil, s.fail(ctx, record, fmt.Errorf("deployment: selected renderer is unavailable"))
	}
	candidateSet := environment.DeploymentSet{
		ID:                    ids.New(),
		EnvironmentID:         env.ID,
		EnvironmentKey:        env.ApplicationKey + "/" + env.Key,
		CreatedByDeploymentID: record.ID,
		Document:              plan.CandidateSet,
		CreatedAt:             now,
	}
	accepted := ""
	if cmd.AcceptLiterals && after != nil {
		accepted = workloadID
	}
	candidateSet.LiteralProofs = workloadconfig.ProveLiterals(snapshot.BaseProofs, plan.CandidateSet, accepted)
	candidateSet.ResourceAliases = workloadconfig.KeepResourceAliases(snapshot.BaseAliases, plan.CandidateSet, after)
	candidateSet.DocumentHash, err = canon.Hash(plan.CandidateSet)
	if err != nil {
		return nil, s.fail(ctx, record, err)
	}
	deltaSnapshot, err := domain.NewDeploymentDeltaSnapshot(ids.New(), record.ID, plan.Delta, domain.DeltaSnapshotMetadata{
		ActorRef:   cmd.Actor,
		Action:     action,
		WorkloadID: workloadID,
	}, now)
	if err != nil {
		return nil, s.fail(ctx, record, err)
	}
	planSnapshot, err := canon.Map(plan)
	if err != nil {
		return nil, s.fail(ctx, record, err)
	}

	// Transaction A: Deployment + Delta Snapshot + Candidate Set + plan (OC-08).
	// The failure path must not reference rows the rolled-back transaction
	// never wrote, so the record only advances once the transaction commits.
	planned := record
	planned.CandidateDeploymentSet = candidateSet.ID
	planned.DeltaSnapshotID = deltaSnapshot.ID
	planned.Status = domain.StatusProvisioning
	if err := s.store.Transact(ctx, func(ctx context.Context) error {
		if err := s.store.SaveDeltaSnapshot(ctx, deltaSnapshot); err != nil {
			return err
		}
		if err := s.store.SaveDeploymentSet(ctx, candidateSet); err != nil {
			return err
		}
		if err := s.store.SavePlan(ctx, record.ID, planSnapshot); err != nil {
			return err
		}
		return s.store.SaveDeployment(ctx, planned)
	}); err != nil {
		return nil, s.fail(ctx, record, err)
	}
	record = planned

	if after != nil {
		if err := s.checkAppliedPlacement(ctx, planCtx, plan, workloadID); err != nil {
			return nil, s.fail(ctx, record, err)
		}
	}
	// Secret delivery must be usable before any infrastructure is touched.
	if err := s.preflightSecretDelivery(ctx, cmd, env, plan, workloadID, after != nil); err != nil {
		return nil, s.fail(ctx, record, err)
	}

	if after != nil && env.DesiredDelivery.Effective().Mode == deliverydomain.CD {
		if s.delivery == nil {
			return nil, s.fail(ctx, record, deliveryapp.ErrUnavailable)
		}
		if err := s.delivery.CheckAvailable(ctx, cmd.OrganizationKey, env); err != nil {
			return nil, s.fail(ctx, record, err)
		}
		instances, err := s.store.ListWorkloadInstancesFor(ctx, app.Key+"/"+env.Key, env.TargetGeneration)
		if err != nil {
			return nil, s.fail(ctx, record, err)
		}
		for _, instance := range instances {
			if instance.Status != domain.InstanceRemoved && instance.TargetRef["deliveryMode"] != "CD" {
				return nil, s.fail(ctx, record, deliverydomain.ErrUnsupported)
			}
		}
	}
	// An applied CD controller retains authority until an explicit handoff has
	// stopped every source unit. A desired toggle never authorizes direct writes
	// or a different binding against those objects.
	if after != nil && env.AppliedDelivery.Effective().Mode == deliverydomain.CD {
		requiresTransfer, err := deliverydomain.RequiresTransfer(env.AppliedDelivery, env.DesiredDelivery, true)
		if err != nil || requiresTransfer {
			return nil, s.fail(ctx, record, deliverydomain.ErrUnsupported)
		}
	}
	// MS-09: UC-08 provisions resource-only batches.
	// UC-06 owns the combined lifecycle order: resources that depend on a workload
	// (plan.AfterWorkloads) run after that workload is applied and Ready.
	provisionRequest := provisioning.Request{
		DeploymentID: record.ID,
		RunID:        cmd.RunID,
		Context:      planCtx,
		Plan:         plan,
		Types:        types,
		Definitions:  definitions,
		Select:       func(descriptor string) bool { _, post := plan.AfterWorkloads[descriptor]; return !post },
	}
	provisionResult, err := s.provisioning.Provision(ctx, provisionRequest)
	if err != nil {
		return nil, s.fail(ctx, record, err)
	}

	result := &DeployResult{DeploymentID: record.ID, PlanHash: plan.PlanHash, WorkloadID: workloadID}
	record.Status = domain.StatusDeploying
	if err := s.store.SaveDeployment(ctx, record); err != nil {
		return nil, s.fail(ctx, record, err)
	}

	if after != nil {
		if err := s.applyWorkload(ctx, &record, planCtx, plan, provisionResult, types, workloadID, cmd.ConfigRevisionID); err != nil {
			return nil, s.fail(ctx, record, err)
		}
		if err := s.provisionPostWorkload(ctx, provisionRequest, provisionResult, workloadID, env); err != nil {
			return nil, s.fail(ctx, record, err)
		}
	} else {
		if err := s.removeWorkload(ctx, planCtx, workloadID, record.ID); err != nil {
			return nil, s.fail(ctx, record, err)
		}
	}
	if !cmd.DeferPublicRoutes {
		if err := s.reconcileRoutes(ctx, app, env, plan.CandidateSet); err != nil {
			return nil, s.fail(ctx, record, err)
		}
	}

	// Transaction B: optimistic commit of the current Deployment Set (MS-12).
	if err := s.store.Transact(ctx, func(ctx context.Context) error {
		if err := s.store.CompareVersionAndSetCurrent(ctx, app.Key, env.Key, record.BaseEnvironmentVersion, candidateSet.ID); err != nil {
			return err
		}
		if env.DesiredDelivery.Effective().Mode == deliverydomain.CD && !cmd.DeferPublicRoutes {
			if err := s.delivery.Activate(ctx, app.Key, env); err != nil {
				return err
			}
		}
		if err := s.markUnreferenced(ctx, cmd.OrganizationKey, record.ID, plan.UnreferencedResources); err != nil {
			return err
		}
		finished := s.clock.Now()
		record.Status = domain.StatusSucceeded
		record.FinishedAt = &finished
		return s.store.SaveDeployment(ctx, record)
	}); err != nil {
		return nil, s.fail(ctx, record, err)
	}

	if failed := s.markInstancesUnreferenced(ctx, env.TargetGeneration, plan.UnreferencedResources); failed > 0 {
		// Safe diagnostics only: counts, never references, descriptors or error text. The
		// committed Deployment and Active Resource history stay as they are; the affected
		// resources are refused for deletion as METADATA_SYNC_PENDING until the State
		// projection is repaired by an operator. There is no automatic recovery.
		log.Printf("deployment: resource metadata sync incomplete (deployment %s, %d of %d unreferenced instances not mirrored)", record.ID, failed, len(plan.UnreferencedResources))
	}

	result.Status = record.Status
	return result, nil
}

// markInstancesUnreferenced mirrors the committed Active Resource marker onto
// the Terraform instances of the same logical identity (the State login owns
// that status, so it cannot be part of the Console transaction). It runs under
// the Deployment's own Environment claim and is best effort: an instance left
// READY just stays ineligible for deletion (METADATA_SYNC_PENDING), which fails closed;
// the number of failures is returned and logged without any reference.
func (s *Service) markInstancesUnreferenced(ctx context.Context, generation int64, unreferenced []resource.ActiveResource) (failed int) {
	if s.instanceMarker == nil {
		return 0
	}
	for _, planned := range unreferenced {
		if err := s.instanceMarker.MarkUnreferenced(ctx, generation, planned.Descriptor.String(), string(planned.Scope.Type), planned.Scope.ID); err != nil {
			failed++
		}
	}
	return failed
}

// markUnreferenced records UC-07 MS-07 inside the final transaction: each
// Active Resource the plan no longer references moves READY -> UNREFERENCED.
// It is a metadata update only; nothing is destroyed. The record is re-read
// in the transaction so identity, outputs, executor state and fingerprint are
// kept, and resources in any other state are left to their own transitions.
func (s *Service) markUnreferenced(ctx context.Context, organizationKey, deploymentID string, unreferenced []resource.ActiveResource) error {
	for _, planned := range unreferenced {
		if planned.OrganizationKey != organizationKey {
			continue
		}
		current, err := s.store.FindByLogicalIdentity(ctx, organizationKey, planned.Descriptor, planned.Scope)
		if errors.Is(err, persistence.ErrNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		if current.Status != resource.StatusReady {
			continue
		}
		current.Status = resource.StatusUnreferenced
		current.LastDeploymentID = deploymentID
		if _, err := s.store.UpsertActiveResource(ctx, current); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) removeWorkload(ctx context.Context, planCtx planning.Context, workloadID, deploymentID string) error {
	instances, err := s.store.ListWorkloadInstancesFor(ctx, planCtx.App.Key+"/"+planCtx.Env.Key, planCtx.Env.TargetGeneration)
	if err != nil {
		return err
	}
	for _, instance := range instances {
		if instance.WorkloadID != workloadID {
			continue
		}
		ownerDeploymentID := workloadOwnerDeploymentID(instance)
		target := execution.Target{Namespace: planCtx.Env.Namespace(), Extra: map[string]string{"application": planCtx.App.Key, "environment": planCtx.Env.Key, "deployment": ownerDeploymentID}}
		if err := restoreEnvironmentTarget(&target, instance.TargetRef, planCtx.OrganizationKey, planCtx.Env); err != nil {
			return err
		}
		instance.Status = domain.InstanceRemoving
		instance.TargetRef = withWorkloadOwner(instance.TargetRef, ownerDeploymentID)
		// The progress snapshot belongs to this removal deployment, while the
		// renderer objects still belong to the deployment recorded in TargetRef.
		// Keep both identities so a failed/unknown delete can be retried safely.
		instance.LastDeploymentID = deploymentID
		instance.ObservedAt = s.clock.Now()
		if err := s.store.UpsertWorkloadProgress(ctx, instance); err != nil {
			return err
		}
		if instance.TargetRef["deliveryMode"] == "CD" || instance.TargetRef["deliveryMode"] == "DIRECT" {
			if s.delivery == nil {
				return deliveryapp.ErrUnavailable
			}
			if err := s.delivery.RemoveWorkload(ctx, planCtx.OrganizationKey, planCtx.Env.ID, workloadID); err != nil {
				return err
			}
			// Auxiliary renderer Secrets are applied directly and excluded from
			// Git. Retire them only after the workload controller is stopped.
			if err := s.deployer.Remove(ctx, target, workloadID); err != nil {
				return err
			}
		} else {
			if err := s.removeObjects(ctx, target, workloadID, instanceInventory(instance)); err != nil {
				instance.Status = domain.InstanceFailed
				_ = s.store.UpsertWorkloadProgress(ctx, instance)
				return err
			}
			if err := s.deployer.Remove(ctx, target, workloadID); err != nil {
				instance.Status = domain.InstanceFailed
				_ = s.store.UpsertWorkloadProgress(ctx, instance)
				return err
			}
		}

		// The workload's secret delivery objects go with it (UC-07 VSO-01);
		// an unverified removal keeps the instance failed for a retry.
		if remover, ok := s.configSync.(execution.WorkloadSecretRemover); ok {
			if _, err := remover.RemoveWorkloadSynchronized(ctx, target, planCtx.App.Key, planCtx.Env.Key, workloadID); err != nil {
				instance.Status = domain.InstanceFailed
				_ = s.store.UpsertWorkloadProgress(ctx, instance)
				return err
			}
		}
		instance.Status = domain.InstanceRemoved
		instance.AppliedConfigRevisionID = ""
		instance.ObservedAt = s.clock.Now()
		return s.store.UpsertWorkloadProgress(ctx, instance)
	}
	return fmt.Errorf("deployment: workload %q has no applied instance", workloadID)
}

// applyWorkload resolves bindings, renders manifests and applies them (MS-10, MS-11).
func (s *Service) applyWorkload(
	ctx context.Context,
	record *domain.Deployment,
	planCtx planning.Context,
	plan *planning.Plan,
	provisionResult *provisioning.Result,
	types map[string]resource.Type,
	workloadID string,
	configRevisionID string,
) error {
	var node planning.Node
	found := false
	for _, candidate := range plan.Graph.Nodes {
		if candidate.Kind == planning.NodeWorkload && candidate.WorkloadID == workloadID {
			node, found = candidate, true
			break
		}
	}
	if !found {
		return fmt.Errorf("deployment: workload %q is not in the plan graph", workloadID)
	}
	wd := node.Descriptor
	module, ok := plan.CandidateSet.Modules[workloadID]
	if !ok {
		return fmt.Errorf("deployment: workload %q is not in the candidate deployment set", workloadID)
	}
	var target execution.Target
	var err error
	if planCtx.DefinitionPlaced() {
		// ADR-016: the workload's own typed references decide the target.
		target, err = provisioning.ResolveWorkloadTarget(plan, provisionResult, wd, planCtx.OrganizationKey)
		if err != nil {
			return err
		}
	} else {
		target = provisioning.ResolveTarget(plan.Graph, provisionResult, wd)
	}
	target.Extra = map[string]string{"application": planCtx.App.Key, "environment": planCtx.Env.Key, "deployment": record.ID}
	if target.Namespace == "" {
		return fmt.Errorf("deployment: workload %q has no resolved namespace", workloadID)
	}
	if target.CredentialBacked() && target.Organization != planCtx.OrganizationKey {
		return fmt.Errorf("deployment: workload %q target belongs to another organization", workloadID)
	}
	var revision configuration.Revision
	if configRevisionID != "" {
		revision, err = s.store.GetConfigurationRevision(ctx, configRevisionID)
		if err != nil {
			return err
		}
		if revision.ApplicationKey != planCtx.App.Key || revision.EnvironmentKey != planCtx.Env.Key {
			return fmt.Errorf("deployment: configuration revision has wrong scope")
		}
	}

	plainEnv := map[string]map[string]string{}
	secretEnv := map[string]map[string]string{}
	storeRefs := map[string]map[string]configport.StoreRef{}
	containerNames := module.ContainerNames()
	deliveryStore := planCtx.Env.SecretStoreKey
	for _, containerName := range containerNames {
		container := module.Spec.Containers[containerName]
		plainEnv[containerName] = map[string]string{}
		secretEnv[containerName] = map[string]string{}
		storeRefs[containerName] = map[string]configport.StoreRef{}
		keys := make([]string, 0, len(container.Variables))
		for key := range container.Variables {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			raw := container.Variables[key]
			if strings.HasPrefix(raw, "${context.uc12.") && strings.HasSuffix(raw, "}") {
				configKey := strings.TrimSuffix(strings.TrimPrefix(raw, "${context.uc12."), "}")
				entry, exists := revision.Entries[configKey]
				if !exists || configRevisionID == "" {
					return fmt.Errorf("deployment: Application key %s is not in the pinned revision", configKey)
				}
				if entry.Kind != configuration.Secret {
					// Ordinary Variables are Orchestrator metadata, not secret delivery.
					plainEnv[containerName][key] = entry.Value
					continue
				}
				storeRefs[containerName][key] = configport.StoreRef{StoreKey: entry.StoreKey, ValueRef: entry.ValueRef}
				continue
			}
			isSecret, err := bindsSecret(raw, node.Bindings, plan.Graph, types)
			if err != nil {
				return err
			}
			resolved, err := placeholder.ExpandString(raw, bindingResolver{
				ctx:       planCtx,
				bindings:  node.Bindings,
				outputs:   provisionResult.Outputs,
				set:       plan.CandidateSet,
				namespace: target.Namespace,
			})
			if err != nil {
				return fmt.Errorf("deployment: resolve %s/%s: %w", containerName, key, err)
			}
			value := placeholder.Stringify(resolved)
			if isSecret {
				secretEnv[containerName][key] = value
			} else {
				plainEnv[containerName][key] = value
			}
		}
	}
	for name, refs := range storeRefs {
		if len(refs) == 0 {
			delete(storeRefs, name)
		}
	}

	if e := module.Spec.Execution; e != nil && len(e.SecretFiles) > 0 {
		if s.configSync == nil {
			return environment.ErrExecutionUnavailable
		}
		for i, file := range e.SecretFiles {
			key := strings.TrimSuffix(strings.TrimPrefix(file.Ref, "${context.uc12."), "}")
			entry, ok := revision.Entries[key]
			if !ok || entry.Kind != configuration.Secret {
				return environment.ErrExecution
			}
			if storeRefs[file.Container] == nil {
				storeRefs[file.Container] = map[string]configport.StoreRef{}
			}
			storeRefs[file.Container][fmt.Sprintf("orchestrator-file-%d", i)] = configport.StoreRef{StoreKey: entry.StoreKey, ValueRef: entry.ValueRef}
		}
	}
	var vaultInjection *execution.VaultInjection
	var configSecretName string
	var configSecretKeys map[string]map[string]string
	var secretDelivery map[string]any
	if len(storeRefs) > 0 {
		if s.registry == nil || deliveryStore == "" {
			return configport.ErrNoStore
		}
		if s.configSync != nil {
			bundle, err := s.registry.PrepareBundle(ctx, configport.BundleRequest{
				OrganizationKey: planCtx.OrganizationKey, ApplicationKey: planCtx.App.Key, EnvironmentKey: planCtx.Env.Key, WorkloadID: workloadID,
				Namespace: target.Namespace, RevisionID: configRevisionID, DeploymentID: record.ID, DeliveryStoreKey: deliveryStore, Refs: storeRefs,
			})
			if err != nil {
				return err
			}
			if err := s.configSync.Sync(ctx, target, execution.ConfigBundle{StoreKey: bundle.StoreKey, Address: bundle.Address, Mount: bundle.Mount, AuthMount: bundle.AuthMount, Path: bundle.Path, Role: bundle.Role, ServiceAccount: bundle.ServiceAccount, SecretName: bundle.SecretName, CAPEM: bundle.CAPEM, Keys: bundle.Keys}); err != nil {
				return err
			}
			configSecretName, configSecretKeys = bundle.SecretName, bundle.Keys
			secretDelivery = map[string]any{"storeKey": bundle.StoreKey, "address": bundle.Address, "authMount": bundle.AuthMount, "mount": bundle.Mount, "path": bundle.Path, "secretName": bundle.SecretName}
		} else {
			refs := []string{}
			bindings := map[string]map[string]string{}
			for container, entries := range storeRefs {
				bindings[container] = map[string]string{}
				for name, ref := range entries {
					if ref.StoreKey != deliveryStore {
						return fmt.Errorf("deployment: agent delivery cannot mix secret stores")
					}
					refs = append(refs, ref.ValueRef)
					bindings[container][name] = ref.ValueRef
				}
			}
			access, err := s.registry.PrepareAccess(ctx, configport.AccessRequest{
				OrganizationKey: planCtx.OrganizationKey, ApplicationKey: planCtx.App.Key, EnvironmentKey: planCtx.Env.Key, WorkloadID: workloadID,
				Namespace: target.Namespace, RevisionID: configRevisionID, StoreKey: deliveryStore, Refs: refs,
			})
			if err != nil {
				return err
			}
			vaultInjection = &execution.VaultInjection{Address: access.Address, Role: access.Role, ServiceAccount: access.ServiceAccount, Bindings: bindings}
			secretDelivery = map[string]any{"storeKey": deliveryStore, "address": access.Address}
		}
	}

	storage := map[string]execution.StorageBinding{}
	if module.Spec.Execution != nil {
		for _, mount := range module.Spec.Execution.Mounts {
			resolver := bindingResolver{ctx: planCtx, bindings: node.Bindings, outputs: provisionResult.Outputs, set: plan.CandidateSet, namespace: target.Namespace}
			claim, err := resolver.ResolveResource(mount.Resource, "claimName")
			if err != nil {
				return err
			}
			ns, err := resolver.ResolveResource(mount.Resource, "namespace")
			if err != nil {
				return err
			}
			claimName, ok := claim.(string)
			if !ok || claimName == "" {
				return environment.ErrExecution
			}
			namespace, ok := ns.(string)
			if !ok || namespace != target.Namespace {
				return environment.ErrExecution
			}
			storage[mount.Resource] = execution.StorageBinding{ClaimName: claimName, Namespace: namespace}
		}
	}
	manifests, err := s.renderer.Render(ctx, execution.RenderRequest{
		Storage:          storage,
		Selection:        plan.Rendering[workloadID],
		WorkloadID:       workloadID,
		Module:           module,
		Namespace:        target.Namespace,
		PlainEnv:         plainEnv,
		SecretEnv:        secretEnv,
		DeploymentID:     record.ID,
		ImagePullSecret:  s.imagePullSecret,
		Vault:            vaultInjection,
		ConfigSecretName: configSecretName,
		ConfigSecretKeys: configSecretKeys,
		Labels:           map[string]string{"orchestrator.io/application": planCtx.App.Key, "orchestrator.io/environment": planCtx.Env.Key, "orchestrator.io/deployment-id": record.ID},
		Services:         execution.ServicePortsOf(plan.CandidateSet.Modules),
	})
	if err != nil {
		return err
	}
	digest, err := manifestDigest(manifests)
	if err != nil {
		return err
	}
	// ADR-024: until the new set is ready the inventory keeps every previous
	// object too, so a failed or interrupted deploy leaks nothing.
	previous, _, err := s.previousInstance(ctx, planCtx.App.Key+"/"+planCtx.Env.Key, planCtx.Env.TargetGeneration, workloadID)
	if err != nil {
		return err
	}
	rendered := renderedInventory(manifests, record.ID)
	union, stale := mergeInventory(instanceInventory(previous), rendered)

	instance := domain.WorkloadInstance{
		EnvironmentKey:          planCtx.App.Key + "/" + planCtx.Env.Key,
		WorkloadID:              workloadID,
		LastDeploymentID:        record.ID,
		AppliedConfigRevisionID: configRevisionID,
		Generation:              planCtx.Env.TargetGeneration,
		TargetRef:               withInventory(withWorkloadOwner(withDelivery(withPlacement(targetRef(target), plan.Placements[wd]), secretDelivery), record.ID), union),
		ManifestDigest:          digest,
		Status:                  domain.InstanceApplying,
		ObservedAt:              s.clock.Now(),
		ManifestReview:          deploymentreview.RenderedSnapshot(plan, workloadID, manifests),
	}
	if s.delivery != nil {
		instance.TargetRef["deliveryMode"] = string(planCtx.Env.DesiredDelivery.Effective().Mode)
	}
	if err := s.store.UpsertWorkloadProgress(ctx, instance); err != nil {
		return err
	}

	if planCtx.Env.DesiredDelivery.Effective().Mode == deliverydomain.CD || s.delivery != nil {
		if s.delivery == nil {
			return deliveryapp.ErrUnavailable
		}
		auxiliary := []execution.Manifest{}
		for _, m := range manifests {
			if m.Auxiliary || m.Secret {
				auxiliary = append(auxiliary, m)
			}
		}
		if len(auxiliary) > 0 {
			if err := s.deployer.Apply(ctx, target, auxiliary); err != nil {
				return err
			}
		}
		if err := s.delivery.Deploy(ctx, deliveryapp.DeployRequest{OrganizationKey: planCtx.OrganizationKey, ApplicationKey: planCtx.App.Key, Environment: planCtx.Env, WorkloadID: workloadID, DeploymentID: record.ID, Target: target, Manifests: manifests}); err != nil {
			instance.Status = domain.InstanceFailed
			_ = s.store.UpsertWorkloadProgress(ctx, instance)
			return err
		}
	} else {
		if err := s.applyPhases(ctx, target, workloadID, manifests); err != nil {
			instance.Status = domain.InstanceFailed
			instance.ObservedAt = s.clock.Now()
			_ = s.store.UpsertWorkloadProgress(ctx, instance)
			return err
		}
		// Objects the new render no longer contains are deleted only after the
		// new set is ready; the inventory then becomes exactly the new set.
		if err := s.removeObjects(ctx, target, workloadID, stale); err != nil {
			instance.Status = domain.InstanceFailed
			instance.ObservedAt = s.clock.Now()
			_ = s.store.UpsertWorkloadProgress(ctx, instance)
			return err
		}
	}

	instance.TargetRef = withInventory(instance.TargetRef, rendered)
	instance.Status = domain.InstanceReady
	instance.ObservedAt = s.clock.Now()
	return s.store.UpsertWorkloadProgress(ctx, instance)
}

// PublicHost is the canonical public host of an Environment: staging has its own
// host, every other Environment uses the Application's.
func PublicHost(subdomain, env, baseDomain string) string {
	if env == "staging" {
		return "staging." + subdomain + "." + baseDomain
	}
	return subdomain + "." + baseDomain
}

// ReconcilePublicRoutes applies the complete current Environment route set.
// Pending Deploy calls this once after every workload in the batch is ready.
func (s *Service) ReconcilePublicRoutes(ctx context.Context, appKey, envKey string) error {
	if s.publicRoutes == nil {
		return nil
	}
	app, err := s.store.GetApplication(ctx, appKey)
	if err != nil {
		return err
	}
	env, err := s.store.GetEnvironment(ctx, appKey, envKey)
	if err != nil {
		return err
	}
	set, err := s.store.GetDeploymentSet(ctx, env.CurrentDeploymentSetID)
	if err != nil {
		return err
	}
	return s.reconcileRoutes(ctx, app, env, set.Document)
}

func (s *Service) reconcileRoutes(ctx context.Context, app appdomain.Application, env environment.Environment, set environment.Document) error {
	return s.routesFor(ctx, app, env, set, false)
}

// ReconcileRoutesFor applies the complete Environment-owned route set of one
// target generation from a Deployment Set (transition cutover).
func (s *Service) ReconcileRoutesFor(ctx context.Context, appKey string, env environment.Environment, set environment.Document) error {
	app, err := s.store.GetApplication(ctx, appKey)
	if err != nil {
		return err
	}
	return s.routesFor(ctx, app, env, set, false)
}

// RemoveRoutesFor deletes the Environment-owned Ingress at one generation.
func (s *Service) RemoveRoutesFor(ctx context.Context, appKey string, env environment.Environment) error {
	app, err := s.store.GetApplication(ctx, appKey)
	if err != nil {
		return err
	}
	return s.routesFor(ctx, app, env, environment.NewDocument(), true)
}

// RoutesManaged reports whether a public route manager is configured.
func (s *Service) RoutesManaged() bool { return s.publicRoutes != nil }

// GenerationTarget rebuilds the execution target of one generation from its
// persisted workload instance, or from the Connection when none exists yet.
func (s *Service) GenerationTarget(ctx context.Context, app appdomain.Application, env environment.Environment) (execution.Target, error) {
	instances, err := s.store.ListWorkloadInstancesFor(ctx, app.Key+"/"+env.Key, env.TargetGeneration)
	if err != nil {
		return execution.Target{}, err
	}
	base := execution.Target{Namespace: env.Namespace(), Extra: map[string]string{"application": app.Key, "environment": env.Key}}
	for _, instance := range instances {
		candidate := base
		candidate.Extra = map[string]string{"application": app.Key, "environment": env.Key}
		if err := restoreEnvironmentTarget(&candidate, instance.TargetRef, app.OrganizationKey, env); err != nil {
			return execution.Target{}, err
		}
		if candidate.Explicit() {
			return candidate, nil
		}
	}
	return execution.Target{}, fmt.Errorf("deployment: no persisted workload target exists for this generation")
}

func (s *Service) routesFor(ctx context.Context, app appdomain.Application, env environment.Environment, set environment.Document, remove bool) error {
	if s.publicRoutes == nil {
		return nil
	}
	route := execution.PublicRoute{ApplicationID: app.Key, EnvironmentID: env.Key, Host: PublicHost(app.Subdomain, env.Key, s.baseDomain)}
	if !remove {
		if err := planning.ValidatePublicRoutes(set); err != nil {
			return err
		}
		for _, id := range set.ModuleIDs() {
			var paths []environment.PublicPath
			if service := set.Modules[id].Spec.Service; service != nil {
				paths = service.PublicRoutes
			}
			for _, path := range paths {
				port := 0
				if service := set.Modules[id].Spec.Service; service != nil {
					port = service.Ports[path.Port].Port
				}
				route.Paths = append(route.Paths, execution.PublicPath{Path: path.Path, WorkloadID: id, PortName: path.Port, Port: port})
			}
		}
	}
	instances, err := s.store.ListWorkloadInstancesFor(ctx, app.Key+"/"+env.Key, env.TargetGeneration)
	if err != nil {
		return err
	}
	if len(instances) == 0 {
		return nil
	}
	base := execution.Target{Namespace: env.Namespace(), Extra: map[string]string{"application": app.Key, "environment": env.Key}}
	target := base
	for _, instance := range instances {
		candidate := base
		candidate.Extra = map[string]string{"application": app.Key, "environment": env.Key}
		if err := restoreEnvironmentTarget(&candidate, instance.TargetRef, app.OrganizationKey, env); err != nil {
			return err
		}
		target = candidate
		if candidate.Explicit() {
			break
		}
	}
	if env.DesiredDelivery.Effective().Mode == deliverydomain.CD || s.delivery != nil {
		if s.delivery == nil {
			return deliveryapp.ErrUnavailable
		}
		renderer, ok := s.publicRoutes.(execution.PublicRouteRenderer)
		if !ok {
			return deliverydomain.ErrUnsupported
		}
		manifests, err := renderer.RenderRoute(target, route)
		if err != nil {
			return err
		}
		if len(manifests) == 0 {
			return s.delivery.RemoveWorkload(ctx, app.OrganizationKey, env.ID, "_routes")
		}
		return s.delivery.Deploy(ctx, deliveryapp.DeployRequest{OrganizationKey: app.OrganizationKey, ApplicationKey: app.Key, Environment: env, WorkloadID: "_routes", DeploymentID: persistence.OperationFrom(ctx), Target: target, Manifests: manifests})
	}
	return s.publicRoutes.Reconcile(ctx, target, route)
}

// withPlacement pins the nonsecret placement of an applied workload (cluster
// and namespace resources, Definition and account identity) beside its target,
// so Update, Remove and restart reach the original target (ADR-016).
func withPlacement(ref map[string]any, placement planning.Placement) map[string]any {
	if placement.Node == "" {
		return ref
	}
	ref["placement"] = map[string]any{
		"cluster": placement.Cluster, "clusterDefinition": placement.ClusterDefinition,
		"clusterDefinitionVersion": float64(placement.ClusterDefinitionVersion),
		"account":                  placement.Account, "accountVersion": float64(placement.AccountVersion),
		"namespace": placement.Namespace, "provider": placement.Provider, "region": placement.Region,
	}
	return ref
}

// ErrTargetChanged reports a catalog edit that would move an applied
// workload to another cluster, account or namespace resource: replacement is
// never implicit (UC-05 V2-04).
// ErrDependencyNotReady: a post-workload resource needs another workload that is not applied yet.
var ErrDependencyNotReady = errors.New("deployment: a required workload dependency is not ready")

var ErrTargetChanged = errors.New("deployment: the workload target changed; it cannot be moved by an update")

// checkAppliedPlacement refuses to retarget an applied workload before any
// provisioning side effect.
func (s *Service) checkAppliedPlacement(ctx context.Context, planCtx planning.Context, plan *planning.Plan, workloadID string) error {
	if !planCtx.DefinitionPlaced() {
		return nil
	}
	instances, err := s.store.ListWorkloadInstancesFor(ctx, planCtx.App.Key+"/"+planCtx.Env.Key, planCtx.Env.TargetGeneration)
	if err != nil {
		return err
	}
	var want planning.Placement
	for _, p := range plan.Placements {
		if p.Namespace != "" {
			if node, ok := plan.Graph.Node(p.Node); ok && node.WorkloadID == workloadID {
				want = p
			}
		}
	}
	for _, instance := range instances {
		if instance.WorkloadID != workloadID || instance.Status == domain.InstanceRemoved {
			continue
		}
		applied, _ := instance.TargetRef["placement"].(map[string]any)
		if applied == nil {
			continue
		}
		text := func(key string) string { v, _ := applied[key].(string); return v }
		if text("cluster") != want.Cluster || text("account") != want.Account || text("namespace") != want.Namespace || text("provider") != want.Provider || text("region") != want.Region {
			return ErrTargetChanged
		}
	}
	return nil
}

// withDelivery pins the nonsecret secret-delivery metadata of the store that
// served a workload, so history keeps the actual store, address and auth mount.
func withDelivery(ref map[string]any, delivery map[string]any) map[string]any {
	if delivery != nil {
		ref["secretDelivery"] = delivery
	}
	return ref
}

const workloadOwnerDeploymentIDKey = "orchestrator.io/owner-deployment-id"

func workloadOwnerDeploymentID(instance domain.WorkloadInstance) string {
	if value, exists := instance.TargetRef[workloadOwnerDeploymentIDKey]; exists {
		owner, ok := value.(string)
		if !ok {
			return ""
		}
		return owner
	}
	return instance.LastDeploymentID
}

func withWorkloadOwner(ref map[string]any, deploymentID string) map[string]any {
	ref[workloadOwnerDeploymentIDKey] = deploymentID
	return ref
}

// preflightSecretDelivery refuses a deploy whose secrets cannot be delivered
// before any provisioning side effect: a store must be selected and its
// workload Kubernetes auth must verify.
func (s *Service) preflightSecretDelivery(ctx context.Context, cmd DeployCommand, env environment.Environment, plan *planning.Plan, workloadID string, applying bool) error {
	if !applying {
		return nil
	}
	module, ok := plan.CandidateSet.Modules[workloadID]
	if !ok {
		return nil
	}
	if cmd.ConfigRevisionID == "" {
		if module.Spec.Execution != nil && len(module.Spec.Execution.SecretFiles) > 0 {
			return configport.ErrNoStore
		}
		return nil
	}
	revision, err := s.store.GetConfigurationRevision(ctx, cmd.ConfigRevisionID)
	if err != nil {
		return err
	}
	needs := false
	if module.Spec.Execution != nil && len(module.Spec.Execution.SecretFiles) > 0 {
		if s.configSync == nil {
			return environment.ErrExecutionUnavailable
		}
		needs = true
		for _, file := range module.Spec.Execution.SecretFiles {
			key := strings.TrimSuffix(strings.TrimPrefix(file.Ref, "${context.uc12."), "}")
			if revision.Entries[key].Kind != configuration.Secret {
				return configport.ErrNoStore
			}
		}
	}
	for _, containerName := range module.ContainerNames() {
		for _, raw := range module.Spec.Containers[containerName].Variables {
			if strings.HasPrefix(raw, "${context.uc12.") && strings.HasSuffix(raw, "}") {
				if revision.Entries[strings.TrimSuffix(strings.TrimPrefix(raw, "${context.uc12."), "}")].Kind == configuration.Secret {
					needs = true
				}
			}
		}
	}
	if !needs {
		return nil
	}
	if env.SecretStoreKey == "" || s.registry == nil {
		return configport.ErrNoStore
	}
	return s.registry.CheckWorkloadAuth(ctx, cmd.OrganizationKey, env.SecretStoreKey)
}

// restoreEnvironmentTarget restores a stored target and rejects a credential
// reference that names a Connection other than the one the Environment
// generation is pinned to; the stored identity is never trusted blindly.
func restoreEnvironmentTarget(target *execution.Target, ref map[string]any, organizationKey string, env environment.Environment) error {
	if err := restoreTarget(target, ref, organizationKey); err != nil {
		return err
	}
	// Definition-placed: the persisted workload target must name a scoped account, never a host default.
	if target.Connection == "" {
		return fmt.Errorf("deployment: workload target has no scoped account")
	}
	return nil
}

// RestoreTarget rebuilds a target from a persisted TargetRef (exported for
// target transitions); a foreign-Organization reference fails closed.
func RestoreTarget(target *execution.Target, ref map[string]any, organizationKey string) error {
	return restoreTarget(target, ref, organizationKey)
}

// targetRef is the persisted WorkloadInstance target: cluster, namespace,
// context and, for credential-backed Connections, only the opaque
// Organization/Connection identity. No credential or temporary path.
func targetRef(target execution.Target) map[string]any {
	ref := map[string]any{"cluster": target.ClusterName, "namespace": target.Namespace, "context": target.Context}
	if target.CredentialBacked() {
		ref["organization"] = target.Organization
		ref["connection"] = target.Connection
	}
	if target.Provider != "" {
		ref["provider"] = target.Provider
		ref["region"] = target.Region
	}
	return ref
}

// restoreTarget rebuilds a target from a persisted TargetRef so removal and
// route reconciliation after a restart reach the same scoped cluster. A
// credential-backed reference of another Organization fails closed.
func restoreTarget(target *execution.Target, ref map[string]any, organizationKey string) error {
	text := func(key string) string {
		value, _ := ref[key].(string)
		return value
	}
	if namespace := text("namespace"); namespace != "" {
		target.Namespace = namespace
	}
	target.Context = text("context")
	target.ClusterName = text("cluster")
	target.Kind = "kubernetes"
	target.Provider = text("provider")
	target.Region = text("region")
	if connection := text("connection"); connection != "" {
		if text("organization") != organizationKey {
			return fmt.Errorf("deployment: workload target belongs to another organization")
		}
		target.Organization, target.Connection = organizationKey, connection
	}
	if target.Provider != "" {
		if target.Provider != "eks" || target.Region == "" || target.ClusterName == "" || target.Connection == "" {
			return fmt.Errorf("deployment: workload target provider metadata is incomplete or unsupported")
		}
	} else if target.Region != "" {
		return fmt.Errorf("deployment: workload target has a region without a provider")
	}
	return nil
}

func (s *Service) fail(ctx context.Context, record domain.Deployment, cause error) error {
	finished := s.clock.Now()
	record.Status = domain.StatusFailed
	record.FailureReason = PublicFailure(cause)
	record.FinishedAt = &finished
	_ = s.store.SaveDeployment(ctx, record)
	return cause
}

func parseScore(doc map[string]any) (*score.Document, error) {
	if doc == nil {
		return nil, nil
	}
	return score.FromMap(doc)
}

// bindsSecret reports whether a variable binds at least one secret output.
func bindsSecret(raw string, bindings map[string]string, g planning.Graph, types map[string]resource.Type) (bool, error) {
	refs, err := placeholder.Refs(raw)
	if err != nil {
		return false, err
	}
	for _, ref := range refs {
		if ref.Kind != placeholder.KindResource {
			continue
		}
		descriptor, ok := bindings[ref.Resource]
		if !ok {
			return false, fmt.Errorf("deployment: unknown resource binding %q", ref.Resource)
		}
		node, ok := g.Node(descriptor)
		if !ok {
			return false, fmt.Errorf("deployment: unknown node %q", descriptor)
		}
		typ, ok := types[node.ResourceType]
		if !ok {
			return false, fmt.Errorf("deployment: resource type %q is not registered", node.ResourceType)
		}
		field, ok := typ.Output(ref.OutputKey)
		if !ok {
			return false, fmt.Errorf("deployment: output %q is not in the %q contract", ref.OutputKey, typ.Key)
		}
		if field.Secret {
			return true, nil
		}
	}
	return false, nil
}

func manifestDigest(manifests []execution.Manifest) (string, error) {
	summary := make([]map[string]any, 0, len(manifests))
	for _, m := range manifests {
		if m.Secret {
			summary = append(summary, map[string]any{"kind": m.Kind, "name": m.Name, "namespace": m.Namespace})
			continue
		}
		summary = append(summary, map[string]any{"kind": m.Kind, "name": m.Name, "namespace": m.Namespace, "object": m.Object})
	}
	return canon.Hash(summary)
}

// bindingResolver resolves workload bindings from the outputs collected in this run.
type bindingResolver struct {
	ctx       planning.Context
	bindings  map[string]string
	outputs   map[string]map[string]any
	set       environment.Document
	namespace string
}

// ResolveResource returns a provider output value.
func (r bindingResolver) ResolveResource(binding, outputKey string) (any, error) {
	descriptor, ok := r.bindings[binding]
	if !ok {
		return nil, fmt.Errorf("deployment: unknown resource binding %q", binding)
	}
	outputs, ok := r.outputs[descriptor]
	if !ok {
		return nil, fmt.Errorf("deployment: provider %q produced no outputs", descriptor)
	}
	value, ok := outputs[outputKey]
	if !ok {
		return nil, fmt.Errorf("deployment: provider %q has no output %q", descriptor, outputKey)
	}
	return value, nil
}

// ResolveContext returns a non-secret context value.
func (r bindingResolver) ResolveContext(path string) (any, error) {
	if strings.HasPrefix(path, "service.") {
		parts := strings.Split(path, ".")
		if len(parts) != 3 {
			return nil, fmt.Errorf("deployment: invalid Service reference")
		}
		module, ok := r.set.Modules[parts[1]]
		if !ok || module.Spec.Service == nil {
			return nil, fmt.Errorf("deployment: referenced Service does not exist")
		}
		port, ok := module.Spec.Service.Ports[parts[2]]
		if !ok || port.Port < 1 {
			return nil, fmt.Errorf("deployment: referenced Service port does not exist")
		}
		return fmt.Sprintf("http://%s.%s.svc.cluster.local:%d", parts[1], r.namespace, port.Port), nil
	}
	return r.ctx.ResolveContext(path)
}

// provisionPostWorkload executes the resource-only subset that depends on the
// workload just applied (UC-06 BR-08). A resource that also depends on another
// workload waits until every one of its workloads has an applied instance.
func (s *Service) provisionPostWorkload(ctx context.Context, req provisioning.Request, result *provisioning.Result, workloadID string, env environment.Environment) error {
	plan := req.Plan
	if len(plan.AfterWorkloads) == 0 {
		return nil
	}
	workloadOf := map[string]string{}
	var self string
	for _, node := range plan.Graph.Nodes {
		if node.Kind != planning.NodeWorkload {
			continue
		}
		workloadOf[node.Descriptor] = node.WorkloadID
		if node.WorkloadID == workloadID {
			self = node.Descriptor
		}
	}
	// Only instances of THIS generation whose pinned cluster and account equal the plan's placement of
	// that workload unlock a post resource (never an old generation or another target).
	instances, err := s.store.ListWorkloadInstancesFor(ctx, env.ApplicationKey+"/"+env.Key, env.TargetGeneration)
	if err != nil {
		return err
	}
	suitable := func(descriptor string) bool {
		want, ok := plan.Placements[descriptor]
		if !ok {
			return false
		}
		for _, instance := range instances {
			if instance.WorkloadID != workloadOf[descriptor] || instance.Status != domain.InstanceReady || instance.Generation != env.TargetGeneration {
				continue
			}
			placement, _ := instance.TargetRef["placement"].(map[string]any)
			if placement["cluster"] == want.Cluster && placement["account"] == want.Account {
				return true
			}
		}
		return false
	}
	ready := map[string]bool{}
	for descriptor, workloads := range plan.AfterWorkloads {
		follows := false
		for _, w := range workloads {
			follows = follows || w == self
		}
		if !follows {
			continue // it runs in the deployment of the workload it follows
		}
		for _, w := range workloads {
			if w != self && !suitable(w) {
				// A required post action must never be dropped while the Deployment commits SUCCEEDED.
				return fmt.Errorf("%w: a resource that follows this workload also needs workload %q, which has no ready instance on the planned target in the current generation", ErrDependencyNotReady, workloadOf[w])
			}
		}
		ready[descriptor] = true
	}
	if len(ready) == 0 {
		return nil
	}
	req.Prior = result
	req.Select = func(descriptor string) bool { return ready[descriptor] }
	_, err = s.provisioning.Provision(ctx, req)
	return err
}
