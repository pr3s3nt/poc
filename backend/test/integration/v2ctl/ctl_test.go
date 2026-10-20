package main_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"orchestrator/test/integration/v2run"
)

var ctlBin, supBin string

func TestMain(m *testing.M) {
	dir, _ := os.MkdirTemp("", "v2ctl-")
	ctlBin, supBin = filepath.Join(dir, "v2ctl"), filepath.Join(dir, "v2supervisor")
	for bin, pkg := range map[string]string{ctlBin: "orchestrator/test/integration/v2ctl", supBin: "orchestrator/test/integration/v2supervisor"} {
		if out, err := exec.Command("go", "build", "-o", bin, pkg).CombinedOutput(); err != nil {
			os.Stderr.Write(out)
			os.Exit(1)
		}
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// fakeKubectl is a shell stand-in for kubectl over a directory "cluster": it
// exists to exercise v2ctl's subprocess wiring (guard/delete/verify exit codes,
// reverse order, handle env), NOT as acceptance evidence.
const fakeKubectl = `#!/bin/sh
C="$(dirname "$0")/cluster"
# drop --kubeconfig X --context Y
[ "$1" = "--kubeconfig" ] && shift 2
[ "$1" = "--context" ] && shift 2
echo "$@" >> "$C/calls.log"
if [ "$1" = "-n" ]; then
  ns="$2"; shift 2
  case "$1" in
    get) case "$2" in
           deployments,daemonsets,replicasets,jobs,cronjobs,services,ingresses,secrets,roles,rolebindings,configmaps,serviceaccounts) [ -e "$C/ns.json" ] && cat "$C/delivery.json" 2>/dev/null; exit 0;;
           statefulsets*) [ -e "$C/ns.json" ] && cat "$C/app" 2>/dev/null; exit 0;;
         esac;;
    delete) case "$2" in
           deployments*) echo delivery >> "$C/order.log"; : > "$C/delivery"; exit 0;;
           statefulsets*) echo app-resources >> "$C/order.log"; : > "$C/app"; : > "$C/pv.items"; exit 0;;
         esac;;
  esac
fi
if [ "$1" = "get" ] && [ "$2" = "namespace" ]; then
  if [ ! -e "$C/ns.json" ]; then echo 'Error from server (NotFound): namespaces "x" not found' >&2; exit 1; fi
  if [ "$4" = "-o" ] && [ "$5" = "name" ]; then echo "namespace/$3"; exit 0; fi
  cat "$C/ns.json"; exit 0
fi
if [ "$1" = "get" ] && [ "$2" = "pv" ]; then
  printf '{"items":[%s]}' "$(cat "$C/pv.items" 2>/dev/null)"; exit 0
fi
if [ "$1" = "delete" ] && [ "$2" = "namespace" ]; then echo namespace >> "$C/order.log"; rm -f "$C/ns.json"; exit 0; fi
echo "unexpected: $@" >&2; exit 99
`

// fakeCleanup stands in for v2cleanup (the real one needs the product wiring,
// Vault and a cluster): it only lets this test prove the supervisor/v2ctl
// wiring, ordering and isolation. The real engine path is covered by the
// tfexec safety-net tests on a real PostgreSQL and the actual Terraform CLI.
const fakeCleanup = `#!/bin/sh
C="$(dirname "$0")/cluster"
case "$1" in
  delivery) echo delivery >> "$C/order.log"; : > "$C/delivery"; printf '{"items":[]}' > "$C/delivery.json";;
  terraform) echo app-resources >> "$C/order.log"; : > "$C/app"; : > "$C/pv.items"; echo namespace >> "$C/order.log"; rm -f "$C/ns.json";;
  status) [ ! -e "$C/ns.json" ] || exit 3;;
  *) exit 2;;
esac
`

type env struct {
	t       *testing.T
	cleanup string
	handles string
	dir     string
	cluster string
	private string
	cfgPath string
	mpath   string
	cfg     v2run.Config
}

func newEnv(t *testing.T, namespaceRunID string) *env {
	t.Helper()
	dir := t.TempDir()
	e := &env{t: t, dir: dir, cluster: filepath.Join(dir, "cluster"), private: filepath.Join(dir, "private")}
	for _, d := range []string{e.cluster, e.private} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	e.cleanup = filepath.Join(dir, "v2cleanup")
	if err := os.WriteFile(e.cleanup, []byte(fakeCleanup), 0o700); err != nil {
		t.Fatal(err)
	}
	e.handles = filepath.Join(dir, "cleanup-handles.json")
	handles := v2run.CleanupHandles{
		RunID: "a01-0001", OrganizationKey: "acme", ApplicationKey: "acceptance", EnvironmentKey: "staging", NamespaceIdentity: "orch-v2-test-a01-0001", Owner: "codex",
		Terraform: "/opt/t", Kubectl: "/opt/k", ScoreK8s: "/opt/s", Git: "/usr/bin/git", DatabaseURLFile: "/p/console.url", StateDatabaseURLFile: "/p/state.url", StateKeyFile: "/p/state.key", StateKeyID: "v2-a01",
		RecoveryDir: "/p/recovery", TerraformRoot: "/p/tfroot", PluginCacheDir: "/p/plugins", ModuleCacheDir: "/p/modules", VaultAddress: "http://127.0.0.1:18200", VaultMount: "secret",
		ConnectionTokenFile: "/p/ct", OutputTokenFile: "/p/ot", CleanupBudget: v2run.Duration{Duration: 10 * time.Minute},
	}
	hraw, _ := json.Marshal(handles)
	if err := v2run.WriteFilePrivate(e.handles, hraw); err != nil {
		t.Fatal(err)
	}
	kubectl := filepath.Join(dir, "kubectl")
	if err := os.WriteFile(kubectl, []byte(fakeKubectl), 0o700); err != nil {
		t.Fatal(err)
	}
	bin := func(p string) v2run.Binary { return v2run.Binary{Path: p, SHA256: strings.Repeat("a", 64)} }
	img := func(w string) v2run.Image {
		return v2run.Image{Ref: "r/" + w + ":m04", Digest: "sha256:" + strings.Repeat("b", 64)}
	}
	// The configuration Vault holds nothing of this run: every list answers 404.
	vaultSrv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(vaultSrv.Close)
	if err := v2run.WriteFilePrivate(filepath.Join(e.private, "secret-store.token"), []byte("scoped-token-value")); err != nil {
		t.Fatal(err)
	}
	if err := v2run.WriteFilePrivate(filepath.Join(e.private, "vault-snapshot.json"), []byte(`{"policies":[],"roles":[]}`)); err != nil {
		t.Fatal(err)
	}
	e.cfg = v2run.Config{
		RunID: "a01-0001", Scenario: "A01", Target: v2run.TargetK8s4f, Owner: "codex", EvidenceDir: filepath.Join(dir, "evidence"),
		Execution: v2run.Duration{Duration: 30 * time.Minute}, Cleanup: v2run.Duration{Duration: 10 * time.Minute},
		Binaries: map[string]v2run.Binary{"terraform": bin("/opt/t"), "score-k8s": bin("/opt/s"), "kubectl": bin(kubectl)},
		NodeArch: "amd64", Images: map[string]map[string]v2run.Image{"amd64": {"frontend": img("f"), "backend": img("b"), "worker": img("w")}},
		Kube: &v2run.KubeConfig{Context: v2run.KubeContextK8s4f, Kubeconfig: filepath.Join(e.private, "kubeconfig"), Namespace: "orch-v2-test-a01-0001",
			Sentinel: v2run.Sentinel{Namespace: "kube-public", Kind: "configmap", Name: "x", UID: "u1"}},
		PostgresAdminURLFile: "/p/a", VaultBin: "/opt/vault",
		SecretStore: &v2run.SecretStoreConfig{Name: "platform-vault", BackendAddress: vaultSrv.URL,
			WorkloadAddress: "http://vault.vault.svc:8200", TokenFile: filepath.Join(e.private, "secret-store.token"), Mount: "kv", AuthMount: "kubernetes"},
	}
	_ = os.WriteFile(e.cfg.Kube.Kubeconfig, []byte("synthetic"), 0o600)
	raw, _ := json.Marshal(e.cfg)
	e.cfgPath = filepath.Join(dir, "config.json")
	if err := v2run.WriteFilePrivate(e.cfgPath, raw); err != nil {
		t.Fatal(err)
	}
	e.mpath = filepath.Join(e.private, "manifest.json")
	m := v2run.NewManifest(e.cfg, time.Now())
	_ = m.RecordNamespace("orch-v2-test-a01-0001", 0, "orch-v2-test-a01-0001")
	_ = m.AddPin(v2run.Pin{Name: "terraform", Version: "1.9.8"})
	_ = m.AddPlanned("kubernetes_namespace_v1", 1)
	if err := m.Arm(); err != nil {
		t.Fatal(err)
	}
	if err := v2run.SaveManifest(e.mpath, m); err != nil {
		t.Fatal(err)
	}
	ns := fmt.Sprintf(`{"metadata":{"name":"orch-v2-test-a01-0001","labels":{"run-id":%q,"managed-by":%q}}}`, namespaceRunID, v2run.ProductManagedByTag)
	_ = os.WriteFile(filepath.Join(e.cluster, "ns.json"), []byte(ns), 0o600)
	_ = os.WriteFile(filepath.Join(e.cluster, "delivery"), []byte("deployment.apps/web\n"), 0o600)
	_ = os.WriteFile(filepath.Join(e.cluster, "delivery.json"), []byte(`{"items":[{"kind":"Deployment","metadata":{"name":"web","uid":"uid-web"}}]}`), 0o600)
	_ = os.WriteFile(filepath.Join(e.cluster, "app"), []byte("statefulset.apps/redis\n"), 0o600)
	_ = os.WriteFile(filepath.Join(e.cluster, "pv.items"), []byte(`{"metadata":{"name":"pv-1"},"spec":{"claimRef":{"namespace":"orch-v2-test-a01-0001"}}}`), 0o600)
	return e
}

func (e *env) ctl(args ...string) (string, int) {
	cmd := exec.Command(ctlBin, args...)
	cmd.Env = append(os.Environ(), "V2_KUBECTL="+e.cfg.Binaries["kubectl"].Path, "V2_KUBECONFIG="+e.cfg.Kube.Kubeconfig, "V2_KUBECONTEXT="+v2run.KubeContextK8s4f)
	out, err := cmd.CombinedOutput()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	}
	return string(out), code
}

func (e *env) order() string {
	raw, _ := os.ReadFile(filepath.Join(e.cluster, "order.log"))
	return strings.Join(strings.Fields(string(raw)), ",")
}

func startRunner(t *testing.T) *exec.Cmd {
	cmd := exec.Command("sleep", "300")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { _ = cmd.Wait() }()
	t.Cleanup(func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) })
	return cmd
}

