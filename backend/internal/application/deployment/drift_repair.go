package deployment

import (
	"context"
	"encoding/json"
	"orchestrator/internal/application/provisioning"
	"orchestrator/internal/application/tfexec"
	domain "orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/drift"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
	"sort"
)

// RepairConsumers reuses provisioning/render/delivery with immutable applied
// plans. It never calls the planner, selects today's Definitions or commits a
// new Desired State. Historical Deployment records remain untouched.
type RepairConsumers struct {
	Deployment *Service
	Bindings   *provisioning.RepairBindings
}

type repairSource struct {
	instanceID string
	ref        drift.RepairConsumer
	plan       *planning.Plan
}

func (c *RepairConsumers) sources(ctx context.Context, org, app, env string) ([]repairSource, error) {
	s := c.Deployment
	target, err := s.store.GetEnvironment(ctx, app, env)
	if err != nil {
		return nil, err
	}
	application, err := s.store.GetApplication(ctx, app)
	if err != nil || application.OrganizationKey != org {
		return nil, persistence.ErrNotFound
	}
	active, err := s.store.ListActiveResources(ctx, org)
	if err != nil {
		return nil, err
	}
	workloads, err := s.store.ListWorkloadInstancesFor(ctx, app+"/"+env, target.TargetGeneration)
	if err != nil {
		return nil, err
	}
	plans := map[string]*planning.Plan{}
	load := func(id string) (*planning.Plan, error) {
		if p, ok := plans[id]; ok {
			return p, nil
		}
		raw, e := s.store.GetPlan(ctx, id)
		if e != nil {
			return nil, tfexec.ErrBaselineUnavailable
		}
		p := decodePlan(raw)
		if p == nil {
			return nil, tfexec.ErrBaselineUnavailable
		}
		plans[id] = p
		return p, nil
	}
	result := []repairSource{}
	for _, a := range active {
		if a.Status == resource.StatusDestroyed || a.LastDeploymentID == "" {
			continue
		}
		record, e := s.store.GetDeployment(ctx, a.LastDeploymentID)
		if e != nil {
			return nil, e
		}
		if record.ApplicationKey != app || record.EnvironmentKey != env || record.TargetGeneration != target.TargetGeneration {
			continue
		}
		plan, e := load(a.LastDeploymentID)
		if e != nil {
			return nil, e
		}
		node, ok := plan.Graph.Node(a.Descriptor.String())
		if !ok || node.Scope != a.Scope {
			return nil, tfexec.ErrBaselineUnavailable
		}
		// Terraform consumers are reviewed by the State engine; their plans also
		// contribute graph edges for transitive non-Terraform/workload discovery.
		result = append(result, repairSource{instanceID: repairInstanceID(a), ref: drift.RepairConsumer{Descriptor: a.Descriptor.String(), DeploymentID: a.LastDeploymentID, Version: a.Version}, plan: plan})
	}
	for _, w := range workloads {
		if w.Status == domain.InstanceRemoved {
			continue
		}
		if w.Status != domain.InstanceReady {
			return nil, tfexec.ErrRepairBlocked
		}
		plan, e := load(w.LastDeploymentID)
		if e != nil {
			return nil, e
		}
		descriptor := ""
		for _, node := range plan.Graph.Nodes {
			if node.Kind == planning.NodeWorkload && node.WorkloadID == w.WorkloadID {
				descriptor = node.Descriptor
				break
			}
		}
		if descriptor == "" {
			return nil, tfexec.ErrBaselineUnavailable
		}
		result = append(result, repairSource{ref: drift.RepairConsumer{Descriptor: descriptor, WorkloadID: w.WorkloadID, DeploymentID: w.LastDeploymentID, ManifestDigest: w.ManifestDigest}, plan: plan})
	}
	return result, nil
}
func (c *RepairConsumers) PreviewRepairConsumers(ctx context.Context, org, app, env string, roots []string) ([]drift.RepairConsumer, error) {
	sources, err := c.sources(ctx, org, app, env)
	if err != nil {
		return nil, err
	}
	closure := map[string]bool{}
	for _, id := range roots {
		closure[id] = true
	}
	for changed := true; changed; {
		changed = false
		for _, source := range sources {
			for _, edge := range source.plan.Graph.Edges {
				if closure[edge.Provider] && !closure[edge.Consumer] {
					closure[edge.Consumer] = true
					changed = true
				}
			}
		}
	}
	result := []drift.RepairConsumer{}
	for _, source := range sources {
		if !closure[source.ref.Descriptor] {
			continue
		}
		if source.ref.WorkloadID == "" {
			match, ok := source.plan.Matches[source.ref.Descriptor]
			if !ok {
				return nil, tfexec.ErrBaselineUnavailable
			}
			if match.DriverType == resource.DriverTerraform || match.DriverType == resource.DriverExistingCluster {
				continue
			}
		}
		result = append(result, source.ref)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Descriptor < result[j].Descriptor })
	return result, nil
}

