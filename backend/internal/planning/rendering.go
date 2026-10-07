package planning

import (
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/platform/canon"
)

func (s *Service) selectRendering(ctx Context, graph Graph, matcher *definitionMatcher, catalog Catalog) (map[string]resource.RenderingSelection, error) {
	out := map[string]resource.RenderingSelection{}
	// Renderer definitions are separate from infrastructure matches and provision batches.
	renderMatcher := &definitionMatcher{ctx: ctx, defs: matcher.defs}
	for _, def := range matcher.list {
		if def.ResourceTypeKey == "workload" && def.DriverType == resource.DriverScoreK8s {
			renderMatcher.list = append(renderMatcher.list, def)
		}
	}
	for _, node := range graph.Nodes {
		if node.Kind != NodeWorkload {
			continue
		}
		eligible := false
		matchCtx := resource.MatchContext{EnvironmentType: ctx.Env.Type, ApplicationID: ctx.App.Key, EnvironmentID: ctx.Env.Key, ResourceID: descriptorID(node.Descriptor), Class: node.Class}
		for _, def := range renderMatcher.list {
			if def.ExecutionProfile != "" && def.ExecutionProfile != string(ctx.Env.Profile) {
				continue
			}
			if _, _, ok := def.BestCriterion(matchCtx); ok {
				eligible = true
			}
		}
		if !eligible {
			continue
		} // Native renderer: omitted from additive snapshots for legacy compatibility.
		match, err := renderMatcher.match(&node)
		if err != nil {
			return nil, err
		}
		def := matcher.defs[match.DefinitionKey]
		if err := resource.ValidateRenderDefinition(def, catalog.Types["workload"], s.bundles); err != nil {
			return nil, err
		}
		hash, err := canon.Hash(def)
		if err != nil {
			return nil, err
		}
		out[node.WorkloadID] = resource.RenderingSelection{DefinitionKey: def.Key, DefinitionHash: hash, DriverType: def.DriverType, Bundle: s.bundles[def.Variables()["render_bundle"].(string)]}
	}
	return out, nil
}

// SelectWorkloadRendering detects renderer-only pending updates without planning
// or validating unrelated workload changes. It uses the same selection as Plan.
func (s *Service) SelectWorkloadRendering(app application.Application, env environment.Environment, id string, catalog Catalog) (resource.RenderingSelection, error) {
	ctx := Context{App: app, Env: env}
	matcher, err := newDefinitionMatcher(ctx, catalog)
	if err != nil {
		return resource.RenderingSelection{}, stageErr(StageCatalog, err)
	}
	selections, err := s.selectRendering(ctx, Graph{Nodes: []Node{{Descriptor: "workload.default#modules." + id, Kind: NodeWorkload, ResourceType: "workload", Class: "default", WorkloadID: id}}}, matcher, catalog)
	if err != nil {
		return resource.RenderingSelection{}, stageErr(StageCatalog, err)
	}
	return selections[id], nil
}