// The supervisor, started by v2ctl before any create, survives kill -9 of the
// runner group and runs delivery -> app resources -> namespace through the
// real guard/delete/verify subprocesses.
func TestSupervisorStartKillRunnerRunsTheK8sReverseCleanup(t *testing.T) {
	e := newEnv(t, "a01-0001")
	runner := startRunner(t)
	out, code := e.ctl("supervisor-start", "-config", e.cfgPath, "-manifest", e.mpath, "-private", e.private, "-supervisor", supBin, "-cleanup", e.cleanup, "-handles", e.handles, "-runner-pid", fmt.Sprint(runner.Process.Pid))
	if code != 0 {
		t.Fatalf("supervisor-start: %s", out)
	}
	var armed struct{ PID, PGID, SID int }
	if err := json.Unmarshal([]byte(strings.ToLower(strings.TrimSpace(out))), &armed); err == nil {
		_ = armed
	}
	if own, _ := syscall.Getpgid(0); strings.Contains(out, fmt.Sprintf(`"pgid":%d`, own)) {
		t.Fatalf("supervisor shares the caller's process group: %s", out)
	}
	_ = syscall.Kill(-runner.Process.Pid, syscall.SIGKILL)
	out, code = e.ctl("supervisor-wait", "-private", e.private, "-timeout", "30s")
	if code != 0 {
		t.Fatalf("cleanup: %s", out)
	}
	if got := e.order(); got != "delivery,app-resources,namespace" {
		t.Fatalf("order %s", got)
	}
	if _, err := os.Stat(filepath.Join(e.cluster, "ns.json")); !os.IsNotExist(err) {
		t.Fatal("namespace remains")
	}
	if out, code := e.ctl("k8s-absent", "-config", e.cfgPath, "-manifest", e.mpath); code != 0 {
		t.Fatalf("absence not verified: %s", out)
	}
	if out, code := e.ctl("dirty", "-root", e.dir); code != 0 {
		t.Fatalf("a clean run reported dirty: %s", out)
	}
}

