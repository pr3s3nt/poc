package transition

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	appsvc "orchestrator/internal/application/deployment"
	"orchestrator/internal/application/envops"
	"orchestrator/internal/application/pending"
	"orchestrator/internal/application/workloadconfig"
	appdomain "orchestrator/internal/domain/application"
	"orchestrator/internal/domain/configuration"
	"orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning"
	"orchestrator/internal/planning/score"
	"orchestrator/internal/platform/canon"
	"orchestrator/internal/platform/ids"
	configport "orchestrator/internal/ports/configuration"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/ports/persistence"
)

// Service plans, claims and executes target transitions.
type Service struct {
	store     persistence.Store
	planner   *planning.Service
	terraform planning.ModuleInspector
	deployer  *appsvc.Service
	ops       *envops.Manager

	scaler    execution.WorkloadScaler
	postgres  execution.PostgresTransfer
	routes    execution.RouteInspector
	cleaner   execution.NamespaceCleaner
	probe     execution.TargetProbe
	registry  configport.Registry
	workloads *workloadconfig.Service

	directDelivery bool
	async          bool
	wg             sync.WaitGroup
}

// New wires the transition service. Cluster adapters are optional: without
// them DEPLOY_NEW still works and MIGRATE_POSTGRES fails closed at preflight.
func New(store persistence.Store, planner *planning.Service, terraform planning.ModuleInspector, deployer *appsvc.Service, ops *envops.Manager) *Service {
	return &Service{store: store, planner: planner, terraform: terraform, deployer: deployer, ops: ops, directDelivery: true, async: true}
}

// SetCluster installs the Kubernetes data-transfer adapters.
func (s *Service) SetCluster(scaler execution.WorkloadScaler, postgres execution.PostgresTransfer, routes execution.RouteInspector, cleaner execution.NamespaceCleaner) {
	s.scaler, s.postgres, s.routes, s.cleaner = scaler, postgres, routes, cleaner
}

// SetProbe installs the destination connectivity/permission probe.
func (s *Service) SetProbe(probe execution.TargetProbe) { s.probe = probe }

// SetStoreRegistry lets preflight verify the selected store's workload auth.
func (s *Service) SetStoreRegistry(registry configport.Registry) { s.registry = registry }

// SetWorkloads lets Preview validate pinned draft upserts like a normal Preview.
func (s *Service) SetWorkloads(workloads *workloadconfig.Service) { s.workloads = workloads }

// SetDirectDelivery says whether workloads are applied directly (not via Fleet).
func (s *Service) SetDirectDelivery(direct bool) { s.directDelivery = direct }

// SetAsync selects background execution (production) or inline (tests).
func (s *Service) SetAsync(async bool) { s.async = async }

// Wait blocks until every background transition run has finished.
func (s *Service) Wait() { s.wg.Wait() }

type pgSource struct {
	active   resource.ActiveResource
	resource execution.PostgresResource
}

type pgDestination struct {
	descriptor string
	scope      resource.Scope
	definition resource.Definition
	image      string
}

type prepared struct {
	app      appdomain.Application
	env      environment.Environment
	scope    configurationScope
	set      environment.DeploymentSet
	dest     environment.Environment
	srcConn  appdomain.Connection
	destConn appdomain.Connection
	order    []string
	// scores are the DESIRED Scores (current Set merged with pinned drafts);
	// befores are the current Set's Scores of workloads that already exist.
	scores        map[string]map[string]any
	befores       map[string]map[string]any
	actions       map[string]string
	configChanged map[string]bool
	drafts        []environment.WorkloadDraft
	deleted       []string
	startDoc      environment.Document
	finalDoc      environment.Document
	instances     map[string]deployment.WorkloadInstance
	hashes        map[string]string
	sources       []pgSource
	dests         map[string]pgDestination
	mappings      []environment.ResourceMapping
	snapshot      appsvc.PlanningSnapshot
	notes         []string
}

type configurationScope struct {
	Version  int64
	Revision string
}

func eligible(org string, conn appdomain.Connection) bool {
	if conn.OrganizationKey != org || conn.Status != appdomain.ConnectionReady {
		return false
	}
	switch conn.Kind {
	case appdomain.ConnectionKubernetes:
		return true
	case appdomain.ConnectionAWS:
		return conn.ConfigString("region") != ""
	}
	return false
}

func endpoint(env environment.Environment, conn appdomain.Connection) Endpoint {
	name := conn.Name
	if name == "" {
		name = conn.Key
	}
	return Endpoint{ConnectionKey: env.ConnectionKey, ConnectionName: name, Kind: string(conn.Kind), Profile: string(env.Profile), Region: env.Region, Generation: env.TargetGeneration, Namespace: env.Namespace()}
}

