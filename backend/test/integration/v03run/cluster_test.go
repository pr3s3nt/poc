package v03run

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"orchestrator/test/integration/v2run"
)

// fakeKube is an in-memory cluster behind the Kubectl interface.
type fakeKube struct {
	namespaces map[string]fakeNS
	deploy     map[string]string // "ns/name" -> deployment JSON
	services   map[string]bool
	deleted    []string
	nodesReady bool
	fail       map[string]bool
	// replaceOnDelete simulates a namespace replaced between the ownership read and the delete.
	replaceOnDelete bool
	deleteBodies    []string
	pods            map[string]string // namespace -> pod list JSON
	// hideGet makes `get namespace NAME` fail with an API error (not NotFound).
	hideGet bool
}

type fakeNS struct {
	uid     string
	managed bool
	labels  map[string]string
}

const testRun = "v03-test-run"

// productLabels are the exact tags the product stamps on a namespace it created.
func productLabels(instance, terraformRun string) map[string]string {
	return map[string]string{"orchestrator-managed": "true", "run-id": testRun, "managed-by": v2run.ProductManagedByTag,
		"orchestrator-instance": instance, "orchestrator-created-by-run": terraformRun}
}

func (ns fakeNS) allLabels() map[string]string {
	labels := map[string]string{}
	if ns.managed {
		labels["orchestrator-managed"] = "true"
	}
	for k, v := range ns.labels {
		labels[k] = v
	}
	return labels
}

func (f *fakeKube) Run(_ context.Context, args ...string) ([]byte, string, int) {
	joined := strings.Join(args, " ")
	switch {
	case joined == "get nodes -o json":
		status := "False"
		if f.nodesReady {
			status = "True"
		}
		return []byte(`{"items":[{"status":{"conditions":[{"type":"Ready","status":"` + status + `"}]}}]}`), "", 0
	case joined == "get namespaces -o json":
		if f.fail["list"] {
			return nil, "boom", 1
		}
		items := []map[string]any{}
		for name, ns := range f.namespaces {
			items = append(items, map[string]any{"metadata": map[string]any{"name": name, "uid": ns.uid, "labels": ns.allLabels()}})
		}
		raw, _ := json.Marshal(map[string]any{"items": items})
		return raw, "", 0
	case strings.HasPrefix(joined, "get namespace ") && strings.HasSuffix(joined, " -o json"):
		name := strings.Fields(joined)[2]
		if f.hideGet {
			return nil, "connection refused", 1
		}
		ns, ok := f.namespaces[name]
		if !ok {
			return nil, `namespaces "x" not found`, 1
		}
		raw, _ := json.Marshal(map[string]any{"metadata": map[string]any{"uid": ns.uid, "labels": ns.allLabels()}})
		return raw, "", 0
	case strings.HasPrefix(joined, "delete --raw /api/v1/namespaces/"):
		fields := strings.Fields(joined)
		name := strings.TrimPrefix(fields[2], "/api/v1/namespaces/")
		body, err := os.ReadFile(fields[4])
		if err != nil {
			return nil, "no body", 1
		}
		f.deleteBodies = append(f.deleteBodies, string(body))
		var options struct {
			Preconditions struct {
				UID string `json:"uid"`
			} `json:"preconditions"`
		}
		_ = json.Unmarshal(body, &options)
		ns, ok := f.namespaces[name]
		if f.replaceOnDelete && ok {
			ns.uid, ok = "u-replacement", true
			f.namespaces[name] = ns
		}
		if !ok {
			return nil, `namespaces "x" not found (NotFound)`, 1
		}
		if options.Preconditions.UID == "" || options.Preconditions.UID != ns.uid {
			return nil, "Precondition failed: UID in precondition differs", 1
		}
		f.deleted = append(f.deleted, name)
		delete(f.namespaces, name)
		return nil, "", 0
	case strings.Contains(joined, " get deployment "):
		fields := strings.Fields(joined)
		doc, ok := f.deploy[fields[1]+"/"+fields[4]]
		if !ok {
			return nil, "not found", 1
		}
		return []byte(doc), "", 0
	case strings.Contains(joined, " get pods -l "):
		doc, ok := f.pods[strings.Fields(joined)[1]]
		if !ok {
			return []byte(`{"items":[]}`), "", 0
		}
		return []byte(doc), "", 0
	case strings.Contains(joined, " get service "):
		fields := strings.Fields(joined)
		if f.services[fields[1]+"/"+fields[4]] {
			return nil, "", 0
		}
		return nil, "not found", 1
	}
	return nil, "unexpected " + joined, 2
}

