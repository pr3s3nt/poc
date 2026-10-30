package fleet

import domain "orchestrator/internal/domain/delivery"

// Runtime readiness is independent of Fleet's aggregate ready flag, which also
// becomes false for a healthy but Modified release.
func objectHealth(kind string, object map[string]any) domain.Health {
	status, _ := object["status"].(map[string]any)
	spec, _ := object["spec"].(map[string]any)
	md, _ := object["metadata"].(map[string]any)
	number := func(m map[string]any, key string) float64 { value, _ := m[key].(float64); return value }
	switch kind {
	case "Deployment", "StatefulSet", "DaemonSet":
		generation := number(md, "generation")
		if generation < 1 || number(status, "observedGeneration") < generation {
			return domain.Progressing
		}
		conditions, _ := status["conditions"].([]any)
		for _, raw := range conditions {
			c, _ := raw.(map[string]any)
			if (c["type"] == "ReplicaFailure" && c["status"] == "True") || (c["type"] == "Progressing" && c["status"] == "False") {
				return domain.Degraded
			}
		}
		desired := number(spec, "replicas")
		if kind == "DaemonSet" {
			desired = number(status, "desiredNumberScheduled")
			if number(status, "numberReady") < desired || number(status, "updatedNumberScheduled") < desired {
				return domain.Progressing
			}
			return domain.Healthy
		}
		if number(status, "readyReplicas") < desired {
			return domain.Progressing
		}
		if kind == "Deployment" && (number(status, "updatedReplicas") < desired || number(status, "availableReplicas") < desired) {
			return domain.Progressing
		}
		if kind == "StatefulSet" && number(status, "updatedReplicas") < desired {
			return domain.Progressing
		}
	case "Job":
		conditions, _ := status["conditions"].([]any)
		for _, raw := range conditions {
			c, _ := raw.(map[string]any)
			if c["type"] == "Failed" && c["status"] == "True" {
				return domain.Degraded
			}
			if c["type"] == "Complete" && c["status"] == "True" {
				return domain.Healthy
			}
		}
		if number(status, "succeeded") < 1 {
			return domain.Progressing
		}
	}
	return domain.Healthy
}
