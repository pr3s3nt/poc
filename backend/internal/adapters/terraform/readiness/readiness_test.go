package readiness

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"orchestrator/internal/ports/terraformexec"
)

// FAKE Kubernetes and AWS APIs. They prove the readiness rules, bounded
// polling, safe IDs and credential handling; the real queries run only in the
// live acceptance gates.
type fakeKubectl struct {
	mu    sync.Mutex
	calls [][]string
	out   map[string]string // "<kind>/<name>" -> JSON
	fail  bool
	files []string // kubeconfig contents seen
}

func (f *fakeKubectl) run(_ context.Context, _ string, args []string, env []string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, args)
	if f.fail {
		return nil, errors.New("command failed")
	}
	if data, err := os.ReadFile(args[1]); err == nil {
		f.files = append(f.files, string(data))
	}
	for i, a := range args {
		if a == "get" {
			return []byte(f.out[args[i+1]+"/"+args[i+2]]), nil
		}
	}
	return nil, errors.New("unexpected")
}

type fakeAWS struct {
	status map[string]string
	err    error
}

func (f fakeAWS) Status(_ context.Context, _ terraformexec.AWSConnection, q terraformexec.StatusQuery, id string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	s, ok := f.status[string(q)+":"+id]
	if !ok {
		return "", errors.New("not found")
	}
	return s, nil
}

func request(env []string, resources ...terraformexec.StateResource) terraformexec.ReadinessRequest {
	return terraformexec.ReadinessRequest{OrganizationKey: "acme", ResourceType: "redis", Resources: resources, Env: env, Deadline: time.Now().Add(150 * time.Millisecond)}
}

func checker(k *fakeKubectl, aws terraformexec.AWSStatusAPI) *Checker {
	return &Checker{Kubectl: "kubectl", AWS: aws, Interval: 20 * time.Millisecond, Dir: os.TempDir(), Run: k.run}
}

var statefulSet = terraformexec.StateResource{Address: "kubernetes_stateful_set_v1.redis", Type: "kubernetes_stateful_set_v1", ID: "orc-ns/redis"}

const notReadySet = `{"metadata":{"generation":2},"spec":{"replicas":1},"status":{"observedGeneration":2,"readyReplicas":0,"updatedReplicas":1}}`
const readySet = `{"metadata":{"generation":2},"spec":{"replicas":1},"status":{"observedGeneration":2,"readyReplicas":1,"updatedReplicas":1}}`

func TestStatefulSetReadyAndNotReady(t *testing.T) {
	kubeconfigFile := t.TempDir() + "/conn-kubeconfig"
	if err := os.WriteFile(kubeconfigFile, []byte("apiVersion: v1\nkind: Config\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	env := []string{"KUBE_CONFIG_PATH=" + kubeconfigFile, "KUBE_CTX=ctx-1"}
	ready := &fakeKubectl{out: map[string]string{"statefulset/redis": readySet}}
	if err := checker(ready, nil).Check(context.Background(), request(env, statefulSet)); err != nil {
		t.Fatalf("ready: %v", err)
	}
	if got := strings.Join(ready.calls[0], " "); !strings.Contains(got, "--context ctx-1") || !strings.Contains(got, "get statefulset redis -o json -n orc-ns") {
		t.Fatalf("kubectl call %q", got)
	}

	// A controlled invalid image never becomes ready: deterministic ErrNotReady
	// with safe IDs after the bounded deadline.
	stuck := &fakeKubectl{out: map[string]string{"statefulset/redis": notReadySet}}
	err := checker(stuck, nil).Check(context.Background(), request(env, statefulSet))
	if !errors.Is(err, terraformexec.ErrNotReady) || !strings.Contains(err.Error(), "kubernetes_stateful_set_v1.redis=orc-ns/redis") || errors.Is(err, ErrUnavailable) {
		t.Fatalf("not ready: %v", err)
	}
	if len(stuck.calls) < 2 {
		t.Fatalf("the check did not poll: %d calls", len(stuck.calls))
	}
	// A stale observed generation is not ready even with ready replicas.
	stale := &fakeKubectl{out: map[string]string{"statefulset/redis": `{"metadata":{"generation":3},"spec":{"replicas":1},"status":{"observedGeneration":2,"readyReplicas":1,"updatedReplicas":1}}`}}
	if err := checker(stale, nil).Check(context.Background(), request(env, statefulSet)); !errors.Is(err, terraformexec.ErrNotReady) {
		t.Fatalf("stale generation: %v", err)
	}
}

func TestQueryFailureIsUnavailableNotNotReady(t *testing.T) {
	env := []string{"KUBE_CONFIG_PATH=" + t.TempDir() + "/missing"}
	if err := checker(&fakeKubectl{}, nil).Check(context.Background(), request(env, statefulSet)); !errors.Is(err, ErrUnavailable) || errors.Is(err, terraformexec.ErrNotReady) {
		t.Fatalf("missing kubeconfig: %v", err)
	}
	file := t.TempDir() + "/k"
	_ = os.WriteFile(file, []byte("x"), 0o600)
	down := &fakeKubectl{fail: true}
	if err := checker(down, nil).Check(context.Background(), request([]string{"KUBE_CONFIG_PATH=" + file}, statefulSet)); !errors.Is(err, ErrUnavailable) || errors.Is(err, terraformexec.ErrNotReady) {
		t.Fatalf("API down: %v", err)
	}
}

func TestNeverVacuouslyReady(t *testing.T) {
	other := terraformexec.StateResource{Address: "aws_iam_role.x", Type: "aws_iam_role", ID: "role"}
	if err := checker(&fakeKubectl{}, fakeAWS{}).Check(context.Background(), request(nil, other)); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("no query applies: %v", err)
	}
	if err := checker(&fakeKubectl{}, fakeAWS{}).Check(context.Background(), request(nil)); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("no resources: %v", err)
	}
}