// Preview validates the whole transition read-only and issues the pinned token.
// Unsupported capabilities fail here, before anything is stopped.
func (s *Service) Preview(ctx context.Context, req Request) (Preview, error) {
	p, err := s.prepare(ctx, req, true)
	if err != nil {
		return Preview{}, err
	}
	return s.previewOf(p, req)
}

func (s *Service) previewOf(p *prepared, req Request) (Preview, error) {
	preview := Preview{
		ApplicationKey: p.app.Key, EnvironmentKey: p.env.Key, Mode: req.Mode,
		Source: endpoint(p.env, p.srcConn), Destination: endpoint(p.dest, p.destConn),
		Mappings: p.mappings, DowntimeRequired: req.Mode == environment.ModeMigratePostgres, Notes: p.notes, Workloads: []WorkloadImpact{},
	}
	for _, id := range p.order {
		preview.Workloads = append(preview.Workloads, WorkloadImpact{WorkloadID: id, Action: p.actions[id], ConfigChanged: p.configChanged[id]})
	}
	for _, id := range p.deleted {
		preview.Workloads = append(preview.Workloads, WorkloadImpact{WorkloadID: id, Action: "REMOVED"})
	}
	pinned := struct {
		App, Env                 string
		EnvVersion, DraftVersion int64
		ConfigVersion            int64
		SetID, Revision          string
		Source, Destination      environment.Binding
		Store                    string
		High                     int64
		Mode                     environment.TransitionMode
		Mappings                 []environment.ResourceMapping
		Hashes                   map[string]string
		Actions                  map[string]string
		ConfigChanged            map[string]bool
	}{p.app.Key, p.env.Key, p.env.Version, p.env.DraftVersion, p.scope.Version, p.set.ID, p.scope.Revision, p.env.Binding(), p.dest.Binding(), p.env.SecretStoreKey, p.env.GenerationHigh, req.Mode, p.mappings, p.hashes, p.actions, p.configChanged}
	token, err := canon.Hash(pinned)
	if err != nil {
		return Preview{}, err
	}
	preview.Token = token
	return preview, nil
}

func (s *Service) prepare(ctx context.Context, req Request, inspect bool) (*prepared, error) {
	if req.Mode != environment.ModeDeployNew && req.Mode != environment.ModeMigratePostgres {
		return nil, fmt.Errorf("%w: mode must be DEPLOY_NEW or MIGRATE_POSTGRES", errInvalid)
	}
	app, err := s.store.GetApplication(ctx, req.ApplicationKey)
	if err != nil {
		return nil, err
	}
	if app.OrganizationKey != req.OrganizationKey {
		return nil, fmt.Errorf("%w: application %q", persistence.ErrNotFound, req.ApplicationKey)
	}
	env, err := s.store.GetEnvironment(ctx, req.ApplicationKey, req.EnvironmentKey)
	if err != nil {
		return nil, err
	}
	if env.Busy() {
		return nil, persistence.Busy(env.ApplicationKey, env.Key)
	}
	if !env.Configured() {
		return nil, ErrNoRuntime
	}
	hasRuntime, err := envops.HasRuntime(ctx, s.store, app.OrganizationKey, env)
	if err != nil {
		return nil, err
	}
	if !hasRuntime {
		return nil, ErrNoRuntime
	}
	if req.DestinationKey == "" {
		return nil, fmt.Errorf("%w: destinationKey is required", errInvalid)
	}
	if req.DestinationKey == env.ConnectionKey {
		return nil, ErrSameDestination
	}
	destConn, err := s.store.GetConnection(ctx, app.OrganizationKey, req.DestinationKey)
	if err != nil || !eligible(app.OrganizationKey, destConn) {
		return nil, ErrDestinationFailed
	}
	profile, region, status := appdomain.ProfileInternalK8s, "", appdomain.RuntimeReady
	if destConn.Kind == appdomain.ConnectionAWS {
		profile, region, status = appdomain.ProfileAWSEKS, destConn.ConfigString("region"), appdomain.RuntimePending
	}
	generation := env.GenerationHigh
	if env.TargetGeneration > generation {
		generation = env.TargetGeneration
	}
	dest := env.WithBinding(environment.Binding{ConnectionKey: destConn.Key, Profile: profile, Region: region, Scope: environment.ScopeEnvironment, Generation: generation + 1}, status)

	snapshot, err := appsvc.LoadPlanningSnapshot(ctx, s.store, app.OrganizationKey, app.Key, env.Key)
	if err != nil {
		return nil, err
	}
	scope, err := s.store.GetConfigurationScope(ctx, app.Key, env.Key)
	if err != nil {
		return nil, err
	}
	set, err := s.store.GetDeploymentSet(ctx, env.CurrentDeploymentSetID)
	if err != nil {
		return nil, err
	}
	p := &prepared{app: app, env: env, scope: configurationScope{Version: scope.Version, Revision: scope.DesiredRevisionID}, set: set, dest: dest, srcConn: snapshot.Connection, destConn: destConn, snapshot: snapshot,
		scores: map[string]map[string]any{}, actions: map[string]string{}, instances: map[string]deployment.WorkloadInstance{}, hashes: map[string]string{}, dests: map[string]pgDestination{}}

	if err := s.desired(ctx, p); err != nil {
		return nil, err
	}
	instances, err := s.store.ListWorkloadInstances(ctx, app.Key+"/"+env.Key)
	if err != nil {
		return nil, err
	}
	for _, instance := range instances {
		if instance.Status != deployment.InstanceRemoved {
			p.instances[instance.WorkloadID] = instance
		}
	}
	if err := s.planDestination(p); err != nil {
		return nil, err
	}
	if err := s.flagConfigChanges(ctx, p); err != nil {
		return nil, err
	}
	if req.Mode == environment.ModeMigratePostgres {
		if err := s.checkMigration(ctx, p, req, inspect); err != nil {
			return nil, err
		}
	} else {
		p.notes = append(p.notes, "The destination starts with new, empty resources. No data is copied.")
		p.mappings = []environment.ResourceMapping{}
	}
	if inspect {
		if err := s.preflightCapabilities(ctx, p); err != nil {
			return nil, err
		}
	}
	p.notes = append(p.notes, "The source generation is retained and quiesced after cutover; it is deleted only by an explicit cleanup.")
	return p, nil
}

