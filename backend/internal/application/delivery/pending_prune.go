package delivery

import (
	"context"
	"sort"
	"time"

	domain "orchestrator/internal/domain/delivery"
	port "orchestrator/internal/ports/delivery"
)

func (c *Coordinator) prunePending(ctx context.Context, provider port.Provider, i port.Intent, receipt port.Receipt, observed domain.Observation) error {
	observed = observed.Effective(time.Now().UTC(), ObservationInterval)
	if observed.Validate(i.Revision) != nil || !observed.Complete || observed.ObservedRevision != i.Revision.ID || observed.PayloadDigest != i.Revision.PayloadDigest || observed.Health != domain.Healthy || observed.Sync != domain.OutOfSync || observed.Drift != domain.Detected || len(observed.Objects) == 0 {
		return nil
	}
	pruner, ok := provider.(port.PendingPrune)
	if !ok {
		return nil
	}
	current := map[string]bool{}
	for _, object := range observed.Objects {
		if object.Namespace != i.Target.Namespace || object.UID == "" {
			return domain.ErrStale
		}
		current[object.APIVersion+"/"+object.Kind+"/"+object.Name] = true
	}
	stale := []port.AppliedObject{}
	for _, old := range i.SourceInventory {
		if !current[old.Key()] {
			if old.UID == "" {
				return domain.ErrStale
			}
			stale = append(stale, old)
		}
	}
	if len(stale) == 0 {
		return nil
	}
	sort.Slice(stale, func(a, b int) bool { return stale[a].Key() < stale[b].Key() })
	_, err := pruner.PrunePendingReleasedObjects(ctx, i, receipt, stale)
	return err
}
