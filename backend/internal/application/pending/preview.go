// Package pending coordinates UC-05 preview and UC-06/07 deployment of UC-12/16 desired changes.
package pending

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	appsvc "orchestrator/internal/application/deployment"
	"orchestrator/internal/application/deploymentreview"
	"orchestrator/internal/application/envops"
	"orchestrator/internal/application/target"
	"orchestrator/internal/application/workloadconfig"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/configuration"
	domain "orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/planning"
	"orchestrator/internal/planning/score"
	"orchestrator/internal/platform/canon"
	"orchestrator/internal/ports/persistence"
)

var ErrStalePreview = errors.New("pending: preview is stale; preview changes again")
var ErrInvalid = errors.New("pending: invalid desired configuration")

type Change struct {
	Rendering  resource.RenderingSelection `json:"rendering"`
	WorkloadID string                      `json:"workloadId"`
	Action     domain.Action               `json:"action"`
	Delta      domain.DeltaDocument        `json:"delta"`
	Resources  planning.Classification     `json:"resources"`
	// ResourceIntents is the safe resource projection (UC-05 BR-10): no
	// params, resolved inputs, secret or state reference.
	ResourceIntents []planning.ResourceIntent `json:"resourceIntents"`
	// Placements are the safe per-workload/resource cluster and namespace pins.
	Placements []planning.Placement `json:"placements"`
	PlanHash   string               `json:"planHash"`
}

type Preview struct {
	Review         deploymentreview.Review                `json:"review"`
	lastPlan       *planning.Plan                         `json:"-"`
	previous       map[string][]deploymentreview.Artifact `json:"-"`
	lastIntents    []planning.ResourceIntent              `json:"-"`
	final          environment.Document                   `json:"-"`
	Token          string                                 `json:"token"`
	ApplicationKey string                                 `json:"applicationKey"`
	EnvironmentKey string                                 `json:"environmentKey"`
	BaseSetID      string                                 `json:"baseDeploymentSetId"`
	BaseVersion    int64                                  `json:"baseVersion"`
	DraftVersion   int64                                  `json:"draftVersion"`
	// ConfigVersion, Binding and SecretStoreKey pin the rest of the snapshot
	// (ADR-012): any Settings, configuration or draft change invalidates the token.
	ConfigVersion    int64               `json:"configVersion"`
	Binding          environment.Binding `json:"binding"`
	SecretStoreKey   string              `json:"secretStoreKey,omitempty"`
	RoutePending     bool                `json:"routePending,omitempty"`
	ConfigRevisionID string              `json:"configRevisionId,omitempty"`
	RunID            string              `json:"runId"`
	Changes          []Change            `json:"changes"`
}

type Service struct {
	store             persistence.Store
	planner           *planning.Service
	workloads         *workloadconfig.Service
	terraform         planning.ModuleInspector
	deployer          *appsvc.Service
	imageRegistryHost string
	ops               *envops.Manager
	reviewer          deploymentreview.Assembler
}

func (s *Service) SetReviewer(reviewer deploymentreview.Assembler) { s.reviewer = reviewer }

// SetOperations shares the Environment claim manager with the other services.
func (s *Service) SetOperations(ops *envops.Manager) { s.ops = ops }

func (s *Service) operations() *envops.Manager {
	if s.ops == nil {
		s.ops = envops.NewManager(s.store)
	}
	return s.ops
}

func NewService(store persistence.Store, planner *planning.Service, workloads *workloadconfig.Service, inspector planning.ModuleInspector) *Service {
	return &Service{store: store, planner: planner, workloads: workloads, terraform: inspector}
}

// SetImageRegistryHost constrains the optional Fleet mode at Preview time,
// before UC-08 provisions a namespace or other resources.
func (s *Service) SetImageRegistryHost(host string) { s.imageRegistryHost = host }

