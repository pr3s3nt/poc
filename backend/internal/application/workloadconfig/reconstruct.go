package workloadconfig

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"orchestrator/internal/domain/environment"
)

var setReference = regexp.MustCompile(`\$\{(externals|shared)\.([A-Za-z0-9_-]+)\.([A-Za-z_][A-Za-z0-9_-]*)\}`)
var configReference = regexp.MustCompile(`\$\{context\.uc12\.([A-Za-z_][A-Za-z0-9_]*)\}`)
var serviceReference = regexp.MustCompile(`\$\{context\.service\.([A-Za-z0-9_-]+)\.([A-Za-z0-9_-]+)\}`)

// ReconstructScore exposes only reference-based workload configurations to the
// UC-16 editor. Legacy literals may contain secrets and are never returned.
func ReconstructScore(workload string, module environment.Module, set environment.Document) (map[string]any, error) {
	if module.Profile != environment.ModuleProfile {
		return nil, fmt.Errorf("workloadconfig: unsupported deployed workload profile")
	}
	resources := map[string]any{}
	for alias, entry := range module.Externals {
		resources[alias] = resourceMap(entry, "")
	}
	sharedAliases := map[string]string{}
	sharedIDs := make([]string, 0, len(set.Shared))
	for id := range set.Shared {
		sharedIDs = append(sharedIDs, id)
	}
	sort.Strings(sharedIDs)
	for i, id := range sharedIDs {
		alias := fmt.Sprintf("shared_%d", i+1)
		for resources[alias] != nil {
			alias += "_"
		}
		sharedAliases[id] = alias
	}
	containers := map[string]any{}
	for name, container := range module.Spec.Containers {
		encoded, err := json.Marshal(container)
		if err != nil {
			return nil, err
		}
		var body map[string]any
		if err := json.Unmarshal(encoded, &body); err != nil {
			return nil, err
		}
		delete(body, "id")
		variables := map[string]any{}
		for key, value := range container.Variables {
			if setReference.FindString(value) != value && configReference.FindString(value) != value && serviceReference.FindString(value) != value {
				return nil, fmt.Errorf("workloadconfig: legacy literal binding cannot be shown in editor")
			}
			value = setReference.ReplaceAllStringFunc(value, func(ref string) string {
				parts := setReference.FindStringSubmatch(ref)
				if parts[1] == "externals" {
					return "${resources." + parts[2] + "." + parts[3] + "}"
				}
				alias := sharedAliases[parts[2]]
				if alias == "" {
					return ref
				}
				resources[alias] = resourceMap(set.Shared[parts[2]], parts[2])
				return "${resources." + alias + "." + parts[3] + "}"
			})
			value = configReference.ReplaceAllStringFunc(value, func(ref string) string {
				resources["env"] = map[string]any{"type": "environment"}
				return "${resources.env." + configReference.FindStringSubmatch(ref)[1] + "}"
			})
			value = serviceReference.ReplaceAllStringFunc(value, func(ref string) string {
				parts := serviceReference.FindStringSubmatch(ref)
				alias := "svc_" + parts[1] + "_" + parts[2]
				resources[alias] = map[string]any{"type": "service", "params": map[string]any{"workload": parts[1], "port": parts[2]}}
				return "${resources." + alias + ".url}"
			})
			if strings.Contains(value, "${externals.") || strings.Contains(value, "${shared.") || strings.Contains(value, "${context.") {
				return nil, fmt.Errorf("workloadconfig: unsupported deployed binding")
			}
			variables[key] = value
		}
		if len(variables) > 0 {
			body["variables"] = variables
		}
		containers[name] = body
	}
	out := map[string]any{"apiVersion": "score.dev/v1b1", "metadata": map[string]any{"name": workload}, "containers": containers}
	if len(resources) > 0 {
		out["resources"] = resources
	}
	if module.Spec.Service != nil {
		out["service"] = module.Spec.Service
	}
	if module.Spec.Replicas != nil {
		out["replicas"] = *module.Spec.Replicas
	}
	return out, nil
}

func resourceMap(entry environment.ResourceEntry, id string) map[string]any {
	out := map[string]any{"type": entry.Type}
	if entry.Class != "" && entry.Class != "default" {
		out["class"] = entry.Class
	}
	if id != "" {
		out["id"] = id
	}
	if len(entry.Params) > 0 {
		out["params"] = entry.Params
	}
	return out
}