// postgres16Image accepts an explicit PostgreSQL 16 image tag only; "postgres:160"
// or "postgres:latest" are not PostgreSQL 16.
var postgres16Image = regexp.MustCompile(`^(?:[a-z0-9.-]+(?::[0-9]+)?/)*postgres:16(?:\.[0-9]+)?(?:-[a-z0-9.]+)?(?:@sha256:[0-9a-f]{64})?$`)

var errInvalid = errors.New("transition: invalid request")

// IsInvalid reports a request validation failure.
func IsInvalid(err error) bool { return errors.Is(err, errInvalid) }

// preflightCapabilities runs the live checks that must pass before any writer
// stops: the destination answers with its own credential and permissions, and
// the selected store's Kubernetes auth works for every workload that reads
// store-backed configuration.
func (s *Service) preflightCapabilities(ctx context.Context, p *prepared) error {
	if s.probe != nil {
		target := appsvc.ConnectionTarget(p.app.OrganizationKey, p.dest, p.destConn)
		if err := s.probe.Probe(ctx, target); err != nil {
			return fmt.Errorf("%w: the destination connection failed its connectivity and permission check", ErrDestinationFailed)
		}
	}
	if err := s.preflightRouting(ctx, p); err != nil {
		return err
	}
	needs, err := s.usesStoreBackedConfiguration(ctx, p)
	if err != nil || !needs {
		return err
	}
	if p.env.SecretStoreKey == "" || s.registry == nil {
		return unsupported("the environment reads secrets but has no selected secret store")
	}
	if err := s.registry.CheckWorkloadAuth(ctx, p.app.OrganizationKey, p.env.SecretStoreKey); err != nil {
		return unsupported("the selected secret store cannot authenticate workloads on the destination (Kubernetes auth is not ready)")
	}
	return nil
}

func (s *Service) usesStoreBackedConfiguration(ctx context.Context, p *prepared) (bool, error) {
	if p.scope.Revision == "" {
		return false, nil
	}
	revision, err := s.store.GetConfigurationRevision(ctx, p.scope.Revision)
	if err != nil {
		return false, err
	}
	for _, id := range p.order {
		module := p.finalDoc.Modules[id]
		for _, container := range module.Spec.Containers {
			for _, raw := range container.Variables {
				if strings.HasPrefix(raw, "${context.uc12.") && strings.HasSuffix(raw, "}") {
					if revision.Entries[strings.TrimSuffix(strings.TrimPrefix(raw, "${context.uc12."), "}")].UsesStore() {
						return true, nil
					}
				}
			}
		}
	}
	return false, nil
}

