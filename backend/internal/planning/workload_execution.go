package planning

import (
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/renderbundle"
)

// validateExecution proves that the selected renderer implements every explicit
// runtime intent before any provisioning can begin.
func validateExecution(plan *Plan, catalog Catalog) error {
	for id, module := range plan.CandidateSet.Modules {
		e := module.Spec.Execution
		if e == nil {
			continue
		}
		selection, ok := plan.Rendering[id]
		if !ok || !selection.Bundle.Git() {
			return environment.ErrExecutionUnavailable
		}
		bundle, ok := catalog.RenderBundles[selection.Bundle.ID]
		if !ok {
			return environment.ErrExecutionUnavailable
		}
		if _, _, err := renderbundle.MatchWorkloadTemplate(bundle.Contract.Manifest.WorkloadTemplates, e.Template.ID, e.Template.Version, e.Template.SHA256, e.Kind, e.Inputs); err != nil {
			return environment.ErrExecutionUnavailable
		}
		var bindings map[string]string
		for _, node := range plan.Graph.Nodes {
			if node.WorkloadID == id && node.Kind == NodeWorkload {
				bindings = node.Bindings
				break
			}
		}
		for _, mount := range e.Mounts {
			descriptor, ok := bindings[mount.Resource]
			if !ok {
				return environment.ErrExecution
			}
			found := false
			for _, node := range plan.Graph.Nodes {
				if node.Descriptor != descriptor {
					continue
				}
				typ, ok := catalog.Types[node.ResourceType]
				if !ok {
					return environment.ErrExecution
				}
				fields := map[string]bool{}
				for _, output := range typ.Outputs {
					if !output.Secret && output.Type == "string" {
						fields[output.Name] = true
					}
				}
				found = fields["claimName"] && fields["namespace"]
			}
			if !found {
				return environment.ErrExecution
			}
		}
	}
	return nil
}
