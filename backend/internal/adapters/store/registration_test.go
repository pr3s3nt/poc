package store

import (
	"encoding/json"
	"os"
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

// V01: the account version is persisted in the JSON snapshot, compare-and-set
// survives reopen, and a snapshot written before the version existed is
// rejected instead of being defaulted.
func TestConnectionVersionSurvivesJSONReopenAndOldSnapshotsAreRejected(t *testing.T) {
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
	if fixture.Versioned.Version != 3 {
		t.Fatalf("fixture version %d", fixture.Versioned.Version)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]map[string]map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	for _, record := range doc["connections"] {
		delete(record, "version")
	}
	old, _ := json.Marshal(doc)
	oldPath := filepath.Join(t.TempDir(), "old.json")
	if err := os.WriteFile(oldPath, old, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewWithSnapshot(oldPath); err == nil {
		t.Fatal("a snapshot without connection versions was accepted")
	}
	if after, _ := os.ReadFile(oldPath); string(after) != string(old) {
		t.Fatal("rejecting an old snapshot rewrote it")
	}
}
