package jobcache

import (
	"context"
	"sync"
	"time"

	"orchestrator/examples/acceptance-app/internal/jobstore"
)

// Outcome describes how one lookup was served. It is evidence for the runner
// and carries no job data.
type Outcome string

const (
	OutcomeHit              Outcome = "hit"
	OutcomeMissRefilled     Outcome = "miss-refilled"
	OutcomeMissRefillFailed Outcome = "miss-refill-failed"
	OutcomeErrorDBFallback  Outcome = "error-db-fallback"
	OutcomePending          Outcome = "pending"
	OutcomeNotFound         Outcome = "not-found"
)

// Source is the durable store the resolver falls back to.
type Source interface {
	Get(ctx context.Context, id int64) (jobstore.Job, bool, error)
}

// Lookup is the result of resolving one job.
type Lookup struct {
	Job     jobstore.Job
	Found   bool
	Outcome Outcome
	// Reason is set when the cache failed, in which case the job (if found) was
	// served from PostgreSQL and the failure must be reported.
	Reason Reason
}

// Resolver reads the cache first and falls back to PostgreSQL.
type Resolver struct {
	cache    *Cache // nil when the binding is invalid; every lookup then reports a failure
	source   Source
	evidence *Evidence
}

// NewResolver builds a resolver. cache may be nil for an invalid binding.
func NewResolver(cache *Cache, source Source, evidence *Evidence) *Resolver {
	return &Resolver{cache: cache, source: source, evidence: evidence}
}

// Evidence returns the recorder the resolver reports to.
func (r *Resolver) Evidence() *Evidence { return r.evidence }

// Lookup resolves one job. The database error is returned as is; a cache
// failure is reported through Lookup.Reason and never makes the lookup fail.
func (r *Resolver) Lookup(ctx context.Context, id int64) (Lookup, error) {
	return r.lookup(ctx, id, false)
}

func (r *Resolver) lookup(ctx context.Context, id int64, cacheDown bool) (Lookup, error) {
	if r.cache == nil {
		return r.fallback(ctx, id, ReasonConfig)
	}
	if cacheDown {
		return r.fallback(ctx, id, ReasonUnreachable)
	}
	job, found, err := r.cache.Get(ctx, id)
	if err == nil && found {
		return r.record(id, Lookup{Job: job, Found: true, Outcome: OutcomeHit}), nil
	}
	if err != nil && err != ErrInvalidEntry {
		return r.fallback(ctx, id, Classify(err))
	}

	// Cache miss (or an unusable entry): read the same PostgreSQL row.
	job, found, err = r.source.Get(ctx, id)
	if err != nil {
		return Lookup{}, err
	}
	switch {
	case !found:
		return r.record(id, Lookup{Outcome: OutcomeNotFound}), nil
	case job.Status != jobstore.StatusDone:
		// A pending job is never cached, so the next read sees it complete.
		return r.record(id, Lookup{Job: job, Found: true, Outcome: OutcomePending}), nil
	}
	if putErr := r.cache.Put(ctx, job); putErr != nil {
		return r.record(id, Lookup{Job: job, Found: true, Outcome: OutcomeMissRefillFailed, Reason: Classify(putErr)}), nil
	}
	return r.record(id, Lookup{Job: job, Found: true, Outcome: OutcomeMissRefilled}), nil
}

// fallback serves the durable row after a cache failure. It never writes to
// the cache and never retries it.
func (r *Resolver) fallback(ctx context.Context, id int64, reason Reason) (Lookup, error) {
	job, found, err := r.source.Get(ctx, id)
	if err != nil {
		return Lookup{}, err
	}
	return r.record(id, Lookup{Job: job, Found: found, Outcome: OutcomeErrorDBFallback, Reason: reason}), nil
}

func (r *Resolver) record(id int64, l Lookup) Lookup {
	if r.evidence != nil {
		r.evidence.Record(id, l.Outcome, l.Reason)
	}
	return l
}

