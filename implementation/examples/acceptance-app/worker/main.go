// Command worker is the acceptance application background workload. It polls the
// shared PostgreSQL database, processes pending jobs and writes the results back.
package main

import (
	"context"
	"log"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

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

	log.Printf("worker: connecting to database %s:%d/%s as %s", cfg.Host, cfg.Port, cfg.Database, cfg.User)
	store, err := jobstore.Open(ctx, cfg, 60)
	if err != nil {
		log.Fatalf("worker: %v", err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		log.Fatalf("worker: %v", err)
	}
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
			}
		}
	}
}

// transform is the observable unit of work: the result proves the worker, not
// the backend, produced it.
func transform(payload string) string {
	return "processed:" + strings.ToUpper(payload)
}
