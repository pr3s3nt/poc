package drift

import (
	"context"
	"encoding/json"
	"orchestrator/internal/application/tfexec"
	domain "orchestrator/internal/domain/drift"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/ports/persistence"
	"sort"
)

type RepairConsumers interface {
	PreviewRepairConsumers(context.Context, string, string, string, []string) ([]domain.RepairConsumer, error)
	RefreshRepairConsumers(context.Context, string, string, string, string, []domain.RepairConsumer) error
}

type repairDependencies interface {
	Dependencies(context.Context, string) ([]tfresource.Dependency, error)
}

func (s *Service) repairScope(ctx context.Context, org string, env environment.Environment, roots []string) ([]domain.RepairInstance, error) {
	graph, ok := s.Instances.(repairDependencies)
	if !ok {
		return nil, tfexec.ErrBaselineUnavailable
	}
	selected := map[string]bool{}
	edges := map[string][]string{}
	var visit func(string) error
	visit = func(id string) error {
		if selected[id] {
			return nil
		}
		selected[id] = true
		inst, err := s.Instances.GetInstance(ctx, id)
		if err != nil || inst.Identity.OrganizationID != org || inst.Identity.EnvironmentID != env.ID || inst.Identity.TargetGeneration != env.TargetGeneration || (inst.Status != tfresource.InstanceReady && inst.Status != tfresource.InstanceUnreferenced) {
			return tfexec.ErrBaselineUnavailable
		}
		deps, err := graph.Dependencies(ctx, id)
		if err != nil {
			return err
		}
		for _, d := range deps {
			if !d.Holds() {
				continue
			}
			if d.ConsumerID == id {
				edges[id] = append(edges[id], d.ProviderID)
			}
			if d.ProviderID == id {
				if err := visit(d.ConsumerID); err != nil {
					return err
				}
			}
		}
		return nil
	}
	for _, id := range roots {
		if err := visit(id); err != nil {
			return nil, err
		}
	}
	if len(selected) > 200 {
		return nil, tfexec.ErrRepairBlocked
	}
	// Stable topological order makes both the visible scope and stage order deterministic.
	ids := make([]string, 0, len(selected))
	for id := range selected {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	state := map[string]int{}
	ordered := []string{}
	var order func(string) error
	order = func(id string) error {
		if state[id] == 2 {
			return nil
		}
		if state[id] == 1 {
			return tfexec.ErrRepairBlocked
		}
		state[id] = 1
		providers := edges[id]
		sort.Strings(providers)
		for _, provider := range providers {
			if selected[provider] {
				if err := order(provider); err != nil {
					return err
				}
			}
		}
		state[id] = 2
		ordered = append(ordered, id)
		return nil
	}
	for _, id := range ids {
		if err := order(id); err != nil {
			return nil, err
		}
	}
	out := []domain.RepairInstance{}
	for _, id := range ordered {
		inst, err := s.Instances.GetInstance(ctx, id)
		if err != nil {
			return nil, err
		}
		baseline, err := s.Instances.LatestExecutionBaseline(ctx, org, id)
		if err != nil {
			return nil, tfexec.ErrBaselineUnavailable
		}
		out = append(out, domain.RepairInstance{InstanceID: id, InstanceVersion: inst.Version, BaselineID: baseline.ID, Descriptor: inst.Identity.Descriptor, Blockers: []string{}})
	}
	return out, nil
}

// A continuation binds the complete post-stage checkpoint. A deployment, repair
// or state acknowledgement since that checkpoint invalidates it before effects.
func (s *Service) verifyRepairPins(ctx context.Context, org string, pins []domain.RepairInstance) error {
	for _, pin := range pins {
		inst, err := s.Instances.GetInstance(ctx, pin.InstanceID)
		if err != nil || inst.Identity.OrganizationID != org || inst.Version != pin.InstanceVersion {
			return tfexec.ErrRepairStale
		}
		b, err := s.Instances.LatestExecutionBaseline(ctx, org, pin.InstanceID)
		if err != nil || b.ID != pin.BaselineID {
			return tfexec.ErrRepairStale
		}
	}
	return nil
}
func decodeRepairCheckpoint(raw any) ([]domain.RepairInstance, error) {
	body, err := json.Marshal(raw)
	var pins []domain.RepairInstance
	if err != nil || json.Unmarshal(body, &pins) != nil || len(pins) == 0 {
		return nil, persistence.ErrImmutable
	}
	return pins, nil
}

func (s *Service) deferredBy(ctx context.Context, id string, changes map[string]bool) (bool, error) {
	deps, ok := s.Instances.(repairDependencies)
	if !ok {
		return false, tfexec.ErrBaselineUnavailable
	}
	edges, err := deps.Dependencies(ctx, id)
	if err != nil {
		return false, err
	}
	for _, e := range edges {
		if e.ConsumerID == id && e.Holds() && changes[e.ProviderID] {
			return true, nil
		}
	}
	return false, nil
}

func repairDependsOnChange(id string, deps map[string][]string, changed, visited map[string]bool) bool {
	if visited[id] {
		return false
	}
	visited[id] = true
	for _, provider := range deps[id] {
		if changed[provider] || repairDependsOnChange(provider, deps, changed, visited) {
			return true
		}
	}
	return false
}
func orderRepairMembers(members []domain.RepairInstance, deps map[string][]string) ([]domain.RepairInstance, error) {
	byName := map[string]domain.RepairInstance{}
	for _, m := range members {
		byName[m.Descriptor] = m
	}
	visiting := map[string]int{}
	out := []domain.RepairInstance{}
	var visit func(string) error
	visit = func(id string) error {
		if visiting[id] == 2 {
			return nil
		}
		if visiting[id] == 1 {
			return tfexec.ErrRepairBlocked
		}
		visiting[id] = 1
		for _, p := range deps[id] {
			if err := visit(p); err != nil {
				return err
			}
		}
		visiting[id] = 2
		if m, exists := byName[id]; exists {
			out = append(out, m)
		}
		return nil
	}
	for _, m := range members {
		if err := visit(m.Descriptor); err != nil {
			return nil, err
		}
	}
	return out, nil
}