// desired builds the complete desired Environment: the current Set merged with
// the pinned pending drafts (upserts and deletes). It reads only; deletes mean
// absence at the destination, never an action on the source.
func (s *Service) desired(ctx context.Context, p *prepared) error {
	drafts, err := s.store.ListWorkloadDrafts(ctx, p.app.Key, p.env.Key)
	if err != nil {
		return err
	}
	p.drafts = drafts
	current := map[string]map[string]any{}
	for id, module := range p.set.Document.Modules {
		raw, err := workloadconfig.ReconstructForRedeploy(id, module, p.set.Document)
		if err != nil {
			return fmt.Errorf("%w: workload %s cannot be rebuilt: %v", errInvalid, id, err)
		}
		current[id] = raw
	}
	p.befores = current
	byWorkload := map[string]environment.WorkloadDraft{}
	for _, draft := range drafts {
		byWorkload[draft.WorkloadID] = draft
	}
	desired := map[string]environment.WorkloadDraft{}
	for id, raw := range current {
		p.scores[id] = raw
		p.actions[id] = "UNCHANGED"
	}
	for _, draft := range drafts {
		switch draft.State {
		case environment.DraftDelete:
			if _, exists := current[draft.WorkloadID]; exists {
				p.deleted = append(p.deleted, draft.WorkloadID)
				delete(p.scores, draft.WorkloadID)
				p.actions[draft.WorkloadID] = "REMOVED"
			}
		case environment.DraftUpsert:
			if s.workloads != nil {
				if err := s.workloads.ValidateImport(ctx, p.app.Key, p.env.Key, draft.Score); err != nil {
					return fmt.Errorf("%w: pending workload %s: %v", errInvalid, draft.WorkloadID, err)
				}
			}
			before, existed := current[draft.WorkloadID]
			p.scores[draft.WorkloadID] = draft.Score
			switch {
			case !existed:
				p.actions[draft.WorkloadID] = "ADDED"
			case sameScore(before, draft.Score):
				p.actions[draft.WorkloadID] = "UNCHANGED"
			default:
				p.actions[draft.WorkloadID] = "UPDATED"
			}
		}
	}
	sort.Strings(p.deleted)
	for id, raw := range p.scores {
		desired[id] = environment.WorkloadDraft{ApplicationKey: p.app.Key, EnvironmentKey: p.env.Key, WorkloadID: id, State: environment.DraftUpsert, Score: raw}
	}
	if p.order, err = pending.OrderDrafts(desired); err != nil {
		return err
	}
	if len(p.order) == 0 {
		return fmt.Errorf("%w: the desired environment has no workload to deploy", errInvalid)
	}
	return nil
}

func sameScore(a, b map[string]any) bool {
	left, err1 := canon.Hash(a)
	right, err2 := canon.Hash(b)
	return err1 == nil && err2 == nil && left == right
}

var configRef = regexp.MustCompile(`\$\{resources\.env\.([A-Za-z_][A-Za-z0-9_]*)\}`)

// flagConfigChanges marks workloads whose referenced keys differ between the
// revision they run and the desired revision (they pick up new configuration).
func (s *Service) flagConfigChanges(ctx context.Context, p *prepared) error {
	p.configChanged = map[string]bool{}
	var desired configuration.Revision
	if p.scope.Revision != "" {
		revision, err := s.store.GetConfigurationRevision(ctx, p.scope.Revision)
		if err != nil {
			return err
		}
		desired = revision
	}
	for _, id := range p.order {
		raw, _ := canon.Map(p.scores[id])
		text := fmt.Sprint(raw)
		var applied configuration.Revision
		if instance, ok := p.instances[id]; ok && instance.AppliedConfigRevisionID != "" {
			revision, err := s.store.GetConfigurationRevision(ctx, instance.AppliedConfigRevisionID)
			if err != nil {
				return err
			}
			applied = revision
		}
		for _, match := range configRef.FindAllStringSubmatch(text, -1) {
			if desired.Entries[match[1]] != applied.Entries[match[1]] {
				p.configChanged[id] = true
			}
		}
	}
	return nil
}

