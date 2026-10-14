package deployment

import (
	"context"
	"encoding/json"
	"errors"

	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning"
	"orchestrator/internal/ports/persistence"
)

// Baselines are the persisted plans that last executed the existing resources
// of a planning snapshot, captured and decoded INSIDE the same read-only store
// snapshot as the Active Resources they belong to (UC-05 BR-01, OC-06). The
// Preview projection reads only these captured records: no store access after
// the snapshot closes. A missing, unreadable or undecodable plan is recorded as
// no evidence (the projection then shows UNVERIFIED, never unchanged); any
// other store failure fails the capture.
type Baselines struct {
	organization string
	byKey        map[string]resource.ActiveResource
	plans        map[string]*planning.Plan
}

// CaptureBaselines reads the plans of the Active Resources through view, which
// must be a snapshot view. Each deployment plan is read at most once.
func CaptureBaselines(ctx context.Context, view persistence.Store, organizationKey string, active []resource.ActiveResource) (*Baselines, error) {
	b := &Baselines{organization: organizationKey, byKey: map[string]resource.ActiveResource{}, plans: map[string]*planning.Plan{}}
	for _, a := range active {
		if a.Status == resource.StatusDestroyed {
			continue // a tombstone is not a baseline: the descriptor is created again
		}
		b.byKey[a.LogicalKey()] = a
		id := a.LastDeploymentID
		if id == "" {
			continue
		}
		if _, seen := b.plans[id]; seen {
			continue
		}
		snapshot, err := view.GetPlan(ctx, id)
		switch {
		case errors.Is(err, persistence.ErrNotFound):
			b.plans[id] = nil
		case err != nil:
			return nil, err
		default:
			b.plans[id] = decodePlan(snapshot)
		}
	}
	return b, nil
}

func decodePlan(snapshot map[string]any) *planning.Plan {
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return nil
	}
	var decoded planning.Plan
	if json.Unmarshal(raw, &decoded) != nil {
		return nil
	}
	return &decoded
}

// Reader returns the pure lookup the Preview projection uses: the captured plan
// of an existing node, or nil when there is no usable evidence.
func (b *Baselines) Reader() func(planning.Node) *planning.Plan {
	if b == nil {
		return func(planning.Node) *planning.Plan { return nil }
	}
	return func(node planning.Node) *planning.Plan {
		descriptor, err := resource.ParseDescriptor(node.Descriptor)
		if err != nil {
			return nil
		}
		found, ok := b.byKey[resource.LogicalKey(b.organization, descriptor, node.Scope)]
		if !ok || found.LastDeploymentID == "" {
			return nil
		}
		return b.plans[found.LastDeploymentID]
	}
}
