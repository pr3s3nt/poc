package store

import (
	"orchestrator/internal/ports/persistence/persistencetest"
	"path/filepath"
	"testing"
)

func TestDeliveryBindingsSurviveJSONRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	first, err := NewWithSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	b := persistencetest.DeliveryBindings(t, first)
	persistencetest.DeliverySelections(t, first, b)
	next, err := NewWithSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	persistencetest.AssertDeliveryBinding(t, next, b)
}