// preflightRouting proves, before any writer stops, that public traffic can move.
// The product owns an Environment Ingress per cluster: it can transfer routing
// only between Connections that reach the SAME physical cluster and ingress
// controller. A distinct cluster needs external ingress/DNS cutover, which is not
// automated here, so it fails safely instead of claiming traffic moved.
func (s *Service) preflightRouting(ctx context.Context, p *prepared) error {
	if !s.deployer.RoutesManaged() || !(hasRoutes(p.set.Document) || hasRoutes(p.finalDoc)) {
		return nil
	}
	if s.probe == nil {
		return unsupported("public routes cannot be moved: no cluster identity check is available")
	}
	source, err := s.deployer.GenerationTarget(ctx, p.app, p.env)
	if err != nil {
		return err
	}
	sourceID, err := s.probe.ClusterIdentity(ctx, source)
	if err != nil {
		return unsupported("the source cluster identity could not be read, so routing ownership cannot be proven")
	}
	destinationID, err := s.probe.ClusterIdentity(ctx, appsvc.ConnectionTarget(p.app.OrganizationKey, p.dest, p.destConn))
	if err != nil {
		return unsupported("the destination cluster identity could not be read, so routing ownership cannot be proven")
	}
	if sourceID != destinationID {
		return unsupported("the destination is a different cluster: moving public traffic between clusters needs an external ingress or DNS cutover, which is not automated; use a destination on the same cluster or move routing yourself")
	}
	return nil
}

// planDestination plans the complete desired Environment against the
// destination: removals first (planning only), then every desired workload in
// dependency order with chained candidate Sets, collecting PostgreSQL nodes and
// plan hashes. It performs no I/O.
func (s *Service) planDestination(p *prepared) error {
	base := p.set.Document
	runID := "transition-" + shortHash(p.app.Key, p.env.Key, fmt.Sprint(p.dest.TargetGeneration))
	request := func(id string, before, after map[string]any, action deployment.Action, base environment.Document) (*planning.Plan, error) {
		var beforeDoc, afterDoc *score.Document
		var err error
		if before != nil {
			if beforeDoc, err = score.FromMap(before); err != nil {
				return nil, err
			}
		}
		if after != nil {
			if afterDoc, err = score.FromMap(after); err != nil {
				return nil, err
			}
		}
		plan, err := s.planner.Plan(planning.Request{
			OrganizationKey: p.app.OrganizationKey, App: p.app, Env: p.dest, Connection: p.destConn,
			BaseSet: base, Before: beforeDoc, After: afterDoc, WorkloadID: id, Action: action,
			Catalog: p.snapshot.Catalog, Active: p.snapshot.Active, Terraform: s.terraform, RunID: runID,
			AllowIntermediateEnvironmentState: true,
		})
		if err != nil {
			if message, ok := planning.PublicMessage(err); ok {
				return nil, fmt.Errorf("%w: workload %s on the destination: %s", errInvalid, id, message)
			}
			return nil, err
		}
		return plan, nil
	}
	for _, id := range p.deleted {
		plan, err := request(id, p.befores[id], nil, deployment.ActionRemove, base)
		if err != nil {
			return err
		}
		base = plan.CandidateSet
	}
	p.startDoc = base
	for _, id := range p.order {
		before, action := p.befores[id], deployment.ActionUpdate
		if _, existed := p.set.Document.Modules[id]; !existed {
			before, action = nil, deployment.ActionDeploy
		}
		plan, err := request(id, before, p.scores[id], action, base)
		if err != nil {
			return err
		}
		p.hashes[id] = plan.PlanHash
		for _, node := range plan.Graph.Nodes {
			if node.Kind != planning.NodeResource || node.ResourceType != "postgres" {
				continue
			}
			match := plan.Matches[node.Descriptor]
			def := p.snapshot.Definitions[match.DefinitionKey]
			image, _ := def.Variables()["image"].(string)
			p.dests[node.Descriptor] = pgDestination{descriptor: node.Descriptor, scope: node.Scope, definition: def, image: image}
		}
		base = plan.CandidateSet
	}
	if err := planning.ValidatePublicRoutes(base); err != nil {
		return fmt.Errorf("%w: the desired environment is invalid: %v", errInvalid, err)
	}
	if err := planning.ValidateServiceReferences(base); err != nil {
		return fmt.Errorf("%w: the desired environment is invalid: %v", errInvalid, err)
	}
	p.finalDoc = base
	return nil
}

func shortHash(parts ...string) string {
	h, _ := canon.Hash(parts)
	return h[:12]
}

