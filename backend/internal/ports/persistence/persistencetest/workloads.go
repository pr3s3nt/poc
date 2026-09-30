// Package persistencetest holds adapter-neutral contract checks that every
// persistence.Store implementation must pass. Adapter tests call them with a
// fresh store so the in-memory, JSON and PostgreSQL adapters stay in parity.
package persistencetest

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"orchestrator/internal/domain/deployment"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/seed"
)

// Fixture is what a contract run created, so adapter tests can reopen the
// store and check the same rows survived a restart.
type Fixture struct {
	Options     seed.Options
	FirstID     string
	SecondID    string
	FirstReady  deployment.WorkloadSnapshot
	SecondState deployment.WorkloadSnapshot
}

// SaveDeployment writes a running Deployment in the seeded Environment.
func SaveDeployment(t *testing.T, st persistence.Store, opts seed.Options, status deployment.Status, startedAt time.Time) deployment.Deployment {
	t.Helper()
	ctx := context.Background()
	env, err := st.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}
	record := deployment.Deployment{
		ID: ids.New(), EnvironmentID: env.ID, OrganizationKey: opts.OrganizationKey,
		ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey,
		ExecutionProfile: "internal-k8s", Action: deployment.ActionDeploy, WorkloadID: "api",
		ActorRef: "contract", Status: status, BaseEnvironmentVersion: env.Version, StartedAt: startedAt,
	}
	record.Status = deployment.StatusPlanning
	if err := st.SaveDeployment(ctx, record); err != nil {
		t.Fatal(err)
	}
	if status == deployment.StatusPlanning || status == deployment.StatusFailed {
		record.Status = status
		if err := st.SaveDeployment(ctx, record); err != nil {
			t.Fatal(err)
		}
		return record
	}
	// Past planning, a Deployment must own exactly one Delta Snapshot.
	snapshot, err := deployment.NewDeploymentDeltaSnapshot(ids.New(), record.ID, deployment.DeltaDocument{}, deployment.DeltaSnapshotMetadata{Action: record.Action, WorkloadID: record.WorkloadID}, startedAt)
	if err != nil {
		t.Fatal(err)
	}
	err = st.Transact(ctx, func(ctx context.Context) error {
		if err := st.SaveDeltaSnapshot(ctx, snapshot); err != nil {
			return err
		}
		record.Status = status
		record.DeltaSnapshotID = snapshot.ID
		return st.SaveDeployment(ctx, record)
	})
	if err != nil {
		t.Fatal(err)
	}
	return record
}

