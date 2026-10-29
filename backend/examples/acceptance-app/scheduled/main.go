// Command scheduled records one real scheduled invocation and exits.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "golang.org/x/crypto/x509roots/fallback" // public CA roots for the scratch image

	"orchestrator/examples/acceptance-app/internal/jobstore"
	"orchestrator/examples/acceptance-app/internal/volume"
)

func readHash(ctx context.Context, client *http.Client, base string) (string, error) {
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("invalid backend URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+"/api/storage", nil)
	if err != nil {
		return "", errors.New("invalid backend URL")
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", errors.New("backend storage unavailable")
	}
	defer resp.Body.Close()
	var state volume.Snapshot
	if resp.StatusCode != 200 || json.NewDecoder(io.LimitReader(resp.Body, 32<<10)).Decode(&state) != nil || !state.Configured || !state.Present {
		return "", errors.New("backend storage value not ready")
	}
	hash := sha256.Sum256([]byte(state.Value))
	if strings.TrimSpace(state.Value) == "" || len(state.Value) > volume.MaxValue || state.SHA256 != hex.EncodeToString(hash[:]) {
		return "", errors.New("backend storage hash mismatch")
	}
	return state.SHA256, nil
}
func run(ctx context.Context) error {
	runID, invocation := os.Getenv("ACCEPTANCE_SCHEDULE_RUN"), os.Getenv("HOSTNAME")
	if !jobstore.ValidScheduleID(runID) || !jobstore.ValidScheduleID(invocation) {
		return errors.New("scheduled run ID and HOSTNAME required")
	}
	hash, err := readHash(ctx, &http.Client{Timeout: 10 * time.Second}, os.Getenv("BACKEND_URL"))
	if err != nil {
		return err
	}
	cfg, err := jobstore.ConfigFromEnv()
	if err != nil {
		return errors.New("invalid database binding")
	}
	store, err := jobstore.Open(ctx, cfg, 1)
	if err != nil {
		return errors.New("database unavailable")
	}
	defer store.Close()
	if err := store.RecordScheduledRun(ctx, runID, invocation, hash); err != nil {
		return err
	}
	log.Print("scheduled: receipt committed")
	return nil
}
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := run(ctx); err != nil {
		log.Printf("scheduled: %s", err)
		os.Exit(1)
	}
}
