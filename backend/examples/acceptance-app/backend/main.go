// Command backend is the acceptance application API workload. It owns the
// PostgreSQL schema, accepts jobs and reads job results back.
package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
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
		log.Fatalf("backend: %v", err)
	}
	log.Printf("backend: connecting to database %s:%d/%s as %s", cfg.Host, cfg.Port, cfg.Database, cfg.User)

	store, err := jobstore.Open(ctx, cfg, 60)
	if err != nil {
		log.Fatalf("backend: %v", err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		log.Fatalf("backend: %v", err)
	}
	log.Print("backend: schema is ready")

	// Redis is required. An invalid or unreachable binding must not stop the
	// backend: it keeps serving the durable PostgreSQL results while the
	// existing readiness and diagnostics report the cache as failed.
	var cache *jobcache.Cache
	redisCfg, err := jobcache.ConfigFromEnv(os.Getenv)
	if err != nil {
		log.Printf("backend: cache binding rejected (reason=%s)", jobcache.Classify(err))
	} else {
		cache = jobcache.New(redisCfg)
		defer cache.Close()
		log.Printf("backend: cache %s tls=%t ttl=%s scope=%s/%s/%s", redisCfg.Addr(), redisCfg.TLS, redisCfg.TTL, redisCfg.App, redisCfg.Env, redisCfg.Run)
	}
	evidence := jobcache.NewEvidence()
	resolver := jobcache.NewResolver(cache, store, evidence)

	mux := newMux(store, cfg, cache, redisCfg, resolver, evidence)

	addr := ":" + jobstore.EnvOr("PORT", "8080")
	server := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	log.Printf("backend: listening on %s", addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("backend: %v", err)
		os.Exit(1)
	}
}

