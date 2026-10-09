package deployment

import (
	"testing"

	"orchestrator/internal/ports/execution"
)

func TestRestoreEnvironmentTargetRejectsOtherConnection(t *testing.T) {
	ref := map[string]any{"organization": "acme", "connection": "other", "context": "c", "cluster": "k"}
	var target execution.Target
	if err := restoreEnvironmentTarget(&target, ref, "acme", "lab"); err == nil {
		t.Fatal("stored target on another connection was reused")
	}
	if err := restoreEnvironmentTarget(&target, ref, "acme", "other"); err != nil {
		t.Fatal(err)
	}
	legacy := map[string]any{"context": "c", "cluster": "k"}
	target = execution.Target{}
	if err := restoreEnvironmentTarget(&target, legacy, "acme", "lab"); err != nil {
		t.Fatalf("legacy host-context target: %v", err)
	}
}
