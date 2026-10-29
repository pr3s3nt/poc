package kubernetes

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"orchestrator/internal/ports/execution"
)

func TestJobHistoryFollowsControllerUIDAndRejectsForeignOwner(t *testing.T) {
	reader, source, _ := telemetryReaderFixture(t, false)
	now := reader.Now()
	labels := map[string]string{"app.kubernetes.io/managed-by": "orchestrator", "app.kubernetes.io/name": "backend", "orchestrator.io/application": "app", "orchestrator.io/environment": "staging", "orchestrator.io/deployment-id": "deployment-one"}
	cron := map[string]any{"metadata": map[string]any{"name": "backend", "namespace": "app-staging", "uid": "cron-one", "labels": labels}}
	job := func(name, owner string) any {
		return map[string]any{"metadata": map[string]any{"name": name, "namespace": "app-staging", "uid": name + "-uid", "creationTimestamp": now.Add(-time.Minute), "ownerReferences": []any{map[string]any{"uid": owner, "kind": "CronJob", "controller": true}}}, "status": map[string]any{"conditions": []any{map[string]any{"type": "Complete", "status": "True"}}, "completionTime": now}}
	}
	jobs, _ := json.Marshal(map[string]any{"items": []any{job("own-job", "cron-one"), job("foreign-job", "other-cron")}, "metadata": map[string]string{"continue": "more"}})
	script := func() {
		raw, _ := json.Marshal(cron)
		content := "#!/bin/sh\ncase \"$*\" in\n *'get cronjob'*) cat <<'CRON'\n" + string(raw) + "\nCRON\n;;\n *'/jobs?'*) cat <<'JOBS'\n" + string(jobs) + "\nJOBS\n;;\n *) echo 'synthetic-private-provider-diagnostic' >&2;exit 1;;\nesac\n"
		reader.KubectlPath = filepath.Join(t.TempDir(), "kubectl")
		if err := os.WriteFile(reader.KubectlPath, []byte(content), 0700); err != nil {
			t.Fatal(err)
		}
	}
	script()
	ref := execution.ObjectRef{APIVersion: "batch/v1", Kind: "CronJob", Name: "backend", Owners: []string{"deployment-one"}}
	history := reader.InspectJobs(context.Background(), source, ref)
	if history.Code != "OK" || !history.Truncated || len(history.Jobs) != 1 || history.Jobs[0].Name != "own-job" || history.Jobs[0].State != "Succeeded" {
		t.Fatalf("history: %+v", history)
	}
	labels["orchestrator.io/deployment-id"] = "foreign-deployment"
	script()
	history = reader.InspectJobs(context.Background(), source, ref)
	raw, _ := json.Marshal(history)
	if history.Code != "FORBIDDEN" || len(history.Jobs) != 0 || strings.Contains(string(raw), "synthetic-private") {
		t.Fatalf("foreign owner escaped: %s", raw)
	}
}

func TestJobViewsDistinguishPendingActiveAndTerminal(t *testing.T) {
	job := inspectedJob{}
	if got := jobView(job); got.State != "Pending" {
		t.Fatal(got)
	}
	job.Status.Active = 1
	if got := jobView(job); got.State != "Running" {
		t.Fatal(got)
	}
}
