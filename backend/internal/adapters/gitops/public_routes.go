package gitops

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	k8s "orchestrator/internal/adapters/kubernetes"
	"orchestrator/internal/platform/canon"
	"orchestrator/internal/ports/execution"
)

// Reconcile stores the Environment Ingress in its own Fleet bundle and waits
// for the exact desired route revision to be observed on the cluster.
func (d *Deployer) Reconcile(ctx context.Context, target execution.Target, route execution.PublicRoute) error {
	if route.ApplicationID != target.Extra["application"] || route.EnvironmentID != target.Extra["environment"] {
		return fmt.Errorf("gitops: route target scope mismatch")
	}
	base, err := d.scope(target, "route")
	if err != nil {
		return err
	}
	path := filepath.Join(filepath.Dir(base), "_routes")
	var manifests []execution.Manifest
	hash := ""
	if len(route.Paths) > 0 {
		object, err := k8s.PublicIngress(target.Namespace, route)
		if err != nil {
			return err
		}
		hash, err = canon.Hash(object["spec"])
		if err != nil {
			return err
		}
		hash = hash[:32] // Kubernetes label values are limited to 63 bytes.
		metadata := object["metadata"].(map[string]any)
		metadata["labels"].(map[string]any)["orchestrator.io/route-hash"] = hash
		manifests = []execution.Manifest{{APIVersion: "networking.k8s.io/v1", Kind: "Ingress", Name: "orch-public", Namespace: target.Namespace, Object: object}}
	} else {
		if _, err := os.Stat(filepath.Join(d.opts.RepoDir, path)); os.IsNotExist(err) {
			_, exists, getErr := d.cli().Get(ctx, target.Namespace, "ingress", "orch-public")
			if getErr != nil {
				return getErr
			}
			if !exists {
				return nil
			}
			return fmt.Errorf("gitops: existing public Ingress has no Fleet route bundle to prune")
		} else if err != nil {
			return err
		}
	}
	d.mu.Lock()
	commit, err := d.writeAndPush(ctx, path, target.Namespace, manifests, len(route.Paths) == 0)
	d.mu.Unlock()
	if err != nil {
		return err
	}
	if err := d.waitCommit(ctx, commit); err != nil {
		return err
	}
	deadline, cancel := context.WithTimeout(ctx, d.opts.Timeout)
	defer cancel()
	for {
		object, exists, err := d.cli().Get(deadline, target.Namespace, "ingress", "orch-public")
		if err == nil {
			if hash == "" && !exists {
				return nil
			}
			if exists {
				metadata, _ := object["metadata"].(map[string]any)
				labels, _ := metadata["labels"].(map[string]any)
				if labels["orchestrator.io/route-hash"] == hash {
					return nil
				}
			}
		}
		if err := pause(deadline); err != nil {
			return fmt.Errorf("gitops: Fleet did not apply the expected public routes: %w", err)
		}
	}
}

var _ execution.PublicRouteManager = (*Deployer)(nil)
