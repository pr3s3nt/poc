package store

import (
	"path/filepath"
	"testing"

	"orchestrator/internal/ports/persistence/persistencetest"
)

func TestRegistrationContract(t *testing.T) {
	persistencetest.Registration(t, New())
}

func TestRegistrationContractSurvivesJSONReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	first, err := NewWithSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	fixture := persistencetest.Registration(t, first)
	reopened, err := NewWithSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	persistencetest.AssertRegistration(t, reopened, fixture)
}
