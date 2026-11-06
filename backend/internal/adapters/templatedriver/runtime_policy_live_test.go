package templatedriver

import (
	"context"
	"encoding/json"
	"fmt"
	"orchestrator/internal/adapters/kubernetes"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/renderengine"
	"os"
	"strings"
	"testing"
	"time"
)

// Real adapter acceptance; opt-in is limited to the recorded task-owned kind.
func TestRuntimeDefaultsOnOwnedKind(t *testing.T) {
	kubeconfig := os.Getenv("ORCH_ISSUE37_KUBECONFIG")
	if kubeconfig == "" {
		t.Skip("ORCH_ISSUE37_KUBECONFIG is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 9*time.Minute)
	defer cancel()
	cli := kubernetes.NewCLI("kubectl", "", kubeconfig)
	contextName, err := cli.Run(ctx, nil, "config", "current-context")
	if err != nil || !strings.HasPrefix(strings.TrimSpace(string(contextName)), "kind-orch-reconciliation-issue37-") {
		t.Fatal("requires dedicated issue37 kind")
	}
	namespace := "issue37-" + time.Now().Format("20060102-150405")
	ns := map[string]any{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]any{"name": namespace, "labels": map[string]string{"orchestrator.io/test-run": "issue37"}}}
	raw, _ := json.Marshal(ns)
	if _, err := cli.Run(ctx, raw, "create", "-f", "-"); err != nil {
		t.Fatal(err)
	}
	original, found, err := cli.Get(ctx, "", "namespace", namespace)
	if err != nil || !found {
		t.Fatal("missing owned namespace")
	}
	uid := object(original["metadata"])["uid"]
	t.Cleanup(func() {
		if t.Failed() {
			t.Log("retained failed run namespace for diagnosis:", namespace, "uid", uid)
			return
		}
		cleanCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		latest, exists, err := cli.Get(cleanCtx, "", "namespace", namespace)
		if err != nil || !exists || object(latest["metadata"])["uid"] != uid {
			t.Error("namespace identity changed; cleanup refused")
			return
		}
		deletion, _ := json.Marshal(map[string]any{"apiVersion": "v1", "kind": "DeleteOptions", "preconditions": map[string]any{"uid": uid}})
		if _, err := cli.Run(cleanCtx, deletion, "delete", "--raw", "/api/v1/namespaces/"+namespace, "-f", "-"); err != nil {
			t.Error(err)
		}
	})
	t.Log("namespace", namespace, "uid", uid)
	r, pins := gitRenderer(t, map[string]map[string][]byte{"standard": readBundle(t, standardDir)})
	bundle, err := renderengine.Validate(readBundle(t, standardDir))
	if err != nil {
		t.Fatal(err)
	}
	var nginx environment.TemplatePin
	for _, template := range bundle.Contract.Manifest.WorkloadTemplates {
		if template.ID == "nginx-readonly" {
			nginx = environment.TemplatePin{ID: template.ID, Version: template.Version, SHA256: template.SHA256()}
		}
	}
	makeReq := func(name string, spec environment.ModuleSpec) execution.RenderRequest {
		return execution.RenderRequest{Selection: pins["standard"], WorkloadID: name, Namespace: namespace, Module: environment.Module{Profile: environment.ModuleProfile, Spec: spec}, Labels: map[string]string{"orchestrator.io/application": "issue37", "orchestrator.io/environment": "staging", "orchestrator.io/deployment-id": "issue37-run"}}
	}
	target := execution.Target{Namespace: namespace, Kubeconfig: kubeconfig, Extra: map[string]string{"application": "issue37", "environment": "staging", "deployment": "issue37-run"}}
	deployer := kubernetes.NewDeployer("kubectl")
	deployer.Timeout = 3 * time.Minute
	apply := func(req execution.RenderRequest) []execution.Manifest {
		t.Helper()
		manifests, err := r.Render(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		if err := deployer.Apply(ctx, target, manifests); err != nil {
			t.Fatal(err)
		}
		return manifests
	}
	wait := func(kind, name string) {
		t.Helper()
		if err := deployer.WaitReady(ctx, target, []execution.WorkloadRef{{Kind: kind, Name: name, Namespace: namespace}}); err != nil {
			t.Fatal(err)
		}
	}
	two := 2
	web := makeReq("web", environment.ModuleSpec{Replicas: &two, Containers: map[string]environment.Container{"main": {}}, Execution: &environment.WorkloadExecution{Version: 1, Kind: "Deployment", Template: nginx, Defaults: &environment.DefaultsPolicy{Version: "workload-defaults-v1", Environment: "staging"}, PodDisruptionBudget: &environment.PodDisruptionBudget{Enabled: true, MaxUnavailable: 1, UnhealthyPodEvictionPolicy: "AlwaysAllow"}}})
	apply(web)
	wait("Deployment", "web")
	result, err := cli.Run(ctx, nil, "-n", namespace, "exec", "deployment/web", "--", "sh", "-c", "id -u; echo issue37-cache >/var/cache/nginx/issue37; echo issue37-tmp >/tmp/issue37; wget -q -O - http://127.0.0.1:8080/")
	if err != nil || !strings.Contains(string(result), "Welcome to nginx") {
		t.Fatal("read-only Nginx runtime failed", err, string(result))
	}
	if strings.HasPrefix(string(result), "0\n") {
		t.Fatal("profile ran as root")
	}
	pdb, found, err := cli.Get(ctx, namespace, "poddisruptionbudget", "web")
	if err != nil || !found || object(pdb["spec"])["maxUnavailable"] != float64(1) {
		t.Fatal("PDB missing", err)
	}
	// Disable and prune exactly the previous inventory object through the product adapter.
	web.Module.Spec.Execution.PodDisruptionBudget = &environment.PodDisruptionBudget{Enabled: false}
	apply(web)
	if err := deployer.RemoveObjects(ctx, target, "web", []execution.ObjectRef{{APIVersion: "policy/v1", Kind: "PodDisruptionBudget", Name: "web", Owners: []string{"issue37-run"}}}); err != nil {
		t.Fatal(err)
	}
	if _, found, err := cli.Get(ctx, namespace, "poddisruptionbudget", "web"); err != nil || found {
		t.Fatal("PDB was not pruned")
	}
	// An independently owned shared PVC is reused by two workload consumers.
	// Setup creates the source; rendering must only reference it, never clone it.
	shared := map[string]any{"apiVersion": "v1", "kind": "PersistentVolumeClaim", "metadata": map[string]any{"name": "shared-data", "namespace": namespace, "labels": map[string]string{"orchestrator.io/test-run": "issue37"}}, "spec": map[string]any{"accessModes": []string{"ReadWriteOnce"}, "resources": map[string]any{"requests": map[string]string{"storage": "32Mi"}}}}
	sharedRaw, _ := json.Marshal(shared)
	if _, err := cli.Run(ctx, sharedRaw, "create", "-f", "-"); err != nil {
		t.Fatal(err)
	}
	sharedObject, found, err := cli.Get(ctx, namespace, "pvc", "shared-data")
	if err != nil || !found {
		t.Fatal("shared source missing", err)
	}
	sharedUID := object(sharedObject["metadata"])["uid"]
	for i, name := range []string{"consumer-a", "consumer-b"} {
		consumer := makeReq(name, environment.ModuleSpec{Containers: map[string]environment.Container{"main": {Image: "busybox:1.36", Command: []string{"sh", "-c"}, Args: []string{"sleep 3600"}}}, Execution: &environment.WorkloadExecution{Version: 1, Kind: "Deployment", Defaults: &environment.DefaultsPolicy{Version: "workload-defaults-v1", Environment: "staging"}, Mounts: []environment.StorageMount{{Resource: "data", Container: "main", Path: "/data"}}}})
		consumer.Storage = map[string]execution.StorageBinding{"data": {ClaimName: "shared-data", Namespace: namespace}}
		manifests := apply(consumer)
		for _, m := range manifests {
			if m.Kind == "PersistentVolumeClaim" {
				t.Fatal("consumer cloned shared PVC")
			}
		}
		wait("Deployment", name)
		command := []string{"-n", namespace, "exec", "deployment/" + name, "--", "sh", "-c", "cat /data/proof"}
		if i == 0 {
			command[len(command)-1] = "echo shared-issue37 >/data/proof; cat /data/proof"
		}
		proof, err := cli.Run(ctx, nil, command...)
		if err != nil || strings.TrimSpace(string(proof)) != "shared-issue37" {
			t.Fatal("shared data reuse failed", err)
		}
		refs := []execution.ObjectRef{}
		for _, m := range manifests {
			refs = append(refs, execution.ObjectRef{APIVersion: m.APIVersion, Kind: m.Kind, Name: m.Name, Owners: []string{"issue37-run"}})
		}
		if err := deployer.RemoveObjects(ctx, target, name, refs); err != nil {
			t.Fatal(err)
		}
		latest, exists, err := cli.Get(ctx, namespace, "pvc", "shared-data")
		if err != nil || !exists || object(latest["metadata"])["uid"] != sharedUID {
			t.Fatal("consumer removal changed source identity", err)
		}
	}
	sharedDelete, _ := json.Marshal(map[string]any{"apiVersion": "v1", "kind": "DeleteOptions", "preconditions": map[string]any{"uid": sharedUID}})
	if _, err := cli.Run(ctx, sharedDelete, "delete", "--raw", "/api/v1/namespaces/"+namespace+"/persistentvolumeclaims/shared-data", "-f", "-"); err != nil {
		t.Fatal(err)
	}
	one := 1
	uid1000 := int64(1000)
	yes := true
	stateful := makeReq("stateful", environment.ModuleSpec{Replicas: &one, Containers: map[string]environment.Container{"main": {Image: "busybox:1.36", Command: []string{"sh", "-c"}, Args: []string{"sleep 3600"}, SecurityContext: &environment.ContainerSecurityContext{RunAsNonRoot: &yes, RunAsUser: &uid1000, ReadOnlyRootFilesystem: &yes}}}, Execution: &environment.WorkloadExecution{Version: 1, Kind: "StatefulSet", Defaults: &environment.DefaultsPolicy{Version: "workload-defaults-v1", Environment: "staging"}, PodSecurityContext: &environment.PodSecurityContext{FSGroup: &uid1000}, VolumeClaimTemplates: []environment.ClaimTemplate{{Name: "data", Size: "32Mi", AccessModes: []string{"ReadWriteOnce"}}}, ClaimMounts: []environment.ClaimMount{{Claim: "data", Container: "main", Path: "/data"}}}})
	objects := apply(stateful)
	wait("StatefulSet", "stateful")
	claim, found, err := cli.Get(ctx, namespace, "pvc", "data-stateful-0")
	if err != nil || !found {
		t.Fatal("claim missing", err)
	}
	claimUID := object(claim["metadata"])["uid"]
	if _, err := cli.Run(ctx, nil, "-n", namespace, "exec", "stateful-0", "--", "sh", "-c", "echo retained-issue37 >/data/proof"); err != nil {
		t.Fatal(err)
	}
	// Operational scale-to-zero proves retention; product authoring still permits 1 only.
	if _, err := cli.Run(ctx, nil, "-n", namespace, "scale", "statefulset/stateful", "--replicas=0"); err != nil {
		t.Fatal(err)
	}
	if _, err := cli.Run(ctx, nil, "-n", namespace, "wait", "--for=delete", "pod/stateful-0", "--timeout=120s"); err != nil {
		t.Fatal(err)
	}
	claim, found, err = cli.Get(ctx, namespace, "pvc", "data-stateful-0")
	if err != nil || !found || object(claim["metadata"])["uid"] != claimUID {
		t.Fatal("scale down changed claim identity")
	}
	apply(stateful)
	wait("StatefulSet", "stateful")
	proof, err := cli.Run(ctx, nil, "-n", namespace, "exec", "stateful-0", "--", "cat", "/data/proof")
	if err != nil || strings.TrimSpace(string(proof)) != "retained-issue37" {
		t.Fatal("retained data lost", err)
	}
	refs := []execution.ObjectRef{}
	for _, m := range objects {
		refs = append(refs, execution.ObjectRef{APIVersion: m.APIVersion, Kind: m.Kind, Name: m.Name, Owners: []string{"issue37-run"}})
	}
	if err := deployer.RemoveObjects(ctx, target, "stateful", refs); err != nil {
		t.Fatal(err)
	}
	claim, found, err = cli.Get(ctx, namespace, "pvc", "data-stateful-0")
	if err != nil || !found || object(claim["metadata"])["uid"] != claimUID {
		t.Fatal("workload deletion removed retained claim")
	}
	// Explicit owned data cleanup, not workload removal. Preserve exact incarnation.
	deletion, _ := json.Marshal(map[string]any{"apiVersion": "v1", "kind": "DeleteOptions", "preconditions": map[string]any{"uid": claimUID}})
	if _, err := cli.Run(ctx, deletion, "delete", "--raw", fmt.Sprintf("/api/v1/namespaces/%s/persistentvolumeclaims/data-stateful-0", namespace), "-f", "-"); err != nil {
		t.Fatal(err)
	}
	t.Log("PASS: real Nginx read-only/non-root/two emptyDirs, PDB enable/prune, shared PVC UID/data reuse, claim UID and data retention after scale down/workload removal")
}
