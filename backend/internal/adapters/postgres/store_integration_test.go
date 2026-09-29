package postgres

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"orchestrator/internal/domain/deployment"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/seed"
)

func TestNormalizedStoreRestartAndSnapshotConstraints(t *testing.T) {
	url := os.Getenv("ORCHESTRATOR_POSTGRES_TEST_URL")
	if url == "" {
		t.Skip("ORCHESTRATOR_POSTGRES_TEST_URL is not set")
	}
	ctx := context.Background()
	first, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	opts := seed.Defaults()
	if err = seed.Apply(ctx, first, opts); err != nil {
		t.Fatal(err)
	}
	env, err := first.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}

	orphan, err := deployment.NewDeploymentDeltaSnapshot(ids.New(), ids.New(), deployment.DeltaDocument{}, deployment.DeltaSnapshotMetadata{Action: deployment.ActionDeploy, WorkloadID: "api"}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err = first.SaveDeltaSnapshot(ctx, orphan); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("orphan snapshot error = %v", err)
	}

	record := deployment.Deployment{ID: ids.New(), EnvironmentID: env.ID, OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey, ExecutionProfile: "internal-k8s", Action: deployment.ActionDeploy, WorkloadID: "api", ActorRef: "test", Status: deployment.StatusPlanning, BaseEnvironmentVersion: env.Version, StartedAt: time.Now().UTC()}
	if err = first.SaveDeployment(ctx, record); err != nil {
		t.Fatal(err)
	}
	snapshot, err := deployment.NewDeploymentDeltaSnapshot(ids.New(), record.ID, deployment.DeltaDocument{}, deployment.DeltaSnapshotMetadata{Action: record.Action, WorkloadID: record.WorkloadID}, record.StartedAt)
	if err != nil {
		t.Fatal(err)
	}
	err = first.Transact(ctx, func(ctx context.Context) error {
		if err := first.SaveDeltaSnapshot(ctx, snapshot); err != nil {
			return err
		}
		record.Status = deployment.StatusProvisioning
		return first.SaveDeployment(ctx, record)
	})
	if err != nil {
		t.Fatal(err)
	}
	first.Close()

	second, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	got, err := second.GetDeltaSnapshot(ctx, snapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.DeploymentID != record.ID {
		t.Fatalf("deployment id = %s, want %s", got.DeploymentID, record.ID)
	}

	invalid := record
	invalid.ID = ids.New()
	invalid.Status = deployment.StatusProvisioning
	if err = second.SaveDeployment(ctx, invalid); err == nil {
		t.Fatal("expected deferred constraint to reject deployment without snapshot")
	}
}
