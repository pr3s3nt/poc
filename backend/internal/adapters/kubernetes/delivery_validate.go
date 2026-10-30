package kubernetes

import (
	"context"
	"orchestrator/internal/ports/execution"
)

// ValidateDeliveryIncarnations verifies all source identities before any release
// or metadata patch. Job absence is permitted only for an acknowledged completed
// phase; it is never permission to recreate an absent execution.
func ValidateDeliveryIncarnations(ctx context.Context, cli CLI, target execution.Target, refs []execution.ObjectRef, uids map[string]string, manager DeliveryManager, completedJobs bool) error {
	live, err := InspectDeliveryObjects(ctx, cli, target, refs)
	if err != nil {
		return err
	}
	for _, ref := range refs {
		uid := uids[ref.Key()]
		if uid == "" {
			return ErrDeliveryOwnership
		}
		obj, found := live[ref.Key()]
		if !found {
			if completedJobs && ref.Kind == "Job" {
				continue
			}
			return ErrDeliveryOwnership
		}
		if _, err := ownershipPatch(obj, target, uid, manager, manager); err != nil {
			return err
		}
	}
	return nil
}
