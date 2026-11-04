package planning

import (
	"errors"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/templatesource"
	"orchestrator/internal/platform/canon"
)

// renderMatcher narrows the Definition list to workload template Definitions.
// Renderer definitions are separate from infrastructure matches and provision batches.
func renderMatcher(ctx Context, matcher *definitionMatcher) *definitionMatcher {
	rm := &definitionMatcher{ctx: ctx, defs: matcher.defs, accounts: matcher.accounts}
	for _, def := range matcher.list {
		if def.ResourceTypeKey == "workload" && def.DriverType == resource.DriverTemplate {
			rm.list = append(rm.list, def)
		}
	}
	return rm
}

// pickWorkload selects and validates the workload Definition of one workload
// node. skipped is true when no Definition applies and the native renderer
// keeps the additive planner-reference behavior.
func (s *Service) pickWorkload(ctx Context, rm *definitionMatcher, node Node, catalog Catalog) (def resource.Definition, match Match, skipped bool, err error) {
	matchCtx := resource.MatchContext{EnvironmentType: ctx.Env.Type, ApplicationID: ctx.App.Key, EnvironmentID: ctx.Env.Key, ResourceID: descriptorID(node.Descriptor), Class: node.Class}
	eligible := false
	for _, candidate := range rm.list {
		if _, _, ok := candidate.BestCriterion(matchCtx); ok {
			eligible = true
		}
	}
	if !eligible && !s.requireRenderer {
		return resource.Definition{}, Match{}, true, nil
	}
	match, err = rm.match(&node)
	if err != nil {
		if s.requireRenderer {
			return resource.Definition{}, Match{}, false, rendererMatchError(err)
		}
		return resource.Definition{}, Match{}, false, err
	}
	def = rm.defs[match.DefinitionKey]
	if _, _, err := resource.ResolveRenderDefinition(def, catalog.Types["workload"], s.bundles, catalog.TemplateSources, ctx.DefinitionPlaced()); err != nil {
		if s.requireRenderer {
			return resource.Definition{}, Match{}, false, &StageError{Stage: StageCatalog, Reason: ReasonRendererUnavailable, Err: err}
		}
		return resource.Definition{}, Match{}, false, err
	}
	return def, match, false, nil
}

func (s *Service) selectRendering(ctx Context, graph Graph, matcher *definitionMatcher, catalog Catalog) (map[string]resource.RenderingSelection, error) {
	out := map[string]resource.RenderingSelection{}
	rm := renderMatcher(ctx, matcher)
	for _, node := range graph.Nodes {
		if node.Kind != NodeWorkload {
			continue
		}
		def, _, skipped, err := s.pickWorkload(ctx, rm, node, catalog)
		if err != nil {
			return nil, err
		}
		if skipped {
			continue
		}
		hashDefinition := def
		hashDefinition.RenderPin = nil // The rendering selection already pins the bundle independently.
		hash, err := canon.Hash(hashDefinition)
		if err != nil {
			return nil, err
		}
		pin, inputs, err := resource.ResolveRenderDefinition(def, catalog.Types["workload"], s.bundles, catalog.TemplateSources, ctx.DefinitionPlaced())
		if err != nil {
			return nil, &StageError{Stage: StageCatalog, Reason: ReasonRendererUnavailable, Err: err}
		}
		selection := resource.RenderingSelection{DefinitionKey: def.Key, DefinitionHash: hash, DriverType: def.DriverType, Class: node.Class, Bundle: pin}
		if refs, ok := def.Variables()["resource_references"].(map[string]any); ok {
			selection.References = map[string]string{}
			for name, value := range refs {
				selection.References[name], _ = value.(string)
				binding, output, _ := resource.WorkloadReference(selection.References[name])
				descriptor := node.Bindings[binding]
				for _, provider := range graph.Nodes {
					if provider.Descriptor == descriptor {
						if field, ok := catalog.Types[provider.ResourceType].Output(output); ok && field.Secret {
							return nil, &StageError{Stage: StageReference, Err: errors.New("template resource references cannot expose secret outputs")}
						}
					}
				}
			}
		}
		if pin.Git() {
			// The effective inputs and their hash are pinned so a bundle- or
			// input-only change is a pending workload update (UC-05 RB-01).
			selection.Inputs = inputs
			if selection.InputsHash, err = templatesource.InputsHash(inputs); err != nil {
				return nil, err
			}
		}
		out[node.WorkloadID] = selection
	}
	return out, nil
}

// SelectWorkloadRendering detects renderer-only pending updates without planning
// or validating unrelated workload changes. It uses the same selection as Plan.
func (s *Service) SelectWorkloadRendering(app application.Application, env environment.Environment, id, class string, catalog Catalog) (resource.RenderingSelection, error) {
	if class == "" {
		class = ClassDefault
	}
	ctx := Context{App: app, Env: env}
	matcher, err := newDefinitionMatcher(ctx, catalog)
	if err != nil {
		return resource.RenderingSelection{}, stageErr(StageCatalog, err)
	}
	selections, err := s.selectRendering(ctx, Graph{Nodes: []Node{{Descriptor: "workload." + class + "#modules." + id, Kind: NodeWorkload, ResourceType: "workload", Class: class, WorkloadID: id}}}, matcher, catalog)
	if err != nil {
		return resource.RenderingSelection{}, stageErr(StageCatalog, err)
	}
	return selections[id], nil
}

// rendererMatchError classifies a failed required-renderer match as no match,
// tie or another catalog failure (for example a Connection conflict).
func rendererMatchError(err error) error {
	var staged *StageError
	if errors.As(err, &staged) {
		return err
	}
	reason := ""
	switch {
	case errors.Is(err, errNoMatch):
		reason = ReasonRendererNoMatch
	case errors.Is(err, errAmbiguous):
		reason = ReasonRendererAmbiguous
	}
	return &StageError{Stage: StageCatalog, Reason: reason, Err: err}
}
