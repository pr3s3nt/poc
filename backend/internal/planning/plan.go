package planning

import (
	"fmt"
	"reflect"
	"sort"

	"orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning/placeholder"
	"orchestrator/internal/planning/score"
	"orchestrator/internal/platform/canon"
)

// Service is the deterministic planning pipeline shared by UC-05, UC-06 and UC-07.
type Service struct {
	bundles map[string]resource.RenderBundle
}

// NewService captures immutable rendering configuration. Planning performs no
// I/O or side effects (OC-07).
func NewService(bundles ...map[string]resource.RenderBundle) *Service {
	s := &Service{}
	if len(bundles) > 0 {
		s.bundles = resource.CopyRenderBundles(bundles[0])
	}
	return s
}

// Plan runs the full pipeline and returns an immutable Deployment Plan.
func (s *Service) Plan(req Request) (*Plan, error) {
	if err := req.App.Validate(); err != nil {
		return nil, err
	}
	if err := req.Env.Validate(); err != nil {
		return nil, err
	}
	if req.WorkloadID == "" {
		return nil, fmt.Errorf("planning: workload id is required")
	}
	if !req.Env.Configured() {
		return nil, &StageError{Stage: StageCatalog, Reason: ReasonUnconfigured, Err: ErrEnvironmentUnconfigured}
	}
	ctx := Context{
		OrganizationKey:  req.OrganizationKey,
		App:              req.App,
		Env:              req.Env,
		Connection:       req.Connection,
		RunID:            req.RunID,
		ReferenceCluster: req.ReferenceCluster,
	}
	if err := ctx.checkImplicitConnection(); err != nil {
		return nil, stageErr(StageCatalog, err)
	}

	base := normalize(req.BaseSet)

	var beforeFragment, afterFragment *score.Fragment
	var err error
	if req.Before != nil {
		if beforeFragment, err = req.Before.Fragment(req.Catalog.Types); err != nil {
			return nil, stageErr(StageScore, err)
		}
	}
	if req.After != nil {
		if afterFragment, err = req.After.Fragment(req.Catalog.Types); err != nil {
			return nil, stageErr(StageScore, err)
		}
	}
	if err := validateBeforeState(base, req.WorkloadID, beforeFragment); err != nil {
		return nil, err
	}

	built, err := DeltaBuilder{}.BuildHumanitecDelta(base, req.WorkloadID, beforeFragment, afterFragment)
	if err != nil {
		return nil, stageErr(StageCandidate, err)
	}
	candidate := built.Candidate
	if !req.AllowIntermediateEnvironmentState {
		if err := ValidatePublicRoutes(candidate); err != nil {
			return nil, stageErr(StageRoutes, err)
		}
		if err := ValidateServiceReferences(candidate); err != nil {
			return nil, stageErr(StageServiceRefs, err)
		}
	}

	builder := newGraphBuilder(ctx, req.Catalog)
	namespace, err := builder.enrichProfile()
	if err != nil {
		return nil, stageErr(StageGraph, err)
	}
	if err := builder.buildFromSet(candidate, namespace); err != nil {
		return nil, stageErr(StageGraph, err)
	}

	matcher, err := newDefinitionMatcher(ctx, req.Catalog)
	if err != nil {
		return nil, stageErr(StageCatalog, err)
	}
	matches := map[string]Match{}
	refs, err := builder.expand(matcher, matches)
	if err != nil {
		return nil, stageErr(StageGraph, err)
	}
	if err := builder.validateReferenceOutputs(refs, matches, matcher, req.Terraform); err != nil {
		return nil, stageErr(StageReference, err)
	}
	contracts, err := builder.inspectTerraform(matches, matcher, req.Terraform)
	if err != nil {
		return nil, stageErr(StageContract, err)
	}

	graph := builder.graph()
	batches, err := scheduleBatches(graph)
	if err != nil {
		return nil, stageErr(StageSchedule, err)
	}

	action := req.Action
	if action == "" {
		action = deployment.ActionDeploy
	}

	plan := &Plan{
		WorkloadID:     req.WorkloadID,
		Action:         action,
		Delta:          built.Delta,
		BaseSet:        base,
		CandidateSet:   candidate,
		Graph:          graph,
		Matches:        matches,
		Terraform:      contracts,
		Batches:        batches,
		Classification: classify(ctx, graph, req.Active),
		Target:         req.Env.Binding(),
	}
	plan.Rendering, err = s.selectRendering(ctx, graph, matcher, req.Catalog)
	if err != nil {
		return nil, stageErr(StageCatalog, err)
	}
	plan.UnreferencedResources = unreferencedResources(ctx, graph, req.Active)
	if req.Before != nil {
		if plan.ScoreBefore, err = canon.Map(req.Before); err != nil {
			return nil, err
		}
	}
	if req.After != nil {
		if plan.ScoreAfter, err = canon.Map(req.After); err != nil {
			return nil, err
		}
	}
	hashInput := map[string]any{
		"workloadId":   plan.WorkloadID,
		"candidateSet": plan.CandidateSet,
		"graph":        plan.Graph,
		"matches":      plan.Matches,
		"terraform":    plan.Terraform,
		"batches":      plan.Batches,
		"target":       plan.Target,
	}
	if len(plan.Rendering) > 0 {
		hashInput["rendering"] = plan.Rendering
	}
	hash, err := canon.Hash(hashInput)
	if err != nil {
		return nil, err
	}
	plan.PlanHash = hash
	return plan, nil
}

