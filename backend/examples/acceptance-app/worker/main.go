// Command worker is the acceptance application background workload. It polls the
// shared PostgreSQL database, processes pending jobs and writes the results back.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"orchestrator/examples/acceptance-app/internal/jobcache"
	"orchestrator/examples/acceptance-app/internal/jobstore"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := jobstore.ConfigFromEnv()
	if err != nil {
		log.Fatalf("worker: %v", err)
	}
	interval, err := strconv.Atoi(jobstore.EnvOr("POLL_INTERVAL_MS", "500"))
	if err != nil || interval <= 0 {
		interval = 500
	}
	workerID := jobstore.EnvOr("HOSTNAME", "worker")

	// Redis is required: without a valid, reachable and authenticated cache the
	// worker must not start processing, otherwise results would silently skip
	// the write-through.
	redisCfg, err := jobcache.ConfigFromEnv(os.Getenv)
	if err != nil {
		log.Fatalf("worker: cache binding rejected (reason=%s): %v", jobcache.Classify(err), err)
	}
	cache := jobcache.New(redisCfg)
	defer cache.Close()

	log.Printf("worker: connecting to database %s:%d/%s as %s", cfg.Host, cfg.Port, cfg.Database, cfg.User)
	store, err := jobstore.Open(ctx, cfg, 60)
	if err != nil {
		log.Fatalf("worker: %v", err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		log.Fatalf("worker: %v", err)
	}
	if err := waitForCache(ctx, cache, 60); err != nil {
		log.Fatalf("worker: %v", err)
	}
	log.Printf("worker: cache %s tls=%t ttl=%s scope=%s/%s/%s", redisCfg.Addr(), redisCfg.TLS, redisCfg.TTL, redisCfg.App, redisCfg.Env, redisCfg.Run)
	log.Printf("worker %s: polling every %dms", workerID, interval)

	ticker := time.NewTicker(time.Duration(interval) * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Print("worker: shutting down")
			return
		case <-ticker.C:
			job, claimed, err := store.ClaimAndProcess(ctx, workerID, transform)
			if err != nil {
				log.Printf("worker: %v", err)
				continue
			}
			if claimed {
				log.Printf("worker %s: processed job %d", workerID, job.ID)
				writeThrough(ctx, store, cache, job)
			}
		}
	}
}

// transform is the observable unit of work: the result proves the worker, not
// the backend, produced it.
func transform(payload string) string {
	return "processed:" + strings.ToUpper(payload)
}

// writeThrough runs strictly after the result is committed to PostgreSQL. A
// cache failure leaves the committed job untouched: it is not retried, the job
// is not processed again, and the failure is recorded as a safe marker so the
// backend diagnostics report it.
func writeThrough(ctx context.Context, store *jobstore.Store, cache *jobcache.Cache, job jobstore.Job) {
	err := cache.Put(ctx, job)
	if err != nil {
		log.Printf("worker: cache write for job %d failed after commit (reason=%s)", job.ID, jobcache.Classify(err))
	}
	// Use a fresh context: the marker must be recorded even while shutting down.
	markCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if markErr := store.RecordCacheWrite(markCtx, job.ID, err == nil); markErr != nil {
		log.Printf("worker: could not record the cache write outcome for job %d", job.ID)
	}
}

// waitForCache retries so the worker can start before Redis accepts
// connections, then fails closed.
func waitForCache(ctx context.Context, cache *jobcache.Cache, attempts int) error {
	var reason jobcache.Reason
	for i := 0; i < attempts; i++ {
		err := cache.Ping(ctx)
		if err == nil {
			return nil
		}
		reason = jobcache.Classify(err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return fmt.Errorf("cache is not usable (reason=%s)", reason)
}
