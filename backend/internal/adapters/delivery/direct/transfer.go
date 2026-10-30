package direct

import (
	"context"
	"encoding/json"
	"fmt"
	k8s "orchestrator/internal/adapters/kubernetes"
	domain "orchestrator/internal/domain/delivery"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/execution"
)

func (p *Provider) ValidateInventory(ctx context.Context, i port.Intent, r port.Receipt, objects []port.AppliedObject) error {
	cli, close, err := k8s.OpenCLI(ctx, p.KubectlPath, p.Credentials, i.Target)
	if err != nil {
		return err
	}
	defer close()
	refs := []execution.ObjectRef{}
	uids := map[string]string{}
	for _, o := range objects {
		refs = append(refs, o.ObjectRef)
		uids[o.Key()] = o.UID
	}
	if err := k8s.ValidateDeliveryIncarnations(ctx, cli, i.Target, refs, uids, k8s.DeliveryManager{}, i.Revision.Phase == domain.Job); err != nil {
		return err
	}
	return nil
}
func (p *Provider) ValidateTransfer(ctx context.Context, i port.Intent, r port.Receipt, objects []port.AppliedObject) error {
	if err := p.ValidateInventory(ctx, i, r, objects); err != nil {
		return err
	}
	if i.Revision.Phase != domain.Job {
		observed, err := p.Observe(ctx, i, r)
		if err != nil {
			return err
		}
		if !observed.Ready(i.Revision, observed.ObservedAt, 30e9) {
			return domain.ErrStale
		}
	}
	return nil
}
func (p *Provider) Adopt(ctx context.Context, i port.Intent, prepared port.Prepared, source port.TransferSource) (port.Receipt, error) {
	if p.Guard == nil {
		return port.Receipt{}, domain.ErrUnsupported
	}
	e, err := decode(prepared.Payload)
	if err != nil {
		return port.Receipt{}, err
	}
	cli, close, err := k8s.OpenCLI(ctx, p.KubectlPath, p.Credentials, i.Target)
	if err != nil {
		return port.Receipt{}, err
	}
	defer close()
	sourceManager := k8s.DeliveryManager{Release: source.Receipt.Handle["release"], Namespace: source.Receipt.Handle["releaseNamespace"]}
	for _, o := range source.Inventory {
		if err := p.Guard(ctx, i); err != nil {
			return port.Receipt{}, err
		}
		if i.Revision.Phase == domain.Job {
			live, err := k8s.InspectDeliveryObjects(ctx, cli, i.Target, []execution.ObjectRef{o.ObjectRef})
			if err != nil {
				return port.Receipt{}, err
			}
			if len(live) == 0 {
				continue
			}
		}
		if err := k8s.TransferDeliveryOwnership(ctx, cli, i.Target, o.ObjectRef, o.UID, sourceManager, k8s.DeliveryManager{}, i.Revision.ID); err != nil {
			return port.Receipt{}, fmt.Errorf("direct: adopt %s/%s metadata: %w", o.Kind, o.Name, err)
		}
	}
	inventory, _ := json.Marshal(refsOf(e))
	ready, _ := json.Marshal(waitRefs(e))
	handle := map[string]string{"inventory": string(inventory), "ready": string(ready)}
	if i.Revision.Phase == domain.Job {
		handle["completionOnly"] = "true"
		raw, _ := json.Marshal(source.Inventory)
		handle["completedInventory"] = string(raw)
	}
	return port.Receipt{IntentID: i.ID, RevisionID: i.Revision.ID, PayloadDigest: i.Revision.PayloadDigest, Handle: handle}, nil
}
