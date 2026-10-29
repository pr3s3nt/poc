package jobstore_test

import (
	"context"
	"strings"
	"testing"

	"orchestrator/examples/acceptance-app/internal/jobstore"
	"orchestrator/examples/acceptance-app/internal/testfixture"
)

func TestScheduledReceiptIsScopedIdempotentAndSurvivesReopen(t *testing.T) {
	cfg := testfixture.PostgresConfigWithPassword(t, "scheduled-test-password")
	ctx := context.Background()
	s, e := jobstore.Open(ctx, cfg, 1)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if e = s.Migrate(ctx); e != nil {
		t.Fatal(e)
	}
	job, e := s.Submit(ctx, "existing-job")
	if e != nil {
		t.Fatal(e)
	}
	hash := strings.Repeat("a", 64)
	for i := 0; i < 2; i++ {
		if e = s.RecordScheduledRun(ctx, "run-a", "pod-a", hash); e != nil {
			t.Fatal(e)
		}
	}
	rows, e := s.ScheduledRuns(ctx, "run-a")
	if e != nil || len(rows) != 1 {
		t.Fatalf("%+v %v", rows, e)
	}
	stamp := rows[0].CompletedAt
	if e = s.RecordScheduledRun(ctx, "run-a", "pod-a", strings.Repeat("b", 64)); e == nil {
		t.Fatal("conflict must fail")
	}
	if e = s.RecordScheduledRun(ctx, "run-b", "pod-a", hash); e != nil {
		t.Fatal(e)
	}
	if e = s.Migrate(ctx); e != nil {
		t.Fatal(e)
	}
	other, e := jobstore.Open(ctx, cfg, 1)
	if e != nil {
		t.Fatal(e)
	}
	defer other.Close()
	rows, e = other.ScheduledRuns(ctx, "run-a")
	if e != nil || len(rows) != 1 || !rows[0].CompletedAt.Equal(stamp) {
		t.Fatalf("reopen: %+v %v", rows, e)
	}
	if _, found, e := other.Get(ctx, job.ID); e != nil || !found {
		t.Fatal("migration lost existing job")
	}
}
