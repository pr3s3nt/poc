package fleet

import (
	"context"
	"encoding/json"
	k8s "orchestrator/internal/adapters/kubernetes"
	domain "orchestrator/internal/domain/delivery"
	port "orchestrator/internal/ports/delivery"
)

// RecoverSubmission neither pushes Git nor calls Submit/apply. It requires the
// persisted exact controller/publication receipt and source incarnation proof.
func (p *Provider) RecoverSubmission(ctx context.Context, i port.Intent, prepared port.Prepared, r port.Receipt, source port.TransferSource) (port.Receipt, error) {
	if p.Manager == nil || p.Manager.RecoveryGuard == nil || p.Publishers == nil {
		return r, domain.ErrUnsupported
	}
	if err := p.Manager.RecoveryGuard(ctx, i); err != nil {
		return r, err
	}
	var pkg Package
	if json.Unmarshal(prepared.Payload, &pkg) != nil || pkg.Publication.ID != i.ID || pkg.Publication.TreeDigest != i.Revision.PayloadDigest || r.IntentID != i.ID || r.RevisionID != i.Revision.ID || r.PayloadDigest != i.Revision.PayloadDigest {
		return r, ErrOwnership
	}
	publisher, err := p.Publishers.ResolvePublisher(ctx, i.Binding)
	if err != nil {
		return r, err
	}
	published, found, err := publisher.ObservePublication(ctx, pkg.Publication)
	if err != nil {
		return r, err
	}
	if !found || published.Commit != r.Handle["commit"] || published.TreeDigest != r.PayloadDigest {
		return r, domain.ErrOutcomeUnknown
	}
	cli, close, cfg, err := p.Manager.open(ctx, i)
	if err != nil {
		return r, err
	}
	defer close()
	if r.Handle["release"] != pkg.ReleaseName || r.Handle["releaseNamespace"] != cfg.ReleaseNamespace || r.Handle["namespace"] != i.Target.Namespace || r.Handle["commit"] == "" || r.Handle["gitrepoUID"] == "" {
		return r, ErrOwnership
	}
	if cfg.GitRepoMode == "EXISTING" {
		if r.Handle["gitrepo"] != cfg.ExistingGitRepo || r.Handle["path"] != pkg.Publication.Path || r.Handle["gitRepoMode"] != "EXISTING" {
			return r, ErrOwnership
		}
	} else if r.Handle["gitrepo"] != pkg.UnitName {
		return r, ErrOwnership
	}
	repo, found, err := cli.Get(ctx, cfg.Workspace, "gitrepos.fleet.cattle.io", r.Handle["gitrepo"])
	if err != nil || !found {
		return r, ErrController
	}
	md, _ := repo["metadata"].(map[string]any)
	if md["uid"] != r.Handle["gitrepoUID"] {
		return r, ErrOwnership
	}
	if cfg.GitRepoMode == "EXISTING" {
		if _, err := checkExisting(ctx, cli, cfg); err != nil {
			return r, err
		}
	} else {
		annotations, _ := md["annotations"].(map[string]any)
		spec, _ := repo["spec"].(map[string]any)
		if !owns(repo, i, r.Handle["gitrepo"]) || controllerRevision(repo) != i.Revision.ID || annotations["orchestrator.io/delivery-intent"] != i.ID || spec["revision"] != r.Handle["commit"] {
			return r, ErrOwnership
		}
	}
	if len(source.Inventory) == 0 {
		return r, p.wakePendingReceipt(ctx, cli, cfg, i, r)
	}
	targetCLI, targetClose, err := k8s.OpenCLI(ctx, p.Manager.Verifier.KubectlPath, p.Manager.Verifier.Credentials, i.Target)
	if err != nil {
		return r, err
	}
	defer targetClose()
	manager := k8s.DeliveryManager{Release: r.Handle["release"], Namespace: r.Handle["releaseNamespace"]}
	expected := map[string]bool{}
	for _, o := range pkg.Objects {
		expected[o.Key()] = true
	}
	for _, o := range source.Inventory {
		if !expected[o.Key()] {
			continue
		}
		if err := p.Manager.RecoveryGuard(ctx, i); err != nil {
			return r, err
		}
		if err := k8s.ReleaseDeliveryMetadata(ctx, targetCLI, i.Target, o.ObjectRef, o.UID, manager, source.Revision.ID, i.Revision.ID); err != nil {
			return r, err
		}
	}
	if err := p.reclaimPolicyFields(ctx, targetCLI, cfg, i, pkg, source.Inventory, source.Revision.ID, p.Manager.RecoveryGuard); err != nil {
		return r, err
	}
	return r, p.wakePendingReceipt(ctx, cli, cfg, i, r)
}