func TestEKSTargetKubeconfigIsPrivateAndTokenNeverInError(t *testing.T) {
	env := []string{"KUBE_HOST=https://eks.example", "KUBE_TOKEN=k8s-aws-v1.SECRET-TOKEN", "KUBE_CLUSTER_CA_CERT_DATA=-----BEGIN CERTIFICATE-----\nAA\n-----END CERTIFICATE-----\n"}
	k := &fakeKubectl{out: map[string]string{"statefulset/redis": notReadySet}}
	err := checker(k, nil).Check(context.Background(), request(env, statefulSet))
	if !errors.Is(err, terraformexec.ErrNotReady) || strings.Contains(err.Error(), "SECRET-TOKEN") {
		t.Fatalf("error %v", err)
	}
	if len(k.files) == 0 || !strings.Contains(k.files[0], `"server":"https://eks.example"`) || !strings.Contains(k.files[0], "SECRET-TOKEN") {
		t.Fatalf("kubeconfig content %v", k.files)
	}
	for _, call := range k.calls {
		if strings.Contains(strings.Join(call, " "), "SECRET-TOKEN") {
			t.Fatal("the token reached kubectl argv")
		}
	}
}

func TestAWSStatusReadiness(t *testing.T) {
	cluster := terraformexec.StateResource{Address: "aws_rds_cluster.db", Type: "aws_rds_cluster", ID: "orc-db"}
	group := terraformexec.StateResource{Address: "aws_eks_node_group.ng", Type: "aws_eks_node_group", ID: "orc-cluster:orc-ng"}
	env := []string{"AWS_ACCESS_KEY_ID=AKID", "AWS_SECRET_ACCESS_KEY=S", "AWS_REGION=us-east-1"}
	ready := fakeAWS{status: map[string]string{"rds-cluster:orc-db": "available", "eks-nodegroup:orc-cluster/orc-ng": "ACTIVE"}}
	if err := checker(nil, ready).Check(context.Background(), request(env, cluster, group)); err != nil {
		t.Fatalf("ready: %v", err)
	}
	creating := fakeAWS{status: map[string]string{"rds-cluster:orc-db": "creating", "eks-nodegroup:orc-cluster/orc-ng": "ACTIVE"}}
	err := checker(nil, creating).Check(context.Background(), request(env, cluster, group))
	if !errors.Is(err, terraformexec.ErrNotReady) || !strings.Contains(err.Error(), "aws_rds_cluster.db=orc-db") || strings.Contains(err.Error(), "AKID") {
		t.Fatalf("creating: %v", err)
	}
	if err := checker(nil, fakeAWS{err: errors.New("denied")}).Check(context.Background(), request(env, cluster)); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("API error: %v", err)
	}
	if err := checker(nil, ready).Check(context.Background(), request(nil, cluster)); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("no scoped AWS key: %v", err)
	}
}

// FAKE kubectl: namespace inspection lists the namespace UID and the real
// object kinds with owners (UIDs) and managers; missing discovery, unreadable
// contents or objects without UIDs are errors, never empty.
type scriptedKubectl struct {
	apiResources string
	namespace    string
	items        string
	failGet      bool
	calls        [][]string
}

func (k *scriptedKubectl) run(_ context.Context, _ string, args []string, _ []string) ([]byte, error) {
	k.calls = append(k.calls, args)
	for i, a := range args {
		switch a {
		case "api-resources":
			return []byte(k.apiResources), nil
		case "get":
			if args[i+1] == "namespace" {
				return []byte(k.namespace), nil
			}
			if k.failGet {
				return nil, errors.New("failed")
			}
			return []byte(k.items), nil
		}
	}
	return nil, errors.New("unexpected")
}

