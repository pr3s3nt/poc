package kubernetes

import (
	"context"
	"fmt"
	"strings"
	"time"

	"orchestrator/internal/ports/execution"
)

// Deployer applies workload manifests and observes readiness through kubectl.
type Deployer struct {
	KubectlPath string
	Timeout     time.Duration
	// Credentials resolves credential-backed targets; nil fails them closed.
	Credentials execution.KubeconfigSource
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
	cli, cleanup, err := OpenCLI(ctx, d.KubectlPath, d.Credentials, target)
	if err != nil {
		return err
	}
	defer cleanup()
	return cli.Apply(ctx, objects)
}

// WaitReady blocks until every referenced object reports readiness.
func (d *Deployer) WaitReady(ctx context.Context, target execution.Target, refs []execution.WorkloadRef) error {
	cli, cleanup, err := OpenCLI(ctx, d.KubectlPath, d.Credentials, target)
	if err != nil {
		return err
	}
	defer cleanup()
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

// Remove deletes only the exact application/environment/deployment-owned
// workload objects. A same-name object with different or missing ownership
// labels makes the operation fail before any deletion.
func (d *Deployer) Remove(ctx context.Context, target execution.Target, workloadID string) error {
	if target.Namespace == "" {
		return fmt.Errorf("kubernetes: remove without a namespace")
	}
	if !target.Explicit() {
		return fmt.Errorf("kubernetes: remove without an explicit cluster target")
	}
	application, environment, deployment := target.Extra["application"], target.Extra["environment"], target.Extra["deployment"]
	if application == "" || environment == "" || deployment == "" || workloadID == "" || strings.Contains(workloadID, "/") {
		return fmt.Errorf("kubernetes: remove without the persisted application, environment and deployment identity")
	}
	cli, cleanup, err := OpenCLI(ctx, d.KubectlPath, d.Credentials, target)
	if err != nil {
		return err
	}
	defer cleanup()
	type ownedObject struct{ kind, name, uid string }
	owned := []ownedObject{{kind: "deployment", name: workloadID}, {kind: "service", name: workloadID}, {kind: "secret", name: workloadID + "-env"}}
	for i := range owned {
		object := &owned[i]
		doc, found, err := cli.Get(ctx, target.Namespace, object.kind, object.name)
		if err != nil {
			return err
		}
		if !found {
			continue
		}
		if !hasOwnershipLabels(doc, application, environment, deployment, workloadID) {
			return fmt.Errorf("kubernetes: refusing to remove %s/%s with unverified application ownership", object.kind, object.name)
		}
		metadata, _ := doc["metadata"].(map[string]any)
		object.uid, _ = metadata["uid"].(string)
		if object.uid == "" {
			return fmt.Errorf("kubernetes: refusing to remove %s/%s without a verified UID", object.kind, object.name)
		}
	}
	for _, object := range owned {
		if object.uid == "" {
			continue
		}
		if err := cli.DeleteUID(ctx, target.Namespace, object.kind, object.name, object.uid, d.Timeout); err != nil {
			return err
		}
	}
	return nil
}

func hasOwnershipLabels(object map[string]any, application, environment, deployment, workloadID string) bool {
	metadata, ok := object["metadata"].(map[string]any)
	if !ok {
		return false
	}
	labels, ok := metadata["labels"].(map[string]any)
	if !ok {
		return false
	}
	return labels["orchestrator.io/application"] == application &&
		labels["orchestrator.io/environment"] == environment &&
		labels["orchestrator.io/deployment-id"] == deployment &&
		labels["app.kubernetes.io/managed-by"] == "orchestrator" &&
		labels["app.kubernetes.io/name"] == workloadID
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
