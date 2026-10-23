package deploymentreview

import (
	"context"
	"testing"
	"time"

	domain "orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/ports/persistence"
)

type reviewHistory struct {
	persistence.Store
	deployments []domain.Deployment
	plans       map[string]map[string]any
	resources   map[string][]domain.Resource
}

func (h reviewHistory) ListDeployments(context.Context, string, string) ([]domain.Deployment, error) {
	return h.deployments, nil
}
func (h reviewHistory) GetPlan(_ context.Context, id string) (map[string]any, error) {
	if plan, ok := h.plans[id]; ok {
		return plan, nil
	}
	return nil, persistence.ErrNotFound
}
func (h reviewHistory) ListDeploymentResources(_ context.Context, id string) ([]domain.Resource, error) {
	return h.resources[id], nil
}
func (h reviewHistory) ListDeploymentWorkloads(context.Context, string) ([]domain.WorkloadSnapshot, error) {
	return nil, nil
}

func TestReviewBaselineNeedsExactCurrentReadyAssociation(t *testing.T) {
	ctx := context.Background()
	descriptor := resource.Descriptor{Type: "postgres", Class: "default"}
	active := resource.ActiveResource{ID: "active-1", OrganizationKey: "org", Descriptor: descriptor, Scope: resource.Scope{Type: resource.ScopeEnvironment, ID: "app.staging"}, Status: resource.StatusReady}
	node := descriptor.String()
	oldReview := Review{Version: 1, Nodes: []Node{{ID: node, Scope: active.Scope.Key()}}, Artifacts: []Artifact{{NodeID: node, Kind: "terraform-source", Path: "main.tf", After: Side{Availability: "PARTIAL", Text: "old safe content", RedactedPaths: []string{}, UnknownPaths: []string{}}}}}
	newest := domain.Deployment{ID: "new", OrganizationKey: "org", ApplicationKey: "app", EnvironmentKey: "staging", TargetGeneration: 3, StartedAt: time.Now()}
	older := newest
	older.ID = "old"
	h := reviewHistory{deployments: []domain.Deployment{newest, older}, plans: map[string]map[string]any{"old": {"review": oldReview}}, resources: map[string][]domain.Resource{
		"new": {{NodeDescriptor: node, ActiveResourceID: "active-1", Status: domain.ResourceReady}},
		"old": {{NodeDescriptor: node, ActiveResourceID: "active-1", Status: domain.ResourceReady}},
	}}
	previous, err := CapturePrevious(ctx, h, "org", "app", "staging", 3, []resource.ActiveResource{active})
	if err != nil || len(previous[node]) != 0 {
		t.Fatalf("missing newest review fell back to stale content: %#v %v", previous, err)
	}
	h.resources["new"] = []domain.Resource{{NodeDescriptor: node, ActiveResourceID: "active-1", Status: domain.ResourceFailed}}
	previous, err = CapturePrevious(ctx, h, "org", "app", "staging", 3, []resource.ActiveResource{active})
	if err != nil || len(previous[node]) != 1 || previous[node][0].After.Text != "old safe content" || previous[node][0].After.Provenance["baselineDeploymentId"] != "old" {
		t.Fatalf("failed newer attempt displaced successful baseline: %#v %v", previous, err)
	}
	h.resources["old"] = []domain.Resource{{NodeDescriptor: node, ActiveResourceID: "another-active", Status: domain.ResourceReady}}
	previous, err = CapturePrevious(ctx, h, "org", "app", "staging", 3, []resource.ActiveResource{active})
	if err != nil || len(previous[node]) != 0 {
		t.Fatalf("non-current resource association accepted: %#v %v", previous, err)
	}
}