// WorkloadSnapshots checks per-Deployment workload history: ownership,
// terminal immutability, isolation from later runs and from caller aliases.
func WorkloadSnapshots(t *testing.T, st persistence.Store) Fixture {
	t.Helper()
	ctx := context.Background()
	opts := seed.Defaults()
	if err := seed.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	environmentKey := opts.ApplicationKey + "/" + opts.EnvironmentKey
	first := SaveDeployment(t, st, opts, deployment.StatusDeploying, now)

	target := map[string]any{"namespace": "team-a", "labels": map[string]any{"tier": "api"}, "hosts": []any{"a.example"}}
	progress := deployment.WorkloadInstance{
		EnvironmentKey: environmentKey, WorkloadID: "api", LastDeploymentID: first.ID,
		TargetRef: target, ManifestDigest: "sha256:first", Status: deployment.InstanceReady, ObservedAt: now,
	}
	if err := st.UpsertWorkloadProgress(ctx, progress); err != nil {
		t.Fatalf("progress: %v", err)
	}
	// Mutating the caller's map after the write must not change stored state.
	target["namespace"] = "mutated"
	target["labels"].(map[string]any)["tier"] = "mutated"
	target["hosts"].([]any)[0] = "mutated"
	firstRows := listOne(t, st, first.ID)
	if firstRows.TargetRef["namespace"] != "team-a" || firstRows.TargetRef["labels"].(map[string]any)["tier"] != "api" || firstRows.TargetRef["hosts"].([]any)[0] != "a.example" {
		t.Fatalf("stored snapshot aliases caller map: %#v", firstRows.TargetRef)
	}
	// Mutating a returned map must not change stored state either.
	firstRows.TargetRef["namespace"] = "mutated"
	firstRows.TargetRef["labels"].(map[string]any)["tier"] = "mutated"
	if again := listOne(t, st, first.ID); again.TargetRef["namespace"] != "team-a" || again.TargetRef["labels"].(map[string]any)["tier"] != "api" {
		t.Fatalf("stored snapshot aliases returned map: %#v", again.TargetRef)
	}
	current, err := st.ListWorkloadInstances(ctx, environmentKey)
	if err != nil || len(current) != 1 || current[0].TargetRef["namespace"] != "team-a" {
		t.Fatalf("current instance aliases caller map: %#v, %v", current, err)
	}

	wrongEnvironment := progress
	wrongEnvironment.EnvironmentKey = opts.ApplicationKey + "/not-this-environment"
	if err := st.UpsertWorkloadProgress(ctx, wrongEnvironment); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("progress for another Environment = %v", err)
	}
	missing := progress
	missing.LastDeploymentID = ids.New()
	if err := st.UpsertWorkloadProgress(ctx, missing); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("progress for missing Deployment = %v", err)
	}

	finished := now.Add(time.Second)
	first.Status = deployment.StatusSucceeded
	first.FinishedAt = &finished
	if err := st.SaveDeployment(ctx, first); err != nil {
		t.Fatal(err)
	}
	late := progress
	late.TargetRef = map[string]any{"namespace": "late"}
	late.Status = deployment.InstanceFailed
	if err := st.UpsertWorkloadProgress(ctx, late); !errors.Is(err, persistence.ErrImmutable) {
		t.Fatalf("progress after terminal = %v", err)
	}

	second := SaveDeployment(t, st, opts, deployment.StatusDeploying, now.Add(2*time.Second))
	redeploy := deployment.WorkloadInstance{
		EnvironmentKey: environmentKey, WorkloadID: "api", LastDeploymentID: second.ID,
		TargetRef: map[string]any{"namespace": "team-b"}, ManifestDigest: "sha256:second",
		Status: deployment.InstanceFailed, ObservedAt: now.Add(3 * time.Second),
	}
	if err := st.UpsertWorkloadProgress(ctx, redeploy); err != nil {
		t.Fatal(err)
	}
	// A post-commit current-row correction touches only the current row.
	correction := redeploy
	correction.ManifestDigest = "sha256:correction"
	if err := st.UpsertWorkloadInstance(ctx, correction); err != nil {
		t.Fatal(err)
	}
	firstAfter := listOne(t, st, first.ID)
	if firstAfter.ManifestDigest != "sha256:first" || firstAfter.Status != deployment.InstanceReady || firstAfter.TargetRef["namespace"] != "team-a" {
		t.Fatalf("old snapshot followed a later run: %#v", firstAfter)
	}
	secondAfter := listOne(t, st, second.ID)
	if secondAfter.ManifestDigest != "sha256:second" || secondAfter.Status != deployment.InstanceFailed {
		t.Fatalf("new snapshot followed the current-row correction: %#v", secondAfter)
	}
	return Fixture{Options: opts, FirstID: first.ID, SecondID: second.ID, FirstReady: firstAfter, SecondState: secondAfter}
}

// AssertReloaded checks that a reopened store still holds the fixture rows and
// still refuses to change a terminal Deployment's snapshot.
func AssertReloaded(t *testing.T, st persistence.Store, fixture Fixture) {
	t.Helper()
	if got := listOne(t, st, fixture.FirstID); !sameSnapshot(got, fixture.FirstReady) {
		t.Fatalf("first snapshot after reload = %#v, want %#v", got, fixture.FirstReady)
	}
	if got := listOne(t, st, fixture.SecondID); !sameSnapshot(got, fixture.SecondState) {
		t.Fatalf("second snapshot after reload = %#v, want %#v", got, fixture.SecondState)
	}
	late := deployment.WorkloadInstance{
		EnvironmentKey: fixture.Options.ApplicationKey + "/" + fixture.Options.EnvironmentKey, WorkloadID: "api",
		LastDeploymentID: fixture.FirstID, TargetRef: map[string]any{}, Status: deployment.InstanceFailed, ObservedAt: time.Now().UTC(),
	}
	if err := st.UpsertWorkloadProgress(context.Background(), late); !errors.Is(err, persistence.ErrImmutable) {
		t.Fatalf("terminal snapshot writable after reload: %v", err)
	}
}

