// Package deployment implements UC-06 (deploy workload) and UC-09 (deployment view).
package deployment

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"orchestrator/internal/application/envops"
	"orchestrator/internal/application/provisioning"
	"orchestrator/internal/application/target"
	appdomain "orchestrator/internal/domain/application"
	"orchestrator/internal/domain/configuration"
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
	ExpectedPlanHash string
	ConfigRevisionID string
	// DeferPublicRoutes is set only by the UC-16 pending batch, whose final
	// Environment state Preview validated: routes reconcile after the batch
	// and this step may plan an intermediate Environment state.
	DeferPublicRoutes bool

	// Destination, BaseSetID and HoldSet are set only by the transition
	// service under its own claim (ADR-012). Destination replaces the stored
	// binding and generation for this run without touching the Environment
	// row; BaseSetID chains the planning base across workloads; HoldSet keeps
	// the current Set, runtime status and Unreferenced marks untouched until
	// the atomic cutover commit.
	Destination *environment.Environment
	BaseSetID   string
	HoldSet     bool
	// ProvisionOnly stops after UC-08 provisioning: no workload is applied,
	// no route reconciled and no Set committed (transition PROVISIONING).
	ProvisionOnly bool
}

// DeployResult is returned to the actor at UC-06 MS-13.
type DeployResult struct {
	DeploymentID string        `json:"deploymentId"`
	Status       domain.Status `json:"status"`
	PlanHash     string        `json:"planHash"`
	WorkloadID   string        `json:"workloadId"`
	// CandidateSetID is the Set this run produced (chained by transitions).
	CandidateSetID string `json:"-"`
}