// newMux wires the backend API. cache is nil when the Redis binding is invalid.
func newMux(store *jobstore.Store, cfg jobstore.Config, cache *jobcache.Cache, redisCfg jobcache.Config, resolver *jobcache.Resolver, evidence *jobcache.Evidence) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "workload": "backend"})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		pingCtx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if err := store.Ping(pingCtx); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "database unavailable"})
			return
		}
		if reason := cacheReason(pingCtx, cache); reason != jobcache.ReasonNone {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "cache unavailable", "reason": reason})
			return
		}
		if w1 := workerWrites(pingCtx, store); !w1.proven() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "cache write-through not verified", "workerWrites": w1})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "ready"})
	})
	mux.HandleFunc("GET /api/info", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"workload": "backend",
			"database": cfg.Database,
			"host":     cfg.Host,
			"port":     cfg.Port,
		})
	})
	mux.HandleFunc("GET /api/checks", func(w http.ResponseWriter, r *http.Request) {
		pingCtx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		dbErr := store.Ping(pingCtx)
		reason := cacheReason(pingCtx, cache)
		writes := workerWrites(pingCtx, store)
		checks := acceptanceChecks(os.Getenv("ACCEPTANCE_CONFIG"), os.Getenv("ACCEPTANCE_SECRET"), os.Getenv("ACCEPTANCE_SECRET_SHA256"), dbErr)
		checks["cache"] = reason == jobcache.ReasonNone && writes.proven()
		writeJSON(w, http.StatusOK, map[string]any{
			"workload": "backend",
			"checks":   checks,
			"cache":    cacheEvidence(redisCfg, cache != nil, reason, writes, evidence),
		})
	})
	mux.HandleFunc("POST /api/jobs", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Payload string `json:"payload"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Payload) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "payload is required"})
			return
		}
		job, err := store.Submit(r.Context(), body.Payload)
		if err != nil {
			log.Printf("backend: submit failed: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not submit the job"})
			return
		}
		writeJSON(w, http.StatusCreated, job)
	})
	mux.HandleFunc("GET /api/jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid job id"})
			return
		}
		lookup, err := resolver.Lookup(r.Context(), id)
		if err != nil {
			log.Printf("backend: read job failed: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not read the job"})
			return
		}
		if !lookup.Found {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "job not found"})
			return
		}
		writeJSON(w, http.StatusOK, toResponse(lookup))
	})
	mux.HandleFunc("GET /api/jobs", func(w http.ResponseWriter, r *http.Request) {
		summaries, err := store.ListSummaries(r.Context(), 20)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not list jobs"})
			return
		}
		lookups, err := resolver.LookupCompleted(r.Context(), summaries)
		if err != nil {
			log.Printf("backend: list jobs failed: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not list jobs"})
			return
		}
		jobs := make([]jobResponse, 0, len(lookups))
		for i, l := range lookups {
			if !l.Found { // deleted between the two reads
				continue
			}
			l.Job.Payload = summaries[i].Payload
			jobs = append(jobs, toResponse(l))
		}
		writeJSON(w, http.StatusOK, map[string]any{"jobs": jobs})
	})
	return mux
}

// acceptanceChecks deliberately reports only booleans: neither secret contents
// nor connection credentials may leave the workload through diagnostics.
func acceptanceChecks(config, secret, expectedSecretHash string, databaseErr error) map[string]bool {
	hash := sha256.Sum256([]byte(secret))
	actualHash := hex.EncodeToString(hash[:])
	return map[string]bool{
		"environment": config == "acceptance-config-ok",
		"secret":      secret != "" && len(expectedSecretHash) == len(actualHash) && subtle.ConstantTimeCompare([]byte(actualHash), []byte(expectedSecretHash)) == 1,
		"database":    databaseErr == nil,
	}
}

// jobResponse is a job plus how the cache served it. The cache fields carry
// classifications only, never cache contents or raw errors.
type jobResponse struct {
	jobstore.Job
	Cache       jobcache.Outcome `json:"cache,omitempty"`
	CacheReason jobcache.Reason  `json:"cacheReason,omitempty"`
}

func toResponse(l jobcache.Lookup) jobResponse {
	return jobResponse{Job: l.Job, Cache: l.Outcome, CacheReason: l.Reason}
}

// writeOutcomes is the durable, PostgreSQL-recorded result of the worker's
// write-through per completed job. Only counts are exposed.
type writeOutcomes struct {
	OK         int64 `json:"ok"`
	Failed     int64 `json:"failed"`
	Unverified int64 `json:"unverified"`
	// Unknown is set when the outcomes could not be read at all.
	Unknown bool `json:"unknown,omitempty"`
}

// proven reports whether every completed job has a recorded successful write.
// A recorded failure (sticky within the run), a missing marker or an unreadable
// state all count as not proven.
func (w writeOutcomes) proven() bool { return !w.Unknown && w.Failed == 0 && w.Unverified == 0 }

func workerWrites(ctx context.Context, store *jobstore.Store) writeOutcomes {
	ok, failed, unverified, err := store.CacheWriteCounts(ctx)
	if err != nil {
		return writeOutcomes{Unknown: true}
	}
	return writeOutcomes{OK: ok, Failed: failed, Unverified: unverified}
}

// cacheReason pings Redis with the bound credentials and returns the safe
// failure classification, or ReasonNone when the cache is usable.
func cacheReason(ctx context.Context, cache *jobcache.Cache) jobcache.Reason {
	if cache == nil {
		return jobcache.ReasonConfig
	}
	if err := cache.Ping(ctx); err != nil {
		return jobcache.Classify(err)
	}
	return jobcache.ReasonNone
}

// cacheEvidence is the safe hit/miss/refill evidence for the acceptance runner:
// counts, job ids, outcome and reason classifications, and the key scope.
// It never includes the password, cache contents or raw errors.
func cacheEvidence(cfg jobcache.Config, configured bool, reason jobcache.Reason, writes writeOutcomes, evidence *jobcache.Evidence) map[string]any {
	counters, recent := evidence.Snapshot()
	out := map[string]any{
		"configured":   configured,
		"reachable":    reason == jobcache.ReasonNone,
		"reason":       reason,
		"lookups":      counters,
		"recent":       recent,
		"workerWrites": writes,
	}
	if configured {
		out["tls"] = cfg.TLS
		out["ttlSeconds"] = int(cfg.TTL / time.Second)
		out["scope"] = cfg.App + "/" + cfg.Env + "/" + cfg.Run
	}
	return out
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