func TestForeignNamespaceIsNeverDeletedAndLeavesTheRunDirty(t *testing.T) {
	e := newEnv(t, "someone-else")
	runner := startRunner(t)
	if out, code := e.ctl("supervisor-start", "-config", e.cfgPath, "-manifest", e.mpath, "-private", e.private, "-supervisor", supBin, "-cleanup", e.cleanup, "-handles", e.handles, "-runner-pid", fmt.Sprint(runner.Process.Pid)); code != 0 {
		t.Fatal(out)
	}
	_ = syscall.Kill(-runner.Process.Pid, syscall.SIGKILL)
	out, code := e.ctl("supervisor-wait", "-private", e.private, "-timeout", "30s")
	// Every step is verified NOT owned: nothing is deleted and nothing is failed.
	if code != 0 {
		t.Fatalf("%s", out)
	}
	if e.order() != "" {
		t.Fatalf("deleted something of a foreign namespace: %s", e.order())
	}
	if _, err := os.Stat(filepath.Join(e.cluster, "ns.json")); err != nil {
		t.Fatal("foreign namespace was deleted")
	}
	// ...but it is not verified absent either, so the independent absence check fails.
	if _, code := e.ctl("k8s-absent", "-config", e.cfgPath, "-manifest", e.mpath); code == 0 {
		t.Fatal("absence check passed while the namespace still exists")
	}
}

