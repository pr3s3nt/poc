package store

import (
	"path/filepath"
	"testing"

	"orchestrator/internal/ports/persistence/persistencetest"
)

func TestDiagnosticsContract(t *testing.T) {
	persistencetest.Diagnostics(t, New())
}

func TestDiagnosticsContractSurvivesJSONReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	first, err := NewWithSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	fixture := persistencetest.Diagnostics(t, first)
	reopened, err := NewWithSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	persistencetest.AssertDiagnostics(t, reopened, fixture)
}
