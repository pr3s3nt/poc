package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"orchestrator/internal/domain/application"

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

// A sweep that finds nothing must not rewrite the snapshot file, so a periodic
// sweeper cannot race with whoever owns the snapshot directory.
func TestIdleSweepDoesNotRewriteTheSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	st, err := NewWithSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveOrganization(context.Background(), application.Organization{Key: "acme", Name: "acme"}); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	if n, err := st.ExpireLeases(context.Background(), time.Now()); err != nil || n != 0 {
		t.Fatalf("expire: %d %v", n, err)
	}
	if _, ok, err := st.ClaimCleanup(context.Background(), "me", time.Now().Add(time.Minute), time.Now()); err != nil || ok {
		t.Fatalf("claim: %v %v", ok, err)
	}
	after, _ := os.Stat(path)
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("an idle sweep rewrote the snapshot")
	}
}
