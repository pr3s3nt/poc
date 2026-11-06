package deploymentreview

import (
	"encoding/json"
	"gopkg.in/yaml.v3"
	"orchestrator/internal/domain/environment"
	"strings"
)

// Only the nonsecret runtime policy is projected, never env values, arguments,
// template inputs, source file data or Secret references.
func defaultsProjection(spec environment.ModuleSpec, profile *environment.RuntimeDefaults) Side {
	effective, sources := spec.ResolveDefaults(profile)
	raw, _ := json.Marshal(effective)
	var tree map[string]any
	_ = json.Unmarshal(raw, &tree)
	result := map[string]any{}
	execution, _ := tree["workloadExecution"].(map[string]any)
	keys := []string{"defaults", "podSecurityContext", "automountServiceAccountToken", "serviceAccountName", "updateStrategy", "revisionHistoryLimit", "podDisruptionBudget", "volumeClaimTemplates", "persistentVolumeClaimRetentionPolicy"}
	safeExecution := policyFields(execution, keys...)
	if defaults, ok := safeExecution["defaults"].(map[string]any); ok {
		safeExecution["defaults"] = selectObject(defaults, "version", "environment")
	}
	result["workloadExecution"] = safeExecution
	if effective.Replicas != nil {
		result["replicas"] = *effective.Replicas
	}
	containers := map[string]any{}
	declared, _ := tree["containers"].(map[string]any)
	allowed := []string{"resources", "securityContext", "ports", "readinessProbe", "livenessProbe", "startupProbe"}
	cleanSources := map[string]string{}
	for name, raw := range declared {
		c, _ := raw.(map[string]any)
		safe := policyFields(c, allowed...)
		for _, field := range []string{"readinessProbe", "livenessProbe", "startupProbe"} {
			if probe, ok := safe[field].(map[string]any); ok {
				if _, present := probe["command"]; present {
					copy := selectObject(probe, "type", "path", "port", "scheme", "initialDelaySeconds", "periodSeconds", "timeoutSeconds", "failureThreshold", "successThreshold")
					copy["command"] = []string{"<redacted>"}
					safe[field] = copy
				}
			}
		}
		containers[name] = safe
	}
	result["containers"] = containers
	for path, source := range sources {
		keep := path == "replicas"
		for _, key := range keys {
			keep = keep || strings.HasPrefix(path, "workloadExecution."+key)
		}
		if strings.HasPrefix(path, "containers.") {
			for name := range declared {
				for _, key := range allowed {
					keep = keep || strings.HasPrefix(path, "containers."+name+"."+key)
				}
			}
		}
		if keep {
			cleanSources[path] = source
		}
	}
	result["sources"] = cleanSources
	encoded, err := yaml.Marshal(result)
	if err != nil || len(encoded) > MaxSideBytes {
		return unavailable("RENDER_UNAVAILABLE")
	}
	return Side{Availability: "AVAILABLE", Text: string(encoded), RedactedPaths: []string{}, UnknownPaths: []string{}}
}

// The caller supplies a closed runtime-only field list from typed JSON; unlike
// selectObject this preserves nested maps and arrays needed for policy review.
func policyFields(input map[string]any, fields ...string) map[string]any {
	out := map[string]any{}
	for _, key := range fields {
		if value, present := input[key]; present {
			out[key] = value
		}
	}
	return out
}
