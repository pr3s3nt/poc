package fleet

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	domain "orchestrator/internal/domain/delivery"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/execution"
)

// FAKE kubectl for UC-06 CD-11 (issue #68): a Fleet-synced StatefulSet whose
// pg-0 is stuck on a bad image; the fixture records every call.
func stuckFixture(t *testing.T, current, update, deliveryRevision string, deleteFails bool) (*Provider, string) {
	t.Helper()
	dir := t.TempDir()
	stale := time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339)
	set := map[string]any{
		"metadata": map[string]any{"name": "pg", "uid": "set-uid", "annotations": map[string]any{"meta.helm.sh/release-name": "rel", "orchestrator.io/delivery-revision": deliveryRevision}},
		"spec":     map[string]any{"selector": map[string]any{"matchLabels": map[string]any{"app": "pg"}}},
		"status":   map[string]any{"currentRevision": current, "updateRevision": update},
	}
	pods := map[string]any{"items": []any{map[string]any{
		"metadata": map[string]any{"name": "pg-0", "uid": "pod-uid", "labels": map[string]any{"controller-revision-hash": current}, "ownerReferences": []any{map[string]any{"kind": "StatefulSet", "uid": "set-uid", "controller": true}}},
		"status": map[string]any{
			"conditions":        []any{map[string]any{"type": "Ready", "status": "False", "lastTransitionTime": stale}},
			"containerStatuses": []any{map[string]any{"name": "pg", "image": "postgres:bad", "state": map[string]any{"waiting": map[string]any{"reason": "ImagePullBackOff"}}}},
		},
	}}}
	for name, v := range map[string]any{"set": set, "pods": pods} {
		raw, _ := json.Marshal(v)
		if err := os.WriteFile(filepath.Join(dir, name+".json"), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	deleteExit := "0"
	if deleteFails {
		deleteExit = "1"
	}
	script := `#!/bin/sh
printf '%s\n' "$*" >> "` + dir + `/calls"
case "$*" in
 *"get statefulsets pg "*) cat "` + dir + `/set.json";;
 *"get pods -n app-ns -l app=pg -o json"*) cat "` + dir + `/pods.json";;
 *"delete pod pg-0 -n app-ns --wait=false"*) exit ` + deleteExit + `;;
 *) exit 42;;
esac
`
	executable := filepath.Join(dir, "kubectl")
	if err := os.WriteFile(executable, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	v := &Verifier{KubectlPath: executable, Credentials: fixtureCredentials{[]byte("private fixture")}, Targets: fixtureTargets{}}
	return &Provider{Manager: &Manager{Verifier: v}}, dir
}

func stuckIntent() (port.Intent, port.Receipt) {
	inventory, _ := json.Marshal([]execution.ObjectRef{{APIVersion: "apps/v1", Kind: "StatefulSet", Name: "pg"}, {APIVersion: "v1", Kind: "Service", Name: "pg"}})
	i := port.Intent{ID: "intent", OrganizationKey: "org", Revision: domain.Revision{ID: "rev-2"}, Target: execution.Target{Kind: "kubernetes", Organization: "org", Connection: "task-cluster", Namespace: "app-ns"}}
	return i, port.Receipt{RevisionID: "rev-2", Handle: map[string]string{"release": "rel", "inventory": string(inventory)}}
}

func calls(t *testing.T, dir string) string {
	raw, _ := os.ReadFile(filepath.Join(dir, "calls"))
	return string(raw)
}

func TestFleetFixedRevisionReplacesStuckPod(t *testing.T) {
	p, dir := stuckFixture(t, "r1", "r2", "rev-2", false)
	i, r := stuckIntent()
	got, err := p.RecoverStuckPods(context.Background(), i, r)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Replaced) != 1 || got.Replaced[0].Wait.Pod != "pg-0" || got.Replaced[0].Wait.Image != "postgres:bad" || got.Replaced[0].Wait.Object != "app-ns/pg" {
		t.Fatalf("replacement = %+v (calls %s)", got, calls(t, dir))
	}
	if !strings.Contains(calls(t, dir), "delete pod pg-0 -n app-ns --wait=false") {
		t.Fatal("pod not deleted")
	}
}

func TestFleetWithoutFixOrForeignRevisionOnlyReports(t *testing.T) {
	for _, tc := range []struct{ name, current, update, delivery string }{
		{"no new controller revision", "r1", "r1", "rev-2"},
		{"object still carries an older delivery revision", "r1", "r2", "rev-1"},
	} {
		p, dir := stuckFixture(t, tc.current, tc.update, tc.delivery, false)
		i, r := stuckIntent()
		got, err := p.RecoverStuckPods(context.Background(), i, r)
		if err != nil || len(got.Replaced) != 0 || strings.Contains(calls(t, dir), "delete") {
			t.Fatalf("%s: deleted or failed: %+v %v", tc.name, got, err)
		}
		if tc.delivery == "rev-2" && (len(got.Waiting) != 1 || got.Waiting[0].Code != "ImagePullBackOff") {
			t.Fatalf("%s: reason not reported: %+v", tc.name, got)
		}
	}
}

func TestFleetMissingDeletePermissionOnlyReports(t *testing.T) {
	p, _ := stuckFixture(t, "r1", "r2", "rev-2", true)
	i, r := stuckIntent()
	got, err := p.RecoverStuckPods(context.Background(), i, r)
	if err != nil || len(got.Replaced) != 0 || len(got.Waiting) != 1 {
		t.Fatalf("got %+v %v", got, err)
	}
}
