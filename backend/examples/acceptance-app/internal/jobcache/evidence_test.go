package jobcache_test

import (
	"testing"

	"orchestrator/examples/acceptance-app/internal/jobcache"
)

func TestEvidenceKeepsFailuresPerJobAndOperation(t *testing.T) {
	e := jobcache.NewEvidence()
	e.Record(1, jobcache.OutcomeErrorDBFallback, jobcache.ReasonAuth)
	e.Record(2, jobcache.OutcomeMissRefillFailed, jobcache.ReasonAuth)
	// Healthy lookups of other jobs clear nothing.
	e.Record(3, jobcache.OutcomeHit, jobcache.ReasonNone)
	e.Record(4, jobcache.OutcomeMissRefilled, jobcache.ReasonNone)
	got, overflow := e.Failures()
	if overflow || len(got) != 2 || got[0].JobID != 1 || got[0].Kind != jobcache.FailureRead || got[1].JobID != 2 || got[1].Kind != jobcache.FailureRefill {
		t.Fatalf("failures = %+v overflow=%v", got, overflow)
	}
	// A hit of job 2 proves GET only; its refill (SET) failure stays.
	e.Record(2, jobcache.OutcomeHit, jobcache.ReasonNone)
	if got, _ := e.Failures(); len(got) != 2 {
		t.Fatalf("hit must not clear a refill failure: %+v", got)
	}
	// Only the same job's successful refill clears it, and only that job's.
	e.Record(2, jobcache.OutcomeMissRefilled, jobcache.ReasonNone)
	if got, _ := e.Failures(); len(got) != 1 || got[0].JobID != 1 {
		t.Fatalf("%+v", got)
	}
	e.Record(1, jobcache.OutcomeHit, jobcache.ReasonNone)
	if _, failed := e.Failure(); failed {
		t.Fatal("all failures recovered")
	}
}

func TestEvidenceOverflowFailsClosed(t *testing.T) {
	e := jobcache.NewEvidence()
	for id := int64(1); id <= jobcache.MaxTrackedFailures+5; id++ {
		e.Record(id, jobcache.OutcomeErrorDBFallback, jobcache.ReasonAuth)
	}
	got, overflow := e.Failures()
	if len(got) != jobcache.MaxTrackedFailures || !overflow {
		t.Fatalf("len=%d overflow=%v", len(got), overflow)
	}
	for id := int64(1); id <= jobcache.MaxTrackedFailures+5; id++ {
		e.Record(id, jobcache.OutcomeHit, jobcache.ReasonNone)
	}
	f, failed := e.Failure()
	if !failed || f.Reason != jobcache.ReasonOverflow {
		t.Fatalf("forgotten failures can no longer be verified, so it must stay failed: %+v %v", f, failed)
	}
}