// normalize returns a document with non-nil maps.
func normalize(doc environment.Document) environment.Document {
	out := environment.NewDocument()
	for id, m := range doc.Modules {
		out.Modules[id] = m
	}
	for id, entry := range doc.Shared {
		out.Shared[id] = entry
	}
	return out
}

// validateBeforeState proves the plan starts from the caller's snapshot (UC-07 MS-02).
func validateBeforeState(base environment.Document, workloadID string, before *score.Fragment) error {
	current, exists := base.Modules[workloadID]
	if before == nil {
		if exists {
			return beforeErr(ReasonWorkloadExists, "planning: workload %q already exists; a before Score is required", workloadID)
		}
		return nil
	}
	if !exists {
		return beforeErr(ReasonWorkloadNotCurrent, "planning: workload %q is not part of the current Deployment Set", workloadID)
	}
	if before.WorkloadID != workloadID {
		return beforeErr(ReasonBeforeMismatch, "planning: before Score describes workload %q, not %q", before.WorkloadID, workloadID)
	}
	expected, err := canon.Map(before.Module)
	if err != nil {
		return err
	}
	actual, err := canon.Map(current)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(expected, actual) {
		return beforeErr(ReasonBeforeMismatch, "planning: before Score does not match the current contribution of workload %q", workloadID)
	}
	// The shared contribution of the before Score must match the current set too,
	// otherwise the plan starts from a snapshot somebody else has already changed.
	for _, id := range sortedKeys(before.Shared) {
		current, exists := base.Shared[id]
		if !exists {
			return beforeErr(ReasonBeforeMismatch, "planning: before Score declares shared resource %q which is not in the current Deployment Set", id)
		}
		expectedShared, err := canon.Map(before.Shared[id])
		if err != nil {
			return err
		}
		actualShared, err := canon.Map(current)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(expectedShared, actualShared) {
			return beforeErr(ReasonBeforeMismatch, "planning: before Score does not match the current shared resource %q", id)
		}
	}
	return nil
}

// buildCandidate applies the after fragment to the base set. A shared entry is
// kept while at least one module still references it or while the fragment
// being deployed declares it; nothing is ever destroyed implicitly
// (UC-07 BR-03, UC-08 BR-05).
func buildCandidate(base environment.Document, workloadID string, before, after *score.Fragment) (environment.Document, error) {
	candidate := environment.NewDocument()
	for id, m := range base.Modules {
		candidate.Modules[id] = m
	}
	for id, entry := range base.Shared {
		candidate.Shared[id] = entry
	}

	if after == nil {
		delete(candidate.Modules, workloadID)
	} else {
		if after.WorkloadID != workloadID {
			return candidate, fmt.Errorf("planning: Score describes workload %q, not %q", after.WorkloadID, workloadID)
		}
		candidate.Modules[workloadID] = after.Module
	}

	// A workload that stops declaring a shared resource drops its own claim on
	// it. The entry only leaves the set when no other module still references it
	// (UC-07 BR-03); the planner reference has no such rule because it has no
	// cross-workload ownership model.
	if before != nil {
		referencedByOthers, err := sharedReferencesExcept(candidate, workloadID)
		if err != nil {
			return candidate, err
		}
		for _, id := range sortedKeys(before.Shared) {
			if referencedByOthers[id] {
				continue
			}
			if after == nil {
				delete(candidate.Shared, id)
				continue
			}
			if _, stillDeclared := after.Shared[id]; !stillDeclared {
				delete(candidate.Shared, id)
			}
		}
	}

	if after != nil {
		for _, id := range sortedKeys(after.Shared) {
			entry := after.Shared[id]
			existing, exists := candidate.Shared[id]
			declaredBefore := false
			if before != nil {
				_, declaredBefore = before.Shared[id]
			}
			if exists && !declaredBefore {
				same, err := sameEntry(existing, entry)
				if err != nil {
					return candidate, err
				}
				if !same {
					return candidate, fmt.Errorf(
						"planning: shared resource %q already exists with different content; the workload must declare the same type, class and params", id)
				}
			}
			candidate.Shared[id] = entry
		}
	}

	referenced := map[string]bool{}
	if after != nil {
		for id := range after.Shared {
			referenced[id] = true
		}
	}
	for _, id := range candidate.ModuleIDs() {
		ids, err := sharedReferences(candidate.Modules[id])
		if err != nil {
			return candidate, err
		}
		for _, sharedID := range ids {
			referenced[sharedID] = true
		}
	}
	for _, id := range candidate.SharedIDs() {
		if !referenced[id] {
			delete(candidate.Shared, id)
		}
	}
	return candidate, nil
}