// deployment builds a Deployment document; ready is both the available and ready replica count,
// observed false simulates a controller that has not seen the latest generation.
func deployment(image, backend string, ready int) string {
	return deploymentGen(image, backend, ready, true)
}

func deploymentGen(image, backend string, ready int, observed bool) string {
	obs := "2"
	if !observed {
		obs = "1"
	}
	return `{"metadata":{"generation":2},"spec":{"replicas":1,"selector":{"matchLabels":{"app":"frontend"}},"template":{"spec":{"containers":[{"image":"` + image + `","env":[{"name":"BACKEND_URL","value":"` + backend + `"},{"name":"PORT","value":"8080"}]}]}}},"status":{"observedGeneration":` + obs + `,"replicas":1,"updatedReplicas":1,"readyReplicas":` + string(rune('0'+ready)) + `,"availableReplicas":` + string(rune('0'+ready)) + `}}`
}

const digestA = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const digestB = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

// podList is one Running, Ready pod whose running image has the given image ID.
func podList(imageID string, terminating bool) string {
	deleted := ""
	if terminating {
		deleted = `"deletionTimestamp":"2026-10-18T00:00:00Z",`
	}
	return `{"items":[{"metadata":{` + deleted + `"name":"frontend-1"},"status":{"phase":"Running","conditions":[{"type":"Ready","status":"True"}],"containerStatuses":[{"ready":true,"imageID":"` + imageID + `"}]}}]}`
}

func baselineCluster() *fakeKube {
	return &fakeKube{nodesReady: true, services: map[string]bool{}, deploy: map[string]string{}, namespaces: map[string]fakeNS{
		"default": {uid: "u-default"}, "kube-system": {uid: "u-system"}, "shared": {uid: "u-shared"},
	}}
}

func TestValidateRunNamespacesRefusesSharedAndReservedNames(t *testing.T) {
	for _, bad := range [][]string{nil, {"default"}, {"kube-system"}, {"kube-foo"}, {"Upper"}, {"a", "a"}, {""}} {
		if err := ValidateRunNamespaces(bad); err == nil {
			t.Fatalf("%v must be refused", bad)
		}
	}
	if err := ValidateRunNamespaces([]string{"orch-v3-a", "orch-v3-a-aux"}); err != nil {
		t.Fatal(err)
	}
}

func TestPreflightBaselinesTheClusterAndRefusesAnExistingRunNamespace(t *testing.T) {
	k := baselineCluster()
	base, checks, err := Preflight(context.Background(), k, "kind-k8s4f", []string{"orch-v3-a", "orch-v3-a-aux"})
	if err != nil || len(base.Namespaces) != 3 || base.Namespaces["shared"] != "u-shared" || len(checks) != 5 {
		t.Fatalf("base=%+v checks=%+v err=%v", base, checks, err)
	}
	k.namespaces["orch-v3-a"] = fakeNS{uid: "u-old"}
	if _, _, err := Preflight(context.Background(), k, "kind-k8s4f", []string{"orch-v3-a"}); err == nil {
		t.Fatal("an existing run namespace must fail the preflight")
	}
	k.nodesReady = false
	if base, _, err := Preflight(context.Background(), k, "kind-k8s4f", []string{"orch-v3-b"}); err != nil || len(base.Namespaces) == 0 {
		// a NotReady node is reported by the check but the baseline of an otherwise reachable cluster is still taken
		t.Logf("not-ready cluster: %v", err)
	}
	k.fail = map[string]bool{"list": true}
	if _, _, err := Preflight(context.Background(), k, "kind-k8s4f", []string{"orch-v3-c"}); err == nil {
		t.Fatal("an unreadable namespace list must fail")
	}
}

