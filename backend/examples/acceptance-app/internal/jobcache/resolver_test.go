package jobcache_test

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"orchestrator/examples/acceptance-app/internal/jobcache"
	"orchestrator/examples/acceptance-app/internal/jobstore"
	"orchestrator/examples/acceptance-app/internal/testfixture"
)

type rig struct {
	store    *jobstore.Store
	redis    *testfixture.Redis
	cache    *jobcache.Cache
	cfg      jobcache.Config
	resolver *jobcache.Resolver
	evidence *jobcache.Evidence
}

func newRig(t *testing.T, ttl time.Duration) *rig {
	t.Helper()
	store, err := jobstore.Open(context.Background(), testfixture.PostgresConfig(t), 1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	r := testfixture.StartRedis(t, testfixture.RedisOptions{})
	cfg := cfgFor(r)
	cfg.TTL = ttl
	cache := jobcache.New(cfg)
	t.Cleanup(func() { _ = cache.Close() })
	ev := jobcache.NewEvidence()
	return &rig{store: store, redis: r, cache: cache, cfg: cfg, evidence: ev, resolver: jobcache.NewResolver(cache, store, ev)}
}

// complete is the worker's contract: commit to PostgreSQL, then write through.
func (g *rig) complete(t *testing.T, payload string) jobstore.Job {
	t.Helper()
	ctx := context.Background()
	if _, err := g.store.Submit(ctx, payload); err != nil {
		t.Fatal(err)
	}
	job, claimed, err := g.store.ClaimAndProcess(ctx, "worker-1", func(p string) string { return "processed:" + strings.ToUpper(p) })
	if err != nil || !claimed {
		t.Fatalf("claim: %v %v", claimed, err)
	}
	if err := g.cache.Put(ctx, job); err != nil {
		t.Fatalf("write-through: %v", err)
	}
	return job
}

func (g *rig) rows(t *testing.T) int64 {
	t.Helper()
	n, err := g.store.CountJobs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestResolverHitThenExpiredMissReadsSameRowAndRefills(t *testing.T) {
	g := newRig(t, time.Second)
	ctx := context.Background()
	job := g.complete(t, "hello")
	key := g.cfg.KeyFor(job.ID)

	l, err := g.resolver.Lookup(ctx, job.ID)
	if err != nil || l.Outcome != jobcache.OutcomeHit || l.Job.Result != "processed:HELLO" || l.Reason != jobcache.ReasonNone {
		t.Fatalf("expected hit: %+v err=%v", l, err)
	}

	// Wait for the short deterministic TTL to remove the entry.
	deadline := time.Now().Add(5 * time.Second)
	for g.redis.Client(t).Exists(ctx, key).Val() != 0 {
		if time.Now().After(deadline) {
			t.Fatal("entry did not expire")
		}
		time.Sleep(100 * time.Millisecond)
	}
	l, err = g.resolver.Lookup(ctx, job.ID)
	if err != nil || l.Outcome != jobcache.OutcomeMissRefilled || l.Job.ID != job.ID || l.Job.Result != job.Result || l.Job.ProcessedBy != "worker-1" {
		t.Fatalf("expected miss refilled from the same PostgreSQL row: %+v err=%v", l, err)
	}
	if g.redis.Client(t).Exists(ctx, key).Val() != 1 {
		t.Fatal("miss must refill Redis")
	}
	l, _ = g.resolver.Lookup(ctx, job.ID)
	if l.Outcome != jobcache.OutcomeHit {
		t.Fatalf("after refill the next read must hit: %+v", l)
	}
	counters, recent := g.evidence.Snapshot()
	if counters[jobcache.OutcomeHit] != 2 || counters[jobcache.OutcomeMissRefilled] != 1 || len(recent) != 3 {
		t.Fatalf("evidence counters %v recent %d", counters, len(recent))
	}
	if g.rows(t) != 1 {
		t.Fatal("reads must never create or duplicate jobs")
	}
}

func TestResolverPendingIsNeverFrozenByTheCache(t *testing.T) {
	g := newRig(t, 30*time.Second)
	ctx := context.Background()
	submitted, err := g.store.Submit(ctx, "later")
	if err != nil {
		t.Fatal(err)
	}
	l, err := g.resolver.Lookup(ctx, submitted.ID)
	if err != nil || l.Outcome != jobcache.OutcomePending || l.Job.Status != jobstore.StatusPending {
		t.Fatalf("pending: %+v err=%v", l, err)
	}
	if g.redis.Client(t).Exists(ctx, g.cfg.KeyFor(submitted.ID)).Val() != 0 {
		t.Fatal("a pending job must not be cached")
	}
	// A stale pending entry planted in Redis must not hide the completion.
	stale := `{"id":` + strconv.FormatInt(submitted.ID, 10) + `,"payload":"later","status":"PENDING","createdAt":"2026-01-01T00:00:00Z"}`
	if err := g.redis.Client(t).Set(ctx, g.cfg.KeyFor(submitted.ID), stale, time.Minute).Err(); err != nil {
		t.Fatal(err)
	}
	l, _ = g.resolver.Lookup(ctx, submitted.ID)
	if l.Outcome != jobcache.OutcomePending {
		t.Fatalf("stale pending entry must be ignored: %+v", l)
	}
	done, claimed, err := g.store.ClaimAndProcess(ctx, "worker-1", strings.ToUpper)
	if err != nil || !claimed {
		t.Fatal(err)
	}
	// Worker not yet written through: the next read must see DONE, not the stale entry.
	l, err = g.resolver.Lookup(ctx, submitted.ID)
	if err != nil || l.Job.Status != jobstore.StatusDone || l.Job.Result != done.Result || l.Outcome != jobcache.OutcomeMissRefilled {
		t.Fatalf("completion must replace the stale entry: %+v err=%v", l, err)
	}
}

func TestResolverCacheOutageServesDurableResultButReportsFailure(t *testing.T) {
	g := newRig(t, 30*time.Second)
	ctx := context.Background()
	job := g.complete(t, "durable")
	g.redis.Stop()

	l, err := g.resolver.Lookup(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if l.Outcome != jobcache.OutcomeErrorDBFallback || l.Reason != jobcache.ReasonUnreachable || l.Job.Result != job.Result {
		t.Fatalf("durable result with a reported cache failure expected: %+v", l)
	}
	if g.rows(t) != 1 {
		t.Fatal("no duplicate job")
	}
}

func TestResolverWrongPasswordIsFailureNotMiss(t *testing.T) {
	g := newRig(t, 30*time.Second)
	job := g.complete(t, "auth")
	bad := g.cfg
	bad.Password = "not-the-password"
	wrong := jobcache.New(bad)
	defer wrong.Close()
	resolver := jobcache.NewResolver(wrong, g.store, jobcache.NewEvidence())
	l, err := resolver.Lookup(context.Background(), job.ID)
	if err != nil || l.Outcome != jobcache.OutcomeErrorDBFallback || l.Reason != jobcache.ReasonAuth {
		t.Fatalf("wrong password must be reported as an auth failure: %+v err=%v", l, err)
	}
}

func TestResolverInvalidBindingReportsConfigFailure(t *testing.T) {
	g := newRig(t, 30*time.Second)
	job := g.complete(t, "binding")
	resolver := jobcache.NewResolver(nil, g.store, jobcache.NewEvidence())
	l, err := resolver.Lookup(context.Background(), job.ID)
	if err != nil || l.Outcome != jobcache.OutcomeErrorDBFallback || l.Reason != jobcache.ReasonConfig || l.Job.Result != job.Result {
		t.Fatalf("%+v err=%v", l, err)
	}
}

func TestResolverCorruptEntryIsOverwrittenFromDatabase(t *testing.T) {
	g := newRig(t, 30*time.Second)
	ctx := context.Background()
	job := g.complete(t, "corrupt")
	if err := g.redis.Client(t).Set(ctx, g.cfg.KeyFor(job.ID), "{not json", time.Minute).Err(); err != nil {
		t.Fatal(err)
	}
	l, err := g.resolver.Lookup(ctx, job.ID)
	if err != nil || l.Outcome != jobcache.OutcomeMissRefilled || l.Job.Result != job.Result {
		t.Fatalf("%+v err=%v", l, err)
	}
	if l, _ = g.resolver.Lookup(ctx, job.ID); l.Outcome != jobcache.OutcomeHit {
		t.Fatalf("overwritten entry must hit: %+v", l)
	}
}

func TestLookupCompletedFailsFastWhenCacheIsDown(t *testing.T) {
	g := newRig(t, 30*time.Second)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		g.complete(t, "bulk")
	}
	g.redis.Stop()
	summaries, err := g.store.ListSummaries(ctx, 20)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	lookups, err := g.resolver.LookupCompleted(ctx, summaries)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("one timeout per job: %v", time.Since(start))
	}
	for _, l := range lookups {
		if l.Outcome != jobcache.OutcomeErrorDBFallback || l.Job.Result == "" {
			t.Fatalf("every completed job still shows its durable result and the failure: %+v", l)
		}
	}
}
