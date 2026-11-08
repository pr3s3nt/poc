package fleet

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"orchestrator/internal/adapters/delivery/artifact"
	port "orchestrator/internal/ports/delivery"
	"testing"
)

func TestTerminalJobIdentityRejectsUnknownAndForeignOutcomes(t *testing.T) {
	i := packageIntent()
	i.Target.Extra = map[string]string{"application": "app", "environment": "staging"}
	r := port.Receipt{Handle: map[string]string{"release": "release", "releaseNamespace": "fleet-system"}}
	expected := map[string]any{"metadata": map[string]any{"name": "task"}, "spec": map[string]any{"backoffLimit": 0, "template": map[string]any{"spec": map[string]any{"containers": []any{map[string]any{"name": "main", "image": "example/app:1", "args": []any{"run"}}}}}}}
	makeJob := func() map[string]any {
		return map[string]any{"metadata": map[string]any{"name": "task", "namespace": i.Target.Namespace, "uid": "job-original", "resourceVersion": "42", "generation": 1, "labels": map[string]any{"orchestrator.io/managed-by": "orchestrator", "orchestrator.io/application": "app", "orchestrator.io/environment": "staging"}, "annotations": map[string]any{"orchestrator.io/delivery-revision": i.Revision.ID, "meta.helm.sh/release-name": "release", "meta.helm.sh/release-namespace": "fleet-system"}}, "spec": cloneObject(expected)["spec"], "status": map[string]any{"conditions": []any{map[string]any{"type": "Failed", "status": "True"}}}}
	}
	for _, test := range []struct {
		name             string
		change           func(map[string]any)
		wantErr, pending bool
	}{
		{name: "failed"},
		{name: "running", change: func(j map[string]any) { obj(j["status"])["active"] = 1 }, pending: true},
		{name: "target-only-not-terminal", change: func(j map[string]any) {
			obj(j["status"])["conditions"] = []any{map[string]any{"type": "FailureTarget", "status": "True"}}
		}, pending: true},
		{name: "terminating", change: func(j map[string]any) { obj(j["status"])["terminating"] = 1 }, pending: true},
		{name: "foreign-release", change: func(j map[string]any) { obj(obj(j["metadata"])["annotations"])["meta.helm.sh/release-name"] = "other" }, wantErr: true},
		{name: "foreign-namespace", change: func(j map[string]any) { obj(j["metadata"])["namespace"] = "other" }, wantErr: true},
		{name: "changed-spec", change: func(j map[string]any) { obj(j["spec"])["backoffLimit"] = 3 }, wantErr: true},
		{name: "changed-generation", change: func(j map[string]any) { obj(j["metadata"])["generation"] = 2 }, wantErr: true},
		{name: "wrong-pin", change: func(j map[string]any) {
			obj(obj(j["metadata"])["annotations"])["orchestrator.io/delivery-revision"] = "other"
		}, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			j := makeJob()
			if test.change != nil {
				test.change(j)
			}
			uid, terminal, failed, err := terminalJobIdentity(j, expected, i, r)
			if test.wantErr {
				if err == nil {
					t.Fatal("unsafe failure proof admitted")
				}
				return
			}
			if err != nil || uid != "job-original" || terminal == test.pending || (!test.pending && !failed) {
				t.Fatal(uid, terminal, failed, err)
			}
		})
	}
	original := makeJob()
	replaced := makeJob()
	obj(replaced["metadata"])["uid"] = "replacement"
	if sameIncarnation(original, replaced) {
		t.Fatal("replacement UID admitted")
	}
	pod := map[string]any{"metadata": map[string]any{"ownerReferences": []any{map[string]any{"uid": "job-original", "kind": "Job", "name": "task"}}}, "status": map[string]any{"phase": "Failed"}}
	if !jobPodsStopped([]map[string]any{pod}, "job-original", "task") {
		t.Fatal("terminal Pod rejected")
	}
	obj(pod["status"])["phase"] = "Running"
	if jobPodsStopped([]map[string]any{pod}, "job-original", "task") {
		t.Fatal("running Pod admitted")
	}
}
func TestFailedHelmReleaseProofRejectsWrongAndNewerReleases(t *testing.T) {
	release := map[string]any{"name": "release", "namespace": "fleet-system", "info": map[string]any{"status": "failed"}, "manifest": "apiVersion: batch/v1\nkind: Job\nmetadata:\n  name: task\n"}
	raw, _ := json.Marshal(release)
	var compressed bytes.Buffer
	w := gzip.NewWriter(&compressed)
	_, _ = w.Write(raw)
	_ = w.Close()
	data := base64.StdEncoding.EncodeToString([]byte(base64.StdEncoding.EncodeToString(compressed.Bytes())))
	secret := map[string]any{"metadata": map[string]any{"name": "sh.helm.release.v1.release.v1", "namespace": "fleet-system", "uid": "secret-uid", "resourceVersion": "1", "labels": map[string]any{"owner": "helm", "name": "release", "status": "failed", "version": "1"}}, "data": map[string]any{"release": data}}
	selected, err := latestFailedJobRelease([]map[string]any{secret}, "release", "fleet-system")
	if err != nil {
		t.Fatal(err)
	}
	ms, err := helmJobManifest(selected, "release", "fleet-system")
	if err != nil || len(ms) != 1 {
		t.Fatal(ms, err)
	}
	if _, err = helmJobManifest(secret, "other", "fleet-system"); err == nil {
		t.Fatal("foreign release admitted")
	}
	newer := cloneObject(secret)
	obj(newer["metadata"])["name"] = "sh.helm.release.v1.release.v2"
	obj(obj(newer["metadata"])["labels"])["version"] = "2"
	obj(obj(newer["metadata"])["labels"])["status"] = "pending-upgrade"
	if _, err = latestFailedJobRelease([]map[string]any{secret, newer}, "release", "fleet-system"); err == nil {
		t.Fatal("pending new release ignored")
	}
}

func TestTerminalJobManifestIgnoresOnlyFleetTracking(t *testing.T) {
	expected := map[string]any{"apiVersion": "batch/v1", "kind": "Job", "metadata": map[string]any{"name": "task", "labels": map[string]any{"business": "keep"}}, "spec": map[string]any{"backoffLimit": 0}}
	live := cloneObject(expected)
	md := obj(live["metadata"])
	md["annotations"] = map[string]any{"objectset.rio.cattle.io/id": "controller"}
	obj(md["labels"])["objectset.rio.cattle.io/hash"] = "hash"
	same, err := artifact.SameExecution([]map[string]any{cloneObject(expected)}, []map[string]any{terminalJobExecutionObject(live)})
	if err != nil || !same {
		t.Fatal("Fleet post-render provenance rejected", err)
	}
	obj(live["spec"])["backoffLimit"] = 3
	same, err = artifact.SameExecution([]map[string]any{cloneObject(expected)}, []map[string]any{terminalJobExecutionObject(live)})
	if err != nil || same {
		t.Fatal("Changed Job policy admitted", err)
	}
}
