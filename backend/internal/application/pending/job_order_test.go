package pending

import (
	"orchestrator/internal/domain/environment"
	"testing"
)

func TestJobCompletionOrdersBatchBeforeConsumers(t *testing.T) {
	draft := func(name, kind string, wait []string) environment.WorkloadDraft {
		return environment.WorkloadDraft{WorkloadID: name, State: environment.DraftUpsert, Workload: map[string]any{"apiVersion": "orchestrator.dev/workload/v1", "metadata": map[string]any{"name": name}, "containers": map[string]any{"main": map[string]any{"image": "task:1"}}, "workloadExecution": map[string]any{"version": 1, "kind": kind, "waitForJobs": wait}}}
	}
	drafts := map[string]environment.WorkloadDraft{"a-web": draft("a-web", "Deployment", []string{"z-migrate"}), "z-migrate": draft("z-migrate", "Job", nil)}
	delete(drafts["z-migrate"].Workload["workloadExecution"].(map[string]any), "waitForJobs")
	order, err := OrderDrafts(drafts)
	if err != nil {
		t.Fatal(err)
	}
	if len(order) != 2 || order[0] != "z-migrate" {
		t.Fatal(order)
	}
	drafts["z-migrate"] = draft("z-migrate", "Job", []string{"a-web"})
	if _, err := OrderDrafts(drafts); err == nil {
		t.Fatal("cycle accepted")
	}
}
