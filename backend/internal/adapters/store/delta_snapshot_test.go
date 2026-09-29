package store

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/ports/persistence"
)

func testSnapshot(t *testing.T, id string) deployment.DeploymentDeltaSnapshot {
	t.Helper()
	doc := deployment.DeltaDocument{
		Modules: &deployment.ModuleDelta{
			Add: map[string]environment.Module{"api": {
				Profile: environment.ModuleProfile,
				Spec: environment.ModuleSpec{Containers: map[string]environment.Container{
					"main": {ID: "main", Image: "api:v1", Args: []string{"serve"}},
				}},
			}},
			Update: map[string][]deployment.JSONPatchOperation{
				"worker": {{Op: deployment.PatchAdd, Path: "/spec/containers/main/args/-", Value: "--fast"}},
			},
		},
		Shared: []deployment.JSONPatchOperation{{Op: deployment.PatchReplace, Path: "/cache/params/memory", Value: 4}},
	}
	snapshot, err := deployment.NewDeploymentDeltaSnapshot(id, "app", doc,
		deployment.DeltaSnapshotMetadata{ActorRef: "dev", Action: deployment.ActionDeploy, WorkloadID: "api"},
		time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	return snapshot
}

func TestDeltaSnapshotSaveReloadAndImmutability(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.json")
	s, err := NewWithSnapshot(path)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	snapshot := testSnapshot(t, "snap-1")
	if err := s.SaveDeltaSnapshot(ctx, snapshot); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := s.SaveDeltaSnapshot(ctx, snapshot); !errors.Is(err, persistence.ErrImmutable) {
		t.Fatalf("a snapshot must be written only once, got %v", err)
	}

	reopened, err := NewWithSnapshot(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	loaded, err := reopened.GetDeltaSnapshot(ctx, "snap-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if err := loaded.Validate(); err != nil {
		t.Fatalf("reloaded snapshot must keep its fingerprint: %v", err)
	}
	if loaded.DocumentHash != snapshot.DocumentHash || loaded.Metadata != snapshot.Metadata || !loaded.CreatedAt.Equal(snapshot.CreatedAt) {
		t.Fatalf("reloaded snapshot differs: %#v", loaded)
	}
	if !reflect.DeepEqual(loaded.Document.Modules.Add, snapshot.Document.Modules.Add) {
		t.Fatalf("typed modules.add did not round-trip: %#v", loaded.Document.Modules.Add)
	}
	if _, err := reopened.GetDeltaSnapshot(ctx, "missing"); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestDeltaSnapshotRejectsTamperedHash(t *testing.T) {
	snapshot := testSnapshot(t, "snap-1")
	snapshot.DocumentHash = "bogus"
	if err := New().SaveDeltaSnapshot(context.Background(), snapshot); err == nil {
		t.Fatal("expected a fingerprint error")
	}
}

func TestDeploymentReferencesExactlyOneDeltaSnapshot(t *testing.T) {
	ctx := context.Background()
	s := New()
	for _, id := range []string{"snap-1", "snap-2"} {
		if err := s.SaveDeltaSnapshot(ctx, testSnapshot(t, id)); err != nil {
			t.Fatalf("save %s: %v", id, err)
		}
	}

	planning := deployment.Deployment{ID: "dep-1", Status: deployment.StatusPlanning}
	if err := s.SaveDeployment(ctx, planning); err != nil {
		t.Fatalf("a PLANNING deployment has no snapshot yet: %v", err)
	}
	failedPlanning := planning
	failedPlanning.Status = deployment.StatusFailed
	failedPlanning.FailureReason = "planner rejected resource"
	if err := s.SaveDeployment(ctx, failedPlanning); err != nil {
		t.Fatalf("planning failure may keep a NULL snapshot: %v", err)
	}
	if got, err := s.GetDeployment(ctx, planning.ID); err != nil || got.DeltaSnapshotID != "" || got.Status != deployment.StatusFailed {
		t.Fatalf("planning failure record: %#v, %v", got, err)
	}
	provisioning := planning
	provisioning.Status = deployment.StatusProvisioning
	if err := s.SaveDeployment(ctx, provisioning); err == nil {
		t.Fatal("a deployment must not leave PLANNING without a delta snapshot")
	}
	provisioning.DeltaSnapshotID = "missing"
	if err := s.SaveDeployment(ctx, provisioning); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("expected a missing snapshot error, got %v", err)
	}
	provisioning.DeltaSnapshotID = "snap-1"
	if err := s.SaveDeployment(ctx, provisioning); err != nil {
		t.Fatalf("save with snapshot: %v", err)
	}

	moved := provisioning
	moved.DeltaSnapshotID = "snap-2"
	if err := s.SaveDeployment(ctx, moved); !errors.Is(err, persistence.ErrImmutable) {
		t.Fatalf("the association must not change, got %v", err)
	}

	other := deployment.Deployment{ID: "dep-2", Status: deployment.StatusProvisioning, DeltaSnapshotID: "snap-1"}
	if err := s.SaveDeployment(ctx, other); !errors.Is(err, persistence.ErrImmutable) {
		t.Fatalf("a snapshot must belong to one deployment, got %v", err)
	}
	other.DeltaSnapshotID = "snap-2"
	if err := s.SaveDeployment(ctx, other); err != nil {
		t.Fatalf("a second deployment with its own snapshot: %v", err)
	}

	succeeded := provisioning
	succeeded.Status = deployment.StatusSucceeded
	if err := s.SaveDeployment(ctx, succeeded); err != nil {
		t.Fatalf("status change keeps the association: %v", err)
	}
	got, err := s.GetDeployment(ctx, "dep-1")
	if err != nil || got.DeltaSnapshotID != "snap-1" {
		t.Fatalf("reload deployment: %#v, %v", got, err)
	}
}

func TestTransactionRollsBackDeltaSnapshot(t *testing.T) {
	ctx := context.Background()
	s := New()
	wantErr := errors.New("boom")
	err := s.Transact(ctx, func(ctx context.Context) error {
		if err := s.SaveDeltaSnapshot(ctx, testSnapshot(t, "snap-1")); err != nil {
			return err
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected the transaction error, got %v", err)
	}
	if _, err := s.GetDeltaSnapshot(ctx, "snap-1"); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("a rolled-back snapshot must not persist, got %v", err)
	}
}

// mutateNested changes every nested level of a Snapshot's Delta document in
// place: maps, slices, typed module fields and any values.
func mutateNested(snapshot *deployment.DeploymentDeltaSnapshot) {
	doc := &snapshot.Document
	api := doc.Modules.Add["api"]
	main := api.Spec.Containers["main"]
	main.Args[0] = "mutated"
	api.Spec.Containers["main"] = main
	api.Spec.Containers["sidecar"] = environment.Container{Image: "evil:v1"}
	doc.Modules.Add["intruder"] = environment.Module{Profile: "x"}
	doc.Modules.Update["worker"][0].Value = "mutated"
	doc.Modules.Update["worker"] = append(doc.Modules.Update["worker"], deployment.JSONPatchOperation{Op: deployment.PatchRemove, Path: "/spec"})
	doc.Shared[0].Path = "/mutated"
	doc.Shared[0].Value = map[string]any{"nested": "mutated"}
	snapshot.Metadata.WorkloadID = "mutated"
}

func assertStoredUnchanged(t *testing.T, s *Store, want deployment.DeploymentDeltaSnapshot) {
	t.Helper()
	got, err := s.GetDeltaSnapshot(context.Background(), want.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("stored snapshot no longer matches its hash: %v", err)
	}
	gotJSON, _ := json.Marshal(got)
	wantJSON, _ := json.Marshal(want)
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("stored snapshot changed\n got: %s\nwant: %s", gotJSON, wantJSON)
	}
}

func TestDeltaSnapshotSaveDoesNotAliasCallerValue(t *testing.T) {
	s := New()
	input := testSnapshot(t, "snap-1")
	pristine := testSnapshot(t, "snap-1")
	if err := s.SaveDeltaSnapshot(context.Background(), input); err != nil {
		t.Fatalf("save: %v", err)
	}
	mutateNested(&input)
	if input.Document.Modules.Add["api"].Spec.Containers["main"].Args[0] != "mutated" {
		t.Fatal("test setup: the caller value was not mutated")
	}
	assertStoredUnchanged(t, s, pristine)
}

func TestDeltaSnapshotGetDoesNotAliasStoredState(t *testing.T) {
	s := New()
	pristine := testSnapshot(t, "snap-1")
	if err := s.SaveDeltaSnapshot(context.Background(), testSnapshot(t, "snap-1")); err != nil {
		t.Fatalf("save: %v", err)
	}
	first, err := s.GetDeltaSnapshot(context.Background(), "snap-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	mutateNested(&first)
	assertStoredUnchanged(t, s, pristine)
}