func TestVerifyWorkloadChecksImageDigestRolloutEnvAndServiceWithoutPrintingValues(t *testing.T) {
	ctx := context.Background()
	k := baselineCluster()
	k.namespaces["ns"] = fakeNS{uid: "u1", managed: true}
	k.deploy["ns/frontend"] = deployment("img:a", "http://one.invalid:9", 1)
	k.pods = map[string]string{"ns": podList("docker.io/library/img@"+digestA, false)}
	k.services["ns/frontend"] = true
	report, err := VerifyWorkload(ctx, k, "ns", "frontend", "img:a", digestA, map[string]string{"BACKEND_URL": "http://one.invalid:9"}, true)
	if err != nil || !report.Ready || !report.Service || !report.Managed || !report.Observed || report.Pods != 1 || report.DigestPods != 1 {
		t.Fatalf("report=%+v err=%v", report, err)
	}
	// An image ID without a registry prefix (a local image) is accepted only when it IS the digest.
	k.pods["ns"] = podList(digestA, false)
	if _, err := VerifyWorkload(ctx, k, "ns", "frontend", "img:a", digestA, nil, true); err != nil {
		t.Fatalf("a bare image ID equal to the digest: %v", err)
	}
	for name, run := range map[string]func() error{
		"image": func() error {
			_, err := VerifyWorkload(ctx, k, "ns", "frontend", "img:b", "", nil, false)
			return err
		},
		"env": func() error {
			_, err := VerifyWorkload(ctx, k, "ns", "frontend", "img:a", "", map[string]string{"BACKEND_URL": "http://two.invalid:9"}, false)
			return err
		},
		"missing": func() error {
			_, err := VerifyWorkload(ctx, k, "ns", "other", "", "", nil, false)
			return err
		},
		// Deployment.spec.image says A but another image runs: the digest is read from the running pod.
		"running digest": func() error {
			k.pods["ns"] = podList("docker.io/library/img@"+digestB, false)
			defer func() { k.pods["ns"] = podList(digestA, false) }()
			_, err := VerifyWorkload(ctx, k, "ns", "frontend", "img:a", digestA, nil, true)
			return err
		},
		"terminating old pod": func() error {
			k.pods["ns"] = podList(digestA, true)
			defer func() { k.pods["ns"] = podList(digestA, false) }()
			_, err := VerifyWorkload(ctx, k, "ns", "frontend", "img:a", digestA, nil, true)
			return err
		},
		"no pod runs": func() error {
			k.pods["ns"] = `{"items":[]}`
			defer func() { k.pods["ns"] = podList(digestA, false) }()
			_, err := VerifyWorkload(ctx, k, "ns", "frontend", "img:a", digestA, nil, true)
			return err
		},
		"generation not observed": func() error {
			k.deploy["ns/frontend"] = deploymentGen("img:a", "http://one.invalid:9", 1, false)
			defer func() { k.deploy["ns/frontend"] = deployment("img:a", "http://one.invalid:9", 1) }()
			_, err := VerifyWorkload(ctx, k, "ns", "frontend", "img:a", digestA, nil, true)
			return err
		},
	} {
		err := run()
		if err == nil {
			t.Fatalf("%s mismatch must fail", name)
		}
		if strings.Contains(err.Error(), "one.invalid") || strings.Contains(err.Error(), "two.invalid") || strings.Contains(err.Error(), digestA) || strings.Contains(err.Error(), digestB) {
			t.Fatalf("%s: the error echoes a value: %v", name, err)
		}
	}
	// Diagnostics of a digest mismatch: the report carries the digests the Pods ACTUALLY run and the rollout counters,
	// the check still fails, and no environment value is in the report.
	k.pods["ns"] = podList("docker.io/library/img@"+digestB, false)
	report, err = VerifyWorkload(ctx, k, "ns", "frontend", "img:a", digestA, map[string]string{"BACKEND_URL": "http://two.invalid:9"}, true)
	if err == nil || len(report.PodImageIDs) != 1 || report.PodImageIDs[0] != digestB || report.Generation != 2 || report.ObservedGeneration != 2 || report.Total != 1 {
		t.Fatalf("diagnostics: report=%+v err=%v", report, err)
	}
	if len(report.EnvMismatch) != 1 || report.EnvMismatch[0] != "BACKEND_URL" || strings.Contains(err.Error(), "one.invalid") {
		t.Fatalf("env diagnostics must name the variable only: %+v %v", report.EnvMismatch, err)
	}
	k.pods["ns"] = podList(digestA, false)
	k.deploy["ns/frontend"] = deployment("img:a", "http://one.invalid:9", 0)
	if _, err := VerifyWorkload(ctx, k, "ns", "frontend", "img:a", "", nil, true); err == nil {
		t.Fatal("a rollout that is not ready must fail when readiness is required")
	}
}

