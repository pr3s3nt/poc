package fleet

import (
	"context"
	"encoding/json"

	k8s "orchestrator/internal/adapters/kubernetes"
	port "orchestrator/internal/ports/delivery"
)

// Fleet watches metadata annotation changes. Wake only the existing pending
// deployment after validating its receipted controller; never force-sync it.
func (p *Provider) wakePendingReceipt(ctx context.Context, cli k8s.CLI, cfg Config, i port.Intent, r port.Receipt) error {
	raw, err := cli.Run(ctx, nil, "get", "bundles.fleet.cattle.io", "-n", cfg.Workspace, "-l", "fleet.cattle.io/repo-name="+r.Handle["gitrepo"], "-o", "json")
	var bundles struct{ Items []map[string]any }
	if err != nil || json.Unmarshal(raw, &bundles) != nil {
		return ErrController
	}
	matched := []map[string]any{}
	for _, b := range bundles.Items {
		if bundleRelease(b) == r.Handle["release"] {
			matched = append(matched, b)
		}
	}
	if len(matched) != 1 {
		return ErrOwnership
	}
	cluster, found, err := cli.Get(ctx, cfg.Workspace, "clusters.fleet.cattle.io", cfg.TargetCluster)
	if err != nil || !found {
		return ErrController
	}
	md, _ := cluster["metadata"].(map[string]any)
	status, _ := cluster["status"].(map[string]any)
	ns, _ := status["namespace"].(string)
	if md["uid"] != cfg.TargetClusterUID || ns == "" {
		return ErrOwnership
	}
	metadata, _ := matched[0]["metadata"].(map[string]any)
	name, _ := metadata["name"].(string)
	if name == "" {
		return ErrOwnership
	}
	if cfg.GitRepoMode != "EXISTING" {
		labels, _ := metadata["labels"].(map[string]any)
		if !owns(matched[0], i, r.Handle["gitrepo"]) || labels["fleet.cattle.io/commit"] != r.Handle["commit"] {
			return ErrOwnership
		}
	}
	bd, found, err := cli.Get(ctx, ns, "bundledeployments.fleet.cattle.io", name)
	if err != nil || !found {
		return ErrController
	}
	patch, pending, err := pendingRecoveryPatch(bd, i)
	if err != nil || !pending {
		return err
	}
	if err := p.Manager.RecoveryGuard(ctx, i); err != nil {
		return err
	}
	if _, err := cli.Run(ctx, nil, "patch", "bundledeployments.fleet.cattle.io", name, "-n", ns, "--type=json", "--patch", string(patch)); err != nil {
		return ErrController
	}
	return nil
}

func pendingRecoveryPatch(bd map[string]any, i port.Intent) ([]byte, bool, error) {
	spec, _ := bd["spec"].(map[string]any)
	applied, _ := bd["status"].(map[string]any)
	desired, _ := spec["deploymentID"].(string)
	if desired == "" {
		return nil, false, ErrOwnership
	}
	if desired == applied["appliedDeploymentID"] {
		return nil, false, nil
	}
	md, _ := bd["metadata"].(map[string]any)
	uid, _ := md["uid"].(string)
	version, _ := md["resourceVersion"].(string)
	if uid == "" || version == "" {
		return nil, false, ErrOwnership
	}
	annotations, _ := md["annotations"].(map[string]any)
	if annotations == nil {
		annotations = map[string]any{}
	}
	annotations["orchestrator.io/recovery-observation"] = i.Owner.OperationID + "/" + i.Owner.Instance
	patch, _ := json.Marshal([]map[string]any{{"op": "test", "path": "/metadata/uid", "value": md["uid"]}, {"op": "test", "path": "/metadata/resourceVersion", "value": md["resourceVersion"]}, {"op": "test", "path": "/spec/deploymentID", "value": desired}, {"op": "add", "path": "/metadata/annotations", "value": annotations}})
	return patch, true, nil
}
