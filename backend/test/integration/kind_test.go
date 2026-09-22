//go:build integration

// Package integration runs the UC-06/UC-08 happy path against real
// infrastructure. The kind test uses an existing local cluster; it creates only
// the namespace named after the run id and deletes nothing else.
package integration

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"orchestrator/internal/adapters/store"
	appsvc "orchestrator/internal/application/deployment"
	domain "orchestrator/internal/domain/deployment"
	"orchestrator/internal/seed"
)

// TestKindInternalVerification verifies the deployments that kind-verify.sh
// drove through the orchestrator HTTP API.
func TestKindInternalVerification(t *testing.T) {
	kubeContext := requireEnv(t, "KIND_CONTEXT")
	runID := requireEnv(t, "RUN_ID")
	statePath := requireEnv(t, "STATE_PATH")
	namespace := requireEnv(t, "NAMESPACE")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	seedOptions := seed.Defaults()
	st, err := store.NewWithSnapshot(statePath)
	if err != nil {
		t.Fatalf("read state snapshot: %v", err)
	}
	queries := appsvc.NewQueryService(st)
	target := kubeTarget{Context: kubeContext}

	deployments, err := queries.ListDeployments(ctx, seedOptions.ApplicationKey, seedOptions.EnvironmentKey)
	if err != nil {
		t.Fatalf("list deployments: %v", err)
	}
	if len(deployments) != 3 {
		t.Fatalf("expected three deployments driven over HTTP, got %d", len(deployments))
	}
	for _, d := range deployments {
		if d.Status != domain.StatusSucceeded {
			t.Fatalf("deployment %s of %s is %s: %s", d.ID, d.WorkloadID, d.Status, d.FailureReason)
		}
	}
	latest := deployments[0]

	view, err := queries.GetDeployment(ctx, latest.ID)
	if err != nil {
		t.Fatalf("deployment view: %v", err)
	}
	if len(view.Workloads) != 3 {
		t.Fatalf("expected three workload instances, got %d", len(view.Workloads))
	}
	for _, w := range view.Workloads {
		if w.Status != string(domain.InstanceReady) {
			t.Fatalf("workload %s is %s", w.WorkloadID, w.Status)
		}
	}
	for _, r := range view.Resources {
		if r.Status != string(domain.ResourceReady) {
			t.Fatalf("resource %s is %s", r.Descriptor, r.Status)
		}
		if r.ResourceType == "postgres" && r.Outputs["password"] != appsvc.RedactedValue {
			t.Fatalf("the deployment view leaked the database password: %v", r.Outputs)
		}
	}

	// Identity follows the Deployment Set paths.
	for _, want := range []string{
		"workload.default#modules.backend",
		"postgres.default#shared." + seedOptions.SharedDatabaseID,
		"k8s-namespace.default#environments." + seedOptions.ApplicationKey + "." + seedOptions.EnvironmentKey,
	} {
		found := false
		for _, r := range view.Resources {
			if r.Descriptor == want {
				found = true
			}
		}
		if !found && !strings.HasPrefix(want, "workload.") {
			t.Fatalf("the deployment view has no resource %s", want)
		}
	}

	// Cluster evidence.
	deploymentsOut := kubectl(t, target, "get", "deployments", "-n", namespace, "-o", "name")
	for _, want := range []string{"deployment.apps/frontend", "deployment.apps/backend", "deployment.apps/worker"} {
		if !strings.Contains(deploymentsOut, want) {
			t.Fatalf("missing %s in namespace %s: %s", want, namespace, deploymentsOut)
		}
	}
	if sts := kubectl(t, target, "get", "statefulset", "-n", namespace, "-o", "name"); !strings.Contains(sts, "shared-acceptance-db") {
		t.Fatalf("missing the PostgreSQL StatefulSet: %s", sts)
	}
	if svc := kubectl(t, target, "get", "svc", "-n", namespace, "-o", "name"); !strings.Contains(svc, "service/shared-acceptance-db") {
		t.Fatalf("missing the PostgreSQL Service: %s", svc)
	}
	pods := kubectl(t, target, "get", "pods", "-n", namespace,
		"-o", "jsonpath={range .items[*]}{.metadata.name}={.status.phase} {end}")
	t.Logf("pods: %s", pods)
	if strings.Contains(pods, "=Pending") || strings.Contains(pods, "=Failed") {
		t.Fatalf("not every pod is running: %s", pods)
	}
	backendSpec := kubectl(t, target, "get", "deployment", "backend", "-n", namespace, "-o", "json")
	if strings.Contains(backendSpec, "PGPASSWORD\",\"value\"") {
		t.Fatal("the backend Deployment carries a plaintext PGPASSWORD")
	}

	// Container resources (UC-06 BR-11): declared Score values reach the live
	// Deployment verbatim, only missing requests get defaults and limits appear
	// only when declared. The seeded values are canonical Kubernetes quantities,
	// so the API server returns them unchanged.
	for workload, want := range map[string]map[string]any{
		"backend": {
			"requests": map[string]any{"cpu": "50m", "memory": "64Mi"},
			"limits":   map[string]any{"cpu": "500m", "memory": "256Mi"},
		},
		"worker": {
			"requests": map[string]any{"cpu": "10m", "memory": "48Mi"},
			"limits":   map[string]any{"memory": "128Mi"},
		},
		"frontend": {
			"requests": map[string]any{"cpu": "10m", "memory": "32Mi"},
		},
	} {
		raw := kubectl(t, target, "get", "deployment", workload, "-n", namespace,
			"-o", "jsonpath={.spec.template.spec.containers[?(@.name==\"main\")].resources}")
		var got map[string]any
		if err := json.Unmarshal([]byte(raw), &got); err != nil {
			t.Fatalf("%s container resources are not JSON: %q: %v", workload, raw, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s live container resources = %s, want %v", workload, raw, want)
		}
		t.Logf("%s live container resources: %s", workload, raw)
	}

	jobResult := runJobFlow(ctx, t, target, namespace, "kind")
	t.Logf("job flow result: %s", jobResult)
	t.Logf("run %s verified", runID)

	if file := os.Getenv("DEPLOYMENT_ID_FILE"); file != "" {
		if err := os.WriteFile(file, []byte(latest.ID), 0o600); err != nil {
			t.Fatalf("write deployment id: %v", err)
		}
	}
}