func (c *RepairConsumers) RefreshRepairConsumers(ctx context.Context, org, app, env, actor string, refs []drift.RepairConsumer) error {
	s := c.Deployment
	target, err := s.store.GetEnvironment(ctx, app, env)
	if err != nil {
		return err
	}
	application, err := s.store.GetApplication(ctx, app)
	if err != nil || application.OrganizationKey != org {
		return persistence.ErrNotFound
	}
	typesList, err := s.store.ListResourceTypes(ctx, org)
	if err != nil {
		return err
	}
	types := map[string]resource.Type{}
	for _, typ := range typesList {
		types[typ.Key] = typ
	}
	// Order consumer refreshes by the union of their pinned applied DAGs.
	source, err := c.sources(ctx, org, app, env)
	if err != nil {
		return err
	}
	byID := map[string]repairSource{}
	selected := map[string]bool{}
	for _, src := range source {
		byID[src.ref.Descriptor] = src
	}
	for _, ref := range refs {
		src, ok := byID[ref.Descriptor]
		expected := ref
		expected.Completed = false
		if !ok || !sameRepairJSON(src.ref, expected) {
			return tfexec.ErrRepairStale
		}
		selected[ref.Descriptor] = true
	}
	done := map[string]bool{}
	visiting := map[string]bool{}
	var refresh func(string) error
	refresh = func(id string) error {
		if done[id] {
			return nil
		}
		if visiting[id] {
			return tfexec.ErrRepairBlocked
		}
		visiting[id] = true
		src := byID[id]
		for _, provider := range src.plan.Graph.Providers(id) {
			if selected[provider] {
				if err := refresh(provider); err != nil {
					return err
				}
			}
		}
		result, err := c.Bindings.LoadResult(ctx, org, src.plan, target.ID)
		if err != nil {
			return err
		}
		pc := planning.Context{OrganizationKey: org, App: application, Env: target}
		definitions := map[string]resource.Definition{}
		for _, def := range src.plan.DefinitionSnapshots {
			definitions[def.Key] = def
		}
		now := s.clock.Now()
		record := domain.Deployment{ID: ids.New(), EnvironmentID: target.ID, OrganizationKey: org, ApplicationKey: app, EnvironmentKey: env, TargetGeneration: target.TargetGeneration, Action: domain.ActionUpdate, WorkloadID: src.ref.WorkloadID, ActorRef: actor, Status: domain.StatusDeploying, BaseEnvironmentVersion: target.Version, BaseDeploymentSetID: target.CurrentDeploymentSetID, CandidateDeploymentSet: target.CurrentDeploymentSetID, StartedAt: now}
		delta, err := domain.NewDeploymentDeltaSnapshot(ids.New(), record.ID, domain.DeltaDocument{}, domain.DeltaSnapshotMetadata{ActorRef: actor, Action: domain.ActionUpdate, WorkloadID: src.ref.WorkloadID}, now)
		if err != nil {
			return err
		}
		record.DeltaSnapshotID = delta.ID
		raw, err := json.Marshal(src.plan)
		if err != nil {
			return err
		}
		var snapshot map[string]any
		if json.Unmarshal(raw, &snapshot) != nil {
			return tfexec.ErrBaselineUnavailable
		}
		if err := s.store.Transact(ctx, func(ctx context.Context) error {
			initial := record
			initial.Status = domain.StatusPlanning
			initial.DeltaSnapshotID = ""
			if e := s.store.SaveDeployment(ctx, initial); e != nil {
				return e
			}
			if e := s.store.SaveDeltaSnapshot(ctx, delta); e != nil {
				return e
			}
			if e := s.store.SaveDeployment(ctx, record); e != nil {
				return e
			}
			return s.store.SavePlan(ctx, record.ID, snapshot)
		}); err != nil {
			return err
		}
		if src.ref.WorkloadID != "" {
			workloads, e := s.store.ListWorkloadInstancesFor(ctx, app+"/"+env, target.TargetGeneration)
			if e != nil {
				return e
			}
			config := ""
			for _, w := range workloads {
				if w.WorkloadID == src.ref.WorkloadID {
					config = w.AppliedConfigRevisionID
				}
			}
			if err = s.checkAppliedPlacement(ctx, pc, src.plan, src.ref.WorkloadID); err == nil {
				err = s.preflightSecretDelivery(ctx, DeployCommand{OrganizationKey: org, ApplicationKey: app, EnvironmentKey: env, WorkloadID: src.ref.WorkloadID, ConfigRevisionID: config}, target, src.plan, types, src.ref.WorkloadID, true)
			}
			if err == nil {
				err = s.applyWorkload(ctx, &record, pc, src.plan, result, types, src.ref.WorkloadID, config)
			}
		} else {
			_, err = s.provisioning.Provision(ctx, provisioning.Request{DeploymentID: record.ID, RunID: record.ID, Context: pc, Plan: src.plan, Types: types, Definitions: definitions, Prior: result, Select: func(descriptor string) bool { return descriptor == id }})
		}
		finished := s.clock.Now()
		record.FinishedAt = &finished
		if err != nil {
			record.Status = domain.StatusFailed
			record.FailureReason = FailureRuntime
			_ = s.store.SaveDeployment(ctx, record)
			return err
		}
		record.Status = domain.StatusSucceeded
		if err = s.store.SaveDeployment(ctx, record); err != nil {
			return err
		}
		done[id] = true
		delete(visiting, id)
		return nil
	}
	for _, ref := range refs {
		if err := refresh(ref.Descriptor); err != nil {
			return err
		}
	}
	if len(refs) > 0 {
		set, err := s.store.GetDeploymentSet(ctx, target.CurrentDeploymentSetID)
		if err != nil {
			return err
		}
		if err := s.reconcileRoutes(ctx, application, target, set.Document); err != nil {
			return err
		}
	}
	return nil
}
func sameRepairJSON(a, b any) bool {
	x, e := json.Marshal(a)
	y, f := json.Marshal(b)
	return e == nil && f == nil && string(x) == string(y)
}

