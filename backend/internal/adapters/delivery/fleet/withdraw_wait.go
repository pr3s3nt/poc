package fleet

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	k8s "orchestrator/internal/adapters/kubernetes"
	port "orchestrator/internal/ports/delivery"
)

// A disappeared Bundle alone is not controller withdrawal: downstream deletion
// and Helm history cleanup can lag. Keep the receipt/fence until both are gone.
func (m *Manager) waitReleaseGone(ctx context.Context, cli k8s.CLI, cfg Config, i port.Intent, r port.Receipt) error {
	cluster, found, err := cli.Get(ctx, cfg.Workspace, "clusters.fleet.cattle.io", cfg.TargetCluster)
	if err != nil || !found {
		return ErrController
	}
	md, _ := cluster["metadata"].(map[string]any)
	status, _ := cluster["status"].(map[string]any)
	namespace, _ := status["namespace"].(string)
	if md["uid"] != cfg.TargetClusterUID || namespace == "" {
		return ErrOwnership
	}
	target, close, err := k8s.OpenCLI(ctx, m.Verifier.KubectlPath, m.Verifier.Credentials, i.Target)
	if err != nil {
		return err
	}
	defer close()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if err := m.Guard(ctx, i); err != nil {
			return err
		}
		raw, err := cli.Run(ctx, nil, "get", "bundledeployments.fleet.cattle.io", "-n", namespace, "-o", "json")
		var deployments struct {
			Items []struct {
				Spec struct {
					Options       struct{ Helm struct{ ReleaseName string } }
					StagedOptions struct{ Helm struct{ ReleaseName string } }
				}
			}
		}
		if err != nil || json.Unmarshal(raw, &deployments) != nil {
			return ErrController
		}
		pending := false
		for _, deployment := range deployments.Items {
			if deployment.Spec.Options.Helm.ReleaseName == r.Handle["release"] || deployment.Spec.StagedOptions.Helm.ReleaseName == r.Handle["release"] {
				pending = true
			}
		}
		output, err := target.Run(ctx, nil, "get", "secrets", "-n", cfg.ReleaseNamespace, "-l", "owner=helm,name="+r.Handle["release"], "-o", "jsonpath={.items[*].metadata.uid}")
		if err != nil {
			return ErrController
		}
		if !pending && strings.TrimSpace(string(output)) == "" {
			return nil
		}
		select {
		case <-ctx.Done():
			return ErrController
		case <-ticker.C:
		}
	}
}
