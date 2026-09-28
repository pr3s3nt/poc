// Package deployment implements UC-06 (deploy workload) and UC-09 (deployment view).
package deployment

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"orchestrator/internal/application/provisioning"
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
	store           persistence.Store
	planner         *planning.Service
	provisioning    *provisioning.Service
	renderer        execution.WorkloadRenderer
	deployer        execution.WorkloadDeployer
	terraform       planning.ModuleInspector
	clock           clock.Clock
	configProvider  configport.Provider
	imagePullSecret string
	configSync      execution.ConfigSecretSynchronizer
}

func (s *Service) SetConfigurationProvider(provider configport.Provider) { s.configProvider = provider }
func (s *Service) SetImagePullSecret(name string)                        { s.imagePullSecret = name }
func (s *Service) SetConfigSecretSynchronizer(sync execution.ConfigSecretSynchronizer) {
	s.configSync = sync
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

// DeployWorkload runs the full UC-06 main success scenario.
func (s *Service) DeployWorkload(ctx context.Context, cmd DeployCommand) (*DeployResult, error) {
	app, err := s.store.GetApplication(ctx, cmd.ApplicationKey)
	if err != nil {
		return nil, err
	}
	env, err := s.store.GetEnvironment(ctx, cmd.ApplicationKey, cmd.EnvironmentKey)
	if err != nil {
		return nil, err
	}
	conn, err := s.store.GetConnection(ctx, app.ConnectionKey)
	if err != nil {
		return nil, err
	}
	if conn.Status != appdomain.ConnectionReady {
		return nil, fmt.Errorf("deployment: connection %q is %s, want READY", conn.Key, conn.Status)
	}

	// MS-01: read the current Deployment Set and create the Deployment record.
	baseSet := environment.NewDocument()
	baseSetID := env.CurrentDeploymentSetID
	if baseSetID != "" {
		set, err := s.store.GetDeploymentSet(ctx, baseSetID)
		if err != nil {
			return nil, err
		}
		baseSet = set.Document
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
	record := domain.Deployment{
		ID:                     ids.New(),
		OrganizationKey:        cmd.OrganizationKey,
		ApplicationKey:         app.Key,
		EnvironmentKey:         env.Key,
		ExecutionProfile:       string(app.Profile),
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
		Connection:      conn,
		DeploymentID:    record.ID,
		RunID:           cmd.RunID,
	}

	catalog, types, definitions, err := s.loadCatalog(ctx)
	if err != nil {
		return nil, s.fail(ctx, record, err)
	}
	active, err := s.store.ListActiveResources(ctx, cmd.OrganizationKey)
	if err != nil {
		return nil, s.fail(ctx, record, err)
	}

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
	})
	if err != nil {
		return nil, s.fail(ctx, record, err)
	}
	if cmd.ExpectedPlanHash != "" && plan.PlanHash != cmd.ExpectedPlanHash {
		return nil, s.fail(ctx, record, fmt.Errorf("deployment: preview plan is stale"))
	}

	candidateSet := environment.DeploymentSet{
		ID:                    ids.New(),
		EnvironmentKey:        env.ApplicationKey + "/" + env.Key,
		CreatedByDeploymentID: record.ID,
		Document:              plan.CandidateSet,
		CreatedAt:             now,
	}
	candidateSet.DocumentHash, err = canon.Hash(plan.CandidateSet)
	if err != nil {
		return nil, s.fail(ctx, record, err)
	}
	deltaSnapshot, err := domain.NewDeploymentDeltaSnapshot(ids.New(), app.Key, plan.Delta, domain.DeltaSnapshotMetadata{
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

	record.Status = domain.StatusDeploying
	if err := s.store.SaveDeployment(ctx, record); err != nil {
		return nil, s.fail(ctx, record, err)
	}

	result := &DeployResult{DeploymentID: record.ID, PlanHash: plan.PlanHash, WorkloadID: workloadID}

	if after != nil {
		if err := s.applyWorkload(ctx, &record, planCtx, plan, provisionResult, types, workloadID, cmd.ConfigRevisionID); err != nil {
			return nil, s.fail(ctx, record, err)
		}
	} else {
		if err := s.removeWorkload(ctx, planCtx, workloadID, record.ID); err != nil {
			return nil, s.fail(ctx, record, err)
		}
	}

	// Transaction B: optimistic commit of the current Deployment Set (MS-12).
	if err := s.store.Transact(ctx, func(ctx context.Context) error {
		if err := s.store.CompareVersionAndSetCurrent(ctx, app.Key, env.Key, record.BaseEnvironmentVersion, candidateSet.ID); err != nil {
			return err
		}
		if app.Profile == appdomain.ProfileAWSEKS && app.RuntimeStatus != appdomain.RuntimeReady {
			app.RuntimeStatus = appdomain.RuntimeReady
			if err := s.store.SaveApplication(ctx, app); err != nil {
				return err
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

func (s *Service) removeWorkload(ctx context.Context, planCtx planning.Context, workloadID, deploymentID string) error {
	instances, err := s.store.ListWorkloadInstances(ctx, planCtx.App.Key+"/"+planCtx.Env.Key)
	if err != nil {
		return err
	}
	for _, instance := range instances {
		if instance.WorkloadID != workloadID {
			continue
		}
		target := execution.Target{Namespace: planCtx.Env.NamespaceIdentity, Extra: map[string]string{"application": planCtx.App.Key, "environment": planCtx.Env.Key}}
		if value, ok := instance.TargetRef["namespace"].(string); ok && value != "" {
			target.Namespace = value
		}
		if value, ok := instance.TargetRef["context"].(string); ok {
			target.Context = value
		}
		if value, ok := instance.TargetRef["cluster"].(string); ok {
			target.ClusterName = value
		}
		instance.Status = domain.InstanceRemoving
		instance.LastDeploymentID = deploymentID
		instance.ObservedAt = s.clock.Now()
		if err := s.store.UpsertWorkloadInstance(ctx, instance); err != nil {
			return err
		}
		if err := s.deployer.Remove(ctx, target, workloadID); err != nil {
			instance.Status = domain.InstanceFailed
			_ = s.store.UpsertWorkloadInstance(ctx, instance)
			return err
		}
		instance.Status = domain.InstanceRemoved
		instance.AppliedConfigRevisionID = ""
		instance.ObservedAt = s.clock.Now()
		return s.store.UpsertWorkloadInstance(ctx, instance)
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
	vaultBindings := map[string]map[string]string{}
	valueRefs := []string{}
	containerNames := module.ContainerNames()
	for _, containerName := range containerNames {
		container := module.Spec.Containers[containerName]
		plainEnv[containerName] = map[string]string{}
		secretEnv[containerName] = map[string]string{}
		vaultBindings[containerName] = map[string]string{}
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
				vaultBindings[containerName][key] = entry.ValueRef
				valueRefs = append(valueRefs, entry.ValueRef)
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

	var vaultInjection *execution.VaultInjection
	var configSecretName string
	var configSecretKeys map[string]map[string]string
	if len(valueRefs) > 0 {
		if s.configSync != nil {
			preparer, ok := s.configProvider.(configport.WorkloadBundlePreparer)
			if !ok {
				return fmt.Errorf("deployment: VSO workload bundle provider is not configured")
			}
			bundle, err := preparer.PrepareWorkloadBundle(ctx, planCtx.App.Key, planCtx.Env.Key, workloadID, target.Namespace, configRevisionID, record.ID, vaultBindings)
			if err != nil {
				return err
			}
			if err := s.configSync.Sync(ctx, target, execution.ConfigBundle{Address: bundle.Address, Mount: bundle.Mount, Path: bundle.Path, Role: bundle.Role, ServiceAccount: bundle.ServiceAccount, SecretName: bundle.SecretName, Keys: bundle.Keys}); err != nil {
				return err
			}
			configSecretName, configSecretKeys = bundle.SecretName, bundle.Keys
		} else {
			preparer, ok := s.configProvider.(configport.WorkloadAccessPreparer)
			if !ok {
				return fmt.Errorf("deployment: Vault workload access is not configured")
			}
			access, err := preparer.PrepareWorkloadAccess(ctx, planCtx.App.Key, planCtx.Env.Key, workloadID, target.Namespace, configRevisionID, valueRefs)
			if err != nil {
				return err
			}
			vaultInjection = &execution.VaultInjection{Address: access.Address, Role: access.Role, ServiceAccount: access.ServiceAccount, Bindings: vaultBindings}
		}
	}

	manifests, err := s.renderer.Render(ctx, execution.RenderRequest{
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
		TargetRef:               map[string]any{"cluster": target.ClusterName, "namespace": target.Namespace, "context": target.Context},
		ManifestDigest:          digest,
		Status:                  domain.InstanceApplying,
		ObservedAt:              s.clock.Now(),
	}
	if err := s.store.UpsertWorkloadInstance(ctx, instance); err != nil {
		return err
	}

	if err := s.deployer.Apply(ctx, target, manifests); err != nil {
		instance.Status = domain.InstanceFailed
		instance.ObservedAt = s.clock.Now()
		_ = s.store.UpsertWorkloadInstance(ctx, instance)
		return err
	}
	refs := []execution.WorkloadRef{{Kind: "Deployment", Name: workloadID, Namespace: target.Namespace}}
	if err := s.deployer.WaitReady(ctx, target, refs); err != nil {
		instance.Status = domain.InstanceFailed
		instance.ObservedAt = s.clock.Now()
		_ = s.store.UpsertWorkloadInstance(ctx, instance)
		return err
	}
	instance.Status = domain.InstanceReady
	instance.ObservedAt = s.clock.Now()
	return s.store.UpsertWorkloadInstance(ctx, instance)
}

func (s *Service) loadCatalog(ctx context.Context) (planning.Catalog, map[string]resource.Type, map[string]resource.Definition, error) {
	typeList, err := s.store.ListResourceTypes(ctx)
	if err != nil {
		return planning.Catalog{}, nil, nil, err
	}
	defList, err := s.store.ListResourceDefinitions(ctx)
	if err != nil {
		return planning.Catalog{}, nil, nil, err
	}
	types := map[string]resource.Type{}
	for _, t := range typeList {
		types[t.Key] = t
	}
	definitions := map[string]resource.Definition{}
	for _, d := range defList {
		definitions[d.Key] = d
	}
	return planning.Catalog{Types: types, Definitions: defList}, types, definitions, nil
}

func (s *Service) fail(ctx context.Context, record domain.Deployment, cause error) error {
	finished := s.clock.Now()
	record.Status = domain.StatusFailed
	record.FailureReason = cause.Error()
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
