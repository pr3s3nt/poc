package postgres

import (
	"context"
	"errors"
	domain "orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/persistence/persistencetest"
	"orchestrator/internal/seed/seedtest"
	"strings"
	"testing"
	"time"
)

func TestPostgresReviewCorrelationUniqueScopedAndReadableAfterReopen(t *testing.T) {
	ctx := context.Background()
	st, url := openFresh(t)
	opts := seedtest.Defaults()
	if err := seedtest.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	requestID := "aabbccdd-1111-4111-8111-111111111111"
	claim := persistence.OperationClaim{ID: ids.New(), ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey, Owner: "review", Kind: environment.OpDeploy, ReviewRequestID: requestID, Deadline: time.Now().Add(time.Hour)}
	op, err := st.ClaimEnvironment(ctx, claim)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ReleaseOperation(ctx, persistence.Owner{OperationID: op.ID, Owner: op.Owner, Fence: op.Fence}, environment.OpFailed, ""); err != nil {
		t.Fatal(err)
	}
	claim.ID = ids.New()
	claim.ReviewRequestID = strings.ToUpper(requestID)
	if _, err := st.ClaimEnvironment(ctx, claim); !errors.Is(err, persistence.ErrDuplicate) {
		t.Fatalf("duplicate admitted after release: %v", err)
	}
	st.Close()
	reopened, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err := reopened.GetReviewOperation(ctx, opts.ApplicationKey, opts.EnvironmentKey, strings.ToUpper(requestID))
	if err != nil || got.ID != op.ID || got.Status != environment.OpFailed {
		t.Fatalf("correlation lost on reopen: %+v %v", got, err)
	}
	if _, err := reopened.GetReviewOperation(ctx, opts.ApplicationKey, "foreign", requestID); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("foreign scope observed: %v", err)
	}
}

func TestPostgresDeploymentCorrelationCannotBeReassigned(t *testing.T) {
	ctx := context.Background()
	st, _ := openFresh(t)
	opts := seedtest.Defaults()
	if err := seedtest.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	record := persistencetest.SaveDeployment(t, st, opts, domain.StatusDeploying, time.Now().UTC())
	record.ReviewRequestID = "aabbccdd-1111-4111-8111-111111111111"
	if err := st.SaveDeployment(ctx, record); !errors.Is(err, persistence.ErrImmutable) {
		t.Fatalf("correlation reassigned: %v", err)
	}
	got, err := st.GetDeployment(ctx, record.ID)
	if err != nil || got.ReviewRequestID != "" {
		t.Fatalf("record changed: %+v %v", got, err)
	}
}
