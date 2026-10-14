package main

import (
	"bytes"
	"context"
	"io"
	"log"
	"strings"
	"testing"
	"time"

	"orchestrator/examples/acceptance-app/internal/jobcache"
	"orchestrator/examples/acceptance-app/internal/jobstore"
	"orchestrator/examples/acceptance-app/internal/testfixture"
)

func TestTransform(t *testing.T) {
	if got := transform("abc"); got != "processed:ABC" {
		t.Fatal(got)
	}
}

type workerRig struct {
	store *jobstore.Store
	redis *testfixture.Redis
	cfg   jobcache.Config
	cache *jobcache.Cache
	logs  *bytes.Buffer
}

func newWorkerRig(t *testing.T) *workerRig {
	t.Helper()
	ctx := context.Background()
	store, err := jobstore.Open(ctx, testfixture.PostgresConfig(t), 1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	r := testfixture.StartRedis(t, testfixture.RedisOptions{})
	cfg := jobcache.Config{Host: r.Host, Port: r.Port, Username: r.Username, Password: r.Password,
		App: "acceptance", Env: "dev", Run: "run-01", TTL: 30 * time.Second}
	cache := jobcache.New(cfg)
	t.Cleanup(func() { _ = cache.Close() })
	logs := &bytes.Buffer{}
	log.SetOutput(logs)
	t.Cleanup(func() { log.SetOutput(io.Discard) })
	return &workerRig{store: store, redis: r, cfg: cfg, cache: cache, logs: logs}
}

func (w *workerRig) process(t *testing.T) jobstore.Job {
	t.Helper()
	job, claimed, err := w.store.ClaimAndProcess(context.Background(), "worker-1", transform)
	if err != nil || !claimed {
		t.Fatalf("claim: %v %v", claimed, err)
	}
	writeThrough(context.Background(), w.store, w.cache, job)
	return job
}

func TestWorkerCommitsToPostgreSQLThenWritesThroughRedis(t *testing.T) {
	w := newWorkerRig(t)
	ctx := context.Background()
	submitted, err := w.store.Submit(ctx, "write-through")
	if err != nil {
		t.Fatal(err)
	}
	job := w.process(t)

	durable, found, err := w.store.Get(ctx, submitted.ID)
	if err != nil || !found || durable.Status != jobstore.StatusDone || durable.Result != "processed:WRITE-THROUGH" {
		t.Fatalf("PostgreSQL row: %+v %v %v", durable, found, err)
	}
	cached, found, err := w.cache.Get(ctx, job.ID)
	if err != nil || !found || cached.Result != durable.Result || cached.ProcessedBy != "worker-1" {
		t.Fatalf("Redis entry: %+v %v %v", cached, found, err)
	}
	key := w.cfg.KeyFor(job.ID)
	if key != "acceptance:v1:acceptance:dev:run-01:job:"+strings.TrimPrefix(key, "acceptance:v1:acceptance:dev:run-01:job:") {
		t.Fatal(key)
	}
	if ttl := w.redis.Client(t).TTL(ctx, key).Val(); ttl <= 0 || ttl > 30*time.Second {
		t.Fatalf("ttl %v", ttl)
	}
	ok, failed, _, _ := w.store.CacheWriteCounts(ctx)
	if ok != 1 || failed != 0 {
		t.Fatalf("write markers ok=%d failed=%d", ok, failed)
	}
}

func TestWorkerCacheFailureAfterCommitKeepsOneCompletedJob(t *testing.T) {
	w := newWorkerRig(t)
	ctx := context.Background()
	if _, err := w.store.Submit(ctx, "secret-payload"); err != nil {
		t.Fatal(err)
	}
	w.redis.Stop()
	job := w.process(t)

	durable, found, err := w.store.Get(ctx, job.ID)
	if err != nil || !found || durable.Status != jobstore.StatusDone || durable.Result != "processed:SECRET-PAYLOAD" {
		t.Fatalf("the committed result must be kept: %+v %v %v", durable, found, err)
	}
	if _, claimed, _ := w.store.ClaimAndProcess(ctx, "worker-2", transform); claimed {
		t.Fatal("the job must not be reprocessed")
	}
	if n, _ := w.store.CountJobs(ctx); n != 1 {
		t.Fatalf("rows = %d, want exactly one", n)
	}
	ok, failed, _, _ := w.store.CacheWriteCounts(ctx)
	if ok != 0 || failed != 1 {
		t.Fatalf("a failed write-through must be recorded, not reported as success: ok=%d failed=%d", ok, failed)
	}
	logs := w.logs.String()
	if !strings.Contains(logs, "reason=unreachable") {
		t.Fatalf("expected a safe reason in the log: %q", logs)
	}
	for _, leak := range []string{w.cfg.Password, "SECRET-PAYLOAD", "dial tcp", "127.0.0.1:"} {
		if strings.Contains(logs, leak) {
			t.Fatalf("log leaks %q: %s", leak, logs)
		}
	}
}

func TestWaitForCacheFailsClosed(t *testing.T) {
	w := newWorkerRig(t)
	bad := w.cfg
	bad.Password = "nope"
	c := jobcache.New(bad)
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := waitForCache(ctx, c, 1)
	if err == nil || !strings.Contains(err.Error(), "reason=auth") || strings.Contains(err.Error(), "nope") {
		t.Fatalf("err = %v", err)
	}
	if err := waitForCache(ctx, w.cache, 1); err != nil {
		t.Fatalf("good cache: %v", err)
	}
}
