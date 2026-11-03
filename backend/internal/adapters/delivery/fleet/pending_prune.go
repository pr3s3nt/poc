package fleet

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	domain "orchestrator/internal/domain/delivery"
	port "orchestrator/internal/ports/delivery"
)

func deletionOnlyRetirements(modified []any, stale []port.AppliedObject, namespace string) bool {
	if len(modified) == 0 || len(stale) == 0 {
		return false
	}
	allowed := map[string]bool{}
	for _, object := range stale {
		if object.UID == "" {
			return false
		}
		allowed[object.Key()] = true
	}
	seen := map[string]bool{}
	for _, raw := range modified {
		object, ok := raw.(map[string]any)
		if !ok || object["delete"] != true || object["namespace"] != namespace || object["patch"] != nil || object["create"] == true {
			return false
		}
		for key := range object {
			if key != "apiVersion" && key != "kind" && key != "namespace" && key != "name" && key != "delete" {
				return false
			}
		}
		api, _ := object["apiVersion"].(string)
		kind, _ := object["kind"].(string)
		name, _ := object["name"].(string)
		key := api + "/" + kind + "/" + name
		if !allowed[key] || seen[key] {
			return false
		}
		seen[key] = true
	}
	return true
}

// Observe remains truthful/read-only. This mutation is available only to an
// admitted Submit or explicit recovery with the exact receipt and old inventory.
func (p *Provider) PrunePendingReleasedObjects(ctx context.Context, i port.Intent, r port.Receipt, stale []port.AppliedObject) (bool, error) {
	if p.Manager == nil || p.Manager.Guard == nil {
		return false, ErrOwnership
	}
	guard := p.Manager.Guard
	if err := guard(ctx, i); err != nil {
		if p.Manager.RecoveryGuard == nil || p.Manager.RecoveryGuard(ctx, i) != nil {
			return false, err
		}
		guard = p.Manager.RecoveryGuard
	}
	observed, err := p.Observe(ctx, i, r)
	if err != nil {
		return false, err
	}
	observed = observed.Effective(time.Now().UTC(), 30*time.Second)
	if observed.Validate(i.Revision) != nil || !observed.Complete || observed.ObservedRevision != i.Revision.ID || observed.PayloadDigest != i.Revision.PayloadDigest || observed.Health != domain.Healthy || observed.Sync != domain.OutOfSync || observed.Drift != domain.Detected {
		return false, nil
	}
	source := map[string]port.AppliedObject{}
	for _, object := range i.SourceInventory {
		source[object.Key()] = object
	}
	for _, object := range stale {
		old, ok := source[object.Key()]
		if !ok || old.UID != object.UID || old.Release != object.Release {
			return false, ErrOwnership
		}
	}
	cli, close, cfg, err := p.Manager.open(ctx, i)
	if err != nil {
		return false, err
	}
	defer close()
	repo, found, err := cli.Get(ctx, cfg.Workspace, "gitrepos.fleet.cattle.io", r.Handle["gitrepo"])
	if err != nil || !found {
		return false, ErrController
	}
	metadata, _ := repo["metadata"].(map[string]any)
	spec, _ := repo["spec"].(map[string]any)
	if cfg.GitRepoMode != "MANAGED" {
		return false, nil
	}
	if metadata["uid"] != r.Handle["gitrepoUID"] || !owns(repo, i, r.Handle["gitrepo"]) || controllerRevision(repo) != i.Revision.ID || spec["revision"] != r.Handle["commit"] {
		return false, ErrOwnership
	}
	raw, err := cli.Run(ctx, nil, "get", "bundles.fleet.cattle.io", "-n", cfg.Workspace, "-l", "fleet.cattle.io/repo-name="+r.Handle["gitrepo"], "-o", "json")
	if err != nil {
		return false, ErrController
	}
	var bundles struct{ Items []map[string]any }
	if json.Unmarshal(raw, &bundles) != nil || len(bundles.Items) != 1 {
		return false, ErrController
	}
	cluster, found, err := cli.Get(ctx, cfg.Workspace, "clusters.fleet.cattle.io", cfg.TargetCluster)
	if err != nil || !found {
		return false, ErrController
	}
	cm, _ := cluster["metadata"].(map[string]any)
	cs, _ := cluster["status"].(map[string]any)
	if cm["uid"] != cfg.TargetClusterUID {
		return false, ErrOwnership
	}
	ns, _ := cs["namespace"].(string)
	bm, _ := bundles.Items[0]["metadata"].(map[string]any)
	name, _ := bm["name"].(string)
	if ns == "" || name == "" {
		return false, ErrController
	}
	bd, found, err := cli.Get(ctx, ns, "bundledeployments.fleet.cattle.io", name)
	if err != nil || !found {
		return false, ErrController
	}
	bs, _ := bd["spec"].(map[string]any)
	status, _ := bd["status"].(map[string]any)
	if bs["deploymentID"] == nil || bs["deploymentID"] != status["appliedDeploymentID"] || !controllerCommitMatches(r.Handle["commit"], bundles.Items[0], bd) {
		return false, nil
	}
	modified, _ := status["modifiedStatus"].([]any)
	if !deletionOnlyRetirements(modified, stale, i.Target.Namespace) {
		return false, nil
	}
	ordered := append([]port.AppliedObject(nil), stale...)
	priority := func(kind string) int {
		switch kind {
		case "VaultStaticSecret":
			return 0
		case "VaultAuth":
			return 1
		case "VaultConnection":
			return 2
		default:
			return 3
		}
	}
	sort.SliceStable(ordered, func(a, b int) bool { return priority(ordered[a].Kind) < priority(ordered[b].Kind) })
	if err := p.pruneRetainedInventory(ctx, i, r, ordered, observed, guard); err != nil {
		return false, err
	}
	return true, nil
}
