package deployment_test

import (
	"context"
	"errors"
	"testing"

	"encoding/json"
	"orchestrator/internal/adapters/store"
	appsvc "orchestrator/internal/application/deployment"
	domain "orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/environment"
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

type retainedHistoryView struct {
	persistence.Store
	records map[string]domain.Deployment
	sets    map[string]environment.DeploymentSet
}

func (v *retainedHistoryView) GetDeployment(_ context.Context, id string) (domain.Deployment, error) {
	d, ok := v.records[id]
	if !ok {
		return d, persistence.ErrNotFound
	}
	return d, nil
}
func (v *retainedHistoryView) GetDeploymentSet(_ context.Context, id string) (environment.DeploymentSet, error) {
	s, ok := v.sets[id]
	if !ok {
		return s, persistence.ErrNotFound
	}
	return s, nil
}
func TestRetainedDataBaselineFollowsOnlySuccessfulRemovalBaseSetLineage(t *testing.T) {
	ctx := context.Background()
	st, a, node := baselineFixture(t)
	a.Status = resource.StatusUnreferenced
	a.LastDeploymentID = "remove"
	node.Kind = planning.NodeResource
	node.ResourceType = "redis"
	original := planning.Plan{Graph: planning.Graph{Nodes: []planning.Node{node}}, Matches: map[string]planning.Match{node.Descriptor: {DefinitionKey: "def", DriverType: resource.DriverTerraform}}, Terraform: []planning.TerraformContract{{Descriptor: node.Descriptor, Fingerprint: "exact-module"}}, IntentEvidence: map[string]string{node.Descriptor: "authored-pin"}}
	raw, _ := json.Marshal(original)
	var document map[string]any
	json.Unmarshal(raw, &document)
	st.SavePlan(ctx, "executed", document)
	st.SavePlan(ctx, "remove", map[string]any{"workloadId": "removed"})
	view := &retainedHistoryView{Store: st, records: map[string]domain.Deployment{"remove": {OrganizationKey: "org", EnvironmentID: "env", Action: domain.ActionRemove, Status: domain.StatusSucceeded, BaseDeploymentSetID: "base"}, "executed": {OrganizationKey: "org", EnvironmentID: "env", Action: domain.ActionDeploy, Status: domain.StatusSucceeded}}, sets: map[string]environment.DeploymentSet{"base": {EnvironmentID: "env", CreatedByDeploymentID: "executed"}}}
	captured, err := appsvc.CaptureBaselines(ctx, view, "org", []resource.ActiveResource{a})
	if err != nil {
		t.Fatal(err)
	}
	intents := original.ResourceIntents(captured.Reader())
	if len(intents) != 1 || intents[0].Intent != planning.IntentRetain {
		t.Fatal("retained data lost its exact execution evidence", intents)
	}
	original.Terraform[0].Fingerprint = "changed-module"
	if original.ResourceIntents(captured.Reader())[0].Intent != planning.IntentUpdate {
		t.Fatal("real module change hidden")
	}
	// A non-removal hop, another Environment, or missing evidence cannot inherit it.
	for _, mode := range []string{"not-removal", "other-environment", "cycle"} {
		d := view.records["remove"]
		d.Action = domain.ActionRemove
		view.records["remove"] = d
		view.sets["base"] = environment.DeploymentSet{EnvironmentID: "env", CreatedByDeploymentID: "executed"}
		if mode == "not-removal" {
			d.Action = domain.ActionDeploy
			view.records["remove"] = d
		}
		if mode == "other-environment" {
			view.sets["base"] = environment.DeploymentSet{EnvironmentID: "foreign", CreatedByDeploymentID: "executed"}
		}
		if mode == "cycle" {
			view.sets["base"] = environment.DeploymentSet{EnvironmentID: "env", CreatedByDeploymentID: "remove"}
		}
		b, err := appsvc.CaptureBaselines(ctx, view, "org", []resource.ActiveResource{a})
		if err != nil || b.Reader()(node) != nil {
			t.Fatalf("unsafe history %s: %v", mode, err)
		}
	}
}