var testReceipts = []Receipt{
	{Descriptor: "k8s-namespace.default#environments.app.dev", Instance: "aaaaaaaaaaaaaaaa", TerraformRun: "tfrun-1"},
	{Descriptor: "k8s-namespace.default#environments.app.dev-aux", Instance: "bbbbbbbbbbbbbbbb", TerraformRun: "tfrun-2"},
}

func ownedCluster(t *testing.T) (*fakeKube, Baseline) {
	t.Helper()
	k := baselineCluster()
	base, _, err := Preflight(context.Background(), k, "c", []string{"orch-v3-a", "orch-v3-a-aux"})
	if err != nil {
		t.Fatal(err)
	}
	return k, base
}

func TestRecordOwnedNeedsExactTagsAndAProductReceipt(t *testing.T) {
	ctx := context.Background()
	k, base := ownedCluster(t)
	// Partial creation: only the first namespace exists. It is recorded; the other is reported absent.
	k.namespaces["orch-v3-a"] = fakeNS{uid: "u-a", labels: productLabels("aaaaaaaaaaaaaaaa", "tfrun-1")}
	owned, absent, err := RecordOwned(ctx, k, base, testRun, testReceipts, base.RunNamespaces)
	if err != nil || len(owned) != 1 || owned[0].Name != "orch-v3-a" || owned[0].UID != "u-a" || owned[0].Instance != "aaaaaaaaaaaaaaaa" || len(absent) != 1 || absent[0] != "orch-v3-a-aux" {
		t.Fatalf("partial create: owned=%+v absent=%v err=%v", owned, absent, err)
	}
	// A same-name namespace created by someone else (generic label only, or foreign run) is never adopted.
	for name, labels := range map[string]map[string]string{
		"generic label only": {"orchestrator-managed": "true"},
		"other run": func() map[string]string {
			l := productLabels("bbbbbbbbbbbbbbbb", "tfrun-2")
			l["run-id"] = "other-run"
			return l
		}(),
		"other manager": func() map[string]string {
			l := productLabels("bbbbbbbbbbbbbbbb", "tfrun-2")
			l["managed-by"] = "someone"
			return l
		}(),
		"no receipt":       productLabels("cccccccccccccccc", "tfrun-9"),
		"receipt mismatch": productLabels("bbbbbbbbbbbbbbbb", "tfrun-1"),
	} {
		k.namespaces["orch-v3-a-aux"] = fakeNS{uid: "u-x", labels: labels}
		owned, _, err := RecordOwned(ctx, k, base, testRun, testReceipts, []string{"orch-v3-a-aux"})
		if err == nil || len(owned) != 0 {
			t.Fatalf("%s must not be adopted: owned=%+v err=%v", name, owned, err)
		}
	}
	// Supplied names must be a subset of the baseline run namespaces; a pre-existing namespace is never claimed.
	if _, _, err := RecordOwned(ctx, k, base, testRun, testReceipts, []string{"shared"}); err == nil {
		t.Fatal("a name outside the baseline run set must be refused")
	}
	base.Namespaces["orch-v3-a"] = "u-pre"
	if _, _, err := RecordOwned(ctx, k, base, testRun, testReceipts, []string{"orch-v3-a"}); err == nil {
		t.Fatal("a pre-existing namespace must not be claimed")
	}
	// Both created and proven.
	delete(base.Namespaces, "orch-v3-a")
	k.namespaces["orch-v3-a-aux"] = fakeNS{uid: "u-aux", labels: productLabels("bbbbbbbbbbbbbbbb", "tfrun-2")}
	owned, absent, err = RecordOwned(ctx, k, base, testRun, testReceipts, base.RunNamespaces)
	if err != nil || len(owned) != 2 || len(absent) != 0 {
		t.Fatalf("owned=%+v absent=%v err=%v", owned, absent, err)
	}
}