// Service orchestrates plan, provision, render, apply and commit.
type Service struct {
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
}

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
	// MS-01: read the current Deployment Set, connection, catalog and Active
	// Resources from one consistent snapshot shared with UC-05 Preview.
	snapshot, err := LoadPlanningSnapshot(ctx, s.store, cmd.OrganizationKey, cmd.ApplicationKey, cmd.EnvironmentKey)
	if err != nil {
		return nil, err
	}
	app, env, conn := snapshot.App, snapshot.Env, snapshot.Connection
	baseSet, baseSetID := snapshot.BaseSet, snapshot.BaseSetID
	if cmd.Destination != nil {
		env = *cmd.Destination
		conn, err = target.Resolve(ctx, s.store, app.OrganizationKey, env)
		if err != nil {
			return nil, err
		}
	}
	if cmd.BaseSetID != "" {
		chained, err := s.store.GetDeploymentSet(ctx, cmd.BaseSetID)
		if err != nil {
			return nil, err
		}
		baseSet, baseSetID = chained.Document, chained.ID
	}

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
	recordAction := action
	if cmd.ProvisionOnly {
		recordAction = domain.ActionProvision
	}
	record := domain.Deployment{
		ID:                     ids.New(),
		EnvironmentID:          env.ID,
		OrganizationKey:        cmd.OrganizationKey,
		ApplicationKey:         app.Key,
		EnvironmentKey:         env.Key,
		ExecutionProfile:       string(env.Profile),
		ConnectionKey:          env.ConnectionKey,
		TargetGeneration:       env.TargetGeneration,
		Action:                 recordAction,
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
		Connection:      conn,
		DeploymentID:    record.ID,
		RunID:           cmd.RunID,
	}

	catalog, types, definitions, active := snapshot.Catalog, snapshot.Types, snapshot.Definitions, snapshot.Active

	// MS-02..MS-08: planning pipeline.
	plan, err := s.planner.Plan(planning.Request{
		OrganizationKey: cmd.OrganizationKey,
		App:             app,
		Env:             env,
		Connection:      conn,
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
	if cmd.ExpectedPlanHash != "" && plan.PlanHash != cmd.ExpectedPlanHash {
		return nil, s.fail(ctx, record, ErrStalePlan)
	}

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

	// Secret delivery must be usable before any infrastructure is touched.
	if err := s.preflightSecretDelivery(ctx, cmd, env, plan, workloadID, after != nil); err != nil {
		return nil, s.fail(ctx, record, err)
	}

	// MS-09: UC-08 provisions resource-only batches.
	provisionResult, err := s.provisioning.Provision(ctx, provisioning.Request{
		DeploymentID: record.ID,
		RunID:        cmd.RunID,
		Context:      planCtx,
		Plan:         plan,
		Types:        types,
		Definitions:  definitions,
	})
	if err != nil {
		return nil, s.fail(ctx, record, err)
	}

	result := &DeployResult{DeploymentID: record.ID, PlanHash: plan.PlanHash, WorkloadID: workloadID, CandidateSetID: candidateSet.ID}
	if cmd.ProvisionOnly {
		finished := s.clock.Now()
		record.Status = domain.StatusSucceeded
		record.FinishedAt = &finished
		if err := s.store.SaveDeployment(ctx, record); err != nil {
			return nil, s.fail(ctx, record, err)
		}
		result.Status = record.Status
		return result, nil
	}
	record.Status = domain.StatusDeploying
	if err := s.store.SaveDeployment(ctx, record); err != nil {
		return nil, s.fail(ctx, record, err)
	}

	if after != nil {
		if err := s.applyWorkload(ctx, &record, planCtx, plan, provisionResult, types, workloadID, cmd.ConfigRevisionID); err != nil {
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
	// A transition holds the pointer: the target and Set move together at cutover.
	result.CandidateSetID = candidateSet.ID
	if err := s.store.Transact(ctx, func(ctx context.Context) error {
		if !cmd.HoldSet {
			if err := s.store.CompareVersionAndSetCurrent(ctx, app.Key, env.Key, record.BaseEnvironmentVersion, candidateSet.ID); err != nil {
				return err
			}
			if err := s.markUnreferenced(ctx, cmd.OrganizationKey, record.ID, plan.UnreferencedResources); err != nil {
				return err
			}
			if env.Profile == appdomain.ProfileAWSEKS && env.RuntimeStatus != appdomain.RuntimeReady {
				if err := s.store.UpdateRuntimeStatus(ctx, app.Key, env.Key, appdomain.RuntimeReady); err != nil {
					return err
				}
			}
		}
		finished := s.clock.Now()
		record.Status = domain.StatusSucceeded
		record.FinishedAt = &finished
		return s.store.SaveDeployment(ctx, record)
	}); err != nil {
		return nil, s.fail(ctx, record, err)
	}

	result.Status = record.Status
	return result, nil
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
		target := execution.Target{Namespace: planCtx.Env.Namespace(), Extra: map[string]string{"application": planCtx.App.Key, "environment": planCtx.Env.Key}}
		if err := restoreEnvironmentTarget(&target, instance.TargetRef, planCtx.OrganizationKey, planCtx.Env.ConnectionKey); err != nil {
			return err
		}
		instance.Status = domain.InstanceRemoving
		instance.LastDeploymentID = deploymentID
		instance.ObservedAt = s.clock.Now()
		if err := s.store.UpsertWorkloadProgress(ctx, instance); err != nil {
			return err
		}
		if err := s.deployer.Remove(ctx, target, workloadID); err != nil {
			instance.Status = domain.InstanceFailed
			_ = s.store.UpsertWorkloadProgress(ctx, instance)
			return err
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
	wd, err := planning.WorkloadDescriptor(workloadID)
	if err != nil {
		return err
	}
	node, ok := plan.Graph.Node(wd.String())
	if !ok {
		return fmt.Errorf("deployment: workload %q is not in the plan graph", workloadID)
	}
	module, ok := plan.CandidateSet.Modules[workloadID]
	if !ok {
		return fmt.Errorf("deployment: workload %q is not in the candidate deployment set", workloadID)
	}
	target := provisioning.ResolveTarget(plan.Graph, provisionResult, wd.String())
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
				if !entry.UsesStore() {
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

	manifests, err := s.renderer.Render(ctx, execution.RenderRequest{
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
	})
	if err != nil {
		return err
	}
	digest, err := manifestDigest(manifests)
	if err != nil {
		return err
	}

	instance := domain.WorkloadInstance{
		EnvironmentKey:          planCtx.App.Key + "/" + planCtx.Env.Key,
		WorkloadID:              workloadID,
		LastDeploymentID:        record.ID,
		AppliedConfigRevisionID: configRevisionID,
		Generation:              planCtx.Env.TargetGeneration,
		TargetRef:               withDelivery(targetRef(target), secretDelivery),
		ManifestDigest:          digest,
		Status:                  domain.InstanceApplying,
		ObservedAt:              s.clock.Now(),
	}
	if err := s.store.UpsertWorkloadProgress(ctx, instance); err != nil {
		return err
	}

	if err := s.deployer.Apply(ctx, target, manifests); err != nil {
		instance.Status = domain.InstanceFailed
		instance.ObservedAt = s.clock.Now()
		_ = s.store.UpsertWorkloadProgress(ctx, instance)
		return err
	}
	refs := []execution.WorkloadRef{{Kind: "Deployment", Name: workloadID, Namespace: target.Namespace}}
	if err := s.deployer.WaitReady(ctx, target, refs); err != nil {
		instance.Status = domain.InstanceFailed
		instance.ObservedAt = s.clock.Now()
		_ = s.store.UpsertWorkloadProgress(ctx, instance)
		return err
	}
	instance.Status = domain.InstanceReady
	instance.ObservedAt = s.clock.Now()
	return s.store.UpsertWorkloadProgress(ctx, instance)
}

func publicHost(subdomain, env, baseDomain string) string {
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
		if err := restoreEnvironmentTarget(&candidate, instance.TargetRef, app.OrganizationKey, env.ConnectionKey); err != nil {
			return execution.Target{}, err
		}
		if candidate.Explicit() {
			return candidate, nil
		}
	}
	conn, err := s.store.GetConnection(ctx, app.OrganizationKey, env.ConnectionKey)
	if err != nil {
		return execution.Target{}, err
	}
	return ConnectionTarget(app.OrganizationKey, env, conn), nil
}

// ConnectionTarget builds the Kubernetes target of an Environment generation
// directly from its Connection (used before any workload exists there).
func ConnectionTarget(organizationKey string, env environment.Environment, conn appdomain.Connection) execution.Target {
	target := execution.Target{Kind: "kubernetes", Namespace: env.Namespace(), Context: conn.ConfigString("kubeContext"), ClusterName: conn.ConfigString("cluster")}
	if conn.CredentialBacked() {
		target.Organization, target.Connection = organizationKey, conn.Key
	}
	return target
}

func (s *Service) routesFor(ctx context.Context, app appdomain.Application, env environment.Environment, set environment.Document, remove bool) error {
	if s.publicRoutes == nil {
		return nil
	}
	route := execution.PublicRoute{ApplicationID: app.Key, EnvironmentID: env.Key, Host: publicHost(app.Subdomain, env.Key, s.baseDomain)}
	if !remove {
		if err := planning.ValidatePublicRoutes(set); err != nil {
			return err
		}
		for _, id := range set.ModuleIDs() {
			for _, path := range set.Modules[id].Spec.Service.Routes() {
				route.Paths = append(route.Paths, execution.PublicPath{Path: path.Path, WorkloadID: id, PortName: path.Port})
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
		if err := restoreEnvironmentTarget(&candidate, instance.TargetRef, app.OrganizationKey, env.ConnectionKey); err != nil {
			return err
		}
		target = candidate
		if candidate.Explicit() {
			break
		}
	}
	return s.publicRoutes.Reconcile(ctx, target, route)
}

// withDelivery pins the nonsecret secret-delivery metadata of the store that
// served a workload, so history keeps the actual store, address and auth mount.
func withDelivery(ref map[string]any, delivery map[string]any) map[string]any {
	if delivery != nil {
		ref["secretDelivery"] = delivery
	}
	return ref
}

// preflightSecretDelivery refuses a deploy whose secrets cannot be delivered
// before any provisioning side effect: a store must be selected and its
// workload Kubernetes auth must verify.
func (s *Service) preflightSecretDelivery(ctx context.Context, cmd DeployCommand, env environment.Environment, plan *planning.Plan, workloadID string, applying bool) error {
	if !applying || cmd.ConfigRevisionID == "" {
		return nil
	}
	module, ok := plan.CandidateSet.Modules[workloadID]
	if !ok {
		return nil
	}
	revision, err := s.store.GetConfigurationRevision(ctx, cmd.ConfigRevisionID)
	if err != nil {
		return err
	}
	needs := false
	for _, containerName := range module.ContainerNames() {
		for _, raw := range module.Spec.Containers[containerName].Variables {
			if strings.HasPrefix(raw, "${context.uc12.") && strings.HasSuffix(raw, "}") {
				if revision.Entries[strings.TrimSuffix(strings.TrimPrefix(raw, "${context.uc12."), "}")].UsesStore() {
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
func restoreEnvironmentTarget(target *execution.Target, ref map[string]any, organizationKey, connectionKey string) error {
	if err := restoreTarget(target, ref, organizationKey); err != nil {
		return err
	}
	if target.Connection != "" && target.Connection != connectionKey {
		return fmt.Errorf("deployment: workload target uses a connection other than the environment connection")
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
	if connection := text("connection"); connection != "" {
		if text("organization") != organizationKey {
			return fmt.Errorf("deployment: workload target belongs to another organization")
		}
		target.Organization, target.Connection = organizationKey, connection
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
