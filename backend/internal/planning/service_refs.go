package planning

import (
	"fmt"
	"sort"
	"strings"

	"orchestrator/internal/domain/environment"
	"orchestrator/internal/planning/placeholder"
)

// ValidateServiceReferences checks that every workload Service reference
// `${context.service.<workload>.<port>}` in the complete Environment desired
// state resolves to a workload that remains in the set and declares that
// Service port (UC-07 BR-08, OC-07). It uses placeholder semantics: escaped
// `$${...}` literals are skipped. Command, args and variables are inspected.
// Errors name only module IDs, which are validated workload names.
func ValidateServiceReferences(set environment.Document) error {
	for _, consumer := range set.ModuleIDs() {
		module := set.Modules[consumer]
		names := make([]string, 0, len(module.Spec.Containers))
		for name := range module.Spec.Containers {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			container := module.Spec.Containers[name]
			values := append(append([]string{}, container.Command...), container.Args...)
			keys := make([]string, 0, len(container.Variables))
			for key := range container.Variables {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				values = append(values, container.Variables[key])
			}
			for _, value := range values {
				if err := checkServiceRefs(set, consumer, value); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func checkServiceRefs(set environment.Document, consumer, value string) error {
	for _, match := range placeholder.Pattern.FindAllStringSubmatch(value, -1) {
		if strings.HasPrefix(match[0], "$$") {
			continue // escaped literal
		}
		ref, err := placeholder.Parse(match[1])
		if err != nil || ref.Kind != placeholder.KindContext {
			continue // not a Service reference; other rules own it
		}
		if ref.Context != "service" && !strings.HasPrefix(ref.Context, "service.") {
			continue
		}
		segments := strings.Split(ref.Context, ".")
		if len(segments) != 3 || !validSegment(segments[1]) || !validSegment(segments[2]) {
			return fmt.Errorf("planning: workload %q has a malformed Service reference; use context.service.<workload>.<port>", consumer)
		}
		target, port := segments[1], segments[2]
		provider, ok := set.Modules[target]
		if !ok {
			return fmt.Errorf("planning: workload %q references a Service of workload %q, which is not in the resulting Environment", consumer, target)
		}
		if provider.Spec.Service == nil {
			return fmt.Errorf("planning: workload %q references a Service of workload %q, which declares no Service", consumer, target)
		}
		if _, ok := provider.Spec.Service.Ports[port]; !ok {
			return fmt.Errorf("planning: workload %q references a Service port of workload %q that is not declared", consumer, target)
		}
	}
	return nil
}

// validSegment accepts a workload or port name: nonempty, no whitespace.
func validSegment(segment string) bool {
	return segment != "" && !strings.ContainsAny(segment, " \t\r\n")
}
