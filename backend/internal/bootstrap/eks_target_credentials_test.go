package bootstrap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"orchestrator/internal/adapters/credentialmemory"
	k8s "orchestrator/internal/adapters/kubernetes"
	"orchestrator/internal/application/connection"
	"orchestrator/internal/application/deployment"
	appdomain "orchestrator/internal/domain/application"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/ports/terraformexec"
)

func TestKubeconfigSourceForAdapterUsesAWSScopedEKSResolverOnlyForAWSMode(t *testing.T) {
	resolver := connection.NewCredentialResolver(nil, nil)
	if got := kubeconfigSourceFor(AdapterKubernetes, resolver, nil); got != resolver {
		t.Fatal("internal Kubernetes mode changed its KUBECONFIG-only credential source")
	}
	api := &bootstrapEKSAPI{}
	got, ok := kubeconfigSourceFor(AdapterAWS, resolver, api).(*k8s.TargetCredentials)
	if !ok || got.Kubernetes != resolver || got.AWS != resolver || got.API != api {
		t.Fatalf("AWS mode did not install the scoped EKS credential source: %#v", got)
	}
}

type bootstrapEKSAPI struct{}

func (bootstrapEKSAPI) CallerAccount(_ context.Context, conn terraformexec.AWSConnection) (string, error) {
	return conn.AccountID, nil
}
func (bootstrapEKSAPI) DescribeCluster(context.Context, terraformexec.AWSConnection, string) (terraformexec.ClusterEndpoint, error) {
	return terraformexec.ClusterEndpoint{Endpoint: "https://eks.example.test", CAPEM: "-----BEGIN CERTIFICATE-----\nYWJj\n-----END CERTIFICATE-----", Status: "ACTIVE"}, nil
}
func (bootstrapEKSAPI) ClusterToken(terraformexec.AWSConnection, string) (string, error) {
	return "short-lived-token", nil
}

type bootstrapConnectionReader map[string]appdomain.Connection

func (r bootstrapConnectionReader) GetConnection(_ context.Context, org, key string) (appdomain.Connection, error) {
	conn, ok := r[org+"/"+key]
	if !ok {
		return appdomain.Connection{}, fmt.Errorf("missing")
	}
	return conn, nil
}

