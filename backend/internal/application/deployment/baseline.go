package deployment

import (
	"context"
	"encoding/json"

	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning"
	"orchestrator/internal/ports/persistence"
)

// BaselinePlans returns, for an existing resource node, the persisted plan of
// the Deployment that last executed it (a pure store read; nil when it cannot
// be read, which the Preview projection shows as unverified, never as
// unchanged). Standalone and pending Preview share it (UC-05 BR-10).
func BaselinePlans(ctx context.Context, store persistence.Store, org string, active []resource.ActiveResource) func(planning.Node) *planning.Plan {
	byKey := map[string]resource.ActiveResource{}
	for _, a := range active {
		byKey[a.LogicalKey()] = a
	}
	cache := map[string]*planning.Plan{}
	return func(node planning.Node) *planning.Plan {
		descriptor, err := resource.ParseDescriptor(node.Descriptor)
		if err != nil {
			return nil
		}
		found, ok := byKey[resource.LogicalKey(org, descriptor, node.Scope)]
		if !ok || found.LastDeploymentID == "" {
			return nil
		}
		if plan, seen := cache[found.LastDeploymentID]; seen {
			return plan
		}
		var old *planning.Plan
		if snapshot, err := store.GetPlan(ctx, found.LastDeploymentID); err == nil {
			if raw, err := json.Marshal(snapshot); err == nil {
				var decoded planning.Plan
				if json.Unmarshal(raw, &decoded) == nil {
					old = &decoded
				}
			}
		}
		cache[found.LastDeploymentID] = old
		return old
	}
}
