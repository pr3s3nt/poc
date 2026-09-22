package deployment_test

import (
	"context"
	"encoding/json"
	"testing"

	appsvc "orchestrator/internal/application/deployment"
	domain "orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/planning"
	"orchestrator/internal/seed"
)

func TestDeployWorkload_PersistsOneDeltaSnapshotPerDeployment(t *testing.T) {
	ctx := context.Background()
	app, seedOptions := newApp(t)
	results := deployAll(t, app, seedOptions)

	seen := map[string]string{}
	for _, r := range results {
		record, err := app.Store.GetDeployment(ctx, r.DeploymentID)
		if err != nil {
			t.Fatalf("get deployment: %v", err)
		}
		if record.DeltaSnapshotID == "" {
			t.Fatalf("deployment %s has no delta snapshot", r.DeploymentID)
		}
		if owner, dup := seen[record.DeltaSnapshotID]; dup {
			t.Fatalf("snapshot %s is shared by %s and %s", record.DeltaSnapshotID, owner, r.DeploymentID)
		}
		seen[record.DeltaSnapshotID] = r.DeploymentID

		snapshot, err := app.Store.GetDeltaSnapshot(ctx, record.DeltaSnapshotID)
		if err != nil {
			t.Fatalf("get snapshot: %v", err)
		}
		if err := snapshot.Validate(); err != nil {
			t.Fatalf("snapshot: %v", err)
		}
		if snapshot.ApplicationKey != seedOptions.ApplicationKey || snapshot.Metadata.WorkloadID != r.WorkloadID ||
			snapshot.Metadata.Action != domain.ActionDeploy || snapshot.Metadata.ActorRef != "test" {
			t.Fatalf("unexpected snapshot metadata: %#v", snapshot)
		}
		if snapshot.Document.Modules == nil || len(snapshot.Document.Modules.Add) != 1 {
			t.Fatalf("first deploy of %s must add exactly one module: %#v", r.WorkloadID, snapshot.Document)
		}
		if _, ok := snapshot.Document.Modules.Add[r.WorkloadID]; !ok {
			t.Fatalf("expected modules.add.%s", r.WorkloadID)
		}

		base := environment.NewDocument()
		if record.BaseDeploymentSetID != "" {
			set, err := app.Store.GetDeploymentSet(ctx, record.BaseDeploymentSetID)
			if err != nil {
				t.Fatalf("base set: %v", err)
			}
			base = set.Document
		}
		candidate, err := app.Store.GetDeploymentSet(ctx, record.CandidateDeploymentSet)
		if err != nil {
			t.Fatalf("candidate set: %v", err)
		}
		if err := planning.VerifyDelta(base, snapshot.Document, candidate.Document); err != nil {
			t.Fatalf("persisted snapshot: %v", err)
		}

		plan, err := app.Store.GetPlan(ctx, r.DeploymentID)
		if err != nil {
			t.Fatalf("get plan: %v", err)
		}
		if _, ok := plan["delta"]; ok {
			t.Fatal("the persisted plan must not carry a whole-document delta")
		}

		view, err := app.Queries.GetDeployment(ctx, r.DeploymentID)
		if err != nil {
			t.Fatalf("view: %v", err)
		}
		if view.Delta == nil || view.DeltaHash != snapshot.DocumentHash {
			t.Fatalf("the view must expose the persisted delta snapshot: %#v", view.Delta)
		}
		got, _ := json.Marshal(view.Delta)
		want, _ := json.Marshal(snapshot.Document)
		if string(got) != string(want) {
			t.Fatalf("view delta %s != snapshot %s", got, want)
		}
	}
}

func TestDeployWorkload_RedeployWithoutChangePersistsEmptyDelta(t *testing.T) {
	ctx := context.Background()
	app, seedOptions := newApp(t)
	deployAll(t, app, seedOptions)

	score := seed.AcceptanceScores(seedOptions)["backend"]
	result, err := app.Deployments.DeployWorkload(ctx, appsvc.DeployCommand{
		OrganizationKey: seedOptions.OrganizationKey,
		ApplicationKey:  seedOptions.ApplicationKey,
		EnvironmentKey:  seedOptions.EnvironmentKey,
		WorkloadID:      "backend",
		ScoreBefore:     score,
		ScoreAfter:      score,
		Actor:           "test",
	})
	if err != nil {
		t.Fatalf("redeploy: %v", err)
	}
	record, err := app.Store.GetDeployment(ctx, result.DeploymentID)
	if err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	snapshot, err := app.Store.GetDeltaSnapshot(ctx, record.DeltaSnapshotID)
	if err != nil {
		t.Fatalf("get snapshot: %v", err)
	}
	if b, _ := json.Marshal(snapshot.Document); string(b) != "{}" {
		t.Fatalf("expected no-op delta {}, got %s", b)
	}
	if snapshot.Metadata.Action != domain.ActionUpdate {
		t.Fatalf("expected UPDATE metadata, got %s", snapshot.Metadata.Action)
	}
}
