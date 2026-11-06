package planning

import (
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/templatesource"
	"reflect"
)

// validateExecution proves that the selected renderer implements every explicit
// runtime intent before any provisioning can begin.
func validateExecution(plan *Plan, catalog Catalog) error {
	for id, module := range plan.CandidateSet.Modules {
		e := module.Spec.Execution
		needsLifecycle := false
		for _, c := range module.Spec.Containers {
			needsLifecycle = needsLifecycle || c.SecurityContext != nil || c.Role == "init" || c.Role == "sidecar"
		}
		needsProbes := false
		for _, c := range module.Spec.Containers {
			for _, p := range []*environment.Probe{c.ReadinessProbe, c.LivenessProbe, c.StartupProbe} {
				if p != nil && ((p.Type != "" && p.Type != "http") || p.Scheme != "" || p.InitialDelaySeconds != nil || p.TimeoutSeconds != nil || p.PeriodSeconds != nil || p.SuccessThreshold != nil || p.FailureThreshold != nil) {
					needsProbes = true
				}
			}
		}
		if e == nil && !needsLifecycle && !needsProbes {
			continue
		}
		selection, ok := plan.Rendering[id]
		if !ok || !selection.Bundle.Git() {
			return environment.ErrExecutionUnavailable
		}
		bundle, ok := catalog.TemplateSources[selection.Bundle.ID]
		if !ok {
			return environment.ErrExecutionUnavailable
		}
		if err := templatesource.ValidateWorkloadCapabilities(bundle.Contract.Manifest, module.Spec.Containers, e); err != nil {
			return err
		}
		if _, err := templatesource.ExecutionInputs(bundle.Contract.Manifest, e); err != nil {
			return environment.ErrExecutionUnavailable
		}
		var profile *environment.RuntimeDefaults
		if e.HasTemplate() {
			for _, t := range bundle.Contract.Manifest.WorkloadTemplates {
				if t.ID == e.Template.ID && t.Version == e.Template.Version && t.SHA256() == e.Template.SHA256 {
					profile = t.RuntimeDefaults
				}
			}
		}
		if e != nil && e.Defaults != nil && e.HasTemplate() && !reflect.DeepEqual(e.Defaults.Template, profile) {
			return environment.ErrExecutionUnavailable
		}
		effective, _ := module.Spec.ResolveDefaults(profile)
		resources := map[string]bool{}
		if e != nil {
			for _, m := range e.Mounts {
				resources[m.Resource] = true
			}
		}
		if err := effective.ValidateComputePolicy(); err != nil {
			return err
		}
		if err := effective.Execution.Validate(effective.Containers, effective.ReplicaCount(), effective.Service != nil && len(effective.Service.Ports) > 0, resources); err != nil {
			return err
		}
		for _, c := range effective.Containers {
			if c.Image == "" {
				return environment.ErrExecution
			}
		}
		if e == nil {
			continue
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
