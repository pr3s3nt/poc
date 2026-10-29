package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"orchestrator/examples/acceptance-app/internal/jobstore"
	"orchestrator/examples/acceptance-app/internal/volume"
)

type lifecycleView struct {
	Storage            volume.Snapshot
	StorageError       bool
	ScheduleConfigured bool
	ScheduleRun        string
	Runs               []jobstore.ScheduledRun
	ScheduleError      bool
}

func readLifecycle(ctx context.Context, client *http.Client, base string) lifecycleView {
	get := func(path string, target any) bool {
		ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
		if err != nil {
			return false
		}
		resp, err := client.Do(req)
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode == 200 && json.NewDecoder(io.LimitReader(resp.Body, 128<<10)).Decode(target) == nil
	}
	var out lifecycleView
	out.StorageError = !get("/api/storage", &out.Storage)
	var schedule struct {
		Configured bool                    `json:"configured"`
		RunID      string                  `json:"runId"`
		Runs       []jobstore.ScheduledRun `json:"runs"`
	}
	out.ScheduleError = !get("/api/scheduled-runs", &schedule)
	out.ScheduleConfigured, out.ScheduleRun, out.Runs = schedule.Configured, schedule.RunID, schedule.Runs
	return out
}