// checkMigration validates MIGRATE_POSTGRES against every persisted source and
// planned destination before any workload is stopped.
func (s *Service) checkMigration(ctx context.Context, p *prepared, req Request, inspect bool) error {
	if s.scaler == nil || s.postgres == nil {
		return unsupported("this backend has no Kubernetes data-transfer adapter")
	}
	if !s.directDelivery {
		return unsupported("workloads are delivered through Fleet GitOps, which does not support data transfer")
	}
	if p.env.Profile != appdomain.ProfileInternalK8s || p.dest.Profile != appdomain.ProfileInternalK8s {
		return unsupported("automatic data transfer supports Kubernetes-managed PostgreSQL on direct Kubernetes targets only; AWS/Aurora transfer is not supported")
	}
	active, err := s.store.ListActiveResources(ctx, p.app.OrganizationKey)
	if err != nil {
		return err
	}
	allowed := map[string]bool{"postgres": true, "k8s-namespace": true, "k8s-cluster": true, "workload": true}
	unreferenced := 0
	for _, a := range active {
		if a.Scope.Type == resource.ScopeApplication || !envops.InGeneration(a.Scope, p.app.Key, p.env.Key, p.env.TargetGeneration) {
			continue
		}
		if !allowed[a.Descriptor.Type] {
			return unsupported("resource type %q holds state that cannot be transferred", a.Descriptor.Type)
		}
		if a.Descriptor.Type != "postgres" {
			continue
		}
		if a.Status == resource.StatusUnreferenced {
			unreferenced++
			continue
		}
		res, err := s.locateSource(ctx, p, a)
		if err != nil {
			return err
		}
		p.sources = append(p.sources, pgSource{active: a, resource: res})
	}
	sort.Slice(p.sources, func(i, j int) bool {
		return p.sources[i].active.Descriptor.String() < p.sources[j].active.Descriptor.String()
	})
	if unreferenced > 0 {
		p.notes = append(p.notes, fmt.Sprintf("%d unreferenced database(s) stay at the source generation and are not transferred.", unreferenced))
	}
	if len(p.sources) == 0 {
		return unsupported("the environment has no PostgreSQL resource to transfer; use DEPLOY_NEW")
	}
	if err := s.resolveMappings(p, req); err != nil {
		return err
	}
	for _, mapping := range p.mappings {
		dest := p.dests[mapping.DestinationDescriptor]
		if dest.definition.DriverType != resource.DriverKubernetes {
			return unsupported("destination database %s is not a Kubernetes-managed PostgreSQL", mapping.DestinationDescriptor)
		}
		if image := dest.image; image != "" && !postgres16Image.MatchString(image) {
			return unsupported("destination database %s does not use PostgreSQL 16", mapping.DestinationDescriptor)
		}
	}
	if inspect {
		for _, source := range p.sources {
			inventory, err := s.postgres.Inspect(ctx, source.resource)
			if err != nil {
				return unsupported("source database %s cannot be inspected without a password", source.active.Descriptor.String())
			}
			if inventory.ServerVersionNum < 160000 || inventory.ServerVersionNum >= 170000 {
				return unsupported("source database %s is not PostgreSQL 16", source.active.Descriptor.String())
			}
		}
	}
	p.notes = append(p.notes, "Application writers stop before the backup; they restart on the destination after the data is restored and verified.")
	return nil
}

// locateSource resolves a source Active Resource to a Kubernetes PostgreSQL
// identity or reports it unsupported.
func (s *Service) locateSource(ctx context.Context, p *prepared, a resource.ActiveResource) (execution.PostgresResource, error) {
	def, ok := p.snapshot.Definitions[a.DefinitionKey]
	if !ok || def.DriverType != resource.DriverKubernetes {
		return execution.PostgresResource{}, unsupported("database %s is not a Kubernetes-managed PostgreSQL", a.Descriptor.String())
	}
	name, _ := a.ExecutorState["name"].(string)
	namespace, _ := a.ExecutorState["namespace"].(string)
	database, _ := a.Outputs["database"].(string)
	username, _ := a.Outputs["username"].(string)
	if name == "" || namespace == "" || database == "" || username == "" {
		return execution.PostgresResource{}, unsupported("database %s has no recorded server identity", a.Descriptor.String())
	}
	srcTarget, err := s.deployer.GenerationTarget(ctx, p.app, p.env)
	if err != nil {
		return execution.PostgresResource{}, err
	}
	srcTarget.Namespace = namespace
	return execution.PostgresResource{Target: srcTarget, Namespace: namespace, Name: name, Database: database, Username: username}, nil
}

