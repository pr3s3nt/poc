package tfexec

import (
	"io/fs"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/adapters/terraform"
	"orchestrator/internal/adapters/terraform/registry"
	"orchestrator/internal/adapters/terraform/registry/registrytest"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/ports/terraformexec"
)

// The packaged Kubernetes modules run through the executor's real variable
// builder with their REAL registry bundles: the executor supplies the
// credential and the stable tags, nothing else may override them.
func k8sBundle(t *testing.T, id string) tfmodule.Bundle {
	t.Helper()
	svc := registrytest.Open(t, store.New())
	refs, err := registry.PackagedRefs()
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.Bundle(t.Context(), refs[id])
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestKubernetesModuleVariablesExecutorOwnership(t *testing.T) {
	ev := executorValues{Tags: map[string]string{"orchestrator-instance": "0123456789abcdef"}, Secrets: map[string]string{"password": "stable-secret-value"}}
	redis := k8sBundle(t, "redis-k8s")
	fault := "redis:7.4.7-alpine@sha256:095bac20aafb160ab173e6fed571218d4feba5644f7189f3480fd1b07b52ca03"
	vars, err := buildVariables(redis, map[string]any{"namespace": "orch-v2-test-run1", "image": fault, "annotations": map[string]any{"example.test/k": "v"}}, ev)
	if err != nil {
		t.Fatal(err)
	}
	if vars.All["password"] != "stable-secret-value" || vars.All["image"] != fault || !reflect.DeepEqual(vars.SecretNames, []string{"password"}) {
		t.Fatalf("variables %v secrets %v", vars.All, vars.SecretNames)
	}
	if _, leaked := vars.Public["password"]; leaked {
		t.Fatal("the password reached the public snapshot")
	}
	if vars.Public["tags"] == nil || vars.Public["namespace"] != "orch-v2-test-run1" {
		t.Fatalf("public %v", vars.Public)
	}
	// No other source may supply or override executor-owned values or declare
	// unknown/missing/wrong-typed inputs; errors never echo values.
	for name, inputs := range map[string]map[string]any{
		"password param":   {"namespace": "ns", "password": "chosen-by-workload"},
		"tags override":    {"namespace": "ns", "tags": map[string]any{"orchestrator-instance": "x"}},
		"missing required": {},
		"unknown":          {"namespace": "ns", "nodeSelector": "x"},
		"wrong type":       {"namespace": 5},
		"wrong map type":   {"namespace": "ns", "labels": map[string]any{"k": 1}},
	} {
		_, err := buildVariables(redis, inputs, ev)
		if err == nil || strings.Contains(err.Error(), "chosen-by-workload") || strings.Contains(err.Error(), "stable-secret-value") {
			t.Errorf("%s: %v", name, err)
		}
	}
	// A missing executor secret stops before any provider call.
	if _, err := buildVariables(redis, map[string]any{"namespace": "ns"}, executorValues{Tags: ev.Tags}); err == nil {
		t.Fatal("variables without the executor secret were accepted")
	}

	pg := k8sBundle(t, "postgres-k8s")
	if _, err := buildVariables(pg, map[string]any{"namespace": "ns", "database": "acceptance"}, ev); err == nil {
		t.Fatal("PostgreSQL without a username param was accepted")
	}
	if v, err := buildVariables(pg, map[string]any{"namespace": "ns", "database": "acceptance", "username": "app"}, ev); err != nil || v.All["password"] != "stable-secret-value" {
		t.Fatalf("postgres variables %v", err)
	}

	ns := k8sBundle(t, "k8s-namespace")
	if v, err := buildVariables(ns, map[string]any{"name": "orch-v2-test-run1"}, ev); err != nil || len(v.SecretNames) != 0 || v.All["name"] != "orch-v2-test-run1" {
		t.Fatalf("namespace variables %v %v", v.All, err)
	}
}

var moduleResources = regexp.MustCompile(`(?m)^resource "([a-z0-9_]+)"`)

// Every object type the Redis/PostgreSQL modules create is one the namespace
// ownership policy accounts for by state UID, and none is a namespace: a
// module never owns the namespace that holds it.
func TestKubernetesModuleObjectsAreStateOwnedChildren(t *testing.T) {
	for _, id := range []string{"redis-k8s", "postgres-k8s"} {
		src, err := fs.ReadFile(terraform.PackagedModules(), id+"/main.tf")
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range moduleResources.FindAllStringSubmatch(string(src), -1) {
			if namespaceTypes[m[1]] {
				t.Errorf("%s manages a namespace", id)
			}
			if _, ok := ownKinds[m[1]]; !ok {
				t.Errorf("%s creates %s, which the destroy policy does not account for", id, m[1])
			}
		}
	}
}

// Cleanup safety: with the Redis/PostgreSQL instances still present, the
// namespace instance's state does not own them, so destroying the namespace
// is refused; once their own states are destroyed (children first) every
// remaining object is controller-generated and accounted.
func TestNamespaceDestroyNeverBypassesStateOwnedChildren(t *testing.T) {
	nsState := []terraformexec.StateResource{{Address: "kubernetes_namespace_v1.this", Type: "kubernetes_namespace_v1", ID: "orch-v2-test-run1", UID: "uid-ns"}}
	redisObjects := []terraformexec.NamespaceObject{
		obj("StatefulSet", "redis-0123456789abcdef", "uid-sts"),
		obj("Service", "redis-0123456789abcdef", "uid-svc"),
		obj("Secret", "redis-0123456789abcdef-auth", "uid-sec"),
		obj("PersistentVolumeClaim", "redis-0123456789abcdef-data", "uid-pvc"),
		obj("Pod", "redis-0123456789abcdef-0", "uid-pod", owner("StatefulSet", "redis-0123456789abcdef", "uid-sts")),
	}
	foreign, ours := judgeNamespace(stateNamespace{Name: "orch-v2-test-run1", UID: "uid-ns"}, terraformexec.NamespaceInspection{UID: "uid-ns", Objects: redisObjects}, nsState)
	if !ours || len(foreign) != 5 {
		t.Fatalf("a namespace destroy with live children: foreign %v", foreign)
	}
	// The same objects under the Redis instance's own state are all accounted.
	redisState := []terraformexec.StateResource{
		{Type: "kubernetes_stateful_set_v1", ID: "orch-v2-test-run1/redis-0123456789abcdef", UID: "uid-sts"},
		{Type: "kubernetes_service_v1", ID: "orch-v2-test-run1/redis-0123456789abcdef", UID: "uid-svc"},
		{Type: "kubernetes_secret_v1", ID: "orch-v2-test-run1/redis-0123456789abcdef-auth", UID: "uid-sec"},
		{Type: "kubernetes_persistent_volume_claim_v1", ID: "orch-v2-test-run1/redis-0123456789abcdef-data", UID: "uid-pvc"},
	}
	foreign, _ = judgeNamespace(stateNamespace{Name: "orch-v2-test-run1", UID: "uid-ns"}, terraformexec.NamespaceInspection{UID: "uid-ns", Objects: append(redisObjects,
		generated(obj("Endpoints", "redis-0123456789abcdef", "uid-ep")),
		obj("EndpointSlice", "redis-0123456789abcdef-x", "uid-es", owner("Service", "redis-0123456789abcdef", "uid-svc")),
		obj("ControllerRevision", "redis-0123456789abcdef-1", "uid-cr", owner("StatefulSet", "redis-0123456789abcdef", "uid-sts")))}, redisState)
	if len(foreign) != 0 {
		t.Fatalf("module objects are not accounted under their own state: %v", foreign)
	}
}
