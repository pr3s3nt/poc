package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"orchestrator/examples/acceptance-app/internal/jobstore"
	"orchestrator/examples/acceptance-app/internal/volume"
)

func registerStorage(mux *http.ServeMux, files *volume.Store, store *jobstore.Store, run string) {
	mux.HandleFunc("GET /api/storage", func(w http.ResponseWriter, r *http.Request) {
		state, err := files.Read()
		if err != nil {
			writeJSON(w, 503, map[string]string{"error": "storage unavailable"})
			return
		}
		writeJSON(w, 200, state)
	})
	mux.HandleFunc("POST /api/storage", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
		var body struct {
			Value string `json:"value"`
		}
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if decoder.Decode(&body) != nil || decoder.Decode(new(any)) != io.EOF {
			writeJSON(w, 400, map[string]string{"error": "invalid test value"})
			return
		}
		state, err := files.Write(body.Value)
		if err != nil {
			code := 503
			if errors.Is(err, volume.ErrValue) {
				code = 400
			}
			writeJSON(w, code, map[string]string{"error": "could not store test value"})
			return
		}
		writeJSON(w, 200, state)
	})
	mux.HandleFunc("GET /api/scheduled-runs", func(w http.ResponseWriter, r *http.Request) {
		if run == "" {
			writeJSON(w, 200, map[string]any{"configured": false, "runs": []jobstore.ScheduledRun{}})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		runs, err := store.ScheduledRuns(ctx, run)
		if err != nil {
			writeJSON(w, 503, map[string]string{"error": "scheduled receipts unavailable"})
			return
		}
		writeJSON(w, 200, map[string]any{"configured": true, "runId": run, "runs": runs})
	})
}
