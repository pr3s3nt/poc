package readiness

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"orchestrator/internal/ports/terraformexec"
)

// FAKE StatefulSet controller for UC-08 BR-11 (issue #68): pg-0 is stuck on a
// bad image. When the Pod is deleted the controller recreates it from the
// update revision and it becomes Ready.
type fakeController struct {
	mu         sync.Mutex
	current    string
	update     string
	deleted    []string
	denyDelete bool
	// hangAfter > 0: StatefulSet reads after that many block until the
	// deadline cuts them off, as a slow API server does.
	hangAfter int
	reads     int
}

func (f *fakeController) run(ctx context.Context, _ string, args []string, _ []string) ([]byte, error) {
	cmd := strings.Join(args, " ")
	f.mu.Lock()
	if strings.Contains(cmd, "get statefulset pg") {
		f.reads++
		if f.hangAfter > 0 && f.reads > f.hangAfter {
			f.mu.Unlock()
			<-ctx.Done()
			return nil, ctx.Err()
		}
	}
	defer f.mu.Unlock()
	stale := now().Add(-2 * time.Minute).Format(time.RFC3339)
	switch {
	case strings.Contains(cmd, "get statefulset pg"):
		ready := 0
		if len(f.deleted) > 0 {
			ready = 1
		}
		return []byte(`{"metadata":{"name":"pg","uid":"set-uid","generation":2},"spec":{"replicas":1,"selector":{"matchLabels":{"app":"pg"}}},"status":{"observedGeneration":2,"readyReplicas":` + itoa(ready) + `,"updatedReplicas":1,"currentRevision":"` + f.current + `","updateRevision":"` + f.update + `"}}`), nil
	case strings.Contains(cmd, "get pods -n orc-ns -l app=pg -o json"):
		if len(f.deleted) > 0 {
			return []byte(`{"items":[]}`), nil
		}
		return []byte(`{"items":[{"metadata":{"name":"pg-0","uid":"pod-uid","labels":{"controller-revision-hash":"` + f.current + `"},"ownerReferences":[{"kind":"StatefulSet","uid":"set-uid","controller":true}]},"status":{"conditions":[{"type":"Ready","status":"False","lastTransitionTime":"` + stale + `"}],"containerStatuses":[{"name":"postgres","image":"postgres:bad","restartCount":0,"state":{"waiting":{"reason":"ImagePullBackOff","message":"free text"}}}]}}]}`), nil
	case strings.Contains(cmd, "delete pod pg-0 -n orc-ns --wait=false"):
		if f.denyDelete {
			return nil, errors.New("forbidden")
		}
		f.deleted = append(f.deleted, "pg-0")
		return []byte("pod deleted"), nil
	}
	return nil, errors.New("unexpected " + cmd)
}

func itoa(n int) string { return string(rune('0' + n)) }

func now() time.Time { return time.Now() }

func stuckRequest(t *testing.T, observed *[]terraformexec.ReadinessObservation) terraformexec.ReadinessRequest {
	kubeconfig := t.TempDir() + "/kubeconfig"
	if err := os.WriteFile(kubeconfig, []byte("apiVersion: v1\nkind: Config\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	return terraformexec.ReadinessRequest{
		OrganizationKey: "acme", ResourceType: "postgres",
		Resources: []terraformexec.StateResource{{Address: "kubernetes_stateful_set_v1.pg", Type: "kubernetes_stateful_set_v1", ID: "orc-ns/pg"}},
		Env:       []string{"KUBE_CONFIG_PATH=" + kubeconfig}, Deadline: time.Now().Add(300 * time.Millisecond),
		Observe: func(o terraformexec.ReadinessObservation) { mu.Lock(); *observed = append(*observed, o); mu.Unlock() },
	}
}

func TestFixedRevisionReplacesStuckPodAndBecomesReady(t *testing.T) {
	ctrl := &fakeController{current: "r1", update: "r2"}
	var observed []terraformexec.ReadinessObservation
	c := &Checker{Kubectl: "kubectl", Interval: 20 * time.Millisecond, Dir: os.TempDir(), Run: ctrl.run}
	if err := c.Check(context.Background(), stuckRequest(t, &observed)); err != nil {
		t.Fatalf("fixed revision did not become ready: %v", err)
	}
	if len(ctrl.deleted) != 1 {
		t.Fatalf("deleted = %v", ctrl.deleted)
	}
	var replaced *terraformexec.PodReplacement
	for _, o := range observed {
		if o.Replaced != nil {
			replaced = o.Replaced
		}
	}
	if replaced == nil || replaced.Wait.Pod != "pg-0" || replaced.Wait.Code != "ImagePullBackOff" || replaced.Wait.Image != "postgres:bad" || replaced.Wait.Object != "orc-ns/pg" {
		t.Fatalf("replacement not observed: %+v", observed)
	}
	if last := observed[len(observed)-1]; last.Waiting == nil || len(last.Waiting) != 0 {
		t.Fatalf("a ready poll must clear earlier wait reasons: %+v", last)
	}
}

func TestNoNewRevisionReportsReasonWithoutDeleting(t *testing.T) {
	ctrl := &fakeController{current: "r1", update: "r1"}
	var observed []terraformexec.ReadinessObservation
	c := &Checker{Kubectl: "kubectl", Interval: 20 * time.Millisecond, Dir: os.TempDir(), Run: ctrl.run}
	err := c.Check(context.Background(), stuckRequest(t, &observed))
	if !errors.Is(err, terraformexec.ErrNotReady) || !strings.Contains(err.Error(), "pg-0: ImagePullBackOff (image postgres:bad)") || strings.Contains(err.Error(), "free text") {
		t.Fatalf("err = %v", err)
	}
	if len(ctrl.deleted) != 0 {
		t.Fatal("deleted a Pod without a new revision")
	}
	if len(observed) == 0 || len(observed[0].Waiting) != 1 || observed[0].Waiting[0].Code != "ImagePullBackOff" {
		t.Fatalf("waiting reason not observed: %+v", observed)
	}
}

func TestMissingDeletePermissionOnlyReports(t *testing.T) {
	ctrl := &fakeController{current: "r1", update: "r2", denyDelete: true}
	var observed []terraformexec.ReadinessObservation
	c := &Checker{Kubectl: "kubectl", Interval: 20 * time.Millisecond, Dir: os.TempDir(), Run: ctrl.run}
	err := c.Check(context.Background(), stuckRequest(t, &observed))
	if !errors.Is(err, terraformexec.ErrNotReady) || errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	for _, o := range observed {
		if o.Replaced != nil {
			t.Fatal("reported a replacement that was refused")
		}
	}
}

// A query cut off by the deadline keeps the observed not-ready result and the
// last wait reasons instead of turning them into an unknown outcome.
func TestDeadlineCutQueryKeepsTheObservedReason(t *testing.T) {
	ctrl := &fakeController{current: "r1", update: "r1", hangAfter: 2}
	var observed []terraformexec.ReadinessObservation
	c := &Checker{Kubectl: "kubectl", Interval: 20 * time.Millisecond, Dir: os.TempDir(), Run: ctrl.run}
	err := c.Check(context.Background(), stuckRequest(t, &observed))
	if !errors.Is(err, terraformexec.ErrNotReady) || errors.Is(err, ErrUnavailable) || !strings.Contains(err.Error(), "pg-0: ImagePullBackOff") {
		t.Fatalf("err = %v", err)
	}
	if len(observed) == 0 || len(observed[len(observed)-1].Waiting) != 1 {
		t.Fatalf("the cut-off poll cleared the reason: %+v", observed)
	}
}
