package kubernetes

import (
	"context"
	"fmt"
	"time"

	"orchestrator/internal/ports/execution"
)

// Deployer applies workload manifests and observes readiness through kubectl.
type Deployer struct {
	KubectlPath string
	Timeout     time.Duration
}

// NewDeployer returns the Kubernetes workload deployer.
func NewDeployer(kubectlPath string) *Deployer {
	return &Deployer{KubectlPath: kubectlPath, Timeout: 5 * time.Minute}
}

// Apply sends every rendered manifest to the resolved target (UC-06 MS-11).
func (d *Deployer) Apply(ctx context.Context, target execution.Target, manifests []execution.Manifest) error {
	if target.Namespace == "" {
		return fmt.Errorf("kubernetes: apply without a namespace")
	}
	objects := make([]map[string]any, 0, len(manifests))
	for _, m := range manifests {
		objects = append(objects, m.Object)
	}
	cli := NewCLI(d.KubectlPath, target.Context, target.Kubeconfig)
	return cli.Apply(ctx, objects)
}

// WaitReady blocks until every referenced object reports readiness.
func (d *Deployer) WaitReady(ctx context.Context, target execution.Target, refs []execution.WorkloadRef) error {
	cli := NewCLI(d.KubectlPath, target.Context, target.Kubeconfig)
	for _, ref := range refs {
		namespace := ref.Namespace
		if namespace == "" {
			namespace = target.Namespace
		}
		kind := "deployment"
		if ref.Kind != "" {
			kind = kindArg(ref.Kind)
		}
		if err := cli.RolloutStatus(ctx, namespace, kind, ref.Name, d.Timeout); err != nil {
			return fmt.Errorf("kubernetes: %s/%s is not ready: %w", kind, ref.Name, err)
		}
	}
	return nil
}

func kindArg(kind string) string {
	switch kind {
	case "Deployment":
		return "deployment"
	case "StatefulSet":
		return "statefulset"
	case "DaemonSet":
		return "daemonset"
	default:
		return kind
	}
}

var _ execution.WorkloadDeployer = (*Deployer)(nil)
