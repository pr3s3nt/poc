package artifact

import (
	"context"
	"encoding/json"
	k8s "orchestrator/internal/adapters/kubernetes"
	domain "orchestrator/internal/domain/delivery"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/execution"
	"time"
)

// ObserveCompletion imports an acknowledged Job result. Absence is expected
// after TTL; an existing different UID, foreign manager or failed Job is refused.
func ObserveCompletion(ctx context.Context, cli k8s.CLI, i port.Intent, r port.Receipt) (domain.Observation, error) {
	out := domain.Observation{Scope: i.Revision.Scope, IntendedRevision: i.Revision.ID, Sync: domain.SyncUnknown, Health: domain.HealthUnknown, Drift: domain.DriftUnknown, ObservedAt: time.Now().UTC()}
	if i.Revision.Phase != domain.Job || r.RevisionID != i.Revision.ID || r.PayloadDigest != i.Revision.PayloadDigest {
		return out, ErrInvalid
	}
	var objects []port.AppliedObject
	if json.Unmarshal([]byte(r.Handle["completedInventory"]), &objects) != nil || len(objects) == 0 {
		return out, ErrInvalid
	}
	refs := []execution.ObjectRef{}
	uids := map[string]string{}
	for _, o := range objects {
		if o.Kind != "Job" || o.UID == "" {
			return out, ErrInvalid
		}
		refs = append(refs, o.ObjectRef)
		uids[o.Key()] = o.UID
	}
	if err := k8s.ValidateDeliveryIncarnations(ctx, cli, i.Target, refs, uids, k8s.DeliveryManager{}, true); err != nil {
		return out, err
	}
	live, err := k8s.InspectDeliveryObjects(ctx, cli, i.Target, refs)
	if err != nil {
		return out, err
	}
	for _, o := range objects {
		if obj, found := live[o.Key()]; found {
			md, _ := obj["metadata"].(map[string]any)
			a, _ := md["annotations"].(map[string]any)
			status, _ := obj["status"].(map[string]any)
			succeeded, _ := status["succeeded"].(float64)
			if a["orchestrator.io/delivery-revision"] != i.Revision.ID || succeeded < 1 {
				return out, domain.ErrStale
			}
		}
		out.Objects = append(out.Objects, domain.ObjectObservation{APIVersion: o.APIVersion, Kind: o.Kind, Name: o.Name, Namespace: i.Target.Namespace, UID: o.UID})
	}
	out.ObservedRevision = i.Revision.ID
	out.PayloadDigest = i.Revision.PayloadDigest
	out.Complete = true
	out.Sync = domain.InSync
	out.Health = domain.Healthy
	out.Drift = domain.NoDrift
	return out, nil
}