// resolveMappings validates the explicit logical mapping, or proposes the
// identity mapping when none was supplied. Every in-scope source database must
// appear exactly once and every destination must be a planned PostgreSQL node.
func (s *Service) resolveMappings(p *prepared, req Request) error {
	mappings := req.Mappings
	if len(mappings) == 0 {
		for _, source := range p.sources {
			descriptor := source.active.Descriptor.String()
			if _, ok := p.dests[descriptor]; !ok {
				return unsupported("source database %s has no destination database with the same identity; map it explicitly", descriptor)
			}
			mappings = append(mappings, environment.ResourceMapping{SourceDescriptor: descriptor, DestinationDescriptor: descriptor})
		}
	}
	seenSource, seenDest := map[string]bool{}, map[string]bool{}
	valid := map[string]bool{}
	for _, source := range p.sources {
		valid[source.active.Descriptor.String()] = true
	}
	for _, mapping := range mappings {
		if !valid[mapping.SourceDescriptor] || seenSource[mapping.SourceDescriptor] {
			return fmt.Errorf("%w: mapping source %q is unknown or repeated", errInvalid, mapping.SourceDescriptor)
		}
		if _, ok := p.dests[mapping.DestinationDescriptor]; !ok || seenDest[mapping.DestinationDescriptor] {
			return fmt.Errorf("%w: mapping destination %q is not a planned PostgreSQL resource or is repeated", errInvalid, mapping.DestinationDescriptor)
		}
		seenSource[mapping.SourceDescriptor], seenDest[mapping.DestinationDescriptor] = true, true
	}
	for descriptor := range valid {
		if !seenSource[descriptor] {
			return unsupported("source database %s is not accounted for in the mapping", descriptor)
		}
	}
	sort.Slice(mappings, func(i, j int) bool { return mappings[i].SourceDescriptor < mappings[j].SourceDescriptor })
	p.mappings = mappings
	return nil
}

// Execute re-validates the token, claims the Environment atomically and runs
// the staged transition. The claim exists before any workload is stopped.
func (s *Service) Execute(ctx context.Context, req ExecuteRequest) (Detail, error) {
	p, err := s.prepare(ctx, req.Request, true)
	if err != nil {
		return Detail{}, err
	}
	preview, err := s.previewOf(p, req.Request)
	if err != nil {
		return Detail{}, err
	}
	if req.Token == "" || req.Token != preview.Token {
		return Detail{}, ErrStaleToken
	}
	if req.Mode == environment.ModeMigratePostgres && !req.AcknowledgeDowntime {
		return Detail{}, ErrAcknowledge
	}
	lease, err := s.ops.Begin(ctx, envops.Claim{
		ApplicationKey: p.app.Key, EnvironmentKey: p.env.Key, Kind: environment.OpTransition,
		Pins: environment.OperationPins{
			CheckEnvVersion: true, EnvVersion: p.env.Version,
			CheckDraftVersion: true, DraftVersion: p.env.DraftVersion,
			CheckConfigVersion: true, ConfigVersion: p.scope.Version,
			CheckSet: true, CurrentSetID: p.set.ID,
			CheckRevision: true, DesiredRevisionID: p.scope.Revision,
			CheckBinding: true, Binding: p.env.Binding(),
			CheckStore: true, SecretStoreKey: p.env.SecretStoreKey,
		},
		Detail: map[string]any{"mode": string(req.Mode), "destination": req.DestinationKey},
	})
	if errors.Is(err, persistence.ErrVersionConflict) {
		return Detail{}, ErrStaleToken
	}
	if err != nil {
		return Detail{}, err
	}
	generation, err := s.store.AllocateGeneration(lease.Ctx, p.app.Key, p.env.Key)
	if err != nil || generation != p.dest.TargetGeneration {
		_ = lease.End(environment.OpFailed, "the target generation could not be allocated")
		if err == nil {
			err = ErrStaleToken
		}
		return Detail{}, err
	}
	now := time.Now().UTC()
	t := &environment.Transition{
		ID: ids.New(), OperationID: lease.Operation.ID, ApplicationKey: p.app.Key, EnvironmentKey: p.env.Key, Mode: req.Mode,
		Stage: environment.StagePreflight, Status: environment.TransitionRunning,
		Source: p.env.Binding(), Destination: p.dest.Binding(), SourceRuntime: string(p.env.RuntimeStatus), DestinationRuntime: string(p.dest.RuntimeStatus),
		OldSetID: p.set.ID, ConfigRevisionID: p.scope.Revision, SourceNamespace: p.env.Namespace(), DestNamespace: p.dest.Namespace(),
		Replicas: map[string]int{}, Mappings: p.mappings, SourceState: environment.SourceAuthoritative, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.store.SaveTransition(lease.Ctx, *t); err != nil {
		_ = lease.End(environment.OpFailed, "the transition could not be recorded")
		return Detail{}, err
	}
	run := &runner{s: s, lease: lease, p: p, t: t, actor: req.Actor}
	if s.async {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			run.execute()
		}()
		return s.detail(*t, p.srcConn, p.destConn), nil
	}
	run.execute()
	return s.detail(*run.t, p.srcConn, p.destConn), nil
}

