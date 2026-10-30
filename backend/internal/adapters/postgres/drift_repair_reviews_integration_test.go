package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"orchestrator/internal/domain/drift"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/identity"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/terraformstate"
)

func TestRepairReviewIsPrivateActorBoundAndConsumedOnce(t *testing.T) {
	f := newStateFixture(t)
	ctx := context.Background()
	owner := persistence.Owner{OperationID: f.op.ID, Owner: f.op.Owner, Fence: f.op.Fence}
	if err := f.state.DeleteLock(persistence.WithOwner(ctx, owner), f.inst.ID, "lock-1", owner.OperationID, owner.Fence); err != nil {
		t.Fatal(err)
	}
	if err := f.console.ReleaseOperation(ctx, owner, environment.OpSucceeded, ""); err != nil {
		t.Fatal(err)
	}
	actor := identity.UserAccount{ID: ids.New(), OrganizationKey: "acme", Username: "repair-engineer", PasswordHash: "opaque-test-hash", Role: identity.RolePlatformEngineer, Status: identity.AccountActive}
	if err := f.console.SaveUserAccount(ctx, actor); err != nil {
		t.Fatal(err)
	}
	env, err := f.console.GetEnvironment(ctx, "app", "staging")
	if err != nil {
		t.Fatal(err)
	}
	scan, err := f.console.QueueDriftScan(ctx, drift.Scan{ID: ids.New(), OrganizationID: f.inst.Identity.OrganizationID, EnvironmentID: env.ID, ApplicationKey: "app", EnvironmentKey: "staging", RequestID: ids.New(), RequestDigest: strings.Repeat("a", 64), EnvironmentVersion: env.Version, Status: drift.Queued, Result: drift.Unknown, CreatedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	claim := func(kind environment.OperationKind) (environment.Operation, context.Context) {
		t.Helper()
		op, err := f.console.ClaimEnvironment(ctx, persistence.OperationClaim{ID: ids.New(), ApplicationKey: "app", EnvironmentKey: "staging", Kind: kind, Owner: "review-test", Deadline: time.Now().Add(time.Hour)})
		if err != nil {
			t.Fatal(err)
		}
		return op, persistence.WithOwner(ctx, persistence.Owner{OperationID: op.ID, Owner: op.Owner, Fence: op.Fence})
	}
	scanOp, scanCtx := claim(environment.OpDriftScan)
	scan, err = f.console.StartDriftScan(scanCtx, scan.OrganizationID, scan.ID, scan.Version)
	if err != nil {
		t.Fatal(err)
	}
	finished := time.Now().UTC()
	scan.Status, scan.Result, scan.FinishedAt = drift.Succeeded, drift.NotApplicable, &finished
	if err := f.console.FinishDriftScan(scanCtx, scan, scan.Version); err != nil {
		t.Fatal(err)
	}
	if err := f.console.ReleaseOperation(ctx, persistence.Owner{OperationID: scanOp.ID, Owner: scanOp.Owner, Fence: scanOp.Fence}, environment.OpSucceeded, ""); err != nil {
		t.Fatal(err)
	}
	prepareOp, prepareCtx := claim(environment.OpDriftRepair)
	now := time.Now().UTC()
	review := drift.RepairReview{ID: ids.New(), OrganizationID: f.inst.Identity.OrganizationID, EnvironmentID: env.ID, ScanID: scan.ID, ActorID: actor.ID, RequestID: ids.New(), RequestDigest: strings.Repeat("b", 64), EnvironmentVersion: env.Version, Status: drift.Preparing, Instances: []drift.RepairInstance{{InstanceID: f.inst.ID, InstanceVersion: f.inst.Version, BaselineID: ids.New(), Blockers: []string{}}}, Blockers: []string{}, CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute), OperationID: prepareOp.ID, Version: 1}
	row := terraformstate.PrivateRepairReview{Review: review, TokenHash: strings.Repeat("c", 64), Token: persistence.CipherArtifact{ID: ids.New(), OrganizationID: review.OrganizationID, OwnerID: review.ID, Purpose: "TERRAFORM_REVIEW_TOKEN", KeyID: "state-key", Nonce: make([]byte, 12), Ciphertext: []byte("encrypted-random-confirmation-token"), AADVersion: 1, CreatedAt: now}}
	if err := f.state.CreateRepairReview(prepareCtx, row); err != nil {
		t.Fatal(err)
	}
	plan := persistence.CipherArtifact{ID: ids.New(), OrganizationID: review.OrganizationID, OwnerID: f.inst.ID, Purpose: "TERRAFORM_REVIEW", KeyID: "state-key", Nonce: make([]byte, 12), Ciphertext: []byte("encrypted-saved-terraform-plan"), AADVersion: 1, CreatedAt: now}
	if err := f.state.PutRepairArtifact(prepareCtx, plan); err != nil {
		t.Fatal(err)
	}
	row.Review.Status = drift.Ready
	row.Review.Instances[0].ArtifactID = plan.ID
	row.Review.Instances[0].Report = drift.Report{Complete: true, Applyable: true, Proposed: []drift.Finding{{Address: "namespace.this", Actions: []string{"update"}, Paths: []string{"/metadata/labels"}}}}
	if err := f.state.UpdateRepairReview(prepareCtx, row, 1); err != nil {
		t.Fatal(err)
	}
	loaded, err := f.state.GetRepairReview(ctx, review.OrganizationID, env.ID, review.ID)
	if err != nil || loaded.Review.Instances[0].ArtifactID != plan.ID {
		t.Fatal("private plan ref lost", err)
	}
	if _, err := f.console.q(ctx).Exec(ctx, `SELECT token_cipher FROM drift_repair_reviews`); err == nil {
		t.Fatal("Console accessed token ciphertext")
	}
	if _, err := f.state.GetRepairReview(ctx, ids.New(), env.ID, review.ID); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatal("foreign review exposed", err)
	}
	if err := f.console.ReleaseOperation(ctx, persistence.Owner{OperationID: prepareOp.ID, Owner: prepareOp.Owner, Fence: prepareOp.Fence}, environment.OpSucceeded, ""); err != nil {
		t.Fatal(err)
	}
	confirmOp, confirmCtx := claim(environment.OpDriftRepair)
	request := ids.New()
	if _, err := f.state.ConsumeRepairReview(confirmCtx, review.OrganizationID, env.ID, review.ID, ids.New(), request, row.TokenHash, loaded.Review.Version); !errors.Is(err, persistence.ErrVersionConflict) {
		t.Fatal("another actor consumed review", err)
	}
	consumed, err := f.state.ConsumeRepairReview(confirmCtx, review.OrganizationID, env.ID, review.ID, actor.ID, request, row.TokenHash, loaded.Review.Version)
	if err != nil || consumed.Review.ConsumedOperationID != confirmOp.ID || consumed.Review.Status != drift.Consumed {
		t.Fatal("confirmation not durable", err)
	}
	if _, err := f.state.ConsumeRepairReview(confirmCtx, review.OrganizationID, env.ID, review.ID, actor.ID, request, row.TokenHash, consumed.Review.Version); err == nil {
		t.Fatal("review consumed twice")
	}
}
