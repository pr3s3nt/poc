package fleet

import (
	"context"
	"encoding/json"
	domain "orchestrator/internal/domain/delivery"
	port "orchestrator/internal/ports/delivery"
	gitport "orchestrator/internal/ports/gitpublication"
)

// Complete only the admitted update's publication pair. The retained source is
// decrypted and scope-checked by the coordinator, never supplied by an HTTP client.
func (p *Provider) recoverPublishedUpdate(ctx context.Context, i port.Intent, pkg Package, cfg Config, repo map[string]any, pins map[string]unitPin) (port.Receipt, error) {
	prior := i.PriorSubmission
	if prior == nil || pkg.Metadata == nil || i.Revision.Phase == domain.Job || (len(i.SourceInventory) == 0 && !prior.Failed) || prior.Prepared.Revision.ID != i.ExpectedRevision || prior.Prepared.Revision.Scope != i.Revision.Scope {
		return port.Receipt{}, ErrOwnership
	}
	var old Package
	if json.Unmarshal(prior.Prepared.Payload, &old) != nil || old.Metadata == nil || old.ReleaseName != pkg.ReleaseName || old.Publication.Path != pkg.Publication.Path || old.Publication.DestinationKey != pkg.Publication.DestinationKey || old.Publication.Branch != pkg.Publication.Branch || old.Publication.TreeDigest != prior.Prepared.Revision.PayloadDigest || gitport.Digest(old.Publication.Files) != old.Publication.TreeDigest || old.Metadata.Path != pkg.Metadata.Path || old.Metadata.DestinationKey != pkg.Metadata.DestinationKey || old.Metadata.Branch != pkg.Metadata.Branch || gitport.Digest(old.Metadata.Files) != old.Metadata.TreeDigest {
		return port.Receipt{}, ErrOwnership
	}
	pin := pins[pkg.ReleaseName]
	md, _ := repo["metadata"].(map[string]any)
	uid, _ := md["uid"].(string)
	if uid == "" || md["deletionTimestamp"] != nil || prior.Receipt.Handle["gitrepoUID"] != uid || prior.Receipt.Handle["release"] != pkg.ReleaseName || prior.Receipt.RevisionID != i.ExpectedRevision || prior.Receipt.IntentID != old.Publication.ID || prior.Receipt.PayloadDigest != pin.Digest || pin.Intent != old.Publication.ID || pin.Digest != old.Publication.TreeDigest {
		return port.Receipt{}, ErrOwnership
	}
	if err := p.validateApplicationSubmission(ctx, i, pkg, cfg); err != nil {
		return port.Receipt{}, err
	}
	spec, _ := repo["spec"].(map[string]any)
	commit, _ := spec["revision"].(string)
	if err := p.verifySharedTrees(ctx, i, commit, pins); err != nil {
		return port.Receipt{}, err
	}
	if _, err := p.verifyPriorNativeObjects(ctx, i, pkg, cfg); err != nil {
		return port.Receipt{}, err
	}
	publisher, err := p.Publishers.ResolvePublisher(ctx, i.Binding)
	if err != nil {
		return port.Receipt{}, err
	}
	published, found, err := publisher.ObservePublication(ctx, pkg.Publication)
	if err != nil {
		return port.Receipt{}, err
	}
	if !found {
		return port.Receipt{}, domain.ErrOutcomeUnknown
	}
	metadata, found, err := publisher.ObservePublication(ctx, *pkg.Metadata)
	if err != nil {
		return port.Receipt{}, err
	}
	if !found {
		// Never overwrite metadata from a third revision, even under our lease.
		_, oldFound, err := publisher.ObservePublication(ctx, *old.Metadata)
		if err != nil {
			return port.Receipt{}, err
		}
		if !oldFound {
			return port.Receipt{}, ErrOwnership
		}
		if err := p.Manager.RecoveryGuard(ctx, i); err != nil {
			return port.Receipt{}, err
		}
		metadata, err = publisher.Publish(ctx, *pkg.Metadata, func(ctx context.Context) error { return p.Manager.RecoveryGuard(ctx, i) })
		if err != nil {
			return port.Receipt{}, err
		}
	}
	// Reobserve both trees at the same remote head, including all sibling pins.
	published, found, err = publisher.ObservePublication(ctx, pkg.Publication)
	if err != nil || !found {
		return port.Receipt{}, domain.ErrOutcomeUnknown
	}
	metadata, found, err = publisher.ObservePublication(ctx, *pkg.Metadata)
	if err != nil || !found || metadata.Commit != published.Commit {
		return port.Receipt{}, domain.ErrOutcomeUnknown
	}
	manager := *p.Manager
	manager.Guard = p.Manager.RecoveryGuard
	recovering := *p
	recovering.Manager = &manager
	return recovering.submitApplicationAttempt(ctx, i, pkg, published, 0, uid)
}
