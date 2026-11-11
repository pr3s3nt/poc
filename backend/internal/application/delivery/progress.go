package delivery

import (
	"context"
	domain "orchestrator/internal/domain/delivery"
	"orchestrator/internal/domain/progress"
	"sort"
	"time"
)

func reportReadiness(ctx context.Context, id, workload, state, reason string, deadline time.Time, observed domain.Observation, commit string) {
	progress.Report(ctx, progress.Step{ID: id + ":readiness", Workload: workload, Kind: "READINESS", Status: state, Reason: reason, Deadline: &deadline, Pods: observed.Pods, Events: observed.Events, Commit: commit})
	for _, object := range observed.Objects {
		if object.Kind != "VaultStaticSecret" && object.Kind != "Job" {
			continue
		}
		status := "RUNNING"
		if object.Health == domain.Healthy {
			status = "SUCCEEDED"
		}
		if object.Health == domain.Degraded {
			status = "FAILED"
		}
		kind := "VSO"
		if object.Kind == "Job" {
			kind = "JOB"
		}
		progress.Report(ctx, progress.Step{ID: id + ":object:" + object.Kind + ":" + object.Name, Workload: workload, Kind: kind, Status: status, Reason: object.Reason, ResourceKind: object.Kind, ResourceName: object.Name, Namespace: object.Namespace, Deadline: &deadline})
	}
	groups := map[string][]progress.Pod{}
	for _, pod := range observed.Pods {
		if pod.Workload != "" {
			groups[pod.Workload] = append(groups[pod.Workload], pod)
		}
	}
	names := []string{}
	for name := range groups {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		pods := groups[name]
		events := []progress.Event{}
		for _, e := range observed.Events {
			if e.Workload == name {
				events = append(events, e)
			}
		}
		// The enclosing release owns the final outcome. Per-workload diagnostics do
		// not infer success from a Running pod before the stability gate succeeds.
		progress.Report(ctx, progress.Step{ID: id + ":workload:" + name, Workload: name, Kind: "READINESS", Status: state, Reason: reason, Deadline: &deadline, Pods: pods, Events: events, Commit: commit})
	}
}
