package deployment_test

import (
	"context"
	"errors"
	"testing"

	"orchestrator/internal/adapters/store"
	appsvc "orchestrator/internal/application/deployment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning"
	"orchestrator/internal/ports/persistence"
)

type planProbe struct {
	persistence.Store
	inView             bool
	inside, outside    *int
	err                error
	afterFirstSnapRead func()
}

func (p *planProbe) ReadSnapshot(ctx context.Context, fn func(context.Context, persistence.Store) error) error {
	return p.Store.ReadSnapshot(ctx, func(ctx context.Context, view persistence.Store) error {
		return fn(ctx, &planProbe{Store: view, inView: true, inside: p.inside, outside: p.outside, err: p.err, afterFirstSnapRead: p.afterFirstSnapRead})
	})
}

func (p *planProbe) GetPlan(ctx context.Context, id string) (map[string]any, error) {
	if p.inView {
		*p.inside++
	} else {
		*p.outside++
	}
	if p.err != nil {
		return nil, p.err
	}
	plan, err := p.Store.GetPlan(ctx, id)
	if p.afterFirstSnapRead != nil {
		p.afterFirstSnapRead()
		p.afterFirstSnapRead = nil
	}
	return plan, err
}

func baselineFixture(t *testing.T) (*store.Store, resource.ActiveResource, planning.Node) {
	t.Helper()
	ctx := context.Background()
	st := store.New()
	d, err := resource.NewDescriptor("redis", "in-cluster", "cache")
	if err != nil {
		t.Fatal(err)
	}
	scope := resource.Scope{Type: resource.ScopeShared, ID: "cache"}
	active, err := st.UpsertActiveResource(ctx, resource.ActiveResource{OrganizationKey: "org", Descriptor: d, Scope: scope, DefinitionKey: "def", LastDeploymentID: "dep-1", Status: resource.StatusReady})
	if err != nil {
		t.Fatal(err)
	}
	plan := planning.Plan{WorkloadID: "marker-original", Graph: planning.Graph{Nodes: []planning.Node{}}}
	raw := map[string]any{"workloadId": plan.WorkloadID}
	if err := st.SavePlan(ctx, "dep-1", raw); err != nil {
		t.Fatal(err)
	}
	return st, active, planning.Node{Descriptor: d.String(), Scope: scope}
}

// The baseline plans are read through the snapshot view only, once per
// deployment, and later changes cannot reach the captured records.
func TestBaselinesAreCapturedInsideTheSnapshotOnly(t *testing.T) {
	ctx := context.Background()
	st, _, node := baselineFixture(t)
	var inside, outside int
	probe := &planProbe{Store: st, inside: &inside, outside: &outside}
	probe.afterFirstSnapRead = func() {
		_ = st.SavePlan(ctx, "dep-1", map[string]any{"workloadId": "marker-changed"})
		_, _ = st.UpsertActiveResource(ctx, resource.ActiveResource{OrganizationKey: "org", Descriptor: mustDescriptor(t, node.Descriptor), Scope: node.Scope, DefinitionKey: "def", LastDeploymentID: "dep-2", Status: resource.StatusReady})
	}
	active, baselines, err := loadActiveWithBaselines(ctx, probe, "org")
	if err != nil || len(active) != 1 {
		t.Fatalf("%v %v", active, err)
	}
	read := baselines.Reader()
	if got := read(node); got == nil || got.WorkloadID != "marker-original" {
		t.Fatalf("captured baseline = %+v", got)
	}
	if inside != 1 || outside != 0 {
		t.Fatalf("GetPlan inside/outside snapshot = %d/%d", inside, outside)
	}
	// Using the reader performs no further store access.
	_ = read(node)
	_ = read(node)
	if inside != 1 || outside != 0 {
		t.Fatalf("the projection touched the store: %d/%d", inside, outside)
	}
}

func TestBaselinesMissingAndInvalidAreNoEvidenceAndStoreFailureFails(t *testing.T) {
	ctx := context.Background()
	st, _, node := baselineFixture(t)
	var inside, outside int
	// Not found: no evidence.
	_, baselines, err := loadActiveWithBaselines(ctx, &planProbe{Store: st, inside: &inside, outside: &outside, err: persistence.ErrNotFound}, "org")
	if err != nil || baselines.Reader()(node) != nil {
		t.Fatalf("missing plan: %v", err)
	}
	// Undecodable: no evidence.
	if err := st.SavePlan(ctx, "dep-1", map[string]any{"graph": 7}); err != nil {
		t.Fatal(err)
	}
	_, baselines, err = loadActiveWithBaselines(ctx, &planProbe{Store: st, inside: &inside, outside: &outside}, "org")
	if err != nil || baselines.Reader()(node) != nil {
		t.Fatalf("invalid plan: %v", err)
	}
	// A real store failure fails the capture.
	outage := errors.New("outage")
	if _, _, err := loadActiveWithBaselines(ctx, &planProbe{Store: st, inside: &inside, outside: &outside, err: outage}, "org"); !errors.Is(err, outage) {
		t.Fatalf("store failure = %v", err)
	}
	var none *appsvc.Baselines
	if none.Reader()(node) != nil {
		t.Fatal("no baselines means no evidence")
	}
}

// loadActiveWithBaselines mirrors how Preview captures: Active Resources and their
// baselines through one snapshot view.
func loadActiveWithBaselines(ctx context.Context, st persistence.Store, org string) ([]resource.ActiveResource, *appsvc.Baselines, error) {
	var active []resource.ActiveResource
	var baselines *appsvc.Baselines
	err := st.ReadSnapshot(ctx, func(ctx context.Context, view persistence.Store) error {
		list, err := view.ListActiveResources(ctx, org)
		if err != nil {
			return err
		}
		captured, err := appsvc.CaptureBaselines(ctx, view, org, list)
		if err != nil {
			return err
		}
		active, baselines = list, captured
		return nil
	})
	return active, baselines, err
}

func mustDescriptor(t *testing.T, text string) resource.Descriptor {
	t.Helper()
	d, err := resource.ParseDescriptor(text)
	if err != nil {
		t.Fatal(err)
	}
	return d
}
