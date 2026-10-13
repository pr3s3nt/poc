package providers

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/ports/terraformexec"
)

// Fakes: no AWS, no cluster. They prove scoping and fail-closed behavior.
type fakeAWS struct {
	conns map[string]terraformexec.AWSConnection
}

func (f fakeAWS) ResolveAWS(_ context.Context, org, key string) (terraformexec.AWSConnection, error) {
	if org != "acme" {
		return terraformexec.AWSConnection{}, errors.New("foreign")
	}
	c, ok := f.conns[key]
	if !ok {
		return terraformexec.AWSConnection{}, errors.New("missing")
	}
	return c, nil
}

type fakeKube struct{ err error }

func (f fakeKube) ResolveKubernetes(_ context.Context, org, key string) ([]byte, string, error) {
	if f.err != nil || org != "acme" || key != "internal" {
		return nil, "", errors.New("unavailable")
	}
	return []byte("apiVersion: v1\nkind: Config\n"), "ctx-1", nil
}

type fakeAPI struct {
	account string
	status  string
	calls   []string
}

func (f *fakeAPI) CallerAccount(context.Context, terraformexec.AWSConnection) (string, error) {
	f.calls = append(f.calls, "sts")
	return f.account, nil
}
func (f *fakeAPI) DescribeCluster(_ context.Context, _ terraformexec.AWSConnection, name string) (terraformexec.ClusterEndpoint, error) {
	f.calls = append(f.calls, "describe:"+name)
	return terraformexec.ClusterEndpoint{Endpoint: "https://eks.example", CAPEM: "PEM", Status: f.status}, nil
}
func (f *fakeAPI) ClusterToken(_ terraformexec.AWSConnection, name string) (string, error) {
	return "k8s-aws-v1.token-for-" + name, nil
}

func awsConn() terraformexec.AWSConnection {
	return terraformexec.AWSConnection{ConnectionKey: "acct", AccessKeyID: "AKIDSCOPED", SecretAccessKey: "scoped-secret", AccountID: "123456789012", Region: "us-east-1"}
}

func binder(api *fakeAPI) *Binder {
	return &Binder{AWS: fakeAWS{conns: map[string]terraformexec.AWSConnection{"acct": awsConn()}}, Kube: fakeKube{}, API: api, Dir: ""}
}

var awsPin = tfmodule.ProviderPin{Name: "aws", Kind: tfmodule.BindingAWS, Target: tfmodule.TargetEnvironment, ConnectionKey: "acct", ConnectionKind: "AWS", Region: "us-east-1"}

func has(env []string, prefix string) bool {
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			return true
		}
	}
	return false
}

func TestAWSBindingUsesOnlyTheScopedKeyAndNoHostChain(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "HOSTKEY")
	t.Setenv("AWS_PROFILE", "host-profile")
	api := &fakeAPI{account: "123456789012"}
	env, err := binder(api).Bind(context.Background(), terraformexec.BindRequest{OrganizationKey: "acme", Pins: []tfmodule.ProviderPin{awsPin}})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(env.Env, "\n")
	if !strings.Contains(joined, "AWS_ACCESS_KEY_ID=AKIDSCOPED") || strings.Contains(joined, "HOSTKEY") || strings.Contains(joined, "host-profile") ||
		!strings.Contains(joined, "AWS_EC2_METADATA_DISABLED=true") || !has(env.Env, "AWS_SHARED_CREDENTIALS_FILE=") || has(env.Env, "AWS_PROFILE") {
		t.Fatalf("environment:\n%s", joined)
	}
	if env.AccountID != "123456789012" || env.Region != "us-east-1" || len(api.calls) != 1 || api.calls[0] != "sts" {
		t.Fatalf("identity %+v calls %v", env, api.calls)
	}
	var dir string
	for _, e := range env.Env {
		if p, ok := strings.CutPrefix(e, "AWS_CONFIG_FILE="); ok {
			dir = strings.TrimSuffix(p, "/aws-config")
		}
	}
	if info, err := os.Stat(dir); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("private dir: %v %v", info, err)
	}
	if err := env.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("Close left the private directory")
	}
}

func TestAWSBindingFailsClosedOnAccountRegionAndMissingConnection(t *testing.T) {
	ctx := context.Background()
	other := binder(&fakeAPI{account: "999999999999"})
	if _, err := other.Bind(ctx, terraformexec.BindRequest{OrganizationKey: "acme", Pins: []tfmodule.ProviderPin{awsPin}}); !errors.Is(err, terraformexec.ErrProviderBinding) {
		t.Fatalf("another account's key: %v", err)
	}
	region := awsPin
	region.Region = "eu-west-1"
	if _, err := binder(&fakeAPI{account: "123456789012"}).Bind(ctx, terraformexec.BindRequest{OrganizationKey: "acme", Pins: []tfmodule.ProviderPin{region}}); !errors.Is(err, terraformexec.ErrProviderBinding) {
		t.Fatalf("stale region: %v", err)
	}
	if _, err := binder(&fakeAPI{account: "123456789012"}).Bind(ctx, terraformexec.BindRequest{OrganizationKey: "globex", Pins: []tfmodule.ProviderPin{awsPin}}); !errors.Is(err, terraformexec.ErrProviderBinding) {
		t.Fatalf("foreign organization: %v", err)
	}
	if _, err := binder(&fakeAPI{}).Bind(ctx, terraformexec.BindRequest{OrganizationKey: "acme"}); !errors.Is(err, terraformexec.ErrProviderBinding) {
		t.Fatalf("no pins: %v", err)
	}
}

