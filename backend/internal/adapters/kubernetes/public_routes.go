package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"orchestrator/internal/ports/execution"
)

const publicRouteName = "orch-public"

// Public route kinds (ADR-024): an Ingress of the configured class, or a
// Gateway API HTTPRoute attached to the configured Gateway.
const (
	RouteKindIngress   = "ingress"
	RouteKindHTTPRoute = "httproute"
)

var routeHost = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*[a-z0-9]$`)
var routePort = regexp.MustCompile(`^[a-z](?:[a-z0-9-]*[a-z0-9])?$`)

// GatewayRef is the parent Gateway of platform HTTPRoutes.
type GatewayRef struct{ Namespace, Name, Section string }

// ParseGatewayRef reads "namespace/name[/sectionName]".
func ParseGatewayRef(value string) (GatewayRef, error) {
	parts := strings.Split(value, "/")
	if len(parts) < 2 || len(parts) > 3 || !dnsLabel.MatchString(parts[0]) || !kubernetesSubdomain.MatchString(parts[1]) {
		return GatewayRef{}, fmt.Errorf("kubernetes: public gateway must be namespace/name[/sectionName]")
	}
	ref := GatewayRef{Namespace: parts[0], Name: parts[1]}
	if len(parts) == 3 {
		if !dnsLabel.MatchString(parts[2]) {
			return GatewayRef{}, fmt.Errorf("kubernetes: invalid Gateway sectionName")
		}
		ref.Section = parts[2]
	}
	return ref, nil
}

// PublicRoutes owns only the one orchestrator route object (Ingress or
// HTTPRoute) in an Environment namespace. It does not configure DNS, TLS, the
// Gateway or the controller's external port.
type PublicRoutes struct {
	KubectlPath string
	Credentials execution.KubeconfigSource
	// Kind is RouteKindIngress (default) or RouteKindHTTPRoute.
	Kind string
	// IngressClass of the Ingress; empty keeps "traefik".
	IngressClass string
	// Gateway is the parent of HTTPRoutes (required for RouteKindHTTPRoute).
	Gateway GatewayRef
}

type routeObject struct{ resource, apiVersion, kind string }

var (
	ingressObject   = routeObject{resource: "ingress", apiVersion: "networking.k8s.io/v1", kind: "Ingress"}
	httpRouteObject = routeObject{resource: "httproutes.gateway.networking.k8s.io", apiVersion: "gateway.networking.k8s.io/v1", kind: "HTTPRoute"}
)

func (r *PublicRoutes) kinds() (current, other routeObject) {
	if r.Kind == RouteKindHTTPRoute {
		return httpRouteObject, ingressObject
	}
	return ingressObject, httpRouteObject
}

func (r *PublicRoutes) Reconcile(ctx context.Context, target execution.Target, route execution.PublicRoute) error {
	if target.Namespace == "" || !target.Explicit() || route.ApplicationID == "" || route.EnvironmentID == "" {
		return fmt.Errorf("kubernetes: public route requires scoped target and owner")
	}
	if r.Kind == RouteKindHTTPRoute && r.Gateway.Name == "" {
		return fmt.Errorf("kubernetes: HTTPRoute public routes require a Gateway")
	}
	cli, cleanup, err := OpenCLI(ctx, r.KubectlPath, r.Credentials, target)
	if err != nil {
		return err
	}
	defer cleanup()
	current, other := r.kinds()
	exists, err := ownedRoute(ctx, cli, target.Namespace, current, route)
	if err != nil {
		return err
	}
	// A route of the previously configured kind is removed when owned.
	if previous, err := ownedRoute(ctx, cli, target.Namespace, other, route); err != nil && !errors.Is(err, ErrResourceTypeNotFound) {
		return err
	} else if previous {
		if err := cli.Delete(ctx, target.Namespace, other.resource, publicRouteName); err != nil {
			return err
		}
	}
	if len(route.Paths) == 0 {
		if !exists {
			return nil
		}
		return cli.Delete(ctx, target.Namespace, current.resource, publicRouteName)
	}
	var manifest map[string]any
	if r.Kind == RouteKindHTTPRoute {
		manifest, err = PublicHTTPRoute(target.Namespace, route, r.Gateway)
	} else {
		manifest, err = publicIngress(target.Namespace, route, r.IngressClass)
	}
	if err != nil {
		return err
	}
	return cli.Apply(ctx, []map[string]any{manifest})
}

// ownedRoute reports whether the route object exists; one not owned by this
// Environment fails closed.
func ownedRoute(ctx context.Context, cli CLI, namespace string, kind routeObject, route execution.PublicRoute) (bool, error) {
	object, exists, err := cli.Get(ctx, namespace, kind.resource, publicRouteName)
	if err != nil || !exists {
		return false, err
	}
	metadata, _ := object["metadata"].(map[string]any)
	labels, _ := metadata["labels"].(map[string]any)
	if labels["app.kubernetes.io/managed-by"] != "orchestrator" || labels["orchestrator.io/application"] != route.ApplicationID || labels["orchestrator.io/environment"] != route.EnvironmentID {
		return false, fmt.Errorf("kubernetes: public %s is not owned by Environment %q", kind.kind, route.EnvironmentID)
	}
	return true, nil
}

func routeMetadata(namespace string, route execution.PublicRoute) map[string]any {
	return map[string]any{"name": publicRouteName, "namespace": namespace, "labels": map[string]any{
		"app.kubernetes.io/managed-by": "orchestrator", "orchestrator.io/application": route.ApplicationID,
		"orchestrator.io/environment": route.EnvironmentID,
	}}
}

func sortedPaths(route execution.PublicRoute) ([]execution.PublicPath, error) {
	if !routeHost.MatchString(route.Host) || len(route.Paths) == 0 {
		return nil, fmt.Errorf("kubernetes: invalid public host or Service port")
	}
	paths := append([]execution.PublicPath(nil), route.Paths...)
	sort.Slice(paths, func(i, j int) bool { return paths[i].Path < paths[j].Path })
	for _, path := range paths {
		if path.Path == "" || path.Path[0] != '/' || !routePort.MatchString(path.PortName) || !routePort.MatchString(path.WorkloadID) {
			return nil, fmt.Errorf("kubernetes: invalid public path or Service port")
		}
	}
	return paths, nil
}

// PublicIngress builds a deterministic Environment-owned Ingress for both direct and Fleet adapters.
func PublicIngress(namespace string, route execution.PublicRoute) (map[string]any, error) {
	return publicIngress(namespace, route, "")
}

func publicIngress(namespace string, route execution.PublicRoute, class string) (map[string]any, error) {
	paths, err := sortedPaths(route)
	if err != nil {
		return nil, err
	}
	if class == "" {
		class = "traefik"
	}
	httpPaths := make([]any, 0, len(paths))
	for _, path := range paths {
		httpPaths = append(httpPaths, map[string]any{"path": path.Path, "pathType": "Prefix", "backend": map[string]any{"service": map[string]any{"name": path.WorkloadID, "port": map[string]any{"name": path.PortName}}}})
	}
	return map[string]any{
		"apiVersion": ingressObject.apiVersion, "kind": ingressObject.kind,
		"metadata": routeMetadata(namespace, route),
		"spec": map[string]any{"ingressClassName": class, "rules": []any{map[string]any{
			"host": route.Host, "http": map[string]any{"paths": httpPaths},
		}}},
	}, nil
}

// PublicHTTPRoute builds the Environment-owned Gateway API HTTPRoute: one
// PathPrefix rule per public path to the numeric Service port (ADR-024).
func PublicHTTPRoute(namespace string, route execution.PublicRoute, gateway GatewayRef) (map[string]any, error) {
	paths, err := sortedPaths(route)
	if err != nil {
		return nil, err
	}
	rules := make([]any, 0, len(paths))
	for _, path := range paths {
		if path.Port <= 0 || path.Port > 65535 {
			return nil, fmt.Errorf("kubernetes: public path %s has no numeric Service port", path.Path)
		}
		rules = append(rules, map[string]any{
			"matches":     []any{map[string]any{"path": map[string]any{"type": "PathPrefix", "value": path.Path}}},
			"backendRefs": []any{map[string]any{"name": path.WorkloadID, "port": path.Port}},
		})
	}
	parent := map[string]any{"group": "gateway.networking.k8s.io", "kind": "Gateway", "namespace": gateway.Namespace, "name": gateway.Name}
	if gateway.Section != "" {
		parent["sectionName"] = gateway.Section
	}
	return map[string]any{
		"apiVersion": httpRouteObject.apiVersion, "kind": httpRouteObject.kind,
		"metadata": routeMetadata(namespace, route),
		"spec": map[string]any{
			"parentRefs": []any{parent},
			"hostnames":  []any{route.Host},
			"rules":      rules,
		},
	}, nil
}

var _ execution.PublicRouteManager = (*PublicRoutes)(nil)

func (r *PublicRoutes) RenderRoute(target execution.Target, route execution.PublicRoute) ([]execution.Manifest, error) {
	if target.Namespace == "" || route.ApplicationID == "" || route.EnvironmentID == "" {
		return nil, fmt.Errorf("kubernetes: invalid route identity")
	}
	if len(route.Paths) == 0 {
		return nil, nil
	}
	var object map[string]any
	var err error
	if r.Kind == RouteKindHTTPRoute {
		object, err = PublicHTTPRoute(target.Namespace, route, r.Gateway)
	} else {
		object, err = publicIngress(target.Namespace, route, r.IngressClass)
	}
	if err != nil {
		return nil, err
	}
	current, _ := r.kinds()
	return []execution.Manifest{{APIVersion: current.apiVersion, Kind: current.kind, Name: publicRouteName, Namespace: target.Namespace, Object: object}}, nil
}
