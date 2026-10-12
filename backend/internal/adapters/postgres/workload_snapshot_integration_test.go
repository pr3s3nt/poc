package postgres

import (
	"context"
	"testing"
	"time"

	"orchestrator/internal/domain/deployment"
	"orchestrator/internal/ports/persistence/persistencetest"
	"orchestrator/internal/seed"
	"orchestrator/internal/seed/seedtest"
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
	persistencetest.AssertApplicationSaveKeepsHistory(t, first, fixture)
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

// Reopening validates the baseline and never reconstructs per-Deployment
// workload history from the current Workload Instance rows: history that is
// absent stays absent, history that exists stays exactly as it was.
func TestReopenNeverReconstructsWorkloadHistory(t *testing.T) {
	ctx := context.Background()
	st, url := openFresh(t)
	opts := seed.Defaults()
	if err := seedtest.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	older := persistencetest.SaveDeployment(t, st, opts, deployment.StatusPlanning, now)
	latest := persistencetest.SaveDeployment(t, st, opts, deployment.StatusPlanning, now.Add(time.Minute))
	instance := func(deploymentID, digest string) deployment.WorkloadInstance {
		return deployment.WorkloadInstance{
			EnvironmentKey: opts.ApplicationKey + "/" + opts.EnvironmentKey, WorkloadID: "api",
			LastDeploymentID: deploymentID, TargetRef: map[string]any{"namespace": "team"},
			ManifestDigest: digest, Status: deployment.InstanceReady, ObservedAt: now,
		}
	}
	if err := st.UpsertWorkloadProgress(ctx, instance(older.ID, "sha256:older")); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertWorkloadProgress(ctx, instance(latest.ID, "sha256:latest")); err != nil {
		t.Fatal(err)
	}
	st.Close()

	reopened, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	for id, digest := range map[string]string{older.ID: "sha256:older", latest.ID: "sha256:latest"} {
		rows, err := reopened.ListDeploymentWorkloads(ctx, id)
		if err != nil || len(rows) != 1 || rows[0].ManifestDigest != digest {
			t.Fatalf("history of %s = %#v, %v", id, rows, err)
		}
	}
	// With the history rows gone, the current instance does not repopulate them.
	if _, err := reopened.pool.Exec(ctx, `DELETE FROM deployment_workloads`); err != nil {
		t.Fatal(err)
	}
	again, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	for _, id := range []string{older.ID, latest.ID} {
		if rows, err := again.ListDeploymentWorkloads(ctx, id); err != nil || len(rows) != 0 {
			t.Fatalf("history was reconstructed for %s: %#v, %v", id, rows, err)
		}
	}
}

// Needs a disposable database (ORCHESTRATOR_POSTGRES_TEST_URL); skipped otherwise.
func TestPostgresDesiredRenameContract(t *testing.T) {
	st, _ := openFresh(t)
	persistencetest.DesiredRename(t, st)
}
