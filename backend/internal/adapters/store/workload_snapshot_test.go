package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"orchestrator/internal/domain/deployment"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/persistence/persistencetest"
	"orchestrator/internal/seed"
	"orchestrator/internal/seed/seedtest"
)

func TestWorkloadSnapshotContract(t *testing.T) {
	persistencetest.WorkloadSnapshots(t, New())
}

func TestWorkloadSnapshotContractSurvivesJSONReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	first, err := NewWithSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	fixture := persistencetest.WorkloadSnapshots(t, first)
	persistencetest.AssertApplicationSaveKeepsHistory(t, first, fixture)
	reopened, err := NewWithSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	persistencetest.AssertReloaded(t, reopened, fixture)
}

func TestReadSnapshotContract(t *testing.T) {
	persistencetest.ReadSnapshot(t, New())
}

func TestReadSnapshotDoesNotPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	st, err := NewWithSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	persistencetest.ReadSnapshot(t, st)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = st.ReadSnapshot(context.Background(), func(ctx context.Context, view persistence.Store) error {
		_, _ = view.ListApplications(ctx)
		return nil
	})
	after, err := os.ReadFile(path)
	if err != nil || string(before) != string(after) {
		t.Fatalf("read snapshot rewrote the state file: %v", err)
	}
}

func TestTerminalSerializationContract(t *testing.T) {
	persistencetest.TerminalSerialization(t, New())
}

// legacyState writes a JSON state file whose Deployments predate
// per-Deployment workload history: it holds current Workload Instances only.
func legacyState(t *testing.T, path string) (older, latest deployment.Deployment) {
	t.Helper()
	ctx := context.Background()
	st, err := NewWithSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	opts := seed.Defaults()
	if err := seedtest.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	older = persistencetest.SaveDeployment(t, st, opts, deployment.StatusSucceeded, now)
	latest = persistencetest.SaveDeployment(t, st, opts, deployment.StatusSucceeded, now.Add(time.Minute))
	if err := st.UpsertWorkloadInstance(ctx, deployment.WorkloadInstance{
		EnvironmentKey: opts.ApplicationKey + "/" + opts.EnvironmentKey, WorkloadID: "api",
		LastDeploymentID: latest.ID, TargetRef: map[string]any{"namespace": "team"},
		ManifestDigest: "sha256:latest", Status: deployment.InstanceReady, ObservedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	return older, latest
}

// A snapshot without the per-Deployment history aggregate is not the current
// format. It is rejected, never repaired from current Workload Instances.
func TestJSONMissingWorkloadHistoryIsRejectedNotBackfilled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	legacyState(t, path)
	raw := mutateSnapshot(t, path, func(fields map[string]json.RawMessage) { delete(fields, "deploymentWorkloads") })
	_, err := NewWithSnapshot(path)
	if !errors.Is(err, ErrResetRequired) {
		t.Fatalf("want reset-required, got %v", err)
	}
	if after, _ := os.ReadFile(path); string(after) != string(raw) {
		t.Fatal("rejected snapshot was rewritten")
	}
}

// Current history, including older Deployments with no rows, reopens unchanged.
func TestEmptyWorkloadHistoryIsNotBackfilledOnReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	_, latest := legacyState(t, path)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if string(fields["deploymentWorkloads"]) != "{}" {
		t.Fatalf("empty history must be written explicitly, got %q", fields["deploymentWorkloads"])
	}
	reopened, err := NewWithSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	if rows, err := reopened.ListDeploymentWorkloads(context.Background(), latest.ID); err != nil || len(rows) != 0 {
		t.Fatalf("current-row correction became history on reload: %#v, %v", rows, err)
	}
}