// Recheck re-attempts, once, the exact cache operation that last failed (a GET
// or the refill SET for the recorded job) so a recovered Redis clears the
// failure without needing user traffic: a not-ready backend receives none.
// It reads PostgreSQL and Redis only, never reprocesses or inserts a job, and
// does not touch the lookup counters.
func (r *Resolver) Recheck(ctx context.Context) {
	if r.evidence == nil || r.cache == nil {
		return
	}
	f, failed := r.evidence.Failure()
	if !failed {
		return
	}
	switch f.Kind {
	case FailureRead:
		if _, _, err := r.cache.Get(ctx, f.JobID); err == nil || err == ErrInvalidEntry {
			r.evidence.recover(f)
		}
	case FailureRefill:
		job, found, err := r.source.Get(ctx, f.JobID)
		if err == nil && found && job.Status == jobstore.StatusDone && r.cache.Put(ctx, job) == nil {
			r.evidence.recover(f)
		}
	}
}

// LookupCompleted resolves the completed jobs among summaries, keeping order.
// After the first connection-level failure the remaining lookups skip the
// cache instead of waiting for one timeout each; they are still reported as
// cache failures.
func (r *Resolver) LookupCompleted(ctx context.Context, summaries []jobstore.Job) ([]Lookup, error) {
	out := make([]Lookup, len(summaries))
	down := false
	for i, s := range summaries {
		if s.Status != jobstore.StatusDone {
			out[i] = Lookup{Job: s, Found: true, Outcome: OutcomePending}
			continue
		}
		l, err := r.lookup(ctx, s.ID, down)
		if err != nil {
			return nil, err
		}
		if l.Outcome == OutcomeErrorDBFallback {
			down = l.Reason != ReasonInvalid
		}
		out[i] = l
	}
	return out, nil
}

// Evidence accumulates safe, value-free lookup evidence for the process.
type Evidence struct {
	mu       sync.Mutex
	counters map[Outcome]int64
	recent   []Event
	failure  *Failure
}

// FailureKind says which real cache operation last failed.
type FailureKind string

const (
	FailureRead   FailureKind = "read"   // a GET failed
	FailureRefill FailureKind = "refill" // a GET worked but writing the entry (SET) failed
)

// Failure is the last unrecovered failure of a real lookup. It stays set until
// the same operation succeeds again, so a working PING or a database fallback
// can never hide it. Only the job id, kind and safe reason are kept.
type Failure struct {
	Kind   FailureKind `json:"kind"`
	Reason Reason      `json:"reason"`
	JobID  int64       `json:"jobId"`
}

// Event is one recorded lookup: ids and classifications only.
type Event struct {
	JobID   int64     `json:"jobId"`
	Outcome Outcome   `json:"outcome"`
	Reason  Reason    `json:"reason,omitempty"`
	At      time.Time `json:"at"`
}

const maxRecent = 50

// NewEvidence returns an empty recorder.
func NewEvidence() *Evidence { return &Evidence{counters: map[Outcome]int64{}} }

// Record adds one lookup. Pending jobs listed from the database are counted
// too, so counters equal the number of resolutions served.
func (e *Evidence) Record(id int64, outcome Outcome, reason Reason) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.counters[outcome]++
	e.recent = append(e.recent, Event{JobID: id, Outcome: outcome, Reason: reason, At: time.Now().UTC()})
	if len(e.recent) > maxRecent {
		e.recent = e.recent[len(e.recent)-maxRecent:]
	}
	e.track(id, outcome, reason)
}

// track updates the unrecovered-failure state from one real lookup.
func (e *Evidence) track(id int64, outcome Outcome, reason Reason) {
	switch outcome {
	case OutcomeErrorDBFallback:
		e.failure = &Failure{Kind: FailureRead, Reason: reason, JobID: id}
	case OutcomeMissRefillFailed:
		e.failure = &Failure{Kind: FailureRefill, Reason: reason, JobID: id}
	case OutcomeMissRefilled:
		e.failure = nil // GET and SET both worked
	case OutcomeHit:
		if e.failure != nil && e.failure.Kind == FailureRead {
			e.failure = nil // GET works again; a refill failure needs a SET to clear
		}
	}
}

// Failure returns the unrecovered real-lookup failure, if any.
func (e *Evidence) Failure() (Failure, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.failure == nil {
		return Failure{}, false
	}
	return *e.failure, true
}

func (e *Evidence) recover(f Failure) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.failure != nil && *e.failure == f {
		e.failure = nil
	}
}

// Snapshot returns a copy of the counters and recent events.
func (e *Evidence) Snapshot() (map[Outcome]int64, []Event) {
	e.mu.Lock()
	defer e.mu.Unlock()
	counters := make(map[Outcome]int64, len(e.counters))
	for k, v := range e.counters {
		counters[k] = v
	}
	return counters, append([]Event(nil), e.recent...)
}