// Preview plans the pending changes against ONE read-only snapshot (UC-05 BR-01,
// OC-06): the Application, Environment, Connection, Deployment Set, configuration,
// drafts, instances, catalog, Active Resources and the baseline plans behind the
// resource intents are all read through the same view; nothing is read from the
// outer store while planning. Nothing is written.
func (s *Service) Preview(ctx context.Context, appKey, envKey string) (Preview, error) {
	var out Preview
	err := s.store.ReadSnapshot(ctx, func(ctx context.Context, view persistence.Store) error {
		var err error
		out, err = s.previewIn(ctx, view, appKey, envKey, false)
		return err
	})
	if err != nil {
		return Preview{}, err
	}
	if out.lastPlan != nil {
		out.Review = s.reviewer.Assemble(ctx, out.lastPlan, out.lastIntents, out.previous)
		// Desired actions belong to the complete pending batch, not only its last step.
		actions := map[string]string{}
		for _, change := range out.Changes {
			actions[change.WorkloadID] = string(change.Action)
		}
		for i := range out.Review.Nodes {
			node := &out.Review.Nodes[i]
			if node.Kind == "workload" {
				if action, ok := actions[node.Name]; ok {
					node.Intent = action
					if action == "DEPLOY" {
						node.Intent = "CREATE"
					}
				}
			}
		}
	}
	return out, nil
}