const coreDiscovery = "configmaps\nendpoints\nevents\npersistentvolumeclaims\npods\nsecrets\nserviceaccounts\nservices\nstatefulsets.apps\n"

func TestInspectListsObjectsWithUIDsOwnersAndManagers(t *testing.T) {
	file := t.TempDir() + "/k"
	_ = os.WriteFile(file, []byte("x"), 0o600)
	k := &scriptedKubectl{
		apiResources: coreDiscovery,
		namespace:    `{"metadata":{"uid":"uid-ns"}}`,
		items: `{"items":[
		 {"kind":"ConfigMap","metadata":{"name":"kube-root-ca.crt","uid":"u1","annotations":{"kubernetes.io/description":"x"},"managedFields":[{"manager":"kube-controller-manager"}]},"data":{"ca.crt":"x"}},
		 {"kind":"ServiceAccount","metadata":{"name":"default","uid":"u2","managedFields":[{"manager":"kube-controller-manager"},{"manager":"kubectl-edit"}]},"secrets":[],"imagePullSecrets":[{"name":"pull"}]},
		 {"kind":"Pod","metadata":{"name":"redis-0","uid":"u3","ownerReferences":[{"kind":"StatefulSet","name":"redis","uid":"owner-uid"}]}},
		 {"kind":"Endpoints","metadata":{"name":"custom","uid":"u4","labels":{"x":"y"},"managedFields":[{"manager":"kubectl-create"}]}},
		 {"kind":"Secret","metadata":{"name":"foreign","uid":"u5"},"data":{"password":"x"}}]}`,
	}
	c := &Checker{Kubectl: "kubectl", Dir: os.TempDir(), Run: k.run}
	got, err := c.Inspect(context.Background(), []string{"KUBE_CONFIG_PATH=" + file}, "orc-ns")
	if err != nil || got.UID != "uid-ns" || len(got.Objects) != 5 {
		t.Fatalf("inspection %+v: %v", got, err)
	}
	o := got.Objects
	if o[0].Managers[0] != "kube-controller-manager" || o[0].DataKeys[0] != "ca.crt" || o[0].AnnotationKeys[0] != "kubernetes.io/description" ||
		len(o[1].Managers) != 2 || o[1].ImagePullSecrets != 1 ||
		o[2].Owners[0] != (terraformexec.NamespaceOwner{Kind: "StatefulSet", Name: "redis", UID: "owner-uid"}) ||
		o[3].Kind != "Endpoints" || o[3].LabelKeys[0] != "x" || o[3].Managers[0] != "kubectl-create" || o[4].DataKeys[0] != "password" {
		t.Fatalf("object facts %+v", o)
	}
	var get string
	for _, c := range k.calls {
		if strings.Contains(strings.Join(c, " "), "statefulsets.apps") {
			get = strings.Join(c, " ")
		}
	}
	// Endpoints are enumerated (only transient events are skipped) and the
	// managers are requested.
	if !strings.Contains(get, "endpoints") || strings.Contains(get, "events") || !strings.Contains(get, "--show-managed-fields") || !strings.Contains(get, "-n orc-ns") {
		t.Fatalf("get call %q", get)
	}

	env := []string{"KUBE_CONFIG_PATH=" + file}
	fail := func(label string, mutate func(*scriptedKubectl), name string) {
		t.Helper()
		bad := *k
		bad.calls = nil
		mutate(&bad)
		c := &Checker{Kubectl: "kubectl", Dir: os.TempDir(), Run: bad.run}
		if _, err := c.Inspect(context.Background(), env, name); !errors.Is(err, ErrUnavailable) {
			t.Errorf("%s: %v", label, err)
		}
	}
	fail("unsafe namespace name", func(*scriptedKubectl) {}, "Bad Name")
	fail("unreadable contents", func(b *scriptedKubectl) { b.failGet = true }, "orc-ns")
	fail("discovery omits endpoints", func(b *scriptedKubectl) {
		b.apiResources = strings.ReplaceAll(coreDiscovery, "endpoints\n", "")
	}, "orc-ns")
	fail("discovery omits secrets", func(b *scriptedKubectl) {
		b.apiResources = strings.ReplaceAll(coreDiscovery, "secrets\n", "")
	}, "orc-ns")
	fail("empty discovery", func(b *scriptedKubectl) { b.apiResources = "" }, "orc-ns")
	fail("namespace without UID", func(b *scriptedKubectl) { b.namespace = `{"metadata":{}}` }, "orc-ns")
	fail("object without UID", func(b *scriptedKubectl) {
		b.items = `{"items":[{"kind":"Pod","metadata":{"name":"p"}}]}`
	}, "orc-ns")
	fail("malformed items", func(b *scriptedKubectl) { b.items = `not json` }, "orc-ns")
	if _, err := c.Inspect(context.Background(), nil, "orc-ns"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("no scoped access: %v", err)
	}
}
