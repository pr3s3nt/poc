package planning

import (
	"fmt"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/planning/placeholder"
	"sort"
	"strings"
)

// ValidateJobDependencies validates the complete Environment, including unchanged
// consumers, before applying any draft. Intermediate Save states may omit targets.
func ValidateJobDependencies(set environment.Document) error {
	for _, id := range set.ModuleIDs() {
		module := set.Modules[id]
		if module.Spec.Execution == nil {
			continue
		}
		for _, target := range module.Spec.Execution.WaitForJobs {
			provider, ok := set.Modules[target]
			if !ok || provider.Spec.Execution == nil || provider.Spec.Execution.Kind != "Job" || target == id {
				return fmt.Errorf("planning: workload %q must wait for a distinct Job %q in the resulting Environment", id, target)
			}
		}
	}
	b := newGraphBuilder(Context{}, Catalog{})
	// Completion and Service edges are checked together below. The normal full
	// graph also validates dependencies through resources.
	for _, id := range set.ModuleIDs() {
		module := set.Modules[id]
		descriptor, err := WorkloadDescriptorFor(id, module.WorkloadClass())
		if err != nil {
			return err
		}
		if _, err := b.ensure(descriptor, NodeWorkload, OriginWorkload, nil); err != nil {
			return err
		}
	}
	for _, id := range set.ModuleIDs() {
		module := set.Modules[id]
		if module.Spec.Execution == nil {
			continue
		}
		consumer, _ := WorkloadDescriptorFor(id, module.WorkloadClass())
		for _, target := range module.Spec.Execution.WaitForJobs {
			provider, _ := WorkloadDescriptorFor(target, set.Modules[target].WorkloadClass())
			b.addEdge(consumer.String(), provider.String(), ReasonJobCompletion, "")
		}
	}
	if !hasJobWaits(set) {
		return nil
	}
	for _, id := range set.ModuleIDs() {
		consumer, _ := WorkloadDescriptorFor(id, set.Modules[id].WorkloadClass())
		for _, target := range serviceWorkloadTargets(set.Modules[id]) {
			if module, ok := set.Modules[target]; ok {
				provider, _ := WorkloadDescriptorFor(target, module.WorkloadClass())
				b.addEdge(consumer.String(), provider.String(), ReasonWorkloadPlace, "")
			}
		}
	}
	_, _, err := schedule(b.graph())
	return err
}

func hasJobWaits(set environment.Document) bool {
	for _, module := range set.Modules {
		if module.Spec.Execution != nil && len(module.Spec.Execution.WaitForJobs) > 0 {
			return true
		}
	}
	return false
}

// ConfigMap file values are literal: inspect only the same command/argument/env
// expressions used by Service reference validation, never arbitrary file text.
func serviceWorkloadTargets(module environment.Module) []string {
	seen := map[string]bool{}
	targets := []string{}
	for _, c := range module.Spec.Containers {
		values := append(append([]string{}, c.Command...), c.Args...)
		for _, value := range c.Variables {
			values = append(values, value)
		}
		for _, value := range values {
			for _, match := range placeholder.Pattern.FindAllStringSubmatch(value, -1) {
				if strings.HasPrefix(match[0], "$$") {
					continue
				}
				ref, err := placeholder.Parse(match[1])
				if err != nil || ref.Kind != placeholder.KindContext {
					continue
				}
				parts := strings.Split(ref.Context, ".")
				if len(parts) == 3 && parts[0] == "service" && !seen[parts[1]] {
					seen[parts[1]] = true
					targets = append(targets, parts[1])
				}
			}
		}
	}
	sort.Strings(targets)
	return targets
}
