package deployment

import (
	"context"
	"testing"

	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning"
)

// TestDestroyedActiveResourceIsNoBaseline: a verified-destroyed resource has no
// baseline plan, so the descriptor created again is never RETAIN/UPDATE from the
// plan of the destroyed instance. A nil view proves no plan is even read.
func TestDestroyedActiveResourceIsNoBaseline(t *testing.T) {
	d, err := resource.ParseDescriptor("redis.in-cluster#shared.cache")
	if err != nil {
		t.Fatal(err)
	}
	scope := resource.Scope{Type: resource.ScopeShared, ID: "shop.staging"}
	baselines, err := CaptureBaselines(context.Background(), nil, "org", []resource.ActiveResource{
		{OrganizationKey: "org", Descriptor: d, Scope: scope, Status: resource.StatusDestroyed, LastDeploymentID: "11111111-1111-4111-8111-111111111111"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan := baselines.Reader()(planning.Node{Kind: planning.NodeResource, Descriptor: d.String(), Scope: scope}); plan != nil {
		t.Fatalf("a DESTROYED tombstone must have no baseline plan: %+v", plan)
	}
}
