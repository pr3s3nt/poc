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
		checks := acceptanceChecks(os.Getenv("ACCEPTANCE_CONFIG"), os.Getenv("ACCEPTANCE_SECRET"), os.Getenv("ACCEPTANCE_SECRET_SHA256"), store.Ping(pingCtx))
		writeJSON(w, http.StatusOK, map[string]any{"workload": "backend", "checks": checks})
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
		job, found, err := store.Get(r.Context(), id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not read the job"})
			return
		}
		if !found {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "job not found"})
			return
		}
		writeJSON(w, http.StatusOK, job)
	})
	mux.HandleFunc("GET /api/jobs", func(w http.ResponseWriter, r *http.Request) {
		jobs, err := store.List(r.Context(), 20)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not list jobs"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"jobs": jobs})
	})

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

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