func TestSupervisorStartRefusesAnUnarmedManifestAndAWSBeforeI0411(t *testing.T) {
	e := newEnv(t, "a01-0001")
	runner := startRunner(t)
	// An AWS config cannot start before I04-11 passed and was cleaned.
	aws := e.cfg
	aws.Scenario, aws.Target, aws.Kube, aws.NodeArch = "A03", v2run.TargetAWS, nil, "arm64"
	aws.Execution = v2run.Duration{Duration: 180 * time.Minute}
	aws.Binaries["aws"] = v2run.Binary{Path: "/opt/aws", SHA256: strings.Repeat("a", 64)}
	aws.Binaries["docker"] = v2run.Binary{Path: "/opt/docker", SHA256: strings.Repeat("a", 64)}
	aws.Images["arm64"] = aws.Images["amd64"]
	aws.AWS = &v2run.AWSConfig{AccountID: v2run.AWSAccountID, Region: v2run.AWSRegion, PrincipalARN: "arn:aws:iam::452025861381:user/x", ExecutorEgress: "203.0.113.9/32", NodeInstanceType: "t4g.medium", AccessKeyFile: "/p/key.json", EstimateFile: "/p/estimate.json", ECRRepositoryPrefix: "orch-acceptance", ProbePostgresImage: v2run.Image{Ref: "postgres:16-alpine", Digest: "sha256:" + strings.Repeat("c", 64)}, ProbeRedisImage: v2run.Image{Ref: "redis:7.4-alpine", Digest: "sha256:" + strings.Repeat("d", 64)}}
	aws.EvidenceDir = filepath.Join(e.dir, "evidence-aws")
	raw, _ := json.Marshal(aws)
	path := filepath.Join(e.dir, "aws.json")
	_ = v2run.WriteFilePrivate(path, raw)
	out, code := e.ctl("supervisor-start", "-config", path, "-manifest", e.mpath, "-private", e.private, "-supervisor", supBin, "-cleanup", e.cleanup, "-handles", e.handles, "-runner-pid", fmt.Sprint(runner.Process.Pid))
	if code == 0 || !strings.Contains(out, "completed integrated V04 predecessor receipt") {
		t.Fatalf("AWS start was not refused: %s", out)
	}
}