// sharedReferencesExcept lists the shared IDs referenced by every module other
// than the one being deployed.
func sharedReferencesExcept(doc environment.Document, workloadID string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, id := range doc.ModuleIDs() {
		if id == workloadID {
			continue
		}
		ids, err := sharedReferences(doc.Modules[id])
		if err != nil {
			return nil, err
		}
		for _, sharedID := range ids {
			out[sharedID] = true
		}
	}
	return out, nil
}

// sameEntry compares two Deployment Set resource entries structurally.
func sameEntry(a, b environment.ResourceEntry) (bool, error) {
	left, err := canon.Map(a)
	if err != nil {
		return false, err
	}
	right, err := canon.Map(b)
	if err != nil {
		return false, err
	}
	return reflect.DeepEqual(left, right), nil
}

// sortedKeys returns map keys in deterministic order.
func sortedKeys[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// sharedReferences lists the shared resource IDs a module refers to.
func sharedReferences(module environment.Module) ([]string, error) {
	tree, err := canon.Map(module)
	if err != nil {
		return nil, err
	}
	found := map[string]bool{}
	err = placeholder.WalkStrings(tree, "", func(_, value string) error {
		refs, err := placeholder.Refs(value)
		if err != nil {
			return nil // structural errors surface during graph building
		}
		for _, ref := range refs {
			if ref.Kind != placeholder.KindResource {
				continue
			}
			if len(ref.Resource) > len(placeholder.ScopeShared)+1 &&
				ref.Resource[:len(placeholder.ScopeShared)+1] == placeholder.ScopeShared+"." {
				found[ref.Resource[len(placeholder.ScopeShared)+1:]] = true
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(found))
	for id := range found {
		out = append(out, id)
	}
	sort.Strings(out)
	return out, nil
}

// classify records which desired resources already exist and which Active
// Resources are no longer referenced. Classification is evidence only; it never
// implies destroy.
func classify(ctx Context, g Graph, active []resource.ActiveResource) Classification {
	desired := map[string]bool{}
	out := Classification{Existing: []string{}, New: []string{}, Unreferenced: []string{}}
	activeByKey := map[string]resource.ActiveResource{}
	for _, a := range active {
		activeByKey[a.LogicalKey()] = a
	}
	for _, n := range g.Nodes {
		if n.Kind != NodeResource {
			continue
		}
		d, err := resource.ParseDescriptor(n.Descriptor)
		if err != nil {
			continue
		}
		key := resource.LogicalKey(ctx.OrganizationKey, d, n.Scope)
		desired[key] = true
		if _, ok := activeByKey[key]; ok {
			out.Existing = append(out.Existing, n.Descriptor)
		} else {
			out.New = append(out.New, n.Descriptor)
		}
	}
	// Unreferenced follows the same policy as the final marker, so the count
	// shown before Deploy is what the deployment will mark.
	for _, a := range unreferencedResources(ctx, g, active) {
		out.Unreferenced = append(out.Unreferenced, a.Descriptor.String())
	}
	sort.Strings(out.Existing)
	sort.Strings(out.New)
	sort.Strings(out.Unreferenced)
	return out
}

// unreferencedResources returns the Active Resources this Environment's
// deployment marks UNREFERENCED (UC-07 MS-07): READY (the only state machine
// source of READY -> UNREFERENCED), owned by the Environment's own scopes,
// absent from the desired graph, identified by full logical identity.
// Application scope is shared by every Environment of the Application, so one
// Environment's plan never marks it; application-wide cleanup is unsupported.
func unreferencedResources(ctx Context, g Graph, active []resource.ActiveResource) []resource.ActiveResource {
	desired := map[string]bool{}
	for _, n := range g.Nodes {
		if n.Kind != NodeResource {
			continue
		}
		if d, err := resource.ParseDescriptor(n.Descriptor); err == nil {
			desired[resource.LogicalKey(ctx.OrganizationKey, d, n.Scope)] = true
		}
	}
	out := []resource.ActiveResource{}
	for _, a := range active {
		if a.Status != resource.StatusReady || a.OrganizationKey != ctx.OrganizationKey || a.Scope.Type == resource.ScopeApplication || desired[a.LogicalKey()] || !ownedByEnvironment(ctx, a.Scope) {
			continue
		}
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LogicalKey() < out[j].LogicalKey() })
	return out
}

func ownedByEnvironment(ctx Context, scope resource.Scope) bool {
	envScope := environment.ScopeID(ctx.App.Key, ctx.Env.Key, ctx.Env.TargetGeneration)
	switch scope.Type {
	case resource.ScopeApplication:
		return scope.ID == ctx.App.Key
	case resource.ScopeEnvironment, resource.ScopeShared:
		return scope.ID == envScope
	case resource.ScopeWorkload:
		return len(scope.ID) > len(envScope) && scope.ID[:len(envScope)+1] == envScope+"."
	}
	return false
}