// previewIn plans on one view. With all=true every workload of the final
// desired state is planned, including the unchanged ones (diagnostics need the
// whole desired state, Preview only the delta).
func (s *Service) previewIn(ctx context.Context, st persistence.Store, appKey, envKey string, all bool) (Preview, error) {
	app, err := st.GetApplication(ctx, appKey)
	if err != nil {
		return Preview{}, err
	}
	env, err := st.GetEnvironment(ctx, appKey, envKey)
	if err != nil {
		return Preview{}, err
	}

	set, err := st.GetDeploymentSet(ctx, env.CurrentDeploymentSetID)
	if err != nil {
		return Preview{}, err
	}
	scope, err := st.GetConfigurationScope(ctx, appKey, envKey)
	if err != nil {
		return Preview{}, err
	}
	desired := configuration.Revision{Entries: map[string]configuration.Entry{}}
	if scope.DesiredRevisionID != "" {
		desired, err = st.GetConfigurationRevision(ctx, scope.DesiredRevisionID)
		if err != nil {
			return Preview{}, err
		}
	}
	drafts, err := st.ListWorkloadDrafts(ctx, appKey, envKey)
	if err != nil {
		return Preview{}, err
	}
	instances, err := st.ListWorkloadInstances(ctx, appKey+"/"+envKey)
	if err != nil {
		return Preview{}, err
	}
	instanceByID := map[string]domain.WorkloadInstance{}
	for _, instance := range instances {
		instanceByID[instance.WorkloadID] = instance
	}

	types, err := st.ListResourceTypes(ctx, app.OrganizationKey)
	if err != nil {
		return Preview{}, err
	}
	definitions, err := st.ListResourceDefinitions(ctx, app.OrganizationKey)
	if err != nil {
		return Preview{}, err
	}
	// Active Resources and their baseline plans come from the same snapshot view; the intent
	// projection below reads only these captured records.
	active, err := st.ListActiveResources(ctx, app.OrganizationKey)
	if err != nil {
		return Preview{}, err
	}
	baselines, err := appsvc.CaptureBaselines(ctx, st, app.OrganizationKey, active)
	if err != nil {
		return Preview{}, err
	}
	reviewBaselines, err := deploymentreview.CapturePrevious(ctx, st, app.OrganizationKey, appKey, envKey, env.TargetGeneration, active)
	if err != nil {
		return Preview{}, err
	}
	typeMap := map[string]resource.Type{}
	for _, typ := range types {
		typeMap[typ.Key] = typ
	}
	accounts, err := target.Accounts(ctx, st, app.OrganizationKey)
	if err != nil {
		return Preview{}, err
	}
	renderBundles, err := persistence.RenderBundlesOf(ctx, st, app.OrganizationKey)
	if err != nil {
		return Preview{}, err
	}
	catalog := planning.Catalog{Types: typeMap, Definitions: definitions, Accounts: accounts, RenderBundles: renderBundles}

	pending := map[string]environment.WorkloadDraft{}
	for _, draft := range drafts {
		pending[draft.WorkloadID] = draft
	}
	for id, module := range set.Document.Modules {
		if _, exists := pending[id]; exists {
			continue
		}
		if all {
			raw, err := workloadconfig.ReconstructScoreProven(id, module, set.Document, set.LiteralProofs.ProverFor(id), set.ResourceAliases[id])
			if err != nil {
				return Preview{}, fmt.Errorf("%w: workload %s cannot be rebuilt: %v", ErrInvalid, id, err)
			}
			pending[id] = environment.WorkloadDraft{ApplicationKey: appKey, EnvironmentKey: envKey, WorkloadID: id, State: environment.DraftUpsert, Score: raw}
			continue
		}
		changed, err := usesChangedConfiguration(ctx, st, module, desired, instanceByID[id].AppliedConfigRevisionID)
		if err != nil {
			return Preview{}, err
		}
		selected, err := s.planner.SelectWorkloadRendering(app, env, id, module.WorkloadClass(), catalog)
		if err != nil {
			return Preview{}, publicPlanningError(id, err)
		}
		unchangedRenderer, err := sameRendering(ctx, st, instanceByID[id].LastDeploymentID, id, selected)
		if err != nil {
			return Preview{}, err
		}
		if !changed && unchangedRenderer {
			continue
		}
		raw, err := workloadconfig.ReconstructScoreProven(id, module, set.Document, set.LiteralProofs.ProverFor(id), set.ResourceAliases[id])
		if err != nil {
			return Preview{}, fmt.Errorf("%w: workload %s cannot be rebuilt: %v", ErrInvalid, id, err)
		}
		pending[id] = environment.WorkloadDraft{ApplicationKey: appKey, EnvironmentKey: envKey, WorkloadID: id, State: environment.DraftUpsert, Score: raw}
	}
	ordered, err := orderDrafts(pending)
	if err != nil {
		return Preview{}, err
	}
	preview := Preview{ApplicationKey: appKey, EnvironmentKey: envKey, BaseSetID: set.ID, BaseVersion: env.Version, DraftVersion: env.DraftVersion, RoutePending: env.PublicRoutesPending, ConfigRevisionID: scope.DesiredRevisionID,
		ConfigVersion: scope.Version, Binding: env.Binding(), SecretStoreKey: env.SecretStoreKey, Changes: []Change{}, previous: reviewBaselines}
	// Keep the execution identity stable across revisions of one environment.
	// The preview token separately pins the mutable base/draft/configuration state.
	pinHash, err := canon.Hash([]string{appKey, envKey})
	if err != nil {
		return Preview{}, err
	}
	preview.RunID = "pending-" + pinHash[:16]
	base := set.Document
	for _, id := range ordered {
		draft := pending[id]
		var before, after *score.Document
		if module, exists := base.Modules[id]; exists {
			raw, err := workloadconfig.ReconstructScoreProven(id, module, base, set.LiteralProofs.ProverFor(id), set.ResourceAliases[id])
			if err != nil {
				return Preview{}, fmt.Errorf("%w: workload %s cannot be updated: %v", ErrInvalid, id, err)
			}
			before, err = score.FromMap(raw)
			if err != nil {
				return Preview{}, err
			}
		}
		if draft.State == environment.DraftUpsert {
			if err := s.workloads.Bound(st).ValidateImport(ctx, appKey, envKey, draft.Score); err != nil {
				return Preview{}, err
			}
			after, err = score.FromMap(draft.Score)
			if err != nil {
				return Preview{}, err
			}
			if err := validateImageRegistry(after, s.imageRegistryHost); err != nil {
				return Preview{}, err
			}
		}
		action := domain.ActionDeploy
		if after == nil {
			action = domain.ActionRemove
		} else if before != nil {
			action = domain.ActionUpdate
		}
		plan, err := s.planner.Plan(planning.Request{OrganizationKey: app.OrganizationKey, App: app, Env: env, BaseSet: base, Before: before, After: after, WorkloadID: id, Action: action, Catalog: catalog, Active: active, Terraform: s.terraform, RunID: preview.RunID,
			// Each step may be intermediate; the final batch state is validated below.
			AllowIntermediateEnvironmentState: true})
		if err != nil {
			return Preview{}, publicPlanningError(id, err)
		}
		if !all && draft.State == environment.DraftUpsert && before != nil {
			same, err := equivalentSets(base, plan.CandidateSet)
			if err != nil {
				return Preview{}, err
			}
			if same {
				changed, err := usesChangedConfiguration(ctx, st, base.Modules[id], desired, instanceByID[id].AppliedConfigRevisionID)
				if err != nil {
					return Preview{}, err
				}
				if !changed {
					sameRenderer, err := sameRendering(ctx, st, instanceByID[id].LastDeploymentID, id, plan.Rendering[id])
					if err != nil {
						return Preview{}, err
					}
					if sameRenderer {
						continue
					}
				}
			}
		}
		preview.Changes = append(preview.Changes, Change{Rendering: plan.Rendering[id], WorkloadID: id, Action: action, Delta: plan.Delta, Resources: plan.Classification, ResourceIntents: plan.ResourceIntents(baselines.Reader()), Placements: plan.PlacementList(), PlanHash: plan.PlanHash})
		preview.lastPlan = plan
		preview.lastIntents = plan.ResourceIntents(baselines.Reader())
		base = plan.CandidateSet
	}
	// Route and Service-reference rules apply to the final batch state only.
	if err := planning.ValidatePublicRoutes(base); err != nil {
		return Preview{}, publicPlanningError("", &planning.StageError{Stage: planning.StageRoutes, Err: err})
	}
	if err := planning.ValidateServiceReferences(base); err != nil {
		return Preview{}, publicPlanningError("", &planning.StageError{Stage: planning.StageServiceRefs, Err: err})
	}
	preview.final = base
	if preview.lastPlan != nil {
		// Keep execution plans/hashes untouched; review compares the final desired set to the original base.
		final := *preview.lastPlan
		final.BaseSet = set.Document
		preview.lastPlan = &final
	}
	preview.Token, err = canon.Hash(preview)
	return preview, err
}