func TestAWSBootstrapSourceRestoresOpaqueTargetForWorkloadLifecycle(t *testing.T) {
	ctx := context.Background()
	store := credentialmemory.New()
	secretRef, err := store.Put(ctx, "acme", "aws-prod", []byte(`{"accessKeyId":"synthetic-access-key","secretAccessKey":"synthetic-secret-key"}`))
	if err != nil {
		t.Fatal(err)
	}
	conn := appdomain.Connection{
		Key: "aws-prod", OrganizationKey: "acme", Kind: appdomain.ConnectionAWS,
		AuthenticationType: appdomain.AuthAWSAccessKey, Status: appdomain.ConnectionReady,
		SecretRef: secretRef, Config: map[string]any{"accountId": "123456789012", "region": "us-east-1", "allowedRegions": []string{"us-east-1"}},
	}
	resolver := connection.NewCredentialResolver(bootstrapConnectionReader{"acme/aws-prod": conn}, store)
	source := kubeconfigSourceFor(AdapterAWS, resolver, bootstrapEKSAPI{})
	var target execution.Target
	ref := map[string]any{"organization": "acme", "connection": "aws-prod", "provider": "eks", "region": "us-east-1", "cluster": "orchestrator-eks", "namespace": "team-a", "context": "orchestrator-eks"}
	if err := deployment.RestoreTarget(&target, ref, "acme"); err != nil {
		t.Fatal(err)
	}
	target.Extra = map[string]string{"application": "shop", "environment": "staging", "deployment": "deploy-123"}

	tmp := t.TempDir()
	logPath, deletedPath := filepath.Join(tmp, "kubectl.log"), filepath.Join(tmp, "deleted")
	const stub = `#!/bin/sh
kc=""; prev=""
for arg in "$@"; do [ "$prev" = "--kubeconfig" ] && kc="$arg"; prev="$arg"; done
echo "CALL $*" >> "$STUB_LOG"
echo "FILE $kc $(stat -c %a "$kc") $(stat -c %a "$(dirname "$kc")")" >> "$STUB_LOG"
grep -q 'short-lived-token' "$kc" && echo TOKEN_OK >> "$STUB_LOG"
case "$*" in
  *" get deployment api "*) kind=deployments; resource=/apis/apps/v1/namespaces/team-a/deployments/api ;;
  *" get service api "*) kind=services; resource=/api/v1/namespaces/team-a/services/api ;;
  *" get secret api-env "*) kind=secrets; resource=/api/v1/namespaces/team-a/secrets/api-env ;;
  *" rollout status "*) exit 0 ;;
  *" delete --raw "*)
    path=""; previous=""
    for arg in "$@"; do [ "$previous" = "--raw" ] && path="$arg"; previous="$arg"; done
    cat >> "$STUB_LOG"
    echo "$path" >> "$STUB_DELETED"
    exit 0 ;;
  *" get "*) echo "Error from server (NotFound): not found" >&2; exit 1 ;;
esac
if grep -Fqx "$resource" "$STUB_DELETED" 2>/dev/null; then echo "Error from server (NotFound): not found" >&2; exit 1; fi
labels='"labels":{"orchestrator.io/application":"shop","orchestrator.io/environment":"staging","orchestrator.io/deployment-id":"deploy-123","app.kubernetes.io/managed-by":"orchestrator","app.kubernetes.io/name":"api"}'
case "$kind" in
  deployments) uid=11111111-1111-4111-8111-111111111111; name=api ;;
  services) uid=22222222-2222-4222-8222-222222222222; name=api ;;
  secrets) uid=33333333-3333-4333-8333-333333333333; name=api-env ;;
esac
printf '{"metadata":{"name":"%s","uid":"%s",%s}}\n' "$name" "$uid" "$labels"
`
	kubectl := filepath.Join(tmp, "kubectl")
	if err := os.WriteFile(kubectl, []byte(stub), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("STUB_LOG", logPath)
	t.Setenv("STUB_DELETED", deletedPath)
	t.Setenv("TMPDIR", filepath.Join(tmp, "private-tmp"))
	if err := os.Mkdir(os.Getenv("TMPDIR"), 0o700); err != nil {
		t.Fatal(err)
	}
	deployer := k8s.NewDeployer(kubectl)
	deployer.Timeout = 2 * time.Second
	deployer.Credentials = source
	if err := deployer.Apply(ctx, target, []execution.Manifest{{Object: map[string]any{"apiVersion": "v1", "kind": "ConfigMap"}}}); err != nil {
		t.Fatal(err)
	}
	if err := deployer.WaitReady(ctx, target, []execution.WorkloadRef{{Kind: "Deployment", Name: "api"}}); err != nil {
		t.Fatal(err)
	}
	if err := deployer.Remove(ctx, target, "api"); err != nil {
		t.Fatal(err)
	}
	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(log)
	for _, required := range []string{"apply -f -", "rollout status deployment/api", "deployments/api", "services/api", "secrets/api-env", "TOKEN_OK"} {
		if !strings.Contains(text, required) {
			t.Fatalf("production AWS source/direct deployer lifecycle omitted %q", required)
		}
	}
	files := 0
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 4 || fields[0] != "FILE" {
			continue
		}
		files++
		if fields[2] != "600" || fields[3] != "700" {
			t.Fatalf("generated EKS kubeconfig permissions were not private: %s", line)
		}
		if _, err := os.Stat(fields[1]); !os.IsNotExist(err) {
			t.Fatalf("generated EKS kubeconfig remains after operation: %v", err)
		}
	}
	if files < 5 {
		t.Fatalf("expected separate credential materialization for apply/readiness/removal operations, got %d", files)
	}
}