func TestDirtyAndScanGuards(t *testing.T) {
	root := t.TempDir()
	run := filepath.Join(root, "run-1", "private")
	_ = os.MkdirAll(run, 0o700)
	_ = os.WriteFile(filepath.Join(run, "supervisor-spec.json"), []byte("{}"), 0o600)
	dirty, err := v2run.DirtyRuns(root)
	if err != nil || len(dirty) != 1 {
		t.Fatalf("%v %v", dirty, err)
	}
	st, _ := json.Marshal(v2run.Status{RunID: "r", State: v2run.StateClean})
	_ = os.WriteFile(filepath.Join(run, "supervisor-status.json"), st, 0o600)
	if dirty, _ = v2run.DirtyRuns(root); len(dirty) != 1 || dirty[0] != "run-1" {
		t.Fatalf("supervisor CLEAN without final runner closure must remain dirty: %v", dirty)
	}
	st, _ = json.Marshal(v2run.Status{RunID: "r", State: v2run.StateAttention})
	_ = os.WriteFile(filepath.Join(run, "supervisor-status.json"), st, 0o600)
	if dirty, _ = v2run.DirtyRuns(root); len(dirty) != 1 {
		t.Fatal("a legacy NEEDS_ATTENTION run must block a new run")
	}
	markerDir := filepath.Dir(run)
	if err := v2run.WriteRunMarker(markerDir, v2run.RunMarker{RunID: "r", Target: v2run.TargetK8s4f, State: v2run.MarkerStarted}); err != nil {
		t.Fatal(err)
	}
	if dirty, _ = v2run.DirtyRuns(root); len(dirty) != 1 || dirty[0] != "run-1" {
		t.Fatalf("an open marker must remain dirty regardless of supervisor status: %v", dirty)
	}
	if err := v2run.WriteRunMarker(markerDir, v2run.RunMarker{RunID: "r", Target: v2run.TargetK8s4f, State: v2run.MarkerClosedClean}); err != nil {
		t.Fatal(err)
	}
	if dirty, _ = v2run.DirtyRuns(root); len(dirty) != 0 {
		t.Fatalf("explicit final closure should clear cleanup dirtiness: %v", dirty)
	}

	public := t.TempDir()
	secretFile := filepath.Join(t.TempDir(), "console.url")
	_ = os.WriteFile(secretFile, []byte("postgres://console:S3cretPassw0rd@127.0.0.1/db"), 0o600)
	_ = os.WriteFile(filepath.Join(public, "ok.log"), []byte("nothing here"), 0o600)
	if n, err := v2run.ScanPublic(public, []string{secretFile}); err != nil || n != 1 {
		t.Fatalf("%d %v", n, err)
	}
	_ = os.WriteFile(filepath.Join(public, "leak.log"), []byte("pw=S3cretPassw0rd"), 0o600)
	if _, err := v2run.ScanPublic(public, []string{secretFile}); err == nil || strings.Contains(err.Error(), "S3cretPassw0rd") {
		t.Fatalf("leak not caught safely: %v", err)
	}
	_ = os.Remove(filepath.Join(public, "leak.log"))
	_ = os.WriteFile(filepath.Join(public, "key.txt"), []byte("AKIAABCDEFGHIJKLMNOP"), 0o600)
	if _, err := v2run.ScanPublic(public, []string{secretFile}); err == nil {
		t.Fatal("AWS key pattern not caught")
	}
	if _, err := v2run.ScanPublic(public, nil); err == nil {
		t.Fatal("a scan without secret values proves nothing and must fail")
	}
}

