package deployment

import (
	"context"
	"fmt"
	"sort"

	domain "orchestrator/internal/domain/deployment"
	"orchestrator/internal/ports/execution"
)

// inventoryKey is the workload instance TargetRef entry holding the applied
// object inventory (ADR-024 RB-C09).
const inventoryKey = "objects"

// renderedInventory lists the owned objects of one render: every object the
// Git pipeline labelled for the workload, never a platform auxiliary.
func renderedInventory(manifests []execution.Manifest, deploymentID string) []execution.ObjectRef {
	var out []execution.ObjectRef
	for _, m := range manifests {
		if m.Auxiliary || m.Secret {
			continue
		}
		metadata, _ := m.Object["metadata"].(map[string]any)
		labels, _ := metadata["labels"].(map[string]any)
		if _, owned := labels["orchestrator.io/workload"]; !owned {
			continue
		}
		out = append(out, execution.ObjectRef{APIVersion: m.APIVersion, Kind: m.Kind, Name: m.Name, Owners: []string{deploymentID}})
	}
	sortInventory(out)
	return out
}

func sortInventory(refs []execution.ObjectRef) {
	sort.Slice(refs, func(i, j int) bool { return refs[i].Key() < refs[j].Key() })
}

// instanceInventory reads the persisted inventory of a workload instance.
func instanceInventory(instance domain.WorkloadInstance) []execution.ObjectRef {
	raw, _ := instance.TargetRef[inventoryKey].([]any)
	var out []execution.ObjectRef
	for _, entry := range raw {
		item, _ := entry.(map[string]any)
		ref := execution.ObjectRef{}
		ref.APIVersion, _ = item["apiVersion"].(string)
		ref.Kind, _ = item["kind"].(string)
		ref.Name, _ = item["name"].(string)
		owners, _ := item["owners"].([]any)
		for _, owner := range owners {
			if text, ok := owner.(string); ok {
				ref.Owners = append(ref.Owners, text)
			}
		}
		if ref.APIVersion != "" && ref.Kind != "" && ref.Name != "" {
			out = append(out, ref)
		}
	}
	return out
}

func withInventory(ref map[string]any, refs []execution.ObjectRef) map[string]any {
	if len(refs) == 0 {
		delete(ref, inventoryKey)
		return ref
	}
	items := make([]any, 0, len(refs))
	for _, r := range refs {
		owners := make([]any, 0, len(r.Owners))
		for _, owner := range r.Owners {
			owners = append(owners, owner)
		}
		items = append(items, map[string]any{"apiVersion": r.APIVersion, "kind": r.Kind, "name": r.Name, "owners": owners})
	}
	ref[inventoryKey] = items
	return ref
}

// mergeInventory returns the union of the previous and new inventories: an
// object in both may carry either deployment's ownership label until applied.
// stale lists the previous objects that are not part of the new render.
func mergeInventory(previous, next []execution.ObjectRef) (union, stale []execution.ObjectRef) {
	old := map[string]execution.ObjectRef{}
	for _, ref := range previous {
		old[ref.Key()] = ref
	}
	seen := map[string]bool{}
	for _, ref := range next {
		owners := append([]string{}, ref.Owners...)
		if before, ok := old[ref.Key()]; ok {
			owners = appendOwners(owners, before.Owners)
		}
		ref.Owners = owners
		union = append(union, ref)
		seen[ref.Key()] = true
	}
	for _, ref := range previous {
		if !seen[ref.Key()] {
			union = append(union, ref)
			stale = append(stale, ref)
		}
	}
	sortInventory(union)
	sortInventory(stale)
	return union, stale
}

func appendOwners(owners, more []string) []string {
	for _, owner := range more {
		present := false
		for _, have := range owners {
			present = present || have == owner
		}
		if !present {
			owners = append(owners, owner)
		}
	}
	return owners
}

// rollout kinds are applied after every Job completed (RB-C09).
var rolloutKinds = map[string]bool{"Deployment": true, "StatefulSet": true, "DaemonSet": true}

// applyPhases applies and waits for one workload's objects. With Jobs, all
// non-rollout objects go first and every Job must complete before rollout
// objects are applied; otherwise one apply as before ADR-024.
func (s *Service) applyPhases(ctx context.Context, target execution.Target, workloadID string, manifests []execution.Manifest) error {
	var before, rollout []execution.Manifest
	var jobs, ready []execution.WorkloadRef
	scheduled := false
	for _, m := range manifests {
		switch {
		case m.Kind == "CronJob" && m.APIVersion == "batch/v1":
			scheduled = true
			before = append(before, m)
		case m.Kind == "Job" && m.APIVersion == "batch/v1":
			jobs = append(jobs, execution.WorkloadRef{Kind: "Job", Name: m.Name, Namespace: target.Namespace})
			before = append(before, m)
		case rolloutKinds[m.Kind] && m.APIVersion == "apps/v1":
			rollout = append(rollout, m)
			ready = append(ready, execution.WorkloadRef{Kind: m.Kind, Name: m.Name, Namespace: target.Namespace})
		default:
			before = append(before, m)
		}
	}
	if len(ready) == 0 && !scheduled {
		// A render without labelled rollout objects (legacy) waits on the workload Deployment.
		ready = []execution.WorkloadRef{{Kind: "Deployment", Name: workloadID, Namespace: target.Namespace}}
	}
	if len(jobs) == 0 {
		if err := s.deployer.Apply(ctx, target, manifests); err != nil {
			return err
		}
		if len(ready) == 0 {
			return nil
		}
		return s.deployer.WaitReady(ctx, target, ready)
	}
	if err := s.deployer.Apply(ctx, target, before); err != nil {
		return err
	}
	if err := s.deployer.WaitReady(ctx, target, jobs); err != nil {
		return err
	}
	if len(rollout) > 0 {
		if err := s.deployer.Apply(ctx, target, rollout); err != nil {
			return err
		}
	}
	if len(ready) == 0 {
		return nil
	}
	return s.deployer.WaitReady(ctx, target, ready)
}

// removeObjects deletes inventory objects through the deployer. A deployer
// without inventory support cannot prove their removal and fails closed.
func (s *Service) removeObjects(ctx context.Context, target execution.Target, workloadID string, refs []execution.ObjectRef) error {
	if len(refs) == 0 {
		return nil
	}
	remover, ok := s.deployer.(execution.ObjectRemover)
	if !ok {
		return fmt.Errorf("deployment: the workload deployer cannot remove %d inventory objects of %q", len(refs), workloadID)
	}
	return remover.RemoveObjects(ctx, target, workloadID, refs)
}

// previousInstance returns the applied instance of one workload in the
// current generation, if any.
func (s *Service) previousInstance(ctx context.Context, environmentKey string, generation int64, workloadID string) (domain.WorkloadInstance, bool, error) {
	instances, err := s.store.ListWorkloadInstancesFor(ctx, environmentKey, generation)
	if err != nil {
		return domain.WorkloadInstance{}, false, err
	}
	for _, instance := range instances {
		if instance.WorkloadID == workloadID && instance.Status != domain.InstanceRemoved {
			return instance, true, nil
		}
	}
	return domain.WorkloadInstance{}, false, nil
}
