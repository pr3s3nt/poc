package fleet

import (
	"testing"
	"time"

	domain "orchestrator/internal/domain/delivery"
)

func container(ready bool, runningFor time.Duration, restarts float64, now time.Time) map[string]any {
	c := map[string]any{"ready": ready, "restartCount": restarts, "state": map[string]any{"waiting": map[string]any{"reason": "CrashLoopBackOff"}}}
	if runningFor >= 0 {
		c["state"] = map[string]any{"running": map[string]any{"startedAt": now.Add(-runningFor).Format(time.RFC3339)}}
	}
	return c
}

func pod(containers ...map[string]any) map[string]any {
	statuses := []any{}
	for _, c := range containers {
		statuses = append(statuses, c)
	}
	return map[string]any{"status": map[string]any{"containerStatuses": statuses}}
}

// UC-06 CD-10: a momentary ready replica is not a successful rollout.
func TestRolloutPodsRequireStableRunningContainers(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	cases := []struct {
		name string
		pods []map[string]any
		want domain.Health
	}{
		{"stable", []map[string]any{pod(container(true, 20*time.Second, 0, now))}, domain.Healthy},
		{"just started exits at once", []map[string]any{pod(container(true, time.Second, 0, now))}, domain.Progressing},
		{"restarted once then stable", []map[string]any{pod(container(true, 30*time.Second, 2, now))}, domain.Healthy},
		{"crash loop", []map[string]any{pod(container(false, -1, CrashLoopRestarts, now))}, domain.Degraded},
		{"crash loop briefly running", []map[string]any{pod(container(true, 2*time.Second, 5, now))}, domain.Degraded},
		{"early restarts", []map[string]any{pod(container(false, -1, 1, now))}, domain.Progressing},
		{"one of two pods crash looping", []map[string]any{pod(container(true, time.Minute, 0, now)), pod(container(false, -1, 4, now))}, domain.Degraded},
		{"no statuses yet", []map[string]any{{"status": map[string]any{}}}, domain.Progressing},
	}
	for _, tc := range cases {
		if got := podsHealth(tc.pods, now); got != tc.want {
			t.Errorf("%s: got %s want %s", tc.name, got, tc.want)
		}
	}
}

func TestMatchLabelsSelectorIsDeterministicAndStrict(t *testing.T) {
	object := map[string]any{"spec": map[string]any{"selector": map[string]any{"matchLabels": map[string]any{"b": "2", "a": "1"}}}}
	if got, ok := matchLabels(object); !ok || got != "a=1,b=2" {
		t.Fatal(got, ok)
	}
	object["spec"].(map[string]any)["selector"].(map[string]any)["matchExpressions"] = []any{}
	if _, ok := matchLabels(object); ok {
		t.Fatal("matchExpressions accepted")
	}
}

func TestNotReadyObjectsNameTheBlockingWorkload(t *testing.T) {
	got := notReadyObjects("resource Deployment/app-ns/worker not ready. status: Failed, message: Progress deadline exceeded; resource Deployment/other/x not ready", "app-ns")
	if len(got) != 1 || got[0].Kind != "Deployment" || got[0].Name != "worker" || got[0].Health != domain.Degraded {
		t.Fatalf("objects = %+v", got)
	}
}
