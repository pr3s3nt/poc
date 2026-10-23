package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/ports/persistence"
)

func TestReviewRequestIDIsUniquePerEnvironmentAfterRelease(t *testing.T) {
	ctx := context.Background()
	st := New()
	if err := st.SaveApplication(ctx, application.Application{Key: "app", OrganizationKey: "org"}); err != nil {
		t.Fatal(err)
	}
	for _, env := range []string{"staging", "production"} {
		if err := st.SaveEnvironment(ctx, environment.Environment{Key: env, ApplicationKey: "app"}); err != nil {
			t.Fatal(err)
		}
	}
	id := "11111111-1111-4111-8111-111111111111"
	claim := persistence.OperationClaim{ID: "first", ApplicationKey: "app", EnvironmentKey: "staging", Owner: "writer", Kind: environment.OpDeploy, ReviewRequestID: id, Deadline: time.Now().Add(time.Hour)}
	op, err := st.ClaimEnvironment(ctx, claim)
	if err != nil {
		t.Fatal(err)
	}
	if op.ReviewRequestID != id {
		t.Fatal("correlation not persisted")
	}
	if err := st.ReleaseOperation(ctx, persistence.Owner{OperationID: op.ID, Owner: op.Owner, Fence: op.Fence}, environment.OpFailed, ""); err != nil {
		t.Fatal(err)
	}
	claim.ID = "retry"
	if _, err := st.ClaimEnvironment(ctx, claim); !errors.Is(err, persistence.ErrDuplicate) {
		t.Fatalf("reused ID admitted after failure: %v", err)
	}
	claim.EnvironmentKey = "production"
	if _, err := st.ClaimEnvironment(ctx, claim); err != nil {
		t.Fatalf("same ID in distinct scope rejected: %v", err)
	}
}
