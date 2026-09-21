//go:build integration

package integration

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"orchestrator/internal/adapters/store"
	appsvc "orchestrator/internal/application/deployment"
	domain "orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning"
	"orchestrator/internal/seed"
)

// TestAWSCloudVerification verifies the cloud happy path that aws-verify.sh
// drove through the running orchestrator binary over HTTP. It reads the state
// snapshot the binary wrote, so it checks the same path a Web Console user
// takes: console -> HTTP API -> DeploymentService -> Terraform -> AWS.
func TestAWSCloudVerification(t *testing.T) {
	runID := requireEnv(t, "RUN_ID")
	statePath := requireEnv(t, "STATE_PATH")
	namespace := requireEnv(t, "NAMESPACE")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	seedOptions := seed.Defaults()
	st, err := store.NewWithSnapshot(statePath)
	if err != nil {
		t.Fatalf("read state snapshot: %v", err)
	}
	queries := appsvc.NewQueryService(st)

	deployments, err := queries.ListDeployments(ctx, seedOptions.CloudApplicationKey, seedOptions.EnvironmentKey)
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
		if d.ExecutionProfile != "aws-eks" {
			t.Fatalf("deployment %s ran with profile %q", d.ID, d.ExecutionProfile)
		}
	}
	latest := deployments[0]

	clusterDescriptor, err := planning.EKSDescriptor(seedOptions.CloudApplicationKey)
	if err != nil {
		t.Fatalf("descriptor: %v", err)
	}
	clusterResource, err := st.FindByLogicalIdentity(ctx, seedOptions.OrganizationKey, clusterDescriptor,
		resource.Scope{Type: resource.ScopeApplication, ID: seedOptions.CloudApplicationKey})
	if err != nil {
		t.Fatalf("eks active resource: %v", err)
	}
	kubeconfig, _ := clusterResource.ExecutorState["kubeconfig"].(string)
	clusterName, _ := clusterResource.ExecutorState["clusterName"].(string)
	if kubeconfig == "" {
		t.Fatal("the EKS executor state has no kubeconfig")
	}
	target := kubeTarget{Kubeconfig: kubeconfig, Context: clusterName}

	view, err := queries.GetDeployment(ctx, latest.ID)
	if err != nil {
		t.Fatalf("deployment view: %v", err)
	}
	if len(view.Workloads) != 3 {
		t.Fatalf("expected three workload instances, got %d", len(view.Workloads))
	}
	kinds := map[string]bool{}
	for _, r := range view.Resources {
		kinds[r.ResourceType] = true
		if r.Status != string(domain.ResourceReady) {
			t.Fatalf("resource %s is %s", r.Descriptor, r.Status)
		}
		if r.ResourceType == "postgres" {
			if r.Outputs["password"] != appsvc.RedactedValue {
				t.Fatalf("the deployment view leaked the Aurora password: %v", r.Outputs)
			}
			if host, _ := r.Outputs["host"].(string); !strings.Contains(host, "rds.amazonaws.com") {
				t.Fatalf("postgres host %q is not an Aurora endpoint", host)
			}
		}
	}
	for _, want := range []string{"vpc", "k8s-cluster", "k8s-namespace", "postgres"} {
		if !kinds[want] {
			t.Fatalf("the cloud plan is missing a %s resource: %v", want, kinds)
		}
	}
	// Identity follows the Deployment Set paths.
	if _, ok := view.Graph.(map[string]any); !ok {
		t.Fatal("the view must carry the persisted graph")
	}
	if _, err := st.FindByLogicalIdentity(ctx, seedOptions.OrganizationKey,
		mustDescriptor(t, "postgres", "default", "shared."+seedOptions.SharedDatabaseID),
		resource.Scope{Type: resource.ScopeShared, ID: seedOptions.CloudApplicationKey + "." + seedOptions.EnvironmentKey},
	); err != nil {
		t.Fatalf("the shared Aurora Active Resource is missing: %v", err)
	}

	nodes := kubectl(t, target, "get", "nodes", "-o", "wide")
	t.Logf("EKS nodes:\n%s", nodes)
	pods := kubectl(t, target, "get", "pods", "-n", namespace,
		"-o", "jsonpath={range .items[*]}{.metadata.name}={.status.phase} {end}")
	t.Logf("pods: %s", pods)
	if strings.Contains(pods, "=Pending") || strings.Contains(pods, "=Failed") {
		t.Fatalf("not every pod is running: %s", pods)
	}
	services := kubectl(t, target, "get", "svc", "-n", namespace, "-o", "json")
	if strings.Contains(services, "LoadBalancer") {
		t.Fatal("a LoadBalancer Service would create a paid load balancer")
	}

	result := runJobFlow(ctx, t, target, namespace, "aws")
	t.Logf("job flow result: %s", result)

	if file := os.Getenv("DEPLOYMENT_ID_FILE"); file != "" {
		if err := os.WriteFile(file, []byte(latest.ID), 0o600); err != nil {
			t.Fatalf("write deployment id: %v", err)
		}
	}
	if file := os.Getenv("KUBECONFIG_FILE"); file != "" {
		payload := kubeconfig + "\n" + clusterName + "\n" + namespace
		if err := os.WriteFile(file, []byte(payload), 0o600); err != nil {
			t.Fatalf("write kubeconfig pointer: %v", err)
		}
	}
	t.Logf("run %s verified", runID)
}

func mustDescriptor(t *testing.T, typ, class, id string) resource.Descriptor {
	t.Helper()
	d, err := resource.NewDescriptor(typ, class, id)
	if err != nil {
		t.Fatalf("descriptor: %v", err)
	}
	return d
}
