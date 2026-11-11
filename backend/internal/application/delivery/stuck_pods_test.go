package delivery

import (
	"context"
	"testing"
	"time"

	domain "orchestrator/internal/domain/delivery"
	"orchestrator/internal/domain/workloadwait"
	port "orchestrator/internal/ports/delivery"
)

// stuckProvider stays Progressing until it has replaced the stuck Pod once;
// the StatefulSet controller then recreates it from the fixed revision.
type stuckProvider struct {
	*rolloutProvider
	recoveries int
	replaced   bool
}

func (p *stuckProvider) Observe(ctx context.Context, i port.Intent, r port.Receipt) (domain.Observation, error) {
	observed, err := p.neutralProvider.Observe(ctx, i, r)
	if !p.replaced {
		observed.Health = domain.Progressing
	}
	return observed, err
}

func (p *stuckProvider) RecoverStuckPods(context.Context, port.Intent, port.Receipt) (port.StuckPods, error) {
	p.recoveries++
	wait := workloadwait.PodWait{Object: "app-ns/pg", Pod: "pg-0", Code: "ImagePullBackOff", Image: "postgres:bad", StuckSeconds: 60}
	if p.recoveries < 2 {
		return port.StuckPods{Waiting: []workloadwait.PodWait{wait}}, nil
	}
	p.replaced = true
	return port.StuckPods{Waiting: []workloadwait.PodWait{wait}, Replaced: []workloadwait.PodReplacement{{Wait: wait, At: time.Now().UTC()}}}, nil
}

// UC-06 CD-11 (issue #68): the coordinator asks the provider to replace stuck
// Pods while it waits, and stops once the revision is Ready.
func TestDeployRecoversStuckPodsWhileWaiting(t *testing.T) {
	c, original, ctx, req := rolloutFixture(t, nil)
	p := &stuckProvider{rolloutProvider: original}
	c.Registry = port.NewRegistry()
	if err := c.Registry.Register("fleet", p); err != nil {
		t.Fatal(err)
	}
	c.Deadline = time.Second
	if err := c.Deploy(ctx, req); err != nil {
		t.Fatal("the fixed revision did not become ready after the replacement", err)
	}
	if !p.replaced {
		t.Fatal("stuck Pod recovery was not attempted")
	}
	after := p.recoveries
	if after < 2 {
		t.Fatalf("recoveries = %d", after)
	}
}
