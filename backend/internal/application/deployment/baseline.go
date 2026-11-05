package deployment

import (
	"context"
	"encoding/json"
	"errors"

	domain "orchestrator/internal/domain/deployment"
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
		plan, err := b.loadPlan(ctx, view, id)
		if err != nil {
			return nil, err
		}
		if a.Status == resource.StatusUnreferenced && plan != nil && !planContainsResource(plan, a) {
			id, err = b.retainedPlan(ctx, view, a)
			if err != nil {
				return nil, err
			}
			captured := a
			captured.LastDeploymentID = id
			b.byKey[a.LogicalKey()] = captured
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

func (b *Baselines) loadPlan(ctx context.Context, view persistence.Store, id string) (*planning.Plan, error) {
	if p, ok := b.plans[id]; ok {
		return p, nil
	}
	raw, err := view.GetPlan(ctx, id)
	if errors.Is(err, persistence.ErrNotFound) {
		b.plans[id] = nil
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	b.plans[id] = decodePlan(raw)
	return b.plans[id], nil
}
func planContainsResource(p *planning.Plan, a resource.ActiveResource) bool {
	for _, n := range p.Graph.Nodes {
		if n.Kind == planning.NodeResource && n.Descriptor == a.Descriptor.String() && n.Scope == a.Scope {
			return true
		}
	}
	return false
}

// UNREFERENCED records point at the removal that established cleanup ownership.
// Follow only its immutable successful base-set lineage for execution evidence;
// never rewrite that ownership marker or search arbitrary historical plans.
func (b *Baselines) retainedPlan(ctx context.Context, view persistence.Store, a resource.ActiveResource) (string, error) {
	id, environmentID := a.LastDeploymentID, ""
	seen := map[string]bool{}
	for depth := 0; id != "" && depth < 128; depth++ {
		if seen[id] {
			return "", nil
		}
		seen[id] = true
		d, err := view.GetDeployment(ctx, id)
		if errors.Is(err, persistence.ErrNotFound) {
			return "", nil
		}
		if err != nil {
			return "", err
		}
		if d.OrganizationKey != b.organization || d.EnvironmentID == "" || d.Status != domain.StatusSucceeded {
			return "", nil
		}
		if environmentID == "" {
			environmentID = d.EnvironmentID
		}
		if d.EnvironmentID != environmentID {
			return "", nil
		}
		p, err := b.loadPlan(ctx, view, id)
		if err != nil {
			return "", err
		}
		if p == nil {
			return "", nil
		}
		if planContainsResource(p, a) {
			return id, nil
		}
		if d.Action != domain.ActionRemove || d.BaseDeploymentSetID == "" {
			return "", nil
		}
		base, err := view.GetDeploymentSet(ctx, d.BaseDeploymentSetID)
		if errors.Is(err, persistence.ErrNotFound) {
			return "", nil
		}
		if err != nil {
			return "", err
		}
		if base.EnvironmentID != environmentID {
			return "", nil
		}
		id = base.CreatedByDeploymentID
	}
	return "", nil
}
