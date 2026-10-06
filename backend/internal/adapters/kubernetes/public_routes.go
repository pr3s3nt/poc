package kubernetes

import (
	"context"
	"fmt"
	"regexp"
	"sort"

	"orchestrator/internal/ports/execution"
)

const publicIngressName = "orch-public"

var routeHost = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*[a-z0-9]$`)
var routePort = regexp.MustCompile(`^[a-z](?:[a-z0-9-]*[a-z0-9])?$`)

// PublicRoutes owns only the one orchestrator Ingress in an Environment
// namespace. It does not configure DNS, TLS or the controller's external port.
type PublicRoutes struct {
	KubectlPath string
	Credentials execution.KubeconfigSource
}

func (r *PublicRoutes) Reconcile(ctx context.Context, target execution.Target, route execution.PublicRoute) error {
	if target.Namespace == "" || !target.Explicit() || route.ApplicationID == "" || route.EnvironmentID == "" {
		return fmt.Errorf("kubernetes: public route requires scoped target and owner")
	}
	cli, cleanup, err := OpenCLI(ctx, r.KubectlPath, r.Credentials, target)
	if err != nil {
		return err
	}
	defer cleanup()
	object, exists, err := cli.Get(ctx, target.Namespace, "ingress", publicIngressName)
	if err != nil {
		return err
	}
	if exists {
		metadata, _ := object["metadata"].(map[string]any)
		labels, _ := metadata["labels"].(map[string]any)
		if labels["app.kubernetes.io/managed-by"] != "orchestrator" || labels["orchestrator.io/application"] != route.ApplicationID || labels["orchestrator.io/environment"] != route.EnvironmentID {
			return fmt.Errorf("kubernetes: public Ingress is not owned by Environment %q", route.EnvironmentID)
		}
	}
	if len(route.Paths) == 0 {
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

// PublicIngress builds a deterministic Environment-owned Ingress for both direct and Fleet adapters.
func PublicIngress(namespace string, route execution.PublicRoute) (map[string]any, error) {
	if !routeHost.MatchString(route.Host) || len(route.Paths) == 0 {
		return nil, fmt.Errorf("kubernetes: invalid public host or Service port")
	}
	paths := append([]execution.PublicPath(nil), route.Paths...)
	sort.Slice(paths, func(i, j int) bool { return paths[i].Path < paths[j].Path })
	httpPaths := make([]any, 0, len(paths))
	for _, path := range paths {
		if path.Path == "" || path.Path[0] != '/' || !routePort.MatchString(path.PortName) || !routePort.MatchString(path.WorkloadID) {
			return nil, fmt.Errorf("kubernetes: invalid public path or Service port")
		}
		httpPaths = append(httpPaths, map[string]any{"path": path.Path, "pathType": "Prefix", "backend": map[string]any{"service": map[string]any{"name": path.WorkloadID, "port": map[string]any{"name": path.PortName}}}})
	}
	return map[string]any{
		"apiVersion": "networking.k8s.io/v1", "kind": "Ingress",
		"metadata": map[string]any{"name": publicIngressName, "namespace": namespace, "labels": map[string]any{
			"app.kubernetes.io/managed-by": "orchestrator", "orchestrator.io/application": route.ApplicationID,
			"orchestrator.io/environment": route.EnvironmentID,
		}},
		"spec": map[string]any{"ingressClassName": "traefik", "rules": []any{map[string]any{
			"host": route.Host, "http": map[string]any{"paths": httpPaths},
		}}},
	}, nil
}

func publicIngress(namespace string, route execution.PublicRoute) (map[string]any, error) {
	return PublicIngress(namespace, route)
}

var _ execution.PublicRouteManager = (*PublicRoutes)(nil)
