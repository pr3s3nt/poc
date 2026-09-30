package store

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"orchestrator/internal/domain/deployment"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/persistence/persistencetest"
	"orchestrator/internal/seed"
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
	if err := seed.Apply(ctx, st, opts); err != nil {
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

func TestLegacyJSONBackfillsLatestRunOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	older, latest := legacyState(t, path)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	delete(fields, "deploymentWorkloads")
	legacy, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, legacy, 0o600); err != nil {
		t.Fatal(err)
	}

	reopened, err := NewWithSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	rows, err := reopened.ListDeploymentWorkloads(ctx, latest.ID)
	if err != nil || len(rows) != 1 || rows[0].ManifestDigest != "sha256:latest" || rows[0].TargetRef["namespace"] != "team" {
		t.Fatalf("latest run backfill = %#v, %v", rows, err)
	}
	if rows, err := reopened.ListDeploymentWorkloads(ctx, older.ID); err != nil || len(rows) != 0 {
		t.Fatalf("older run history was invented: %#v, %v", rows, err)
	}
}

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