// publicPlanningError replaces a raw planner message, which may quote
// catalog, Definition or other persisted content, with a fixed sentence.
// Unstaged errors are internal and returned unchanged.
func publicPlanningError(workloadID string, err error) error {
	message, ok := planning.PublicMessage(err)
	if !ok {
		return err
	}
	// Staged failures keep their stage/reason (never the raw text) so the
	// public envelope can tell no-match, tie or unavailable renderer and a
	// graph/class mismatch apart.
	var staged *planning.StageError
	if errors.As(err, &staged) {
		if workloadID != "" {
			return fmt.Errorf("%w: workload %s: %s: %w", ErrInvalid, workloadID, message, staged)
		}
		return fmt.Errorf("%w: %s: %w", ErrInvalid, message, staged)
	}
	if workloadID != "" {
		return fmt.Errorf("%w: workload %s: %s", ErrInvalid, workloadID, message)
	}
	return fmt.Errorf("%w: %s", ErrInvalid, message)
}

func equivalentSets(a, b environment.Document) (bool, error) {
	// Compare effective Service defaults and canonical route order.
	a = canonicalServices(a)
	b = canonicalServices(b)
	left, err := canon.Hash(a)
	if err != nil {
		return false, err
	}
	right, err := canon.Hash(b)
	return left == right, err
}

func canonicalServices(set environment.Document) environment.Document {
	copy := environment.Document{Modules: make(map[string]environment.Module, len(set.Modules)), Shared: set.Shared}
	for id, module := range set.Modules {
		module.Spec.Service = module.Spec.Service.Canonical()
		copy.Modules[id] = module
	}
	return copy
}

