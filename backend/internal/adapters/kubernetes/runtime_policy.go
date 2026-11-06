package kubernetes

import (
	"context"
	"encoding/json"
	"fmt"
	"orchestrator/internal/ports/execution"
)

func hasRuntimePolicy(m execution.Manifest) bool {
	spec, _ := m.Object["spec"].(map[string]any)
	if m.Kind == "PodDisruptionBudget" {
		return true
	}
	if m.Kind != "Deployment" && m.Kind != "StatefulSet" && m.Kind != "CronJob" {
		return false
	}
	if _, ok := spec["volumeClaimTemplates"]; ok {
		return true
	}
	if _, ok := spec["revisionHistoryLimit"]; ok {
		return true
	}
	if m.Kind == "CronJob" {
		job, _ := spec["jobTemplate"].(map[string]any)
		spec, _ = job["spec"].(map[string]any)
	}
	template, _ := spec["template"].(map[string]any)
	pod, _ := template["spec"].(map[string]any)
	_, ok := pod["automountServiceAccountToken"]
	return ok
}

// ValidateRuntimeObjects uses server admission without persistence before CD
// publication or direct apply. Unsupported/gated fields fail closed. Retained
// same-name claims cannot be adopted by a newly created StatefulSet.
func ValidateRuntimeObjects(ctx context.Context, cli CLI, target execution.Target, manifests []execution.Manifest) error {
	discovery := newDiscovery(cli)
	for _, m := range manifests {
		if !hasRuntimePolicy(m) {
			continue
		}
		if m.Kind == "StatefulSet" {
			spec, _ := m.Object["spec"].(map[string]any)
			claims, _ := spec["volumeClaimTemplates"].([]any)
			if len(claims) > 0 {
				controller, exists, err := discovery.get(ctx, target.Namespace, m.APIVersion, m.Kind, m.Name)
				if err != nil {
					return err
				}
				for _, raw := range claims {
					claim, _ := raw.(map[string]any)
					meta, _ := claim["metadata"].(map[string]any)
					name, _ := meta["name"].(string)
					pvc, found, err := discovery.get(ctx, target.Namespace, "v1", "PersistentVolumeClaim", name+"-"+m.Name+"-0")
					if err != nil {
						return err
					}
					if found && (!exists || !sameWorkloadOwner(objectLabels(controller), target.Extra["application"], target.Extra["environment"], m.Name) || !sameWorkloadOwner(objectLabels(pvc), target.Extra["application"], target.Extra["environment"], m.Name)) {
						return fmt.Errorf("%w: retained or foreign StatefulSet claim requires explicit data migration", ErrDeliveryOwnership)
					}
				}
			}
		}
		// Client-side apply merges lists with the old live object and can invent
		// duplicate named ports when a template changes its container port. Validate
		// the complete proposed object with create/replace dry-run instead.
		raw, err := json.Marshal(m.Object)
		if err != nil {
			return err
		}
		existing, found, err := discovery.get(ctx, target.Namespace, m.APIVersion, m.Kind, m.Name)
		if err != nil {
			return err
		}
		verb := "create"
		if found {
			verb = "replace"
			var proposed map[string]any
			if err = json.Unmarshal(raw, &proposed); err != nil {
				return err
			}
			meta, _ := proposed["metadata"].(map[string]any)
			oldMeta, _ := existing["metadata"].(map[string]any)
			if meta == nil || oldMeta["resourceVersion"] == nil {
				return ErrDeliveryOwnership
			}
			meta["resourceVersion"] = oldMeta["resourceVersion"]
			raw, err = json.Marshal(proposed)
			if err != nil {
				return err
			}
		}
		if _, err = cli.Run(ctx, raw, verb, "--dry-run=server", "--validate=strict", "-f", "-"); err != nil {
			return fmt.Errorf("kubernetes: runtime policy admission failed: %w", err)
		}
	}
	return nil
}