// ReadSnapshot checks that ReadSnapshot sees one point in time and that
// nothing written through its view is persisted.
func ReadSnapshot(t *testing.T, st persistence.Store) {
	t.Helper()
	ctx := context.Background()
	opts := seed.Defaults()
	if err := seed.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	before := SaveDeployment(t, st, opts, deployment.StatusFailed, time.Now().UTC())
	var concurrent deployment.Deployment
	err := st.ReadSnapshot(ctx, func(readCtx context.Context, view persistence.Store) error {
		first, err := view.ListDeployments(readCtx, opts.ApplicationKey, opts.EnvironmentKey)
		if err != nil {
			return err
		}
		// A commit made after the snapshot began must stay invisible to it.
		concurrent = SaveDeployment(t, st, opts, deployment.StatusFailed, time.Now().UTC().Add(time.Minute))
		second, err := view.ListDeployments(readCtx, opts.ApplicationKey, opts.EnvironmentKey)
		if err != nil {
			return err
		}
		if len(first) != len(second) || !containsID(second, before.ID) || containsID(second, concurrent.ID) {
			t.Errorf("snapshot changed during read: before=%d after=%d", len(first), len(second))
		}
		if _, err := view.GetDeployment(readCtx, "does-not-exist"); !errors.Is(err, persistence.ErrNotFound) {
			t.Errorf("non-UUID Deployment ID = %v, want ErrNotFound", err)
		}
		// Writes through the read view are rejected or discarded.
		changed := before
		changed.FailureReason = "written through read view"
		_ = view.SaveDeployment(readCtx, changed)
		return nil
	})
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	stored, err := st.GetDeployment(ctx, before.ID)
	if err != nil || stored.FailureReason != "" {
		t.Fatalf("read view persisted a write: %#v, %v", stored, err)
	}
	if _, err := st.GetDeployment(ctx, concurrent.ID); err != nil {
		t.Fatalf("concurrent commit lost: %v", err)
	}
}

// TerminalSerialization checks that workload progress racing the terminal
// status transition waits for it and is then refused, never landing after it.
func TerminalSerialization(t *testing.T, st persistence.Store) {
	t.Helper()
	ctx := context.Background()
	opts := seed.Defaults()
	if err := seed.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	record := SaveDeployment(t, st, opts, deployment.StatusDeploying, time.Now().UTC())
	progress := deployment.WorkloadInstance{
		EnvironmentKey: opts.ApplicationKey + "/" + opts.EnvironmentKey, WorkloadID: "racer",
		LastDeploymentID: record.ID, TargetRef: map[string]any{}, ManifestDigest: "sha256:late",
		Status: deployment.InstanceReady, ObservedAt: time.Now().UTC(),
	}
	result := make(chan error, 1)
	err := st.Transact(ctx, func(txCtx context.Context) error {
		finished := time.Now().UTC()
		record.Status = deployment.StatusFailed
		record.FinishedAt = &finished
		if err := st.SaveDeployment(txCtx, record); err != nil {
			return err
		}
		go func() { result <- st.UpsertWorkloadProgress(ctx, progress) }()
		select {
		case err := <-result:
			t.Errorf("progress did not wait for the terminal transition: %v", err)
		case <-time.After(200 * time.Millisecond):
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if !errors.Is(err, persistence.ErrImmutable) {
			t.Fatalf("progress after terminal commit = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("progress never finished")
	}
	if rows, err := st.ListDeploymentWorkloads(ctx, record.ID); err != nil || len(rows) != 0 {
		t.Fatalf("late progress landed after terminal: %#v, %v", rows, err)
	}
}

func listOne(t *testing.T, st persistence.Store, deploymentID string) deployment.WorkloadSnapshot {
	t.Helper()
	rows, err := st.ListDeploymentWorkloads(context.Background(), deploymentID)
	if err != nil || len(rows) != 1 {
		t.Fatalf("workload snapshots of %s = %#v, %v", deploymentID, rows, err)
	}
	return rows[0]
}

func sameSnapshot(a, b deployment.WorkloadSnapshot) bool {
	return a.DeploymentID == b.DeploymentID && a.WorkloadID == b.WorkloadID && a.Status == b.Status &&
		a.ManifestDigest == b.ManifestDigest && a.AppliedConfigRevisionID == b.AppliedConfigRevisionID &&
		a.ObservedAt.Equal(b.ObservedAt) && reflect.DeepEqual(a.TargetRef, b.TargetRef)
}

func containsID(list []deployment.Deployment, id string) bool {
	for _, item := range list {
		if item.ID == id {
			return true
		}
	}
	return false
}