func validateImageRegistry(doc *score.Document, host string) error {
	if host == "" || doc == nil {
		return nil
	}
	for name, container := range doc.Containers {
		if !strings.HasPrefix(container.Image, host+"/") {
			return fmt.Errorf("%w: container %s image must come from the configured Harbor registry", ErrInvalid, name)
		}
	}
	return nil
}

func usesChangedConfiguration(ctx context.Context, store persistence.Store, module environment.Module, desired configuration.Revision, appliedID string) (bool, error) {
	if desired.ID == appliedID {
		return false, nil
	}
	applied := configuration.Revision{Entries: map[string]configuration.Entry{}}
	if appliedID != "" {
		loaded, err := store.GetConfigurationRevision(ctx, appliedID)
		if err != nil {
			return false, err
		}
		applied = loaded
	}
	if e := module.Spec.Execution; e != nil {
		for _, file := range e.SecretFiles {
			key := strings.TrimSuffix(strings.TrimPrefix(file.Ref, "${context.uc12."), "}")
			if desired.Entries[key] != applied.Entries[key] {
				return true, nil
			}
		}
	}
	for _, container := range module.Spec.Containers {
		for _, value := range container.Variables {
			if !strings.HasPrefix(value, "${context.uc12.") || !strings.HasSuffix(value, "}") {
				continue
			}
			key := strings.TrimSuffix(strings.TrimPrefix(value, "${context.uc12."), "}")
			if desired.Entries[key] != applied.Entries[key] {
				return true, nil
			}
		}
	}
	return false, nil
}

