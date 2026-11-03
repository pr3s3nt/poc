package delivery

import (
	"context"
	"strings"
	"testing"
	"time"

	domain "orchestrator/internal/domain/delivery"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/execution"
)

type pendingPruneProvider struct {
	*neutralProvider
	objects []port.AppliedObject
}

func (p *pendingPruneProvider) PrunePendingReleasedObjects(_ context.Context, _ port.Intent, _ port.Receipt, objects []port.AppliedObject) (bool, error) {
	p.objects = objects
	return true, nil
}

func TestPendingPruneDispatchRequiresFreshExactHealthyDesiredInventory(t *testing.T) {
	current := execution.ObjectRef{APIVersion: "v1", Kind: "ConfigMap", Name: "current"}
	old := execution.ObjectRef{APIVersion: "v1", Kind: "ConfigMap", Name: "old"}
	revision := domain.Revision{ID: "new", Scope: domain.Scope{OrganizationID: "org", EnvironmentID: "env", UnitKey: "app/prerequisite"}, Phase: domain.Prerequisite, PayloadDigest: strings.Repeat("a", 64), ArtifactRef: "artifact", Selection: domain.Selection{Mode: domain.CD, BindingID: "binding", BindingVersion: 1}}
	intent := port.Intent{Revision: revision, Target: execution.Target{Namespace: "app"}, SourceInventory: []port.AppliedObject{{ObjectRef: current, UID: "existing-current", Release: "release"}, {ObjectRef: old, UID: "old-uid", Release: "release"}}}
	base := domain.Observation{Scope: revision.Scope, IntendedRevision: revision.ID, ObservedRevision: revision.ID, PayloadDigest: revision.PayloadDigest, Sync: domain.OutOfSync, Drift: domain.Detected, Health: domain.Healthy, Complete: true, ObservedAt: time.Now().UTC(), Objects: []domain.ObjectObservation{{APIVersion: "v1", Kind: "ConfigMap", Name: "current", Namespace: "app", UID: "existing-current"}}}
	for _, tc := range []struct {
		name   string
		change func(*domain.Observation)
		want   int
	}{
		{"exact healthy desired inventory", func(*domain.Observation) {}, 1},
		{"rollout incomplete", func(o *domain.Observation) { o.Health = domain.Progressing }, 0},
		{"wrong revision", func(o *domain.Observation) { o.ObservedRevision = "old" }, 0},
		{"wrong payload", func(o *domain.Observation) { o.PayloadDigest = strings.Repeat("b", 64) }, 0},
		{"stale observation", func(o *domain.Observation) { o.ObservedAt = time.Now().Add(-time.Hour) }, 0},
		{"missing inventory", func(o *domain.Observation) { o.Objects = nil }, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := &pendingPruneProvider{neutralProvider: &neutralProvider{}}
			observed := base
			tc.change(&observed)
			if err := (&Coordinator{}).prunePending(context.Background(), provider, intent, port.Receipt{}, observed); err != nil {
				t.Fatal(err)
			}
			if len(provider.objects) != tc.want {
				t.Fatalf("prune dispatched %d objects", len(provider.objects))
			}
			if tc.want > 0 && (provider.objects[0].Key() != old.Key() || provider.objects[0].UID != "old-uid") {
				t.Fatal("prune did not preserve omitted old UID")
			}
		})
	}
}
