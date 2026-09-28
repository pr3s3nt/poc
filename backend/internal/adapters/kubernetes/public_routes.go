package kubernetes

import (
	"context"
	"fmt"
	"regexp"

	"orchestrator/internal/ports/execution"
)

const publicIngressName = "orch-public"

var routeHost = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*[a-z0-9]$`)
var routePort = regexp.MustCompile(`^[a-z](?:[a-z0-9-]*[a-z0-9])?$`)

// PublicRoutes owns only the one orchestrator Ingress in an Environment
// namespace. It does not configure DNS, TLS or the controller's external port.
type PublicRoutes struct{ KubectlPath string }

func (r *PublicRoutes) Reconcile(ctx context.Context, target execution.Target, route execution.PublicRoute) error {
	if target.Namespace == "" || (target.Context == "" && target.Kubeconfig == "") || route.ApplicationID == "" || route.EnvironmentID == "" || route.WorkloadID == "" {
		return fmt.Errorf("kubernetes: public route requires scoped target and owner")
	}
	cli := NewCLI(r.KubectlPath, target.Context, target.Kubeconfig)
	object, exists, err := cli.Get(ctx, target.Namespace, "ingress", publicIngressName)
	if err != nil {
		return err
	}
	if exists {
		metadata, _ := object["metadata"].(map[string]any)
		labels, _ := metadata["labels"].(map[string]any)
		if labels["app.kubernetes.io/managed-by"] != "orchestrator" || labels["orchestrator.io/application"] != route.ApplicationID || labels["orchestrator.io/environment"] != route.EnvironmentID || labels["orchestrator.io/workload"] != route.WorkloadID {
			return fmt.Errorf("kubernetes: public Ingress is not owned by workload %q", route.WorkloadID)
		}
	}
	if route.PortName == "" {
		if !exists {
			return nil
		}
		return cli.Delete(ctx, target.Namespace, "ingress", publicIngressName)
	}
	manifest, err := publicIngress(target.Namespace, route)
	if err != nil {
		return err
	}
	return cli.Apply(ctx, []map[string]any{manifest})
}

func publicIngress(namespace string, route execution.PublicRoute) (map[string]any, error) {
	if !routeHost.MatchString(route.Host) || !routePort.MatchString(route.PortName) {
		return nil, fmt.Errorf("kubernetes: invalid public host or Service port")
	}
	return map[string]any{
		"apiVersion": "networking.k8s.io/v1", "kind": "Ingress",
		"metadata": map[string]any{"name": publicIngressName, "namespace": namespace, "labels": map[string]any{
			"app.kubernetes.io/managed-by": "orchestrator", "orchestrator.io/application": route.ApplicationID,
			"orchestrator.io/environment": route.EnvironmentID, "orchestrator.io/workload": route.WorkloadID,
		}},
		"spec": map[string]any{"ingressClassName": "traefik", "rules": []any{map[string]any{
			"host": route.Host, "http": map[string]any{"paths": []any{map[string]any{
				"path": "/", "pathType": "Prefix", "backend": map[string]any{"service": map[string]any{
					"name": route.WorkloadID, "port": map[string]any{"name": route.PortName},
				}},
			}}},
		}}},
	}, nil
}

var _ execution.PublicRouteManager = (*PublicRoutes)(nil)