func orderDrafts(drafts map[string]environment.WorkloadDraft) ([]string, error) {
	ids := make([]string, 0, len(drafts))
	for id := range drafts {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	state := map[string]int{}
	ordered := []string{}
	var visit func(string) error
	visit = func(id string) error {
		if state[id] == 1 {
			return fmt.Errorf("%w: Service reference cycle", ErrInvalid)
		}
		if state[id] == 2 {
			return nil
		}
		state[id] = 1
		draft := drafts[id]
		if draft.State == environment.DraftUpsert {
			parsed, err := score.FromMap(draft.Score)
			if err != nil {
				return err
			}
			for _, alias := range parsed.ResourceAliases() {
				resource := parsed.Resources[alias]
				if resource.Type != "service" {
					continue
				}
				target, _ := resource.Params["workload"].(string)
				if dep, exists := drafts[target]; exists && dep.State == environment.DraftUpsert {
					if err := visit(target); err != nil {
						return err
					}
				}
			}
		}
		state[id] = 2
		ordered = append(ordered, id)
		return nil
	}
	for _, id := range ids {
		if err := visit(id); err != nil {
			return nil, err
		}
	}
	return ordered, nil
}

func sameRendering(ctx context.Context, st persistence.Store, deploymentID, workloadID string, selected resource.RenderingSelection) (bool, error) {
	if deploymentID == "" {
		return selected.DriverType == "", nil
	}
	snapshot, err := st.GetPlan(ctx, deploymentID)
	if errors.Is(err, persistence.ErrNotFound) {
		return selected.DriverType == "", nil
	}
	if err != nil {
		return false, err
	}
	selections, _ := snapshot["rendering"].(map[string]any)
	old := selections[workloadID]
	if old == nil {
		return selected.DriverType == "", nil
	}
	a, err := canon.Hash(old)
	if err != nil {
		return false, err
	}
	selectedMap, err := canon.Map(selected)
	if err != nil {
		return false, err
	}
	b, err := canon.Hash(selectedMap)
	return a == b, err
}

// OrderDrafts returns workload IDs in Service-reference dependency order; a
// target transition deploys the complete Environment in the same order.
func OrderDrafts(drafts map[string]environment.WorkloadDraft) ([]string, error) {
	return orderDrafts(drafts)
}

// Desired is the complete desired state of one Environment, planned on ONE
// read-only snapshot together with every record it was planned from, so a
// diagnostic can never mix versions of the drafts, configuration, catalog or
// module pins.
type Desired struct {
	Preview     Preview
	Application application.Application
	Environment environment.Environment
	// ConfigVersion is the desired configuration scope version (0: none yet).
	ConfigVersion int64
	Definitions   []resource.Definition
	// Final is the final desired Deployment Set document (drafts applied).
	Final environment.Document
	// Bundles are the verified module bundles behind the planned resources,
	// keyed by resource descriptor.
	Bundles map[string]tfmodule.Bundle
	// Workloads and PublicRoutes count the final desired state (drafts on top of
	// the applied set) whether or not planning accepted it.
	Workloads, PublicRoutes int
	// PlanErr is the planner's rejection of the desired state, if any; the
	// remaining fields still describe the same snapshot.
	PlanErr error
}

// Desired plans every workload of the final desired state (drafts on top of
// the applied set, unchanged workloads included) on one snapshot. Nothing is
// written. A planner rejection is returned in PlanErr; any other failure is an
// error.
func (s *Service) Desired(ctx context.Context, appKey, envKey string) (Desired, error) {
	var out Desired
	err := s.store.ReadSnapshot(ctx, func(ctx context.Context, view persistence.Store) error {
		app, err := view.GetApplication(ctx, appKey)
		if err != nil {
			return err
		}
		env, err := view.GetEnvironment(ctx, appKey, envKey)
		if err != nil {
			return err
		}
		scope, err := view.GetConfigurationScope(ctx, appKey, envKey)
		if err != nil {
			return err
		}
		defs, err := view.ListResourceDefinitions(ctx, app.OrganizationKey)
		if err != nil {
			return err
		}
		out = Desired{Application: app, Environment: env, ConfigVersion: scope.Version, Definitions: defs, Bundles: map[string]tfmodule.Bundle{}}
		routes := map[string]int{}
		if applied, err := view.GetDeploymentSet(ctx, env.CurrentDeploymentSetID); err == nil {
			for id, module := range applied.Document.Modules {
				routes[id] = 0
				if module.Spec.Service != nil {
					routes[id] = len(module.Spec.Service.PublicRoutes)
				}
			}
		} else if !errors.Is(err, persistence.ErrNotFound) {
			return err
		}
		drafts, err := view.ListWorkloadDrafts(ctx, appKey, envKey)
		if err != nil {
			return err
		}
		for _, d := range drafts {
			if d.State == environment.DraftDelete {
				delete(routes, d.WorkloadID)
				continue
			}
			routes[d.WorkloadID] = 0
			if doc, err := score.FromMap(d.Score); err == nil && doc.Service != nil {
				routes[d.WorkloadID] = len(doc.Service.PublicRoutes)
			}
		}
		out.Workloads = len(routes)
		for _, n := range routes {
			out.PublicRoutes += n
		}
		preview, perr := s.previewIn(ctx, view, appKey, envKey, true)
		if perr != nil {
			var staged *planning.StageError
			if errors.As(perr, &staged) || errors.Is(perr, ErrInvalid) {
				out.PlanErr = perr
				return nil
			}
			return perr
		}
		out.Preview, out.Final = preview, preview.final
		for _, change := range preview.Changes {
			for _, intent := range change.ResourceIntents {
				if intent.Module == nil {
					continue
				}
				ref := tfmodule.SourceRef{SourceID: intent.Module.SourceID, ModuleID: intent.Module.Module, Revision: intent.Module.Revision, Subdirectory: intent.Module.Subdirectory, ContentSHA256: intent.Module.ContentSHA256}
				bundle, err := view.GetModuleBundle(ctx, ref)
				if err != nil {
					if errors.Is(err, persistence.ErrNotFound) {
						continue
					}
					return err
				}
				out.Bundles[intent.Descriptor] = bundle
			}
		}
		return nil
	})
	return out, err
}
