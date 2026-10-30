package fleet

import (
	"context"
	"encoding/json"
	k8s "orchestrator/internal/adapters/kubernetes"
	domain "orchestrator/internal/domain/delivery"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/execution"
	"time"
)

func (p *Provider) ValidateInventory(ctx context.Context, i port.Intent, r port.Receipt, objects []port.AppliedObject) error {
	cli, close, err := k8s.OpenCLI(ctx, p.Manager.Verifier.KubectlPath, p.Manager.Verifier.Credentials, i.Target)
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
	manager := k8s.DeliveryManager{Release: r.Handle["release"], Namespace: r.Handle["releaseNamespace"]}
	if err := k8s.ValidateDeliveryIncarnations(ctx, cli, i.Target, refs, uids, manager, i.Revision.Phase == domain.Job); err != nil {
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
		if !observed.Ready(i.Revision, time.Now().UTC(), 30*time.Second) {
			return domain.ErrStale
		}
	}
	return nil
}
func (p *Provider) Adopt(ctx context.Context, i port.Intent, prepared port.Prepared, source port.TransferSource) (port.Receipt, error) {
	if p.Manager == nil || p.Manager.Guard == nil {
		return port.Receipt{}, ErrOwnership
	}
	var pkg Package
	if json.Unmarshal(prepared.Payload, &pkg) != nil {
		return port.Receipt{}, ErrConfig
	}
	cfg, err := ParseConfig(i.Binding.Config)
	if err != nil {
		return port.Receipt{}, err
	}
	// Handoff leaves no competing Update owner of platform metadata.
	destination := k8s.DeliveryManager{Release: pkg.ReleaseName, Namespace: cfg.ReleaseNamespace}
	if i.Revision.Phase == domain.Job {
		destination = k8s.DeliveryManager{}
	}
	cli, close, err := k8s.OpenCLI(ctx, p.Manager.Verifier.KubectlPath, p.Manager.Verifier.Credentials, i.Target)
	if err != nil {
		return port.Receipt{}, err
	}
	defer close()
	sourceManager := k8s.DeliveryManager{Release: source.Receipt.Handle["release"], Namespace: source.Receipt.Handle["releaseNamespace"]}
	for _, o := range source.Inventory {
		if err := p.Manager.Guard(ctx, i); err != nil {
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
		if err := k8s.TransferDeliveryOwnership(ctx, cli, i.Target, o.ObjectRef, o.UID, sourceManager, destination, i.Revision.ID); err != nil {
			return port.Receipt{}, err
		}
	}
	if i.Revision.Phase == domain.Job {
		completed := append([]port.AppliedObject(nil), source.Inventory...)
		for n := range completed {
			completed[n].Release = ""
		}
		raw, _ := json.Marshal(completed)
		return port.Receipt{IntentID: i.ID, RevisionID: i.Revision.ID, PayloadDigest: i.Revision.PayloadDigest, Handle: map[string]string{"completionOnly": "true", "completedInventory": string(raw)}}, nil
	}
	i.SourceInventory = append([]port.AppliedObject(nil), source.Inventory...)
	i.ExpectedRevision = source.Revision.ID
	return p.Submit(ctx, i, prepared)
}