func repairInstanceID(active resource.ActiveResource) string {
	id, _ := active.ExecutorState["instanceId"].(string)
	return id
}

// RepairTopology includes edges through template resources and workloads, which
// are not all represented by the State-only Terraform dependency table.
func (c *RepairConsumers) RepairTopology(ctx context.Context, org, app, env string, roots []string) (map[string][]string, []string, error) {
	sources, err := c.sources(ctx, org, app, env)
	if err != nil {
		return nil, nil, err
	}
	graph := map[string][]string{}
	seen := map[string]map[string]bool{}
	for _, src := range sources {
		for _, edge := range src.plan.Graph.Edges {
			if seen[edge.Consumer] == nil {
				seen[edge.Consumer] = map[string]bool{}
			}
			seen[edge.Consumer][edge.Provider] = true
		}
	}
	closure := map[string]bool{}
	for _, root := range roots {
		closure[root] = true
	}
	for changed := true; changed; {
		changed = false
		for consumer, providers := range seen {
			for provider := range providers {
				if closure[provider] && !closure[consumer] {
					closure[consumer] = true
					changed = true
				}
			}
		}
	}
	for consumer, providers := range seen {
		if !closure[consumer] {
			continue
		}
		for provider := range providers {
			if closure[provider] {
				graph[consumer] = append(graph[consumer], provider)
			}
		}
		sort.Strings(graph[consumer])
	}
	instances := []string{}
	for _, src := range sources {
		if closure[src.ref.Descriptor] && src.instanceID != "" {
			instances = append(instances, src.instanceID)
		}
	}
	sort.Strings(instances)
	return graph, instances, nil
}

// RepairConsumerCoordinator shares exactly the deployment's provisioning,
// renderer, delivery coordinator, secret store and operation fencing.
func (s *Service) RepairConsumerCoordinator(executor *tfexec.Service) *RepairConsumers {
	return &RepairConsumers{Deployment: s, Bindings: &provisioning.RepairBindings{Provisioning: s.provisioning, Executor: executor}}
}