func (s *Service) detail(t environment.Transition, src, dest appdomain.Connection) Detail {
	d := Detail{
		ID: t.ID, OperationID: t.OperationID, Mode: t.Mode, Stage: t.Stage, Status: t.Status,
		Source: bindingEndpoint(t.Source, t.SourceNamespace, src), Destination: bindingEndpoint(t.Destination, t.DestNamespace, dest),
		SourceState: t.SourceState, Mappings: t.Mappings, Stages: t.Stages, Failure: t.Failure, Compensation: t.Compensation, CompensationFailed: t.CompensationFailed,
		CreatedAt: t.CreatedAt.Format(time.RFC3339), UpdatedAt: t.UpdatedAt.Format(time.RFC3339),
	}
	if d.Stages == nil {
		d.Stages = []environment.StageResult{}
	}
	for _, backup := range t.Backups {
		if !backup.Removed {
			d.BackupRetained = true
		}
	}
	d.Authority = "SOURCE"
	if t.Status == environment.TransitionSucceeded {
		d.Authority = "DESTINATION"
	}
	d.CanCleanupSource = t.Status == environment.TransitionSucceeded && (t.SourceState == environment.SourceQuiesced || t.SourceState == environment.SourceCleanupFailed)
	return d
}

func bindingEndpoint(b environment.Binding, namespace string, conn appdomain.Connection) Endpoint {
	name := conn.Name
	if name == "" {
		name = conn.Key
	}
	if name == "" {
		name = b.ConnectionKey
	}
	return Endpoint{ConnectionKey: b.ConnectionKey, ConnectionName: name, Kind: string(conn.Kind), Profile: string(b.Profile), Region: b.Region, Generation: b.Generation, Namespace: namespace}
}

// Get returns the refresh-stable detail of one transition.
func (s *Service) Get(ctx context.Context, org, appKey, envKey, id string) (Detail, error) {
	t, err := s.scoped(ctx, org, appKey, envKey, id)
	if err != nil {
		return Detail{}, err
	}
	return s.detailOf(ctx, org, t), nil
}

// List returns the transitions of an Environment, newest first.
func (s *Service) List(ctx context.Context, org, appKey, envKey string) ([]Detail, error) {
	app, err := s.store.GetApplication(ctx, appKey)
	if err != nil || app.OrganizationKey != org {
		return nil, fmt.Errorf("%w: application %q", persistence.ErrNotFound, appKey)
	}
	list, err := s.store.ListTransitions(ctx, appKey, envKey)
	if err != nil {
		return nil, err
	}
	out := make([]Detail, 0, len(list))
	for _, t := range list {
		out = append(out, s.detailOf(ctx, org, t))
	}
	return out, nil
}

func (s *Service) detailOf(ctx context.Context, org string, t environment.Transition) Detail {
	src, _ := s.store.GetConnection(ctx, org, t.Source.ConnectionKey)
	dest, _ := s.store.GetConnection(ctx, org, t.Destination.ConnectionKey)
	d := s.detail(t, src, dest)
	// Authority comes from the persisted binding: an interruption after the
	// cutover committed leaves the destination authoritative.
	committed := false
	if env, err := s.store.GetEnvironment(ctx, t.ApplicationKey, t.EnvironmentKey); err == nil {
		committed = env.ConnectionKey == t.Destination.ConnectionKey && env.TargetGeneration == t.Destination.Generation
		d.Authority = "SOURCE"
		if committed {
			d.Authority = "DESTINATION"
		}
	}
	// A record left RUNNING by a stopped process reads INTERRUPTED, not running.
	if t.Status == environment.TransitionRunning {
		if op, err := s.store.GetOperation(ctx, t.OperationID); err != nil || op.Status == environment.OpInterrupted || op.Status == environment.OpRecovering || !op.Status.Holds() {
			d.Status = environment.TransitionInterrupted
		}
	}
	d.CanCleanupSource = d.CanCleanupSource && committed
	return d
}

func (s *Service) scoped(ctx context.Context, org, appKey, envKey, id string) (environment.Transition, error) {
	app, err := s.store.GetApplication(ctx, appKey)
	if err != nil || app.OrganizationKey != org {
		return environment.Transition{}, fmt.Errorf("%w: application %q", persistence.ErrNotFound, appKey)
	}
	t, err := s.store.GetTransition(ctx, id)
	if err != nil {
		return environment.Transition{}, err
	}
	if t.ApplicationKey != appKey || t.EnvironmentKey != envKey {
		return environment.Transition{}, fmt.Errorf("%w: transition %q", persistence.ErrNotFound, id)
	}
	return t, nil
}

func executionTarget(env environment.Environment, app appdomain.Application) execution.Target {
	return execution.Target{Namespace: env.Namespace(), Extra: map[string]string{"application": app.Key, "environment": env.Key}}
}
