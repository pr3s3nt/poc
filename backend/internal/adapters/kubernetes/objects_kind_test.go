package kubernetes

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"orchestrator/internal/ports/execution"
)

// TestOpenObjectLifecycleOnKind exercises ADR-024 against a real cluster:
// discovery and ownership checks before apply, Job-then-rollout readiness,
// inventory removal by UID and the Gateway API public route. It runs only
// with ORCHESTRATOR_KIND_KUBECONFIG naming a disposable cluster that serves
// Gateway API and has a Gateway ORCHESTRATOR_KIND_GATEWAY (namespace/name).
func TestOpenObjectLifecycleOnKind(t *testing.T) {
	kubeconfig := os.Getenv("ORCHESTRATOR_KIND_KUBECONFIG")
	if kubeconfig == "" {
		t.Skip("ORCHESTRATOR_KIND_KUBECONFIG is not set")
	}
	gateway, err := ParseGatewayRef(os.Getenv("ORCHESTRATOR_KIND_GATEWAY"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	namespace := "adr024-" + time.Now().Format("150405")
	cli := NewCLI("kubectl", "", kubeconfig)
	if _, err := cli.Run(ctx, []byte(`{"apiVersion":"v1","kind":"Namespace","metadata":{"name":"`+namespace+`"}}`), "apply", "-f", "-"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = cli.Run(context.Background(), nil, "delete", "namespace", namespace, "--wait=false") })
	target := execution.Target{Namespace: namespace, Kubeconfig: kubeconfig, Extra: map[string]string{"application": "shop", "environment": "staging", "deployment": "d1"}}
	labels := func(workload, deployment string) map[string]any {
		return map[string]any{"app.kubernetes.io/managed-by": "orchestrator", "orchestrator.io/application": "shop", "orchestrator.io/environment": "staging",
			"orchestrator.io/deployment-id": deployment, "orchestrator.io/workload": workload, "app.kubernetes.io/name": workload}
	}
	object := func(api, kind, name string, spec map[string]any, extra map[string]any) execution.Manifest {
		o := map[string]any{"apiVersion": api, "kind": kind, "metadata": map[string]any{"name": name, "namespace": namespace, "labels": labels("web", "d1")}}
		for k, v := range spec {
			o[k] = v
		}
		for k, v := range extra {
			o["metadata"].(map[string]any)[k] = v
		}
		return execution.Manifest{APIVersion: api, Kind: kind, Name: name, Namespace: namespace, Object: o}
	}
	pod := map[string]any{"containers": []any{map[string]any{"name": "main", "image": "registry.k8s.io/pause:3.10", "imagePullPolicy": "IfNotPresent"}}}
	selector := map[string]any{"app.kubernetes.io/name": "web"}
	manifests := []execution.Manifest{
		object("v1", "ConfigMap", "web-files", map[string]any{"data": map[string]any{"app.yaml": "x: 1"}}, nil),
		object("batch/v1", "Job", "web-migrate-d1", map[string]any{"spec": map[string]any{"backoffLimit": 0, "template": map[string]any{"spec": map[string]any{
			"restartPolicy": "Never", "containers": []any{map[string]any{"name": "migrate", "image": "busybox:1.36", "command": []any{"true"}}}}}}}, nil),
		object("apps/v1", "Deployment", "web", map[string]any{"spec": map[string]any{"replicas": 1, "selector": map[string]any{"matchLabels": selector},
			"template": map[string]any{"metadata": map[string]any{"labels": selector}, "spec": pod}}}, nil),
		object("autoscaling/v2", "HorizontalPodAutoscaler", "web", map[string]any{"spec": map[string]any{"minReplicas": 1, "maxReplicas": 3,
			"scaleTargetRef": map[string]any{"apiVersion": "apps/v1", "kind": "Deployment", "name": "web"}}}, nil),
		object("v1", "Service", "web", map[string]any{"spec": map[string]any{"selector": selector, "ports": []any{map[string]any{"name": "http", "port": 80}}}}, nil),
	}
	d := NewDeployer("kubectl")
	d.Timeout = 3 * time.Minute

	// A cluster-scoped kind is refused before any write.
	clusterRole := object("rbac.authorization.k8s.io/v1", "ClusterRole", "adr024-"+namespace, nil, nil)
	if err := d.Apply(ctx, target, []execution.Manifest{clusterRole}); !errors.Is(err, ErrUnsupportedObject) {
		t.Fatalf("cluster-scoped object: %v", err)
	}
	// A pre-existing object of the same name not created by the platform is never overwritten.
	if _, err := cli.Run(ctx, []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"foreign","namespace":"`+namespace+`"},"data":{"k":"v"}}`), "apply", "-f", "-"); err != nil {
		t.Fatal(err)
	}
	foreign := object("v1", "ConfigMap", "foreign", map[string]any{"data": map[string]any{"k": "overwritten"}}, nil)
	if err := d.Apply(ctx, target, []execution.Manifest{foreign}); err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("foreign object: %v", err)
	}

	// Jobs first, then rollout objects, as the deployment service orders them.
	if err := d.Apply(ctx, target, []execution.Manifest{manifests[0], manifests[1], manifests[3], manifests[4]}); err != nil {
		t.Fatal(err)
	}
	if err := d.WaitReady(ctx, target, []execution.WorkloadRef{{Kind: "Job", Name: "web-migrate-d1"}}); err != nil {
		t.Fatal(err)
	}
	if err := d.Apply(ctx, target, manifests[2:3]); err != nil {
		t.Fatal(err)
	}
	if err := d.WaitReady(ctx, target, []execution.WorkloadRef{{Kind: "Deployment", Name: "web"}}); err != nil {
		t.Fatal(err)
	}
	// Re-applying owned objects passes the ownership check.
	if err := d.Apply(ctx, target, manifests); err != nil {
		t.Fatal(err)
	}

	routes := &PublicRoutes{KubectlPath: "kubectl", Kind: RouteKindHTTPRoute, Gateway: gateway}
	route := execution.PublicRoute{ApplicationID: "shop", EnvironmentID: "staging", Host: "staging.shop.kind.test", Paths: []execution.PublicPath{{Path: "/", WorkloadID: "web", PortName: "http", Port: 80}}}
	if err := routes.Reconcile(ctx, target, route); err != nil {
		t.Fatal(err)
	}
	accepted := false
	for deadline := time.Now().Add(90 * time.Second); time.Now().Before(deadline) && !accepted; time.Sleep(2 * time.Second) {
		object, found, err := cli.Get(ctx, namespace, httpRouteObject.resource, publicRouteName)
		if err != nil || !found {
			continue
		}
		status, _ := object["status"].(map[string]any)
		for _, parent := range asList(status["parents"]) {
			for _, condition := range asList(parent.(map[string]any)["conditions"]) {
				c := condition.(map[string]any)
				accepted = accepted || (c["type"] == "Accepted" && c["status"] == "True")
			}
		}
	}
	if !accepted {
		t.Fatal("the Gateway did not accept the platform HTTPRoute")
	}

	// Inventory removal: ownership + UID, every object gone; the foreign object stays.
	var refs []execution.ObjectRef
	for _, m := range manifests {
		refs = append(refs, execution.ObjectRef{APIVersion: m.APIVersion, Kind: m.Kind, Name: m.Name, Owners: []string{"d0", "d1"}})
	}
	if err := d.RemoveObjects(ctx, target, "web", []execution.ObjectRef{{APIVersion: "v1", Kind: "ConfigMap", Name: "foreign", Owners: []string{"d1"}}}); err == nil {
		t.Fatal("an unowned inventory entry must not be deleted")
	}
	if err := d.RemoveObjects(ctx, target, "web", refs); err != nil {
		t.Fatal(err)
	}
	disc := newDiscovery(cli)
	for _, ref := range refs {
		if _, found, err := disc.get(ctx, namespace, ref.APIVersion, ref.Kind, ref.Name); err != nil || found {
			t.Fatalf("%s/%s still present (err %v)", ref.Kind, ref.Name, err)
		}
	}
	if _, found, err := cli.Get(ctx, namespace, "configmap", "foreign"); err != nil || !found {
		t.Fatalf("foreign object must stay: %v", err)
	}
	if err := routes.Reconcile(ctx, target, execution.PublicRoute{ApplicationID: "shop", EnvironmentID: "staging", Host: route.Host}); err != nil {
		t.Fatal(err)
	}
	if _, found, err := cli.Get(ctx, namespace, httpRouteObject.resource, publicRouteName); err != nil || found {
		t.Fatalf("an empty route set removes the HTTPRoute: %v", err)
	}
}

func asList(v any) []any { l, _ := v.([]any); return l }