func TestKubernetesConnectionBindingWritesPrivateKubeconfigAndNoHostContext(t *testing.T) {
	t.Setenv("KUBECONFIG", "/host/kubeconfig")
	pin := tfmodule.ProviderPin{Name: "kubernetes", Kind: tfmodule.BindingKubernetes, Target: tfmodule.TargetEnvironment, ConnectionKey: "internal", ConnectionKind: "KUBERNETES", Provider: "k8s-cluster.internal#connections.internal"}
	env, err := binder(&fakeAPI{}).Bind(context.Background(), terraformexec.BindRequest{OrganizationKey: "acme", Pins: []tfmodule.ProviderPin{pin}})
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close()
	var path string
	for _, e := range env.Env {
		if p, ok := strings.CutPrefix(e, "KUBE_CONFIG_PATH="); ok {
			path = p
		}
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 || !has(env.Env, "KUBE_CTX=ctx-1") || strings.Contains(strings.Join(env.Env, "\n"), "/host/kubeconfig") {
		t.Fatalf("kubeconfig %v %v env %v", info, err, env.Env)
	}
	if _, err := (&Binder{AWS: fakeAWS{}, Kube: fakeKube{err: errors.New("x")}, API: &fakeAPI{}}).Bind(context.Background(), terraformexec.BindRequest{OrganizationKey: "acme", Pins: []tfmodule.ProviderPin{pin}}); !errors.Is(err, terraformexec.ErrProviderBinding) {
		t.Fatalf("unavailable Connection: %v", err)
	}
}

func TestEKSTargetBindingDerivesEndpointAndTokenAndRejectsForeignOrInactive(t *testing.T) {
	pin := tfmodule.ProviderPin{Name: "kubernetes", Kind: tfmodule.BindingKubernetes, Target: tfmodule.TargetResource, Provider: "k8s-cluster.eks#environments.a.b"}
	target := execution.Target{Kind: "kubernetes", Provider: "eks", ClusterName: "orc-cluster", Organization: "acme", Connection: "acct", Region: "us-east-1"}
	req := terraformexec.BindRequest{OrganizationKey: "acme", Pins: []tfmodule.ProviderPin{pin}, Targets: map[string]execution.Target{pin.Provider: target}}
	api := &fakeAPI{account: "123456789012", status: "ACTIVE"}
	env, err := binder(api).Bind(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close()
	joined := strings.Join(env.Env, "\n")
	if !strings.Contains(joined, "KUBE_HOST=https://eks.example") || !strings.Contains(joined, "KUBE_TOKEN=k8s-aws-v1.token-for-orc-cluster") || !strings.Contains(joined, "KUBE_LOAD_CONFIG_FILE=false") || has(env.Env, "KUBE_CONFIG_PATH") {
		t.Fatalf("environment:\n%s", joined)
	}

	foreign := req
	foreign.Targets = map[string]execution.Target{pin.Provider: func() execution.Target { t := target; t.Organization = "globex"; return t }()}
	if _, err := binder(api).Bind(context.Background(), foreign); !errors.Is(err, terraformexec.ErrProviderBinding) {
		t.Fatalf("foreign target: %v", err)
	}
	missing := req
	missing.Targets = nil
	if _, err := binder(api).Bind(context.Background(), missing); !errors.Is(err, terraformexec.ErrProviderBinding) {
		t.Fatalf("missing target: %v", err)
	}
	notEKS := req
	notEKS.Targets = map[string]execution.Target{pin.Provider: {Kind: "kubernetes", Organization: "acme", Connection: "acct"}}
	if _, err := binder(api).Bind(context.Background(), notEKS); !errors.Is(err, terraformexec.ErrProviderBinding) {
		t.Fatalf("target without EKS identity: %v", err)
	}
	if _, err := binder(&fakeAPI{account: "123456789012", status: "CREATING"}).Bind(context.Background(), req); !errors.Is(err, terraformexec.ErrProviderBinding) {
		t.Fatalf("inactive cluster: %v", err)
	}
}

func TestAWSBindingAcceptsAnyAllowedRegionOfTheConnectionAndNoOther(t *testing.T) {
	ctx := context.Background()
	conn := awsConn()
	conn.AllowedRegions = []string{"us-east-1", "eu-west-1"}
	b := func() *Binder {
		return &Binder{AWS: fakeAWS{conns: map[string]terraformexec.AWSConnection{"acct": conn}}, Kube: fakeKube{}, API: &fakeAPI{account: "123456789012"}}
	}
	second := awsPin
	second.Region = "eu-west-1"
	env, err := b().Bind(ctx, terraformexec.BindRequest{OrganizationKey: "acme", Pins: []tfmodule.ProviderPin{second}})
	if err != nil {
		t.Fatalf("a verified allowed region was refused: %v", err)
	}
	defer env.Close()
	joined := strings.Join(env.Env, "\n")
	if env.Region != "eu-west-1" || !strings.Contains(joined, "AWS_REGION=eu-west-1") || !strings.Contains(joined, "AWS_DEFAULT_REGION=eu-west-1") || strings.Contains(joined, "us-east-1") {
		t.Fatalf("the invocation did not bind the pinned allowed region:\n%s", joined)
	}
	unlisted := awsPin
	unlisted.Region = "ap-south-1"
	if _, err := b().Bind(ctx, terraformexec.BindRequest{OrganizationKey: "acme", Pins: []tfmodule.ProviderPin{unlisted}}); !errors.Is(err, terraformexec.ErrProviderBinding) {
		t.Fatalf("an unlisted region was accepted: %v", err)
	}
	empty := awsPin
	empty.Region = ""
	if _, err := b().Bind(ctx, terraformexec.BindRequest{OrganizationKey: "acme", Pins: []tfmodule.ProviderPin{empty}}); !errors.Is(err, terraformexec.ErrProviderBinding) {
		t.Fatalf("an empty pinned region was accepted: %v", err)
	}
}
