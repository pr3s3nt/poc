package fleet

import (
	"context"
	"encoding/json"
	k8s "orchestrator/internal/adapters/kubernetes"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/execution"
	gitport "orchestrator/internal/ports/gitpublication"
)

// Recovery may settle an update rejected before publication only when both new
// publications are absent and the complete prior controller/UID inventory is
// still authoritative. It never submits, adopts, prunes or retries the update.
func (p *Provider) observeUnpublishedUpdate(ctx context.Context, i port.Intent, prepared port.Prepared) (bool, error) {
	var pkg Package
	if json.Unmarshal(prepared.Payload, &pkg) != nil || pkg.Publication.ID != i.ID || pkg.Publication.TreeDigest != i.Revision.PayloadDigest || gitport.Digest(pkg.Publication.Files) != i.Revision.PayloadDigest || pkg.Metadata == nil {
		return false, ErrOwnership
	}
	cli, close, cfg, err := p.Manager.open(ctx, i)
	if err != nil {
		return false, err
	}
	defer close()
	if cfg.ApplicationID == "" {
		return false, nil
	}
	repo, found, err := cli.Get(ctx, cfg.Workspace, "gitrepos.fleet.cattle.io", applicationRepoName(i, cfg))
	if err != nil {
		return false, err
	}
	if !found || !applicationOwns(repo, i, cfg) {
		return false, ErrOwnership
	}
	pins, err := readUnitPins(repo)
	if err != nil {
		return false, err
	}
	pin, exists := pins[pkg.ReleaseName]
	if !unpublishedPriorPin(pin, exists, i, pkg) {
		return false, nil
	}
	// A published failure must still be the exact pin holder it was receipted as.
	if failedPrior(i) && (pin.Intent != i.PriorSubmission.Receipt.IntentID || pin.Digest != i.PriorSubmission.Receipt.PayloadDigest) {
		return false, ErrOwnership
	}
	spec, _ := repo["spec"].(map[string]any)
	commit, _ := spec["revision"].(string)
	if err := p.verifySharedTrees(ctx, i, commit, pins); err != nil {
		return false, err
	}
	publisher, err := p.Publishers.ResolvePublisher(ctx, i.Binding)
	if err != nil {
		return false, err
	}
	for _, publication := range []gitport.Intent{pkg.Publication, *pkg.Metadata} {
		_, found, err := publisher.ObservePublication(ctx, publication)
		if err != nil || found {
			return false, err
		}
	}
	return p.verifyPriorNativeObjects(ctx, i, pkg, cfg)
}

func (p *Provider) verifyPriorNativeObjects(ctx context.Context, i port.Intent, pkg Package, cfg Config) (bool, error) {
	target, done, err := k8s.OpenCLI(ctx, p.Manager.Verifier.KubectlPath, p.Manager.Verifier.Credentials, i.Target)
	if err != nil {
		return false, err
	}
	defer done()
	prior := map[string]port.AppliedObject{}
	for _, o := range i.SourceInventory {
		prior[o.Key()] = o
	}
	failed := failedPrior(i)
	if failed {
		// UC-06 CD-10: the published failure was applied without an acknowledged
		// UID inventory. Its own objects are expected, identified by revision.
		var old Package
		if json.Unmarshal(i.PriorSubmission.Prepared.Payload, &old) != nil || old.ReleaseName != pkg.ReleaseName || old.Publication.TreeDigest != i.PriorSubmission.Prepared.Revision.PayloadDigest {
			return false, ErrOwnership
		}
		for _, ref := range old.Objects {
			if _, ok := prior[ref.Key()]; !ok {
				prior[ref.Key()] = port.AppliedObject{ObjectRef: ref}
			}
		}
	}
	refs := []execution.ObjectRef{}
	for _, o := range prior {
		refs = append(refs, o.ObjectRef)
	}
	refs = append(refs, pkg.Objects...)
	live, err := k8s.InspectDeliveryObjects(ctx, target, i.Target, refs)
	if err != nil {
		return false, err
	}
	for key, obj := range live {
		proof, ok := prior[key]
		if !ok || (proof.UID == "" && !failed) {
			return false, ErrOwnership
		}
		md, _ := obj["metadata"].(map[string]any)
		a, _ := md["annotations"].(map[string]any)
		labels, _ := md["labels"].(map[string]any)
		if (proof.UID != "" && md["uid"] != proof.UID) || a["orchestrator.io/delivery-revision"] != i.ExpectedRevision || a["meta.helm.sh/release-name"] != pkg.ReleaseName || a["meta.helm.sh/release-namespace"] != cfg.ReleaseNamespace || labels["orchestrator.io/managed-by"] != "orchestrator" || labels["orchestrator.io/application"] != i.Target.Extra["application"] || labels["orchestrator.io/environment"] != i.Target.Extra["environment"] {
			return false, ErrOwnership
		}
	}
	// A failed rollout may have pruned objects of the acknowledged source, so only
	// an acknowledged prior must still be complete.
	if !failed && len(live) != len(prior) {
		return false, ErrOwnership
	}
	return true, nil
}

// failedPrior reports a published determined failure supplied as the pin holder.
func failedPrior(i port.Intent) bool {
	p := i.PriorSubmission
	return p != nil && p.Failed && p.Prepared.Revision.ID == i.ExpectedRevision && p.Receipt.RevisionID == i.ExpectedRevision && p.Receipt.IntentID != ""
}
func unpublishedPriorPin(pin unitPin, exists bool, i port.Intent, pkg Package) bool {
	return exists && !pin.Removed && pin.Revision == i.ExpectedRevision && pin.Revision != i.Revision.ID && pin.Intent != i.ID && pin.EnvironmentID == i.Revision.Scope.EnvironmentID && pin.Path == pkg.Publication.Path && pin.Digest != ""
}
