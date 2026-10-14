package jobcache

import (
	"context"
	"sort"
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

// Recheck re-attempts the exact cache operation of each recorded failure (a
// GET or the refill SET for that job's key), at most recheckLimit per call and
// within ctx, so a recovered Redis clears them without user traffic: a not-ready
// backend receives none. A success clears only that job+operation. It reads
// PostgreSQL and Redis only, never reprocesses or inserts a job, and does not
// touch the lookup counters.
func (r *Resolver) Recheck(ctx context.Context) {
	if r.evidence == nil || r.cache == nil {
		return
	}
	failures, _ := r.evidence.Failures()
	for i, f := range failures {
		if i >= recheckLimit || ctx.Err() != nil {
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
	failures map[failureKey]Failure
	overflow bool
}

type failureKey struct {
	job  int64
	kind FailureKind
}

// MaxTrackedFailures bounds the unrecovered failures kept in memory. Past it the
// evidence fails closed: it stays failed (reason failure-overflow) for the life
// of the process, because forgotten failures can no longer be verified.
const MaxTrackedFailures = 64

// recheckLimit bounds how many failed operations one diagnostics call retries.
const recheckLimit = 16

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
func NewEvidence() *Evidence {
	return &Evidence{counters: map[Outcome]int64{}, failures: map[failureKey]Failure{}}
}

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

// track updates the per-job, per-operation failure set from one real lookup.
// A success clears only the failure of the same job and operation, so a healthy
// lookup of job B can never erase the unresolved failure of job A.
func (e *Evidence) track(id int64, outcome Outcome, reason Reason) {
	switch outcome {
	case OutcomeErrorDBFallback:
		e.setFailure(Failure{Kind: FailureRead, Reason: reason, JobID: id})
	case OutcomeMissRefillFailed:
		delete(e.failures, failureKey{id, FailureRead}) // the GET worked
		e.setFailure(Failure{Kind: FailureRefill, Reason: reason, JobID: id})
	case OutcomeMissRefilled:
		delete(e.failures, failureKey{id, FailureRead})
		delete(e.failures, failureKey{id, FailureRefill})
	case OutcomeHit:
		delete(e.failures, failureKey{id, FailureRead}) // GET of this key works; a refill failure needs a SET
	}
}

func (e *Evidence) setFailure(f Failure) {
	k := failureKey{f.JobID, f.Kind}
	if _, exists := e.failures[k]; !exists && len(e.failures) >= MaxTrackedFailures {
		e.overflow = true
		return
	}
	e.failures[k] = f
}

// Failures returns the unrecovered failures ordered by job id then operation,
// and whether older failures were dropped (overflow), which is sticky.
func (e *Evidence) Failures() (list []Failure, overflow bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, f := range e.failures {
		list = append(list, f)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].JobID != list[j].JobID {
			return list[i].JobID < list[j].JobID
		}
		return list[i].Kind < list[j].Kind
	})
	return list, e.overflow
}

// Failure returns the reason to report while any failure is unrecovered.
func (e *Evidence) Failure() (Failure, bool) {
	list, overflow := e.Failures()
	if len(list) > 0 {
		return list[0], true
	}
	if overflow {
		return Failure{Kind: FailureRead, Reason: ReasonOverflow}, true
	}
	return Failure{}, false
}

func (e *Evidence) recover(f Failure) {
	e.mu.Lock()
	defer e.mu.Unlock()
	k := failureKey{f.JobID, f.Kind}
	if cur, ok := e.failures[k]; ok && cur == f {
		delete(e.failures, k)
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
