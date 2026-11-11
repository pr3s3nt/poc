package http

import (
	"testing"
	"time"

	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/workloadwait"
)

// Issue #68: only typed, re-validated Pod wait entries leave the operation detail.
func TestPodWaitsProjectOnlySanitizedEntries(t *testing.T) {
	good := workloadwait.PodWait{Object: "ns/pg", Pod: "pg-0", Code: "ImagePullBackOff", Image: "postgres:bad", StuckSeconds: 60}
	op := environment.Operation{Detail: map[string]any{
		"releaseSnapshot": map[string]any{"secret": "value"},
		"readiness:postgres.main#environments.app.staging": workloadwait.Detail{DeploymentID: "d", Waiting: []workloadwait.PodWait{good, {Object: "ns/pg", Pod: "pg-1", Code: "SomethingElse", Image: "x"}}, Replaced: []workloadwait.PodReplacement{{Wait: good, At: time.Now()}}},
		"rollout:app":   map[string]any{"waiting": []any{map[string]any{"object": "ns/api", "pod": "api-0", "code": "CrashLoopBackOff", "image": "https://user:pass@host/x", "restarts": 4, "message": "free text"}}},
		"rollout:empty": map[string]any{"waiting": []any{}},
	}}
	got := podWaitsOf(op)
	if len(got) != 2 || got[0].Scope != "readiness" || got[1].Scope != "rollout" {
		t.Fatalf("got %+v", got)
	}
	if len(got[0].Waiting) != 1 || got[0].Waiting[0].Pod != "pg-0" || len(got[0].Replaced) != 1 {
		t.Fatalf("readiness = %+v", got[0])
	}
	if w := got[1].Waiting[0]; w.Image != "" || w.Restarts != 4 || w.Code != "CrashLoopBackOff" {
		t.Fatalf("rollout = %+v", w)
	}
}
