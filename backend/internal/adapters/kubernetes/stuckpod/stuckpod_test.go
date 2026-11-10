package stuckpod

import (
	"testing"
	"time"
)

var now = time.Date(2026, 11, 11, 10, 0, 0, 0, time.UTC)

func set(current, update string) map[string]any {
	return map[string]any{
		"metadata": map[string]any{"name": "pg", "uid": "set-uid"},
		"status":   map[string]any{"currentRevision": current, "updateRevision": update},
	}
}

type podOpt func(map[string]any)

func pod(name, revision string, opts ...podOpt) map[string]any {
	p := map[string]any{
		"metadata": map[string]any{
			"name": name, "uid": name + "-uid",
			"labels":          map[string]any{RevisionLabel: revision},
			"ownerReferences": []any{map[string]any{"kind": "StatefulSet", "uid": "set-uid", "controller": true}},
		},
		"status": map[string]any{
			"conditions": []any{map[string]any{"type": "Ready", "status": "False", "lastTransitionTime": now.Add(-2 * time.Minute).Format(time.RFC3339)}},
			"containerStatuses": []any{map[string]any{
				"name": "postgres", "image": "postgres:bad", "restartCount": float64(0),
				"state": map[string]any{"waiting": map[string]any{"reason": "ImagePullBackOff", "message": "secret-looking message"}},
			}},
		},
	}
	for _, o := range opts {
		o(p)
	}
	return p
}

func notReadyFor(d time.Duration) podOpt {
	return func(p map[string]any) {
		p["status"].(map[string]any)["conditions"] = []any{map[string]any{"type": "Ready", "status": "False", "lastTransitionTime": now.Add(-d).Format(time.RFC3339)}}
	}
}

func ready(p map[string]any) {
	p["status"].(map[string]any)["conditions"] = []any{map[string]any{"type": "Ready", "status": "True"}}
}

func terminating(p map[string]any) {
	p["metadata"].(map[string]any)["deletionTimestamp"] = now.Format(time.RFC3339)
}

func waiting(reason string, restarts float64) podOpt {
	return func(p map[string]any) {
		c := p["status"].(map[string]any)["containerStatuses"].([]any)[0].(map[string]any)
		c["restartCount"] = restarts
		if reason == "" {
			c["state"] = map[string]any{"running": map[string]any{"startedAt": now.Format(time.RFC3339)}}
			return
		}
		c["state"] = map[string]any{"waiting": map[string]any{"reason": reason}}
	}
}

func foreignOwner(p map[string]any) {
	p["metadata"].(map[string]any)["ownerReferences"] = []any{map[string]any{"kind": "StatefulSet", "uid": "other", "controller": true}}
}

func TestEvaluate(t *testing.T) {
	cases := []struct {
		name    string
		set     map[string]any
		pods    []map[string]any
		replace string
		reasons int
	}{
		{"fixed revision replaces the stuck old Pod", set("r1", "r2"), []map[string]any{pod("pg-0", "r1")}, "pg-0", 1},
		{"no new revision only reports", set("r1", "r1"), []map[string]any{pod("pg-0", "r1")}, "", 1},
		{"below threshold waits", set("r1", "r2"), []map[string]any{pod("pg-0", "r1", notReadyFor(20*time.Second))}, "", 1},
		{"ready Pod is kept", set("r1", "r2"), []map[string]any{pod("pg-0", "r1", ready)}, "", 0},
		{"terminating Pod is kept", set("r1", "r2"), []map[string]any{pod("pg-0", "r1", terminating)}, "", 0},
		{"normal start-up is kept", set("r1", "r2"), []map[string]any{pod("pg-0", "r1", waiting("ContainerCreating", 0))}, "", 0},
		{"running but not ready is kept", set("r1", "r2"), []map[string]any{pod("pg-0", "r1", waiting("", 0))}, "", 0},
		{"Pod already on the update revision is kept", set("r1", "r2"), []map[string]any{pod("pg-0", "r2")}, "", 1},
		{"Pod of another controller is ignored", set("r1", "r2"), []map[string]any{pod("pg-0", "r1", foreignOwner)}, "", 0},
		{"crash loop is replaced", set("r1", "r2"), []map[string]any{pod("pg-0", "r1", waiting("CrashLoopBackOff", 5))}, "pg-0", 1},
		{"one Pod at a time, highest ordinal first", set("r1", "r2"), []map[string]any{pod("pg-0", "r1"), pod("pg-1", "r1"), pod("pg-2", "r1")}, "pg-2", 3},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := Evaluate(c.set, c.pods, now, DefaultThreshold)
			got := ""
			if d.Replace != nil {
				got = d.Replace.Pod
				if d.Replace.UID != got+"-uid" {
					t.Fatalf("uid = %q", d.Replace.UID)
				}
			}
			if got != c.replace || len(d.Reasons) != c.reasons {
				t.Fatalf("replace = %q (want %q), reasons = %+v", got, c.replace, d.Reasons)
			}
		})
	}
}

func TestReasonsAreSafeSummaries(t *testing.T) {
	d := Evaluate(set("r1", "r1"), []map[string]any{pod("pg-0", "r1"), pod("pg-1", "r1", waiting("CrashLoopBackOff", 7))}, now, DefaultThreshold)
	if got := d.Summary(); got != "pg-0: ImagePullBackOff (image postgres:bad); pg-1: CrashLoopBackOff (restarts 7)" {
		t.Fatal(got)
	}
	if d.Reasons[0].StuckSeconds != 120 {
		t.Fatal(d.Reasons[0].StuckSeconds)
	}
	if safeImage("https://user:pass@x/y") != "" || safeImage("a b") != "" || safeImage("registry.local:5000/pg:16") == "" {
		t.Fatal("unsafe image reference kept")
	}
}