func TestVerifyAbsentProvesCleanupAndProtectsTheBaseline(t *testing.T) {
	ctx := context.Background()
	k, base := ownedCluster(t)
	k.namespaces["orch-v3-a"] = fakeNS{uid: "u-a", labels: productLabels("aaaaaaaaaaaaaaaa", "tfrun-1")}
	owned, _, err := RecordOwned(ctx, k, base, testRun, testReceipts, []string{"orch-v3-a"})
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyAbsent(ctx, k, base, owned); err == nil || !strings.Contains(err.Error(), "still exists") {
		t.Fatalf("a surviving namespace must fail: %v", err)
	}
	delete(k.namespaces, "orch-v3-a")
	if err := VerifyAbsent(ctx, k, base, owned); err != nil {
		t.Fatal(err)
	}
	// Recreated under the same name with a new UID is still a run namespace left behind.
	k.namespaces["orch-v3-a"] = fakeNS{uid: "u-new"}
	if err := VerifyAbsent(ctx, k, base, owned); err == nil {
		t.Fatal("a recreated run namespace must fail")
	}
	delete(k.namespaces, "orch-v3-a")
	k.namespaces["shared"] = fakeNS{uid: "u-other"}
	if err := VerifyAbsent(ctx, k, base, owned); err == nil || !strings.Contains(err.Error(), "changed identity") {
		t.Fatalf("a changed baseline namespace must fail: %v", err)
	}
	k.namespaces["shared"] = fakeNS{uid: "u-shared"}
	delete(k.namespaces, "default")
	if err := VerifyAbsent(ctx, k, base, owned); err == nil || !strings.Contains(err.Error(), "disappeared") {
		t.Fatalf("a deleted baseline namespace must fail: %v", err)
	}
}

