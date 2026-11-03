package fleet

import port "orchestrator/internal/ports/delivery"

func managedRepoMetadata(current map[string]any, i port.Intent, unit, digest string) map[string]any {
	labels, annotations := map[string]any{}, map[string]any{}
	previous, _ := current["metadata"].(map[string]any)
	for field, destination := range map[string]map[string]any{"labels": labels, "annotations": annotations} {
		values, _ := previous[field].(map[string]any)
		for key, value := range values {
			destination[key] = value
		}
	}
	for key, value := range ownerLabels(i, unit) {
		labels[key] = value
	}
	annotations["orchestrator.io/delivery-revision"] = i.Revision.ID
	annotations["orchestrator.io/delivery-digest"] = digest
	annotations["orchestrator.io/delivery-intent"] = i.ID
	metadata := map[string]any{"name": unit, "labels": labels, "annotations": annotations}
	for _, key := range []string{"uid", "finalizers", "ownerReferences"} {
		if value, ok := previous[key]; ok {
			metadata[key] = value
		}
	}
	if previous["resourceVersion"] != nil {
		metadata["resourceVersion"] = previous["resourceVersion"]
	}
	return metadata
}
