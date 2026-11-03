package fleet

import (
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/execution"
	"testing"
)

func TestPendingPruneAcceptsOnlyOmittedAcknowledgedDeletionDifferences(t *testing.T) {
	stale := []port.AppliedObject{{ObjectRef: execution.ObjectRef{APIVersion: "v1", Kind: "ConfigMap", Name: "old"}, UID: "old-uid", Release: "release"}}
	base := func() map[string]any {
		return map[string]any{"apiVersion": "v1", "kind": "ConfigMap", "name": "old", "namespace": "app", "delete": true}
	}
	for _, tc := range []struct {
		name   string
		change func(map[string]any)
		want   bool
	}{
		{"owned old deletion", func(map[string]any) {}, true},
		{"unrelated object", func(v map[string]any) { v["name"] = "foreign" }, false},
		{"other namespace", func(v map[string]any) { v["namespace"] = "other" }, false},
		{"patch drift", func(v map[string]any) { v["patch"] = "replicas changed" }, false},
		{"creation drift", func(v map[string]any) { v["create"] = true }, false},
		{"not deletion", func(v map[string]any) { v["delete"] = false }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			difference := base()
			tc.change(difference)
			if got := deletionOnlyRetirements([]any{difference}, stale, "app"); got != tc.want {
				t.Fatalf("eligible=%v want=%v", got, tc.want)
			}
		})
	}
	stale[0].UID = ""
	if deletionOnlyRetirements([]any{base()}, stale, "app") {
		t.Fatal("unknown old UID admitted")
	}
}