// This subprocess fixture exercises operator wiring only; it is not an AWS run.
func TestOwnedAWSProbeRemovalKeepsOriginalUID(t *testing.T) {
	e := newEnv(t, "a01-0001")
	c := e.cfg
	c.Scenario, c.Target, c.Kube, c.NodeArch = "A03", v2run.TargetAWS, nil, "arm64"
	c.Execution = v2run.Duration{Duration: 180 * time.Minute}
	c.Binaries["aws"] = v2run.Binary{Path: "/opt/aws", SHA256: strings.Repeat("a", 64)}
	c.Binaries["docker"] = v2run.Binary{Path: "/opt/docker", SHA256: strings.Repeat("a", 64)}
	c.Images["arm64"] = c.Images["amd64"]
	c.AWS = &v2run.AWSConfig{AccountID: v2run.AWSAccountID, Region: v2run.AWSRegion, PrincipalARN: "arn:aws:iam::452025861381:user/x", ExecutorEgress: "203.0.113.9/32", NodeInstanceType: "t4g.medium", AccessKeyFile: "/p/key.json", EstimateFile: "/p/estimate.json", ECRRepositoryPrefix: "orch-acceptance", ProbePostgresImage: v2run.Image{Ref: "postgres:16-alpine", Digest: "sha256:" + strings.Repeat("c", 64)}, ProbeRedisImage: v2run.Image{Ref: "redis:7.4-alpine", Digest: "sha256:" + strings.Repeat("d", 64)}}
	raw, _ := json.Marshal(c)
	if err := v2run.WriteFilePrivate(e.cfgPath, raw); err != nil {
		t.Fatal(err)
	}
	uid := "c3efcf94-c21a-4f44-9620-e9ded4c580bb"
	if err := v2run.UpdateManifest(e.mpath, func(m *v2run.Manifest) error {
		if err := m.AddAsset(v2run.Asset{Kind: "eks-cluster", ID: "owned-cluster", Layer: v2run.LayerNamespace, Owner: v2run.OwnerCreatedByRun}); err != nil {
			return err
		}
		return m.AddAsset(v2run.Asset{Kind: "pod", ID: m.Namespace + "/tech-owned", Layer: v2run.LayerDelivery, Owner: v2run.OwnerCreatedByRun})
	}); err != nil {
		t.Fatal(err)
	}
	if out, code := e.ctl("manifest-asset", "-manifest", e.mpath, "-kind", "pod", "-id", "orch-v2-test-a01-0001/tech-owned", "-layer", v2run.LayerDelivery, "-owner", v2run.OwnerCreatedByRun, "-uid", uid); code != 0 {
		t.Fatal(out)
	}
	m, err := v2run.LoadManifest(e.mpath)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range m.Assets {
		if a.Kind == "pod" {
			found = a.UID == uid
		}
	}
	if !found {
		t.Fatal("CLI did not persist creation UID")
	}
	if err := os.WriteFile(filepath.Join(e.private, "eks.kubeconfig"), []byte("synthetic"), 0600); err != nil {
		t.Fatal(err)
	}
	pod := `{"metadata":{"name":"tech-owned","namespace":"orch-v2-test-a01-0001","uid":"` + uid + `","labels":{"run-id":"a01-0001","managed-by":"orchestrator-verification","app.kubernetes.io/component":"technical-probe"}}}`
	if err := os.WriteFile(filepath.Join(e.cluster, "probe.json"), []byte(pod), 0600); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
set -eu
C="$(dirname "$0")/cluster"
shift 4
if [ "$1" = delete ]; then
 [ "$2" = --raw ]
 [ "$3" = /api/v1/namespaces/orch-v2-test-a01-0001/pods/tech-owned ]
 [ "$4" = -f ] && [ "$5" = - ]
 cat > "$C/delete-options.json"
 rm "$C/probe.json"
 exit 0
fi
[ "$1" = -n ] && [ "$2" = orch-v2-test-a01-0001 ]
shift 2
if [ "$1" = wait ]; then [ ! -e "$C/probe.json" ]; exit; fi
[ "$1" = get ]
if [ "$2" = pods ]; then
 printf '{"items":['
 [ ! -e "$C/probe.json" ] || cat "$C/probe.json"
 printf ']}'
 exit 0
fi
[ "$2" = pod ] && [ "$3" = tech-owned ]
echo 'Error from server (NotFound): pods "tech-owned" not found' >&2
exit 1
`
	if err := os.WriteFile(c.Binaries["kubectl"].Path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	if out, code := e.ctl("aws-probes-clean", "-config", e.cfgPath, "-manifest", e.mpath); code != 0 {
		t.Fatal(out)
	}
	options, err := os.ReadFile(filepath.Join(e.cluster, "delete-options.json"))
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		APIVersion    string `json:"apiVersion"`
		Kind          string `json:"kind"`
		Preconditions struct {
			UID string `json:"uid"`
		} `json:"preconditions"`
	}
	if err := json.Unmarshal(options, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.APIVersion != "v1" || payload.Kind != "DeleteOptions" || payload.Preconditions.UID != uid {
		t.Fatalf("delete lost original UID: %s", options)
	}
}
