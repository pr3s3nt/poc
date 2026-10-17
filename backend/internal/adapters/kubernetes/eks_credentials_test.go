package kubernetes

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"orchestrator/internal/ports/execution"
	"orchestrator/internal/ports/terraformexec"
)

type eksAWSIdentity struct {
	conn     terraformexec.AWSConnection
	err      error
	org, key string
}

func (s *eksAWSIdentity) ResolveAWS(_ context.Context, org, key string) (terraformexec.AWSConnection, error) {
	s.org, s.key = org, key
	return s.conn, s.err
}

type eksAWSAPI struct {
	account     string
	endpoint    terraformexec.ClusterEndpoint
	token       string
	callerErr   error
	describeErr error
	tokenErr    error
	seen        terraformexec.AWSConnection
	cluster     string
}

func (s *eksAWSAPI) CallerAccount(_ context.Context, conn terraformexec.AWSConnection) (string, error) {
	s.seen = conn
	return s.account, s.callerErr
}
func (s *eksAWSAPI) DescribeCluster(_ context.Context, conn terraformexec.AWSConnection, cluster string) (terraformexec.ClusterEndpoint, error) {
	s.seen, s.cluster = conn, cluster
	return s.endpoint, s.describeErr
}
func (s *eksAWSAPI) ClusterToken(conn terraformexec.AWSConnection, cluster string) (string, error) {
	s.seen, s.cluster = conn, cluster
	return s.token, s.tokenErr
}

func eksTestTarget() execution.Target {
	return execution.Target{Kind: "kubernetes", Provider: "eks", Organization: "acme", Connection: "aws-prod", Region: "us-east-1", ClusterName: "orch-eks", Namespace: "team-a"}
}

func eksTestSource() (*TargetCredentials, *eksAWSIdentity, *eksAWSAPI) {
	identity := &eksAWSIdentity{conn: terraformexec.AWSConnection{ConnectionKey: "aws-prod", AccountID: "123456789012", Region: "us-east-1", AllowedRegions: []string{"us-east-1"}, AccessKeyID: "access-key-value", SecretAccessKey: "secret-key-value"}}
	api := &eksAWSAPI{account: "123456789012", endpoint: terraformexec.ClusterEndpoint{Endpoint: "https://eks.example.test", CAPEM: "-----BEGIN CERTIFICATE-----\nYWJj\n-----END CERTIFICATE-----\n", Status: "ACTIVE"}, token: "short-lived-eks-token"}
	return &TargetCredentials{AWS: identity, API: api}, identity, api
}

