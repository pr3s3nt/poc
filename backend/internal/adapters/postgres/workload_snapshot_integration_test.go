package postgres

import (
	"context"
	"testing"
	"time"

	"orchestrator/internal/domain/deployment"
	"orchestrator/internal/ports/persistence/persistencetest"
	"orchestrator/internal/seed"
)

func openFresh(t *testing.T) (*Store, string) {
	t.Helper()
	url := persistencetest.FreshPostgresDatabase(t)
	st, err := Open(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	return st, url
}

func TestPostgresWorkloadSnapshotContractSurvivesRestart(t *testing.T) {
	first, url := openFresh(t)
	fixture := persistencetest.WorkloadSnapshots(t, first)
	first.Close()
	reopened, err := Open(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	persistencetest.AssertReloaded(t, reopened, fixture)
}

func TestPostgresReadSnapshotContract(t *testing.T) {
	st, _ := openFresh(t)
	persistencetest.ReadSnapshot(t, st)
}

func TestPostgresTerminalSerializationContract(t *testing.T) {
	st, _ := openFresh(t)
	persistencetest.TerminalSerialization(t, st)
}

// TestMigration4BackfillsLatestRunOnly replays migration 4 over a database
// written before per-Deployment workload history existed.
func TestMigration4BackfillsLatestRunOnly(t *testing.T) {
	ctx := context.Background()
	st, url := openFresh(t)
	opts := seed.Defaults()
	if err := seed.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	older := persistencetest.SaveDeployment(t, st, opts, deployment.StatusSucceeded, now)
	latest := persistencetest.SaveDeployment(t, st, opts, deployment.StatusSucceeded, now.Add(time.Minute))
	if err := st.UpsertWorkloadInstance(ctx, deployment.WorkloadInstance{
		EnvironmentKey: opts.ApplicationKey + "/" + opts.EnvironmentKey, WorkloadID: "api",
		LastDeploymentID: latest.ID, TargetRef: map[string]any{"namespace": "team"},
		ManifestDigest: "sha256:latest", Status: deployment.InstanceReady, ObservedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.pool.Exec(ctx, `DROP TABLE deployment_workloads; DELETE FROM schema_migrations WHERE version=4`); err != nil {
		t.Fatal(err)
	}
	st.Close()

	reopened, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	rows, err := reopened.ListDeploymentWorkloads(ctx, latest.ID)
	if err != nil || len(rows) != 1 || rows[0].ManifestDigest != "sha256:latest" || rows[0].TargetRef["namespace"] != "team" {
		t.Fatalf("latest run backfill = %#v, %v", rows, err)
	}
	if rows, err := reopened.ListDeploymentWorkloads(ctx, older.ID); err != nil || len(rows) != 0 {
		t.Fatalf("older run history was invented: %#v, %v", rows, err)
	}
}
