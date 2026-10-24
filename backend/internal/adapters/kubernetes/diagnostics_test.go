package kubernetes

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	domain "orchestrator/internal/domain/diagnostics"
	probes "orchestrator/internal/ports/diagnostics"
)

// fakeKubectl is a scripted kubectl binary. It runs through the real CLI
// transport (argument building, stderr classification, private files), so these
// tests prove how probes interpret kubectl answers, not any cluster's state.
type kubectlRule struct {
	match  string // substring of the argument list
	cond   string // optional shell test that must succeed
	out    string
	stderr string
	code   int
	touch  string // marker file created when the rule matches
}

type fakeKube struct {
	cli  CLI
	log  string
	dir  string
	mark func(name string) string
}

func newFakeKube(t *testing.T, rules []kubectlRule) *fakeKube {
	t.Helper()
	dir := t.TempDir()
	var script strings.Builder
	script.WriteString("#!/bin/sh\necho \"$*\" >> " + filepath.Join(dir, "calls.log") + "\n")
	mark := func(name string) string { return filepath.Join(dir, name) }
	for i, r := range rules {
		outFile := filepath.Join(dir, fmt.Sprintf("out%d", i))
		if err := os.WriteFile(outFile, []byte(r.out), 0o600); err != nil {
			t.Fatal(err)
		}
		errFile := filepath.Join(dir, fmt.Sprintf("err%d", i))
		if err := os.WriteFile(errFile, []byte(r.stderr), 0o600); err != nil {
			t.Fatal(err)
		}
		body := "cat " + outFile + "; cat " + errFile + " >&2;"
		if r.touch != "" {
			body = "touch " + mark(r.touch) + "; " + body
		}
		test := "case \"$*\" in *'" + r.match + "'*) true ;; *) false ;; esac"
		if r.cond != "" {
			test += " && " + strings.ReplaceAll(r.cond, "$DIR", dir)
		}
		script.WriteString("if " + test + "; then " + body + " exit " + fmt.Sprint(r.code) + "; fi\n")
	}
	script.WriteString("echo \"unexpected kubectl call\" >&2\nexit 1\n")
	bin := filepath.Join(dir, "kubectl")
	if err := os.WriteFile(bin, []byte(script.String()), 0o700); err != nil {
		t.Fatal(err)
	}
	kubeconfig := filepath.Join(dir, "config")
	if err := os.WriteFile(kubeconfig, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	return &fakeKube{cli: NewCLI(bin, "ctx", kubeconfig), log: filepath.Join(dir, "calls.log"), dir: dir, mark: mark}
}

func (f *fakeKube) calls(t *testing.T) string {
	t.Helper()
	b, _ := os.ReadFile(f.log)
	return string(b)
}

func (f *fakeKube) session() *kubeSession {
	return &kubeSession{cli: f.cli, cleanup: func() {}, org: "acme", account: "lab", scope: domain.Scope{Purpose: domain.PurposeUseCase, Mode: domain.ModeExecution, UseCases: []string{"database"}}, nsName: "orch-diag-aaaaaaaaaaaa"}
}

func ids(checks []probes.Check) map[string]bool {
	out := map[string]bool{}
	for _, c := range checks {
		out[c.ID] = true
	}
	return out
}

func scopeOf(purpose domain.Purpose, mode domain.Mode, uses ...string) domain.Scope {
	return domain.Scope{Purpose: purpose, Mode: mode, UseCases: uses, AccountType: "INTERNAL"}
}

func TestPlanCreatesProbeObjectsOnlyForSelectedExecutableWork(t *testing.T) {
	p := Diagnostics{}
	secretsOnly := ids(p.Plan(scopeOf(domain.PurposeUseCase, domain.ModeExecution, "secrets", "https"), nil))
	for _, forbidden := range []string{"k8s.exec.namespace", "k8s.exec.workload", "k8s.exec.volume", "k8s.exec.cleanup", "k8s.storage.classes", "k8s.nodes"} {
		if secretsOnly[forbidden] {
			t.Errorf("secrets/https execution plans %s", forbidden)
		}
	}
	if !secretsOnly["k8s.exec.none"] || !secretsOnly["k8s.vso.crd"] || !secretsOnly["k8s.route.controller"] {
		t.Errorf("secrets/https plan %v", secretsOnly)
	}
	unresolved := ids(p.Plan(scopeOf(domain.PurposeUseCase, domain.ModeExecution, "database"), nil))
	if unresolved["k8s.exec.namespace"] || unresolved["k8s.exec.cleanup"] || !unresolved["k8s.exec.volume"] || unresolved["k8s.exec.workload"] {
		t.Errorf("unresolved storage must not create a namespace: %v", unresolved)
	}
	resolved := &probes.Requirements{Resolved: true, Resources: []probes.ResourceNeed{{Type: "postgres", Cluster: "lab", Storage: probes.StorageNeed{Required: true, Resolved: true}}}}
	withStorage := scopeOf(domain.PurposeUseCase, domain.ModeExecution, "database")
	withStorage.AccountKey = "lab"
	database := ids(p.Plan(withStorage, resolved))
	if !database["k8s.exec.namespace"] || !database["k8s.exec.volume"] || !database["k8s.exec.cleanup"] || database["k8s.exec.workload"] {
		t.Errorf("database execution plan %v", database)
	}
	deploy := ids(p.Plan(scopeOf(domain.PurposeUseCase, domain.ModeExecution, "deploy"), nil))
	if !deploy["k8s.exec.workload"] || deploy["k8s.exec.volume"] || deploy["k8s.storage.classes"] {
		t.Errorf("deploy execution plan %v", deploy)
	}
	conditions := ids(p.Plan(scopeOf(domain.PurposeUseCase, domain.ModeConditions, "database"), nil))
	for id := range conditions {
		if strings.HasPrefix(id, "k8s.exec.") {
			t.Errorf("conditions plan has %s", id)
		}
	}
	general := ids(p.Plan(scopeOf(domain.PurposeGeneral, domain.ModeConditions), nil))
	if !general["k8s.controllers"] || !general["k8s.storage.classes"] || general["k8s.vso.crd"] || general["k8s.route.controller"] {
		t.Errorf("general plan %v", general)
	}
}

func TestStorageInventoryDoesNotFailAStatelessAccount(t *testing.T) {
	empty := newFakeKube(t, []kubectlRule{{match: "get storageclass", out: `{"items":[]}`}})
	f := empty.session().storageClasses(context.Background())
	if f.Verdict != domain.VerdictPass || f.Facts["storageClasses"] != "0" {
		t.Fatalf("zero classes: %+v", f)
	}
	if p := empty.session().provisioner(context.Background(), nil); p.Verdict != domain.VerdictNA {
		t.Fatalf("provisioner without classes: %+v", p)
	}
	csi := newFakeKube(t, []kubectlRule{
		{match: "get storageclass", out: `{"items":[{"metadata":{"name":"fast","annotations":{"storageclass.kubernetes.io/is-default-class":"true"}},"provisioner":"ebs.csi.aws.com"}]}`},
		{match: "get csinodes", out: `{"items":[{"spec":{"drivers":[{"name":"ebs.csi.aws.com"}]}}]}`},
	})
	if p := csi.session().provisioner(context.Background(), nil); p.Verdict != domain.VerdictUnknown || p.Reason != "CSI_CONTROLLER_UNOBSERVED" {
		t.Fatalf("a registered node plugin is not controller proof: %+v", p)
	}
	unknown := newFakeKube(t, []kubectlRule{
		{match: "get storageclass", out: `{"items":[{"metadata":{"name":"x","annotations":{"storageclass.kubernetes.io/is-default-class":"true"}},"provisioner":"example.com/custom"}]}`},
		{match: "get csinodes", out: `{"items":[]}`},
	})
	if p := unknown.session().provisioner(context.Background(), nil); p.Verdict != domain.VerdictUnknown || p.Reason != "PROVISIONER_UNOBSERVABLE" {
		t.Fatalf("an unobserved provisioner is not proof it is broken: %+v", p)
	}
}

func TestUnreadablePodIsUnknownNotAFailure(t *testing.T) {
	forbidden := newFakeKube(t, []kubectlRule{{match: "get pod", stderr: `Error from server (Forbidden): pods "p" is forbidden`, code: 1}})
	reason, _ := forbidden.session().waitPod(context.Background(), "p", 30*time.Second)
	if reason != "READ_FORBIDDEN" {
		t.Fatalf("forbidden read: %s", reason)
	}
	if f := waitVerdict(reason, nil, "x"); f.Verdict != domain.VerdictUnknown {
		t.Fatalf("verdict %+v", f)
	}
	flaky := newFakeKube(t, []kubectlRule{{match: "get pod", stderr: "Unable to connect to the server: dial tcp: i/o timeout", code: 1}})
	reason, _ = flaky.session().waitPod(context.Background(), "p", 30*time.Second)
	if reason != "READ_FAILED" {
		t.Fatalf("transient read failures: %s", reason)
	}
	pending := newFakeKube(t, []kubectlRule{{match: "get pod", out: `{"status":{"phase":"Pending","containerStatuses":[{"state":{"waiting":{"reason":"ImagePullBackOff"}}}]}}`}})
	reason, _ = pending.session().waitPod(context.Background(), "p", 30*time.Second)
	if f := waitVerdict(reason, nil, "x"); f.Verdict != domain.VerdictFail || f.Reason != "IMAGE_PULL_FAILED" {
		t.Fatalf("an observed pull failure is a FAIL: %+v", f)
	}
}

const (
	nsUID  = "11111111-2222-3333-4444-555555555555"
	pvcUID = "66666666-7777-8888-9999-000000000000"
	pvUID  = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
)

func ownedNamespace(run string) string {
	return `{"metadata":{"uid":"` + nsUID + `","labels":{"app.kubernetes.io/managed-by":"orchestrator-diagnostics","orchestrator.io/diagnostic-run":"` + run + `","orchestrator.io/organization":"acme","orchestrator.io/account":"lab"}}}`
}

func resources() []domain.ProbeResource {
	return []domain.ProbeResource{
		{Kind: "namespace", Name: "orch-diag-aaaaaaaaaaaa", UID: nsUID, State: domain.ResourceCreated},
		{Kind: "persistentvolumeclaim", Namespace: "orch-diag-aaaaaaaaaaaa", Name: "probe-data-1", UID: pvcUID, State: domain.ResourceCreated},
	}
}

func byKind(rs []domain.ProbeResource) map[string]domain.ProbeResource {
	out := map[string]domain.ProbeResource{}
	for _, r := range rs {
		out[r.Kind] = r
	}
	return out
}

func pvJSON(policy string) string {
	return `{"items":[{"metadata":{"name":"pvc-` + pvUID + `","uid":"` + pvUID + `"},"spec":{"persistentVolumeReclaimPolicy":"` + policy + `","claimRef":{"namespace":"orch-diag-aaaaaaaaaaaa","name":"probe-data-1","uid":"` + pvcUID + `"}}}]}`
}

func TestCleanupVerifiesBackingVolumesAndNeverDeletesOneByHand(t *testing.T) {
	volumeWait = 2 * time.Second
	defer func() { volumeWait = 90 * time.Second }()
	ctx := context.Background()
	const run = "run-1"

	// The provisioner removes the volume after the namespace is gone: verified.
	removed := newFakeKube(t, []kubectlRule{
		{match: "get namespace orch-diag-aaaaaaaaaaaa", cond: "[ ! -f $DIR/ns-deleted ]", out: ownedNamespace(run)},
		{match: "get namespace orch-diag-aaaaaaaaaaaa", stderr: `Error from server (NotFound): namespaces "x" not found`, code: 1},
		{match: "delete --raw /api/v1/namespaces/orch-diag-aaaaaaaaaaaa", touch: "ns-deleted"},
		{match: "get persistentvolumes", out: pvJSON("Delete")},
		{match: "get persistentvolume pvc-" + pvUID, stderr: `Error from server (NotFound): persistentvolumes "x" not found`, code: 1},
	})
	got := removed.session().Cleanup(ctx, resources(), run)
	k := byKind(got)
	if !domain.AllSettled(got) || k["persistentvolume"].State != domain.ResourceDeleted || k["persistentvolume"].UID != pvUID || k["namespace"].State != domain.ResourceDeleted {
		t.Fatalf("verified cleanup: %+v", got)
	}

	// A Retain volume stays behind: the backing storage is not claimed clean and
	// is never deleted by hand.
	retained := newFakeKube(t, []kubectlRule{
		{match: "get namespace orch-diag-aaaaaaaaaaaa", cond: "[ ! -f $DIR/ns-deleted ]", out: ownedNamespace(run)},
		{match: "get namespace orch-diag-aaaaaaaaaaaa", stderr: `(NotFound) not found`, code: 1},
		{match: "delete --raw /api/v1/namespaces/orch-diag-aaaaaaaaaaaa", touch: "ns-deleted"},
		{match: "get persistentvolumes", out: pvJSON("Retain")},
		{match: "get persistentvolume pvc-" + pvUID, out: `{"metadata":{"uid":"` + pvUID + `"}}`},
	})
	got = retained.session().Cleanup(ctx, resources(), run)
	if domain.AllSettled(got) || byKind(got)["persistentvolume"].State != domain.ResourceUnknown {
		t.Fatalf("a retained volume must stay unresolved: %+v", got)
	}
	if strings.Contains(retained.calls(t), "persistentvolumes/") {
		t.Fatalf("a volume was deleted by hand:\n%s", retained.calls(t))
	}

	// If volumes cannot be listed, the claim is not shown clean either.
	blind := newFakeKube(t, []kubectlRule{
		{match: "get namespace orch-diag-aaaaaaaaaaaa", cond: "[ ! -f $DIR/ns-deleted ]", out: ownedNamespace(run)},
		{match: "get namespace orch-diag-aaaaaaaaaaaa", stderr: `(NotFound) not found`, code: 1},
		{match: "delete --raw /api/v1/namespaces/orch-diag-aaaaaaaaaaaa", touch: "ns-deleted"},
		{match: "get persistentvolumes", stderr: `Error from server (Forbidden): forbidden`, code: 1},
	})
	got = blind.session().Cleanup(ctx, resources(), run)
	if domain.AllSettled(got) || byKind(got)["persistentvolumeclaim"].State != domain.ResourceUnknown {
		t.Fatalf("unobservable volumes: %+v", got)
	}
}

func TestCleanupPreservesForeignAndReplacedNamespaces(t *testing.T) {
	ctx := context.Background()
	foreign := newFakeKube(t, []kubectlRule{{match: "get namespace orch-diag-aaaaaaaaaaaa", out: ownedNamespace("another-run")}})
	got := foreign.session().Cleanup(ctx, resources(), "run-1")
	if byKind(got)["namespace"].State != domain.ResourcePreserved || strings.Contains(foreign.calls(t), "delete") {
		t.Fatalf("foreign namespace: %+v\n%s", got, foreign.calls(t))
	}
	if byKind(got)["persistentvolumeclaim"].State != domain.ResourcePreserved {
		t.Fatalf("children of a preserved namespace: %+v", got)
	}
	replaced := newFakeKube(t, []kubectlRule{{match: "get namespace orch-diag-aaaaaaaaaaaa", out: strings.Replace(ownedNamespace("run-1"), nsUID, "99999999-2222-3333-4444-555555555555", 1)}})
	got = replaced.session().Cleanup(ctx, resources(), "run-1")
	if byKind(got)["namespace"].State != domain.ResourcePreserved || strings.Contains(replaced.calls(t), "delete") {
		t.Fatalf("same-name replacement: %+v", got)
	}
	unreadable := newFakeKube(t, []kubectlRule{{match: "get namespace", stderr: "Unable to connect to the server", code: 1}})
	got = unreadable.session().Cleanup(ctx, resources(), "run-1")
	if domain.AllSettled(got) || byKind(got)["namespace"].State != domain.ResourceUnknown {
		t.Fatalf("unreadable target: %+v", got)
	}
}

func TestExecVolumeTestsTheRequestedClassAndRefusesUnsafePaths(t *testing.T) {
	scope := domain.Scope{Purpose: domain.PurposeUseCase, Mode: domain.ModeExecution, UseCases: []string{"database"}}
	need := func(storage probes.StorageNeed) *probes.Requirements {
		return &probes.Requirements{Resolved: true, Resources: []probes.ResourceNeed{{Descriptor: "d", Type: "postgres", Providers: []probes.ProviderTarget{{Kind: "kubernetes", Connection: "lab"}}, Storage: storage}}}
	}
	run := func(kube *fakeKube, req *probes.Requirements) domain.Finding {
		s := kube.session()
		s.scope = scope
		return s.execVolume(context.Background(), &recorder{}, req)
	}
	classes := `{"items":[{"metadata":{"name":"standard","annotations":{"storageclass.kubernetes.io/is-default-class":"true"}},"reclaimPolicy":"Delete"},{"metadata":{"name":"keep"},"reclaimPolicy":"Retain"}]}`
	kube := newFakeKube(t, []kubectlRule{{match: "get storageclass", out: classes}})

	if f := run(kube, nil); f.Reason != "STORAGE_REQUIREMENT_UNRESOLVED" || f.Verdict != domain.VerdictUnknown {
		t.Fatalf("no context: %+v", f)
	}
	if f := run(kube, need(probes.StorageNeed{Required: true})); f.Reason != "STORAGE_CLASS_UNRESOLVED" || f.Verdict != domain.VerdictUnknown {
		t.Fatalf("an expression is not the default class: %+v", f)
	}
	if f := run(kube, need(probes.StorageNeed{})); f.Reason != "STORAGE_REQUIREMENT_UNDECLARED" {
		t.Fatalf("undeclared storage: %+v", f)
	}
	if f := run(kube, need(probes.StorageNeed{Required: true, Resolved: true, Class: "absent"})); f.Reason != "STORAGE_CLASS_MISSING" || f.Verdict != domain.VerdictFail {
		t.Fatalf("missing class: %+v", f)
	}
	if f := run(kube, need(probes.StorageNeed{Required: true, Resolved: true, Class: "keep"})); f.Reason != "EXECUTION_UNSAFE_RECLAIM_POLICY" || f.Verdict != domain.VerdictUnknown {
		t.Fatalf("retain class: %+v", f)
	}
	if strings.Contains(kube.calls(t), "create") {
		t.Fatalf("an unresolved or unsafe requirement created objects:\n%s", kube.calls(t))
	}
}

type recorder struct{ got []domain.ProbeResource }

func (r *recorder) Record(_ context.Context, p domain.ProbeResource) error {
	r.got = append(r.got, p)
	return nil
}
func (r *recorder) RunID() string { return "run-1" }