func TestEKSTargetCredentialsDeriveScopedShortLivedKubeconfig(t *testing.T) {
	source, identity, api := eksTestSource()
	target := eksTestTarget()
	document, err := source.ResolveKubeconfig(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(document, &got); err != nil {
		t.Fatalf("generated kubeconfig is not valid YAML/JSON: %v", err)
	}
	clusters := got["clusters"].([]any)
	cluster := clusters[0].(map[string]any)["cluster"].(map[string]any)
	if cluster["server"] != api.endpoint.Endpoint || cluster["certificate-authority-data"] != base64.StdEncoding.EncodeToString([]byte(api.endpoint.CAPEM)) {
		t.Fatalf("generated cluster endpoint or CA does not match the EKS API response: %#v", cluster)
	}
	users := got["users"].([]any)
	user := users[0].(map[string]any)["user"].(map[string]any)
	if user["token"] != api.token || got["current-context"] != target.ClusterName {
		t.Fatalf("generated config did not select the scoped token context")
	}
	if identity.org != target.Organization || identity.key != target.Connection || api.seen.Region != target.Region || api.cluster != target.ClusterName {
		t.Fatal("EKS resolution did not use the persisted target identity")
	}
	if strings.Contains(string(document), identity.conn.AccessKeyID) || strings.Contains(string(document), identity.conn.SecretAccessKey) {
		t.Fatal("long-lived AWS credential was copied into kubeconfig")
	}
	if _, hasExec := user["exec"]; hasExec {
		t.Fatal("generated kubeconfig invokes an external exec plugin")
	}
	target.Context = target.ClusterName // the module's executor-owned kubeContext output
	if _, err := source.ResolveKubeconfig(context.Background(), target); err != nil {
		t.Fatalf("the EKS module's cluster-name context was rejected: %v", err)
	}
}

func TestEKSTargetCredentialsRemainPrivateForKubectlAndCloseAfterUse(t *testing.T) {
	source, _, _ := eksTestSource()
	kubectl, logPath := installStub(t)
	t.Setenv("STUB_EXPECT", "short-lived-eks-token")
	target := eksTestTarget()
	target.Extra = map[string]string{"application": "shop", "environment": "staging", "deployment": "deploy-123"}
	deployer := &Deployer{KubectlPath: kubectl, Credentials: source}
	if err := deployer.Apply(context.Background(), target, []execution.Manifest{{Object: map[string]any{"apiVersion": "v1", "kind": "ConfigMap"}}}); err != nil {
		t.Fatal(err)
	}
	if err := deployer.WaitReady(context.Background(), target, []execution.WorkloadRef{{Kind: "Deployment", Name: "api"}}); err != nil {
		t.Fatal(err)
	}
	labels := `"labels":{"orchestrator.io/application":"shop","orchestrator.io/environment":"staging","orchestrator.io/deployment-id":"deploy-123","app.kubernetes.io/managed-by":"orchestrator","app.kubernetes.io/name":"api"}`
	t.Setenv("STUB_DEPLOYMENT", `{"metadata":{"name":"api","uid":"11111111-1111-4111-8111-111111111111",`+labels+`}}`)
	t.Setenv("STUB_SERVICE", `{"metadata":{"name":"api","uid":"22222222-2222-4222-8222-222222222222",`+labels+`}}`)
	t.Setenv("STUB_SECRET", `{"metadata":{"name":"api-env","uid":"33333333-3333-4333-8333-333333333333",`+labels+`}}`)
	if err := deployer.Remove(context.Background(), target, "api"); err != nil {
		t.Fatal(err)
	}
	log := readLog(t, logPath)
	assertPrivateAndRemoved(t, log)
	for _, want := range []string{
		"ARGS --kubeconfig ",
		"delete --raw /apis/apps/v1/namespaces/team-a/deployments/api -f -",
		"delete --raw /api/v1/namespaces/team-a/services/api -f -",
		"delete --raw /api/v1/namespaces/team-a/secrets/api-env -f -",
	} {
		if !strings.Contains(log, want) {
			t.Fatalf("EKS workload lifecycle omitted %q: %s", want, log)
		}
	}
}

func TestEKSTargetCredentialsRejectMismatchedOrUnknownScopeWithoutFallback(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*execution.Target, *eksAWSIdentity, *eksAWSAPI)
	}{
		{name: "foreign connection key", mutate: func(target *execution.Target, _ *eksAWSIdentity, _ *eksAWSAPI) { target.Connection = "other" }},
		{name: "redirected context", mutate: func(target *execution.Target, _ *eksAWSIdentity, _ *eksAWSAPI) { target.Context = "foreign-cluster" }},
		{name: "account mismatch", mutate: func(_ *execution.Target, _ *eksAWSIdentity, api *eksAWSAPI) { api.account = "999999999999" }},
		{name: "inactive cluster", mutate: func(_ *execution.Target, _ *eksAWSIdentity, api *eksAWSAPI) { api.endpoint.Status = "CREATING" }},
		{name: "cluster api failure", mutate: func(_ *execution.Target, _ *eksAWSIdentity, api *eksAWSAPI) {
			api.describeErr = errors.New("private response body")
		}},
		{name: "token failure", mutate: func(_ *execution.Target, _ *eksAWSIdentity, api *eksAWSAPI) {
			api.tokenErr = errors.New("private token error")
		}},
		{name: "AWS credential failure", mutate: func(_ *execution.Target, identity *eksAWSIdentity, _ *eksAWSAPI) {
			identity.err = errors.New("secret material")
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source, identity, api := eksTestSource()
			target := eksTestTarget()
			test.mutate(&target, identity, api)
			if _, err := source.ResolveKubeconfig(context.Background(), target); !errors.Is(err, ErrCredentialTarget) {
				t.Fatalf("invalid EKS target was not refused with a safe error: %v", err)
			} else if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "private") {
				t.Fatalf("credential or provider detail escaped: %v", err)
			}
		})
	}
	// UC-03 V02-03: the account's verification regions are probe evidence, not a
	// deployment allowlist. The Definition's region reaches the same identity.
	t.Run("region outside the verification regions is accepted", func(t *testing.T) {
		source, _, _ := eksTestSource()
		target := eksTestTarget()
		target.Region = "us-west-2"
		if _, err := source.ResolveKubeconfig(context.Background(), target); err != nil {
			t.Fatalf("a Definition region must not be limited by the account verification regions: %v", err)
		}
	})
	t.Run("unknown provider does not fall back to uploaded kubeconfig", func(t *testing.T) {
		internal := &stubResolver{document: []byte(`apiVersion: v1`)}
		source := &TargetCredentials{Kubernetes: internal}
		target := eksTestTarget()
		target.Provider = "unknown"
		if _, err := source.ResolveKubeconfig(context.Background(), target); !errors.Is(err, ErrCredentialTarget) {
			t.Fatalf("unknown provider was accepted: %v", err)
		}
		if len(internal.targets) != 0 {
			t.Fatal("unknown provider fell back to the KUBECONFIG Connection")
		}
	})
}

func TestInternalKubernetesTargetStillUsesOnlyItsKubeconfigSource(t *testing.T) {
	internal := &stubResolver{document: []byte(`apiVersion: v1`)}
	source := &TargetCredentials{Kubernetes: internal}
	target := execution.Target{Kind: "kubernetes", Organization: "acme", Connection: "kind"}
	got, err := source.ResolveKubeconfig(context.Background(), target)
	if err != nil || string(got) != string(internal.document) || len(internal.targets) != 1 {
		t.Fatalf("internal-k8s source changed: data=%q calls=%d err=%v", got, len(internal.targets), err)
	}
}

func TestPrivateKubeconfigFileMaterializationUsesOwnedTempDirectory(t *testing.T) {
	// The higher-level Apply test confirms the returned document is materialized
	// and removed; this assertion protects against accidentally using the
	// repository or process current directory for temporary target data.
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	cli, cleanup, err := materialize("kubectl", "ctx", []byte("private"))
	if err != nil {
		t.Fatal(err)
	}
	path := cli.Kubeconfig
	if !strings.HasPrefix(path, filepath.Clean(dir)+string(os.PathSeparator)) {
		t.Fatalf("kubeconfig escaped temp root: %s", path)
	}
	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("private kubeconfig was not removed: %v", err)
	}
}