func TestGuardDeleteNeverDeletesWhatItCannotProveOwnedAndIsAtomicOnTheUID(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	k := baselineCluster()
	record := OwnedNamespace{Name: "orch-v3-a", UID: "u-a", RunID: testRun, Instance: "aaaaaaaaaaaaaaaa", TerraformRun: "tfrun-1"}
	k.namespaces["orch-v3-a"] = fakeNS{uid: "u-a", labels: productLabels("aaaaaaaaaaaaaaaa", "tfrun-1")}
	// another UID, generic label only, wrong run, incomplete record: all refused, nothing deleted
	wrongUID := record
	wrongUID.UID = "u-other"
	if deleted, err := GuardDelete(ctx, k, dir, wrongUID); deleted || err == nil {
		t.Fatalf("another UID: deleted=%v err=%v", deleted, err)
	}
	k.namespaces["orch-v3-a"] = fakeNS{uid: "u-a", managed: true}
	if deleted, err := GuardDelete(ctx, k, dir, record); deleted || err == nil {
		t.Fatalf("generic label only: deleted=%v err=%v", deleted, err)
	}
	other := productLabels("aaaaaaaaaaaaaaaa", "tfrun-1")
	other["run-id"] = "someone-else"
	k.namespaces["orch-v3-a"] = fakeNS{uid: "u-a", labels: other}
	if deleted, err := GuardDelete(ctx, k, dir, record); deleted || err == nil {
		t.Fatalf("foreign run tag: deleted=%v err=%v", deleted, err)
	}
	if deleted, err := GuardDelete(ctx, k, dir, OwnedNamespace{Name: "orch-v3-a", UID: "u-a"}); deleted || err == nil {
		t.Fatalf("a record without tags: deleted=%v err=%v", deleted, err)
	}
	for _, protected := range []string{"default", "kube-system"} {
		if deleted, err := GuardDelete(ctx, k, dir, OwnedNamespace{Name: protected, UID: "u", RunID: testRun, Instance: "a", TerraformRun: "b"}); deleted || err == nil {
			t.Fatalf("%s must be refused: deleted=%v err=%v", protected, deleted, err)
		}
	}
	if len(k.deleted) != 0 {
		t.Fatalf("deleted %v", k.deleted)
	}
	// Replaced between the read and the delete: the UID precondition stops the delete.
	k.namespaces["orch-v3-a"] = fakeNS{uid: "u-a", labels: productLabels("aaaaaaaaaaaaaaaa", "tfrun-1")}
	k.replaceOnDelete = true
	if deleted, err := GuardDelete(ctx, k, dir, record); deleted || err == nil || !strings.Contains(err.Error(), "replaced") {
		t.Fatalf("a replacement UID must not be deleted: deleted=%v err=%v", deleted, err)
	}
	if ns, ok := k.namespaces["orch-v3-a"]; !ok || ns.uid != "u-replacement" {
		t.Fatalf("the replacement namespace must survive: %+v", k.namespaces)
	}
	k.replaceOnDelete = false
	// The owned object: the delete body carries the UID precondition.
	k.namespaces["orch-v3-a"] = fakeNS{uid: "u-a", labels: productLabels("aaaaaaaaaaaaaaaa", "tfrun-1")}
	if deleted, err := GuardDelete(ctx, k, dir, record); !deleted || err != nil {
		t.Fatalf("owned: deleted=%v err=%v", deleted, err)
	}
	if len(k.deleteBodies) < 1 || !strings.Contains(k.deleteBodies[len(k.deleteBodies)-1], `"uid":"u-a"`) {
		t.Fatalf("the delete must carry the UID precondition: %v", k.deleteBodies)
	}
	if deleted, err := GuardDelete(ctx, k, dir, record); deleted || err != nil {
		t.Fatalf("already gone must be a no-op: deleted=%v err=%v", deleted, err)
	}
	for _, name := range []string{"shared", "default", "kube-system"} {
		if _, ok := k.namespaces[name]; !ok {
			t.Fatalf("%s was removed", name)
		}
	}
	// A cluster API error is not "absent".
	k.namespaces["orch-v3-a"] = fakeNS{uid: "u-a", labels: productLabels("aaaaaaaaaaaaaaaa", "tfrun-1")}
	k.fail = map[string]bool{"list": true}
	k.hideGet = true
	if deleted, err := GuardDelete(ctx, k, dir, record); deleted || err == nil {
		t.Fatalf("an unreadable namespace must be an error, not absent: deleted=%v err=%v", deleted, err)
	}
}

func TestReapDeletesOnlyProvenNamespacesAndReportsTheRest(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	run := func(k *fakeKube, base Baseline, prior []OwnedNamespace, receipts []Receipt) ReapResult {
		return Reap(ctx, k, dir, base, testRun, prior, receipts, 0)
	}
	// Partial create (runner killed after the first namespace): the proven one is deleted, the absent one is fine.
	k, base := ownedCluster(t)
	k.namespaces["orch-v3-a"] = fakeNS{uid: "u-a", labels: productLabels("aaaaaaaaaaaaaaaa", "tfrun-1")}
	result := run(k, base, nil, testReceipts)
	if !result.OK() || len(result.Deleted) != 1 || result.Deleted[0] != "orch-v3-a" || len(result.Absent) != 1 {
		t.Fatalf("partial create: %+v", result)
	}
	// The product database is gone, but the scenario recorded ownership before the runner died.
	k, base = ownedCluster(t)
	k.namespaces["orch-v3-a"] = fakeNS{uid: "u-a", labels: productLabels("aaaaaaaaaaaaaaaa", "tfrun-1")}
	prior := []OwnedNamespace{{Name: "orch-v3-a", UID: "u-a", RunID: testRun, Instance: "aaaaaaaaaaaaaaaa", TerraformRun: "tfrun-1"}}
	if result := run(k, base, prior, nil); !result.OK() || len(result.Deleted) != 1 {
		t.Fatalf("scenario record only: %+v", result)
	}
	// No record and no receipt: a same-name namespace is NOT ours and is never deleted; the cleanup reports it.
	k, base = ownedCluster(t)
	k.namespaces["orch-v3-a"] = fakeNS{uid: "u-stranger", managed: true}
	result = run(k, base, nil, nil)
	if result.OK() || len(result.Unproven) != 1 || len(k.deleted) != 0 {
		t.Fatalf("a same-name stranger must be left alone and reported: %+v deleted=%v", result, k.deleted)
	}
	// Replacement after the scenario recorded another UID: not deleted.
	k, base = ownedCluster(t)
	k.namespaces["orch-v3-a"] = fakeNS{uid: "u-replacement", labels: productLabels("aaaaaaaaaaaaaaaa", "tfrun-1")}
	if result := run(k, base, prior, nil); result.OK() || len(k.deleted) != 0 || len(result.Unproven) != 1 {
		t.Fatalf("a replacement UID must not be deleted: %+v", result)
	}
	// The API refuses the precondition (replaced between read and delete): reported, replacement survives.
	k, base = ownedCluster(t)
	k.namespaces["orch-v3-a"] = fakeNS{uid: "u-a", labels: productLabels("aaaaaaaaaaaaaaaa", "tfrun-1")}
	k.replaceOnDelete = true
	if result := run(k, base, prior, nil); result.OK() || k.namespaces["orch-v3-a"].uid != "u-replacement" {
		t.Fatalf("a raced replacement must survive: %+v", result)
	}
	// A cluster API error is a failure, not success.
	k, base = ownedCluster(t)
	k.fail = map[string]bool{"list": true}
	if result := run(k, base, nil, nil); result.OK() {
		t.Fatalf("an unreadable cluster must not be reported clean: %+v", result)
	}
	// Baseline namespaces are untouched in every case.
	k, base = ownedCluster(t)
	k.namespaces["orch-v3-a"] = fakeNS{uid: "u-a", labels: productLabels("aaaaaaaaaaaaaaaa", "tfrun-1")}
	run(k, base, nil, testReceipts)
	for _, name := range []string{"default", "kube-system", "shared"} {
		if _, ok := k.namespaces[name]; !ok {
			t.Fatalf("%s was removed", name)
		}
	}
	// An already clean cluster is a successful no-op.
	k, base = ownedCluster(t)
	if result := run(k, base, nil, nil); !result.OK() || len(result.Deleted) != 0 {
		t.Fatalf("already absent: %+v", result)
	}
}

func randomSuffix() string {
	raw := make([]byte, 6)
	if _, err := rand.Read(raw); err != nil {
		panic(err)
	}
	return hex.EncodeToString(raw)
}
